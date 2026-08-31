/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	warpgatev1alpha1 "github.com/thereisnotime/warpgate-operator/api/v1alpha1"
)

const (
	// AnnotationBindDomain is the annotation key for the Warpgate bind domain (ExternalHost).
	AnnotationBindDomain = "warpgate.warpgate.warp.tech/http-target-bind-domain"
	// AnnotationTargetGroup is the annotation key for the target group name.
	AnnotationTargetGroup = "warpgate.warpgate.warp.tech/http-target-group"
	// AnnotationConnRef is the annotation key for the WarpgateConnection name to use.
	AnnotationConnRef = "warpgate.warpgate.warp.tech/http-target-connection-ref"

	// LabelManagedBy marks WarpgateTargets that were created by HTTPRoute auto-discovery.
	LabelManagedBy = "warpgate.warpgate.warp.tech/managed-by"
	// LabelManagedByValue is the value set on LabelManagedBy for targets created by this controller.
	LabelManagedByValue = "httproute-discovery"
	// LabelSourceHTTPRoute stores the "<namespace>/<name>" of the HTTPRoute that owns the target.
	LabelSourceHTTPRoute = "warpgate.warpgate.warp.tech/source-httproute"
)

// httprouteGVK is the GVK for the gateway.networking.k8s.io/v1 HTTPRoute resource.
var httprouteGVK = schema.GroupVersionKind{
	Group:   "gateway.networking.k8s.io",
	Version: "v1",
	Kind:    "HTTPRoute",
}

// HTTPRouteWatcherReconciler watches HTTPRoute resources and automatically creates
// WarpgateTarget (HTTP) CRs when the discovery annotations are present.
type HTTPRouteWatcherReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch
// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgatetargets,verbs=get;list;watch;create;update;patch;delete

// Reconcile handles create/update/delete events for HTTPRoute resources.
func (r *HTTPRouteWatcherReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the HTTPRoute as unstructured — no gateway-api dependency needed.
	route := &unstructured.Unstructured{}
	route.SetGroupVersionKind(httprouteGVK)
	if err := r.Get(ctx, req.NamespacedName, route); err != nil {
		// If it's gone, clean up any managed target that may still exist.
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		log.V(1).Info("HTTPRoute not found, checking for orphaned managed targets", "name", req.Name, "namespace", req.Namespace)
		return ctrl.Result{}, r.deleteManaged(ctx, req.Namespace, req.Name)
	}

	annotations := route.GetAnnotations()
	bindDomain := annotations[AnnotationBindDomain]
	targetGroup := annotations[AnnotationTargetGroup]
	connRef := annotations[AnnotationConnRef]

	// If the route is being deleted or required annotations are absent, clean up.
	if !route.GetDeletionTimestamp().IsZero() || bindDomain == "" || targetGroup == "" || connRef == "" {
		if bindDomain == "" || targetGroup == "" || connRef == "" {
			log.V(1).Info("HTTPRoute missing required annotations, skipping or cleaning up",
				"name", req.Name, "namespace", req.Namespace)
		}
		return ctrl.Result{}, r.deleteManaged(ctx, req.Namespace, req.Name)
	}

	// Derive the target URL from spec.hostnames[0].
	targetURL, err := extractFirstHostname(route)
	if err != nil {
		log.Error(err, "unable to derive URL from HTTPRoute spec.hostnames", "name", req.Name)
		return ctrl.Result{}, err
	}

	// Build the WarpgateTarget name: "<namespace>-<httproute-name>".
	targetName := managedTargetName(req.Namespace, req.Name)
	sourceKey := fmt.Sprintf("%s/%s", req.Namespace, req.Name)

	// Try to fetch an existing managed target.
	var existing warpgatev1alpha1.WarpgateTarget
	existing.Name = targetName
	existing.Namespace = req.Namespace
	getErr := r.Get(ctx, client.ObjectKey{Name: targetName, Namespace: req.Namespace}, &existing)

	if getErr != nil && client.IgnoreNotFound(getErr) != nil {
		return ctrl.Result{}, getErr
	}

	desired := r.buildTarget(targetName, req.Namespace, sourceKey, connRef, targetURL, bindDomain, targetGroup)

	if client.IgnoreNotFound(getErr) == nil && getErr != nil {
		// Not found — create it.
		log.Info("creating WarpgateTarget for HTTPRoute", "target", targetName, "route", req.Name)
		if err := r.Create(ctx, desired); err != nil {
			return ctrl.Result{}, fmt.Errorf("creating WarpgateTarget %q: %w", targetName, err)
		}
		return ctrl.Result{}, nil
	}

	// Already exists — update if anything changed.
	if existing.Spec.HTTP == nil ||
		existing.Spec.HTTP.URL != targetURL ||
		existing.Spec.HTTP.ExternalHost != bindDomain ||
		existing.Spec.ConnectionRef != connRef {
		log.Info("updating WarpgateTarget for HTTPRoute", "target", targetName, "route", req.Name)
		existing.Spec = desired.Spec
		existing.Labels = desired.Labels
		if err := r.Update(ctx, &existing); err != nil {
			return ctrl.Result{}, fmt.Errorf("updating WarpgateTarget %q: %w", targetName, err)
		}
	}

	return ctrl.Result{}, nil
}

// buildTarget constructs the desired WarpgateTarget CR.
func (r *HTTPRouteWatcherReconciler) buildTarget(
	name, namespace, sourceKey, connRef, url, externalHost, group string,
) *warpgatev1alpha1.WarpgateTarget {
	return &warpgatev1alpha1.WarpgateTarget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				LabelManagedBy:       LabelManagedByValue,
				LabelSourceHTTPRoute: sourceKey,
			},
			Annotations: map[string]string{
				AnnotationTargetGroup: group,
			},
		},
		Spec: warpgatev1alpha1.WarpgateTargetSpec{
			ConnectionRef: connRef,
			Name:          name,
			Description:   fmt.Sprintf("Auto-discovered from HTTPRoute %s (group: %s)", sourceKey, group),
			HTTP: &warpgatev1alpha1.HTTPTargetSpec{
				URL:          url,
				ExternalHost: externalHost,
			},
		},
	}
}

// deleteManaged removes the WarpgateTarget managed for the given HTTPRoute, if it exists.
func (r *HTTPRouteWatcherReconciler) deleteManaged(ctx context.Context, namespace, routeName string) error {
	targetName := managedTargetName(namespace, routeName)
	var target warpgatev1alpha1.WarpgateTarget
	if err := r.Get(ctx, client.ObjectKey{Name: targetName, Namespace: namespace}, &target); err != nil {
		return client.IgnoreNotFound(err)
	}
	// Only delete if we own it.
	if target.Labels[LabelManagedBy] != LabelManagedByValue {
		return nil
	}
	logf.FromContext(ctx).Info("deleting managed WarpgateTarget", "target", targetName)
	return client.IgnoreNotFound(r.Delete(ctx, &target))
}

// extractFirstHostname parses spec.hostnames[0] from an unstructured HTTPRoute and
// returns it as an https:// URL.
func extractFirstHostname(route *unstructured.Unstructured) (string, error) {
	hostnames, found, err := unstructured.NestedStringSlice(route.Object, "spec", "hostnames")
	if err != nil {
		return "", fmt.Errorf("reading spec.hostnames: %w", err)
	}
	if !found || len(hostnames) == 0 {
		return "", fmt.Errorf("HTTPRoute %q has no spec.hostnames", route.GetName())
	}
	return "https://" + hostnames[0], nil
}

// managedTargetName derives the WarpgateTarget name from the HTTPRoute namespace and name.
// Format: "<namespace>-<name>", truncated to 63 characters if necessary.
func managedTargetName(namespace, name string) string {
	full := namespace + "-" + name
	if len(full) > 63 {
		full = full[:63]
	}
	return full
}

// SetupWithManager registers the controller with the manager to watch HTTPRoute resources.
func (r *HTTPRouteWatcherReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Ensure the HTTPRoute GVK is registered with the scheme as unstructured so controller-runtime
	// can watch it without a typed Go struct.
	httprouteObj := &unstructured.Unstructured{}
	httprouteObj.SetGroupVersionKind(httprouteGVK)

	return ctrl.NewControllerManagedBy(mgr).
		WatchesRawSource(
			source.Kind(mgr.GetCache(), httprouteObj,
				&handler.TypedEnqueueRequestForObject[*unstructured.Unstructured]{}),
		).
		Named("httproute-watcher").
		Complete(r)
}
