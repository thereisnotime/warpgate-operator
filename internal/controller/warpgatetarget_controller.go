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

func (r *WarpgateTargetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var target warpgatev1alpha1.WarpgateTarget
	if err := r.Get(ctx, req.NamespacedName, &target); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	wgClient, err := getWarpgateClient(ctx, r.Client, target.Namespace, target.Spec.ConnectionRef)
	if err != nil {
		log.Error(err, "unable to build warpgate client")
		r.setCondition(&target, metav1.ConditionFalse, "ClientError", err.Error())
		if updateErr := r.Status().Update(ctx, &target); updateErr != nil {
			log.Error(updateErr, "unable to update status")
		}
		return ctrl.Result{RequeueAfter: r.ReconcileInterval}, nil
	}

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

	if !controllerutil.ContainsFinalizer(&target, targetFinalizerName) {
		controllerutil.AddFinalizer(&target, targetFinalizerName)
		if err := r.Update(ctx, &target); err != nil {
			return ctrl.Result{}, err
		}
	}

	targetReq, targetType, err := r.buildTargetRequest(ctx, &target)
	if err != nil {
		log.Error(err, "unable to build target request")
		r.setCondition(&target, metav1.ConditionFalse, "BuildError", err.Error())
		if updateErr := r.Status().Update(ctx, &target); updateErr != nil {
			log.Error(updateErr, "unable to update status")
		}
		return ctrl.Result{RequeueAfter: r.ReconcileInterval}, nil
	}

	if target.Status.ExternalID == "" {
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
		_, err := wgClient.UpdateTarget(target.Status.ExternalID, *targetReq)
		if err != nil {
			if warpgate.IsNotFound(err) {
				// deleted out-of-band; clear ID so next reconcile recreates it
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

	r.setCondition(&target, metav1.ConditionTrue, targetType, "target reconciled successfully")
	if err := r.Status().Update(ctx, &target); err != nil {
		log.Error(err, "unable to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: r.ReconcileInterval}, nil
}

func (r *WarpgateTargetReconciler) buildTargetRequest(ctx context.Context, target *warpgatev1alpha1.WarpgateTarget) (*warpgate.TargetRequest, string, error) {
	spec := &target.Spec
	ns := target.Namespace

	var opts any
	var targetType string
	var err error

	switch {
	case spec.SSH != nil:
		targetType = "SSH"
		opts, err = r.buildSSHOptions(ctx, ns, spec.SSH, target)
	case spec.HTTP != nil:
		targetType = "HTTP"
		opts = r.buildHTTPOptions(spec.HTTP)
	case spec.MySQL != nil:
		targetType = "MySQL"
		opts, err = r.buildMySQLOptions(ctx, ns, spec.MySQL)
	case spec.PostgreSQL != nil:
		targetType = "PostgreSQL"
		opts, err = r.buildPostgreSQLOptions(ctx, ns, spec.PostgreSQL)
	case spec.Kubernetes != nil:
		targetType = "Kubernetes"
		opts, err = r.buildKubernetesOptions(ctx, ns, spec.Kubernetes)
	case spec.RDP != nil:
		targetType = "RDP"
		opts, err = r.buildRDPOptions(ctx, ns, spec.RDP)
	case spec.VNC != nil:
		targetType = "VNC"
		opts, err = r.buildVNCOptions(ctx, ns, spec.VNC)
	default:
		return nil, "", fmt.Errorf("exactly one target type (ssh, http, mysql, postgresql, kubernetes, rdp, vnc) must be specified")
	}
	if err != nil {
		return nil, "", err
	}

	rawOpts, err := warpgate.MarshalOptions(opts)
	if err != nil {
		return nil, "", fmt.Errorf("marshaling target options: %w", err)
	}

	req := &warpgate.TargetRequest{
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
		if err := r.Get(ctx, types.NamespacedName{Namespace: ns, Name: spec.GroupRef}, &group); err != nil {
			return nil, "", fmt.Errorf("getting target group %q: %w", spec.GroupRef, err)
		}
		if group.Status.ExternalID == "" {
			return nil, "", fmt.Errorf("target group %q not yet synced", spec.GroupRef)
		}
		req.GroupID = group.Status.ExternalID
	}

	return req, targetType, nil
}

func (r *WarpgateTargetReconciler) buildSSHOptions(ctx context.Context, ns string, spec *warpgatev1alpha1.SSHTargetSpec, target *warpgatev1alpha1.WarpgateTarget) (any, error) {
	opts := warpgate.SSHOptions{
		Kind:               "Ssh",
		Host:               spec.Host,
		Port:               spec.Port,
		Username:           spec.Username,
		AllowInsecureAlgos: spec.AllowInsecureAlgos,
		Auth:               warpgate.SSHAuth{Kind: spec.AuthKind, KeyID: spec.KeyID},
	}
	if spec.AuthKind == "Password" {
		password, err := r.readSecretValue(ctx, ns, spec.PasswordSecretRef, "password")
		if err != nil {
			return nil, fmt.Errorf("reading SSH password secret: %w", err)
		}
		opts.Auth.Password = password
	}
	if spec.JumpHostRef != "" {
		var jh warpgatev1alpha1.WarpgateTarget
		if err := r.Get(ctx, types.NamespacedName{Namespace: target.Namespace, Name: spec.JumpHostRef}, &jh); err != nil {
			return nil, fmt.Errorf("getting jump host target %q: %w", spec.JumpHostRef, err)
		}
		if jh.Status.ExternalID == "" {
			return nil, fmt.Errorf("jump host target %q not yet synced", spec.JumpHostRef)
		}
		opts.JumpHost = jh.Status.ExternalID
	}
	return opts, nil
}

func (r *WarpgateTargetReconciler) buildHTTPOptions(spec *warpgatev1alpha1.HTTPTargetSpec) any {
	headers := spec.Headers
	if headers == nil {
		headers = map[string]string{}
	}
	return warpgate.HTTPOptions{
		Kind:         "Http",
		URL:          spec.URL,
		TLS:          toWarpgateTLS(spec.TLS),
		Headers:      headers,
		ExternalHost: spec.ExternalHost,
	}
}

func (r *WarpgateTargetReconciler) buildMySQLOptions(ctx context.Context, ns string, spec *warpgatev1alpha1.MySQLTargetSpec) (any, error) {
	auth, err := r.databaseAuth(ctx, ns, spec.AuthKind, spec.PasswordSecretRef, "MySQL")
	if err != nil {
		return nil, err
	}
	return warpgate.MySQLOptions{
		Kind:                "MySql",
		Host:                spec.Host,
		Port:                spec.Port,
		Username:            spec.Username,
		Auth:                auth,
		TLS:                 toWarpgateTLS(spec.TLS),
		DefaultDatabaseName: spec.DefaultDatabaseName,
	}, nil
}

func (r *WarpgateTargetReconciler) buildPostgreSQLOptions(ctx context.Context, ns string, spec *warpgatev1alpha1.PostgreSQLTargetSpec) (any, error) {
	auth, err := r.databaseAuth(ctx, ns, spec.AuthKind, spec.PasswordSecretRef, "PostgreSQL")
	if err != nil {
		return nil, err
	}
	protocolVersion := spec.ProtocolVersion
	if protocolVersion == "" {
		protocolVersion = "3.2"
	}
	return warpgate.PostgresOptions{
		Kind:                "Postgres",
		Host:                spec.Host,
		Port:                spec.Port,
		Username:            spec.Username,
		Auth:                auth,
		TLS:                 toWarpgateTLS(spec.TLS),
		ProtocolVersion:     protocolVersion,
		IdleTimeout:         spec.IdleTimeout,
		DefaultDatabaseName: spec.DefaultDatabaseName,
	}, nil
}

func (r *WarpgateTargetReconciler) buildKubernetesOptions(ctx context.Context, ns string, spec *warpgatev1alpha1.KubernetesTargetSpec) (any, error) {
	auth := warpgate.KubernetesAuth{Kind: spec.AuthKind}
	var err error
	switch spec.AuthKind {
	case "Token":
		if auth.Token, err = r.readSecretValue(ctx, ns, spec.TokenSecretRef, "token"); err != nil {
			return nil, fmt.Errorf("reading Kubernetes token secret: %w", err)
		}
	case "Certificate":
		if auth.Certificate, err = r.readSecretValue(ctx, ns, spec.CertificateSecretRef, "certificate"); err != nil {
			return nil, fmt.Errorf("reading Kubernetes certificate secret: %w", err)
		}
		if auth.PrivateKey, err = r.readSecretValue(ctx, ns, spec.PrivateKeySecretRef, "privateKey"); err != nil {
			return nil, fmt.Errorf("reading Kubernetes private key secret: %w", err)
		}
	}
	return warpgate.KubernetesOptions{
		Kind:       "Kubernetes",
		ClusterURL: spec.ClusterURL,
		TLS:        toWarpgateTLS(spec.TLS),
		Auth:       auth,
	}, nil
}

func (r *WarpgateTargetReconciler) buildRDPOptions(ctx context.Context, ns string, spec *warpgatev1alpha1.RDPTargetSpec) (any, error) {
	var password string
	if spec.PasswordSecretRef != nil {
		var err error
		password, err = r.readSecretValue(ctx, ns, spec.PasswordSecretRef, "password")
		if err != nil {
			return nil, fmt.Errorf("reading RDP password secret: %w", err)
		}
	}
	compression := spec.Compression
	if compression == "" {
		compression = "remotefx"
	}
	tlsSecurity := spec.TLSSecurity
	if tlsSecurity == "" {
		tlsSecurity = "Tls12"
	}
	return warpgate.RDPOptions{
		Kind:             "Rdp",
		Host:             spec.Host,
		Port:             spec.Port,
		Username:         spec.Username,
		Domain:           spec.Domain,
		Auth:             warpgate.PasswordAuth{Kind: "Password", Password: password},
		VerifyTLS:        spec.VerifyTLS,
		Compression:      compression,
		InteractiveLogon: spec.InteractiveLogon,
		TLSSecurity:      tlsSecurity,
	}, nil
}

func (r *WarpgateTargetReconciler) buildVNCOptions(ctx context.Context, ns string, spec *warpgatev1alpha1.VNCTargetSpec) (any, error) {
	auth := warpgate.PasswordAuth{Kind: "None"}
	if spec.PasswordSecretRef != nil {
		password, err := r.readSecretValue(ctx, ns, spec.PasswordSecretRef, "password")
		if err != nil {
			return nil, fmt.Errorf("reading VNC password secret: %w", err)
		}
		auth = warpgate.PasswordAuth{Kind: "Password", Password: password}
	}
	return warpgate.VNCOptions{
		Kind: "Vnc",
		Host: spec.Host,
		Port: spec.Port,
		Auth: auth,
	}, nil
}

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

func (r *WarpgateTargetReconciler) readSecretValue(ctx context.Context, namespace string, ref *warpgatev1alpha1.SecretKeyRef, defaultKey string) (string, error) {
	var secret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref.Name}, &secret); err != nil {
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

func (r *WarpgateTargetReconciler) setCondition(target *warpgatev1alpha1.WarpgateTarget, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&target.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             status,
		ObservedGeneration: target.Generation,
		Reason:             reason,
		Message:            message,
	})
}

func (r *WarpgateTargetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&warpgatev1alpha1.WarpgateTarget{}).
		Named("warpgatetarget").
		Complete(r)
}
