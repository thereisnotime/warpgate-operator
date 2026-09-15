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

const targetFinalizerName = "warpgate.warp.tech/finalizer"

// WarpgateTargetReconciler reconciles a WarpgateTarget object.
type WarpgateTargetReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	ReconcileInterval time.Duration
}

// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgatetargets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgatetargets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgatetargets/finalizers,verbs=update
// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgatetargetgroups,verbs=get;list;watch
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
		return ctrl.Result{RequeueAfter: r.ReconcileInterval}, nil
	}

	// Handle deletion via finalizer.
	if !target.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&target, targetFinalizerName) {
			if target.Status.ExternalID != "" {
				if err := wgClient.DeleteTarget(target.Status.ExternalID); err != nil && !warpgate.IsNotFound(err) {
					log.Error(err, "unable to delete target in warpgate")
					return ctrl.Result{RequeueAfter: r.ReconcileInterval}, nil
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
		return ctrl.Result{RequeueAfter: r.ReconcileInterval}, nil
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
			return ctrl.Result{RequeueAfter: r.ReconcileInterval}, nil
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
			return ctrl.Result{RequeueAfter: r.ReconcileInterval}, nil
		}
	}

	// Success.
	r.setCondition(&target, metav1.ConditionTrue, targetType, "target reconciled successfully")
	if err := r.Status().Update(ctx, &target); err != nil {
		log.Error(err, "unable to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: r.ReconcileInterval}, nil
}

// buildTargetRequest converts the CRD spec into a Warpgate API TargetRequest.
// It returns the request, a human-readable target type string, and any error.
func (r *WarpgateTargetReconciler) buildTargetRequest(ctx context.Context, target *warpgatev1alpha1.WarpgateTarget) (*warpgate.TargetRequest, string, error) {
	spec := &target.Spec
	var opts any
	var targetType string

	// secret reads a referenced Secret value, or "" when no ref is given.
	secret := func(ref *warpgatev1alpha1.SecretKeyRef, defaultKey, what string) (string, error) {
		if ref == nil {
			return "", nil
		}
		v, err := r.readSecretValue(ctx, target.Namespace, ref, defaultKey)
		if err != nil {
			return "", fmt.Errorf("reading %s secret: %w", what, err)
		}
		return v, nil
	}

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
				Kind:  spec.SSH.AuthKind,
				KeyID: spec.SSH.KeyID,
			},
		}
		if spec.SSH.AuthKind == "Password" {
			password, err := secret(spec.SSH.PasswordSecretRef, "password", "SSH password")
			if err != nil {
				return nil, "", err
			}
			sshOpts.Auth.Password = password
		}
		if spec.SSH.JumpHostRef != "" {
			var jumpHostTarget warpgatev1alpha1.WarpgateTarget
			if err := r.Get(ctx, types.NamespacedName{
				Namespace: target.Namespace,
				Name:      spec.SSH.JumpHostRef,
			}, &jumpHostTarget); err != nil {
				return nil, "", fmt.Errorf("getting jump host target %q: %w", spec.SSH.JumpHostRef, err)
			}
			if jumpHostTarget.Status.ExternalID == "" {
				return nil, "", fmt.Errorf("jump host target %q not yet synced", spec.SSH.JumpHostRef)
			}
			sshOpts.JumpHost = jumpHostTarget.Status.ExternalID
		}
		opts = sshOpts

	case spec.HTTP != nil:
		targetType = "HTTP"
		headers := spec.HTTP.Headers
		if headers == nil {
			headers = map[string]string{}
		}
		opts = warpgate.HTTPOptions{
			Kind:         "Http",
			URL:          spec.HTTP.URL,
			TLS:          toWarpgateTLS(spec.HTTP.TLS),
			Headers:      headers,
			ExternalHost: spec.HTTP.ExternalHost,
		}

	case spec.MySQL != nil:
		targetType = "MySQL"
		auth, err := r.databaseAuth(ctx, target.Namespace, spec.MySQL.AuthKind, spec.MySQL.PasswordSecretRef, "MySQL")
		if err != nil {
			return nil, "", err
		}
		opts = warpgate.MySQLOptions{
			Kind:                "MySql",
			Host:                spec.MySQL.Host,
			Port:                spec.MySQL.Port,
			Username:            spec.MySQL.Username,
			Auth:                auth,
			TLS:                 toWarpgateTLS(spec.MySQL.TLS),
			DefaultDatabaseName: spec.MySQL.DefaultDatabaseName,
		}

	case spec.PostgreSQL != nil:
		targetType = "PostgreSQL"
		auth, err := r.databaseAuth(ctx, target.Namespace, spec.PostgreSQL.AuthKind, spec.PostgreSQL.PasswordSecretRef, "PostgreSQL")
		if err != nil {
			return nil, "", err
		}
		protocolVersion := spec.PostgreSQL.ProtocolVersion
		if protocolVersion == "" {
			protocolVersion = "3.2"
		}
		opts = warpgate.PostgresOptions{
			Kind:                "Postgres",
			Host:                spec.PostgreSQL.Host,
			Port:                spec.PostgreSQL.Port,
			Username:            spec.PostgreSQL.Username,
			Auth:                auth,
			TLS:                 toWarpgateTLS(spec.PostgreSQL.TLS),
			ProtocolVersion:     protocolVersion,
			IdleTimeout:         spec.PostgreSQL.IdleTimeout,
			DefaultDatabaseName: spec.PostgreSQL.DefaultDatabaseName,
		}

	case spec.Kubernetes != nil:
		targetType = "Kubernetes"
		k8s := spec.Kubernetes
		auth := warpgate.KubernetesAuth{Kind: k8s.AuthKind}
		var err error
		switch k8s.AuthKind {
		case "Token":
			if auth.Token, err = secret(k8s.TokenSecretRef, "token", "Kubernetes token"); err != nil {
				return nil, "", err
			}
		case "Certificate":
			if auth.Certificate, err = secret(k8s.CertificateSecretRef, "certificate", "Kubernetes certificate"); err != nil {
				return nil, "", err
			}
			if auth.PrivateKey, err = secret(k8s.PrivateKeySecretRef, "privateKey", "Kubernetes private key"); err != nil {
				return nil, "", err
			}
		}
		opts = warpgate.KubernetesOptions{
			Kind:       "Kubernetes",
			ClusterURL: k8s.ClusterURL,
			TLS:        toWarpgateTLS(k8s.TLS),
			Auth:       auth,
		}

	case spec.RDP != nil:
		targetType = "RDP"
		password, err := secret(spec.RDP.PasswordSecretRef, "password", "RDP password")
		if err != nil {
			return nil, "", err
		}
		compression := spec.RDP.Compression
		if compression == "" {
			compression = "remotefx"
		}
		tlsSecurity := spec.RDP.TLSSecurity
		if tlsSecurity == "" {
			tlsSecurity = "Tls12"
		}
		opts = warpgate.RDPOptions{
			Kind:             "Rdp",
			Host:             spec.RDP.Host,
			Port:             spec.RDP.Port,
			Username:         spec.RDP.Username,
			Domain:           spec.RDP.Domain,
			Auth:             warpgate.PasswordAuth{Kind: "Password", Password: password},
			VerifyTLS:        spec.RDP.VerifyTLS,
			Compression:      compression,
			InteractiveLogon: spec.RDP.InteractiveLogon,
			TLSSecurity:      tlsSecurity,
		}

	case spec.VNC != nil:
		targetType = "VNC"
		auth := warpgate.PasswordAuth{Kind: "None"}
		if spec.VNC.PasswordSecretRef != nil {
			password, err := secret(spec.VNC.PasswordSecretRef, "password", "VNC password")
			if err != nil {
				return nil, "", err
			}
			auth = warpgate.PasswordAuth{Kind: "Password", Password: password}
		}
		opts = warpgate.VNCOptions{
			Kind: "Vnc",
			Host: spec.VNC.Host,
			Port: spec.VNC.Port,
			Auth: auth,
		}

	default:
		return nil, "", fmt.Errorf("exactly one target type (ssh, http, mysql, postgresql, kubernetes, rdp, vnc) must be specified")
	}

	rawOpts, err := warpgate.MarshalOptions(opts)
	if err != nil {
		return nil, "", fmt.Errorf("marshaling target options: %w", err)
	}

	targetReq := &warpgate.TargetRequest{
		Name:                     spec.Name,
		Description:              spec.Description,
		Options:                  rawOpts,
		RateLimitBytesPerSecond:  spec.RateLimitBytesPerSecond,
		TicketMaxDurationSeconds: spec.TicketMaxDurationSeconds,
		TicketRequestsDisabled:   boolValue(spec.TicketRequestsDisabled),
		TicketRequireApproval:    boolValue(spec.TicketRequireApproval),
		RequireApproval:          boolValue(spec.RequireApproval),
		TicketMaxUses:            spec.TicketMaxUses,
	}

	if spec.GroupRef != "" {
		var group warpgatev1alpha1.WarpgateTargetGroup
		if err := r.Get(ctx, types.NamespacedName{
			Namespace: target.Namespace,
			Name:      spec.GroupRef,
		}, &group); err != nil {
			return nil, "", fmt.Errorf("getting target group %q: %w", spec.GroupRef, err)
		}
		if group.Status.ExternalID == "" {
			return nil, "", fmt.Errorf("target group %q not yet synced", spec.GroupRef)
		}
		targetReq.GroupID = group.Status.ExternalID
	}

	return targetReq, targetType, nil
}

// databaseAuth builds the MySQL/PostgreSQL auth block. An empty authKind means Password.
func (r *WarpgateTargetReconciler) databaseAuth(ctx context.Context, namespace, authKind string, passwordRef *warpgatev1alpha1.SecretKeyRef, what string) (warpgate.DatabaseAuth, error) {
	if authKind == "IamRole" {
		return warpgate.DatabaseAuth{Kind: "IamRole"}, nil
	}
	auth := warpgate.DatabaseAuth{Kind: "Password"}
	if passwordRef != nil {
		password, err := r.readSecretValue(ctx, namespace, passwordRef, "password")
		if err != nil {
			return auth, fmt.Errorf("reading %s password secret: %w", what, err)
		}
		auth.Password = password
	}
	return auth, nil
}

// toWarpgateTLS converts an optional CRD TLS block; absent means Warpgate's default (Preferred, verify).
func toWarpgateTLS(spec *warpgatev1alpha1.TLSConfigSpec) warpgate.TLSConfig {
	if spec == nil {
		return warpgate.TLSConfig{Mode: "Preferred", Verify: true}
	}
	return warpgate.TLSConfig{Mode: spec.Mode, Verify: spec.Verify}
}

func boolValue(b *bool) bool {
	return b != nil && *b
}

// readSecretValue reads a value from a Kubernetes Secret using the given SecretKeyRef.
// If no key is specified, defaultKey is used.
func (r *WarpgateTargetReconciler) readSecretValue(ctx context.Context, namespace string, ref *warpgatev1alpha1.SecretKeyRef, defaultKey string) (string, error) {
	var secret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      ref.Name,
	}, &secret); err != nil {
		return "", fmt.Errorf("getting secret %q: %w", ref.Name, err)
	}

	key := ref.Key
	if key == "" {
		key = defaultKey
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
