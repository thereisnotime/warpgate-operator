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
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	warpgatev1alpha1 "github.com/thereisnotime/warpgate-operator/api/v1alpha1"
	"github.com/thereisnotime/warpgate-operator/internal/warpgate"
)

const (
	targetFinalizerName = "warpgate.warp.tech/finalizer"
	targetRequeueAfter  = 5 * time.Minute
)

// WarpgateTargetReconciler reconciles a WarpgateTarget object.
type WarpgateTargetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgatetargets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgatetargets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgatetargets/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile moves the actual state of the world closer to the desired state
// described in the WarpgateTarget CR.
func (r *WarpgateTargetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the CR.
	var target warpgatev1alpha1.WarpgateTarget
	if err := r.Get(ctx, req.NamespacedName, &target); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Build the Warpgate API client from the referenced connection.
	wgClient, err := getWarpgateClient(ctx, r.Client, target.Namespace, target.Spec.ConnectionRef)
	if err != nil {
		log.Error(err, "unable to build warpgate client")
		r.setCondition(&target, metav1.ConditionFalse, "ClientError", err.Error())
		if updateErr := r.Status().Update(ctx, &target); updateErr != nil {
			log.Error(updateErr, "unable to update status")
		}
		return ctrl.Result{RequeueAfter: targetRequeueAfter}, nil
	}

	// Handle deletion via finalizer.
	if !target.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&target, targetFinalizerName) {
			if target.Status.ExternalID != "" {
				if err := wgClient.DeleteTarget(target.Status.ExternalID); err != nil && !warpgate.IsNotFound(err) {
					log.Error(err, "unable to delete target in warpgate")
					return ctrl.Result{RequeueAfter: targetRequeueAfter}, nil
				}
			}
			controllerutil.RemoveFinalizer(&target, targetFinalizerName)
			if err := r.Update(ctx, &target); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if it's not present.
	if !controllerutil.ContainsFinalizer(&target, targetFinalizerName) {
		controllerutil.AddFinalizer(&target, targetFinalizerName)
		if err := r.Update(ctx, &target); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Build the TargetRequest from the CR spec.
	targetReq, targetType, err := r.buildTargetRequest(ctx, &target)
	if err != nil {
		log.Error(err, "unable to build target request")
		r.setCondition(&target, metav1.ConditionFalse, "BuildError", err.Error())
		if updateErr := r.Status().Update(ctx, &target); updateErr != nil {
			log.Error(updateErr, "unable to update status")
		}
		return ctrl.Result{RequeueAfter: targetRequeueAfter}, nil
	}

	// Create or update the target in Warpgate.
	if target.Status.ExternalID == "" {
		// Create.
		created, err := wgClient.CreateTarget(*targetReq)
		if err != nil {
			log.Error(err, "unable to create target in warpgate")
			r.setCondition(&target, metav1.ConditionFalse, "CreateError", err.Error())
			if updateErr := r.Status().Update(ctx, &target); updateErr != nil {
				log.Error(updateErr, "unable to update status")
			}
			return ctrl.Result{RequeueAfter: targetRequeueAfter}, nil
		}
		target.Status.ExternalID = created.ID
	} else {
		// Update.
		_, err := wgClient.UpdateTarget(target.Status.ExternalID, *targetReq)
		if err != nil {
			if warpgate.IsNotFound(err) {
				// The target was deleted out-of-band; clear ID and requeue to recreate.
				log.Info("target not found in warpgate, will recreate", "externalID", target.Status.ExternalID)
				target.Status.ExternalID = ""
				r.setCondition(&target, metav1.ConditionFalse, "NotFound", "target was deleted externally, recreating")
				if updateErr := r.Status().Update(ctx, &target); updateErr != nil {
					log.Error(updateErr, "unable to update status")
				}
				return ctrl.Result{Requeue: true}, nil
			}
			log.Error(err, "unable to update target in warpgate")
			r.setCondition(&target, metav1.ConditionFalse, "UpdateError", err.Error())
			if updateErr := r.Status().Update(ctx, &target); updateErr != nil {
				log.Error(updateErr, "unable to update status")
			}
			return ctrl.Result{RequeueAfter: targetRequeueAfter}, nil
		}
	}

	// Success.
	r.setCondition(&target, metav1.ConditionTrue, targetType, "target reconciled successfully")
	if err := r.Status().Update(ctx, &target); err != nil {
		log.Error(err, "unable to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: targetRequeueAfter}, nil
}

// buildTargetRequest converts the CRD spec into a Warpgate API TargetRequest.
// It returns the request, a human-readable target type string, and any error.
func (r *WarpgateTargetReconciler) buildTargetRequest(ctx context.Context, target *warpgatev1alpha1.WarpgateTarget) (*warpgate.TargetRequest, string, error) {
	spec := &target.Spec
	var opts any
	var targetType string

	switch {
	case spec.SSH != nil:
		targetType = "SSH"
		sshOpts := warpgate.SSHOptions{
			Kind:               "Ssh",
			Host:               spec.SSH.Host,
			Port:               spec.SSH.Port,
			Username:           spec.SSH.Username,
			AllowInsecureAlgos: spec.SSH.AllowInsecureAlgos,
			Auth: warpgate.SSHAuth{
				Kind: spec.SSH.AuthKind,
			},
		}
		if spec.SSH.AuthKind == "Password" && spec.SSH.PasswordSecretRef != nil {
			password, err := r.readSecretValue(ctx, target.Namespace, spec.SSH.PasswordSecretRef)
			if err != nil {
				return nil, "", fmt.Errorf("reading SSH password secret: %w", err)
			}
			sshOpts.Auth.Password = password
		}
		opts = sshOpts

	case spec.HTTP != nil:
		targetType = "HTTP"
		httpOpts := warpgate.HTTPOptions{
			Kind:         "Http",
			URL:          spec.HTTP.URL,
			Headers:      spec.HTTP.Headers,
			ExternalHost: spec.HTTP.ExternalHost,
		}
		if spec.HTTP.TLS != nil {
			httpOpts.TLS = &warpgate.TLSConfig{
				Mode:   spec.HTTP.TLS.Mode,
				Verify: spec.HTTP.TLS.Verify,
			}
		}
		opts = httpOpts

	case spec.MySQL != nil:
		targetType = "MySQL"
		mysqlOpts := warpgate.MySQLOptions{
			Kind:     "MySql",
			Host:     spec.MySQL.Host,
			Port:     spec.MySQL.Port,
			Username: spec.MySQL.Username,
		}
		if spec.MySQL.PasswordSecretRef != nil {
			password, err := r.readSecretValue(ctx, target.Namespace, spec.MySQL.PasswordSecretRef)
			if err != nil {
				return nil, "", fmt.Errorf("reading MySQL password secret: %w", err)
			}
			mysqlOpts.Password = password
		}
		if spec.MySQL.TLS != nil {
			mysqlOpts.TLS = &warpgate.TLSConfig{
				Mode:   spec.MySQL.TLS.Mode,
				Verify: spec.MySQL.TLS.Verify,
			}
		}
		opts = mysqlOpts

	case spec.PostgreSQL != nil:
		targetType = "PostgreSQL"
		pgOpts := warpgate.PostgresOptions{
			Kind:     "Postgres",
			Host:     spec.PostgreSQL.Host,
			Port:     spec.PostgreSQL.Port,
			Username: spec.PostgreSQL.Username,
		}
		if spec.PostgreSQL.PasswordSecretRef != nil {
			password, err := r.readSecretValue(ctx, target.Namespace, spec.PostgreSQL.PasswordSecretRef)
			if err != nil {
				return nil, "", fmt.Errorf("reading PostgreSQL password secret: %w", err)
			}
			pgOpts.Password = password
		}
		if spec.PostgreSQL.TLS != nil {
			pgOpts.TLS = &warpgate.TLSConfig{
				Mode:   spec.PostgreSQL.TLS.Mode,
				Verify: spec.PostgreSQL.TLS.Verify,
			}
		}
		opts = pgOpts

	default:
		return nil, "", fmt.Errorf("exactly one target type (ssh, http, mysql, postgresql) must be specified")
	}

	rawOpts, err := warpgate.MarshalOptions(opts)
	if err != nil {
		return nil, "", fmt.Errorf("marshaling target options: %w", err)
	}

	return &warpgate.TargetRequest{
		Name:        spec.Name,
		Description: spec.Description,
		Options:     rawOpts,
	}, targetType, nil
}

// readSecretValue reads a value from a Kubernetes Secret using the given SecretKeyRef.
// If no key is specified, it defaults to "password".
func (r *WarpgateTargetReconciler) readSecretValue(ctx context.Context, namespace string, ref *warpgatev1alpha1.SecretKeyRef) (string, error) {
	var secret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      ref.Name,
	}, &secret); err != nil {
		return "", fmt.Errorf("getting secret %q: %w", ref.Name, err)
	}

	key := ref.Key
	if key == "" {
		key = "password"
	}

	val, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("key %q not found in secret %q", key, ref.Name)
	}

	return string(val), nil
}

// setCondition sets the Ready condition on the target status.
func (r *WarpgateTargetReconciler) setCondition(target *warpgatev1alpha1.WarpgateTarget, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&target.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             status,
		ObservedGeneration: target.Generation,
		Reason:             reason,
		Message:            message,
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *WarpgateTargetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&warpgatev1alpha1.WarpgateTarget{}).
		Named("warpgatetarget").
		Complete(r)
}
