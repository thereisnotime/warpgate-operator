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

const passwordCredentialFinalizer = "warpgate.warp.tech/finalizer"

// WarpgatePasswordCredentialReconciler reconciles a WarpgatePasswordCredential object.
type WarpgatePasswordCredentialReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgatepasswordcredentials,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgatepasswordcredentials/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgatepasswordcredentials/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile handles the reconciliation loop for WarpgatePasswordCredential resources.
func (r *WarpgatePasswordCredentialReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the CR.
	var cred warpgatev1alpha1.WarpgatePasswordCredential
	if err := r.Get(ctx, req.NamespacedName, &cred); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Get the Warpgate API client from the referenced connection.
	wgClient, err := getWarpgateClient(ctx, r.Client, cred.Namespace, cred.Spec.ConnectionRef)
	if err != nil {
		log.Error(err, "failed to get warpgate client")
		meta.SetStatusCondition(&cred.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "ClientError",
			Message:            fmt.Sprintf("Failed to get Warpgate client: %v", err),
			ObservedGeneration: cred.Generation,
		})
		_ = r.Status().Update(ctx, &cred)
		return ctrl.Result{}, err
	}

	// Handle deletion.
	if !cred.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&cred, passwordCredentialFinalizer) {
			if cred.Status.UserID != "" && cred.Status.CredentialID != "" {
				if err := wgClient.DeletePasswordCredential(cred.Status.UserID, cred.Status.CredentialID); err != nil && !warpgate.IsNotFound(err) {
					log.Error(err, "failed to delete password credential in Warpgate")
					return ctrl.Result{}, err
				}
				log.Info("deleted password credential in Warpgate", "credentialID", cred.Status.CredentialID)
			}
			controllerutil.RemoveFinalizer(&cred, passwordCredentialFinalizer)
			if err := r.Update(ctx, &cred); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if missing.
	if !controllerutil.ContainsFinalizer(&cred, passwordCredentialFinalizer) {
		controllerutil.AddFinalizer(&cred, passwordCredentialFinalizer)
		if err := r.Update(ctx, &cred); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Resolve the Warpgate user by username.
	wgUser, err := wgClient.GetUserByUsername(cred.Spec.Username)
	if err != nil {
		log.Error(err, "failed to resolve user by username", "username", cred.Spec.Username)
		meta.SetStatusCondition(&cred.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "UserNotFound",
			Message:            fmt.Sprintf("Failed to resolve user %q: %v", cred.Spec.Username, err),
			ObservedGeneration: cred.Generation,
		})
		_ = r.Status().Update(ctx, &cred)
		return ctrl.Result{}, err
	}
	cred.Status.UserID = wgUser.ID

	// Read the password from the referenced Secret.
	var secret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{
		Name:      cred.Spec.PasswordSecretRef.Name,
		Namespace: cred.Namespace,
	}, &secret); err != nil {
		log.Error(err, "failed to get password secret", "secret", cred.Spec.PasswordSecretRef.Name)
		meta.SetStatusCondition(&cred.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "SecretNotFound",
			Message:            fmt.Sprintf("Failed to get password secret %q: %v", cred.Spec.PasswordSecretRef.Name, err),
			ObservedGeneration: cred.Generation,
		})
		_ = r.Status().Update(ctx, &cred)
		return ctrl.Result{}, err
	}

	key := cred.Spec.PasswordSecretRef.Key
	if key == "" {
		key = "password"
	}
	password, ok := secret.Data[key]
	if !ok {
		err := fmt.Errorf("key %q not found in secret %q", key, cred.Spec.PasswordSecretRef.Name)
		log.Error(err, "password key missing from secret")
		meta.SetStatusCondition(&cred.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "SecretKeyMissing",
			Message:            err.Error(),
			ObservedGeneration: cred.Generation,
		})
		_ = r.Status().Update(ctx, &cred)
		return ctrl.Result{}, err
	}

	// Create the credential if it doesn't exist yet.
	if cred.Status.CredentialID == "" {
		created, err := wgClient.CreatePasswordCredential(wgUser.ID, string(password))
		if err != nil {
			log.Error(err, "failed to create password credential in Warpgate")
			meta.SetStatusCondition(&cred.Status.Conditions, metav1.Condition{
				Type:               "Ready",
				Status:             metav1.ConditionFalse,
				Reason:             "CreateFailed",
				Message:            fmt.Sprintf("Failed to create password credential: %v", err),
				ObservedGeneration: cred.Generation,
			})
			_ = r.Status().Update(ctx, &cred)
			return ctrl.Result{}, err
		}
		cred.Status.CredentialID = created.ID
		log.Info("created password credential in Warpgate", "credentialID", created.ID)
	}

	// Mark as ready.
	meta.SetStatusCondition(&cred.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            "Password credential is in sync with Warpgate",
		ObservedGeneration: cred.Generation,
	})
	if err := r.Status().Update(ctx, &cred); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WarpgatePasswordCredentialReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&warpgatev1alpha1.WarpgatePasswordCredential{}).
		Named("warpgatepasswordcredential").
		Complete(r)
}
