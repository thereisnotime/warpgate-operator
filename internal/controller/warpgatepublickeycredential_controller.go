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

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	warpgatev1alpha1 "github.com/thereisnotime/warpgate-operator/api/v1alpha1"
	"github.com/thereisnotime/warpgate-operator/internal/warpgate"
)

const publicKeyCredentialFinalizer = "warpgate.warp.tech/finalizer"

// WarpgatePublicKeyCredentialReconciler reconciles a WarpgatePublicKeyCredential object.
type WarpgatePublicKeyCredentialReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgatepublickeycredentials,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgatepublickeycredentials/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgatepublickeycredentials/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile handles the reconciliation loop for WarpgatePublicKeyCredential resources.
func (r *WarpgatePublicKeyCredentialReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the CR.
	var cred warpgatev1alpha1.WarpgatePublicKeyCredential
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
		if controllerutil.ContainsFinalizer(&cred, publicKeyCredentialFinalizer) {
			if cred.Status.UserID != "" && cred.Status.CredentialID != "" {
				if err := wgClient.DeletePublicKeyCredential(cred.Status.UserID, cred.Status.CredentialID); err != nil && !warpgate.IsNotFound(err) {
					log.Error(err, "failed to delete public key credential in Warpgate")
					return ctrl.Result{}, err
				}
				log.Info("deleted public key credential in Warpgate", "credentialID", cred.Status.CredentialID)
			}
			controllerutil.RemoveFinalizer(&cred, publicKeyCredentialFinalizer)
			if err := r.Update(ctx, &cred); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if missing.
	if !controllerutil.ContainsFinalizer(&cred, publicKeyCredentialFinalizer) {
		controllerutil.AddFinalizer(&cred, publicKeyCredentialFinalizer)
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

	pubKeyReq := warpgate.PublicKeyCredentialRequest{
		Label:            cred.Spec.Label,
		OpenSSHPublicKey: cred.Spec.OpenSSHPublicKey,
	}

	if cred.Status.CredentialID == "" {
		// Create the credential.
		created, err := wgClient.CreatePublicKeyCredential(wgUser.ID, pubKeyReq)
		if err != nil {
			log.Error(err, "failed to create public key credential in Warpgate")
			meta.SetStatusCondition(&cred.Status.Conditions, metav1.Condition{
				Type:               "Ready",
				Status:             metav1.ConditionFalse,
				Reason:             "CreateFailed",
				Message:            fmt.Sprintf("Failed to create public key credential: %v", err),
				ObservedGeneration: cred.Generation,
			})
			_ = r.Status().Update(ctx, &cred)
			return ctrl.Result{}, err
		}
		cred.Status.CredentialID = created.ID
		log.Info("created public key credential in Warpgate", "credentialID", created.ID)
	} else {
		// Update the existing credential.
		if _, err := wgClient.UpdatePublicKeyCredential(wgUser.ID, cred.Status.CredentialID, pubKeyReq); err != nil {
			if warpgate.IsNotFound(err) {
				log.Info("public key credential not found in Warpgate, will recreate", "credentialID", cred.Status.CredentialID)
				cred.Status.CredentialID = ""
				meta.SetStatusCondition(&cred.Status.Conditions, metav1.Condition{
					Type:               "Ready",
					Status:             metav1.ConditionFalse,
					Reason:             "NotFound",
					Message:            "Credential was deleted externally, will recreate on next reconcile",
					ObservedGeneration: cred.Generation,
				})
				_ = r.Status().Update(ctx, &cred)
				return ctrl.Result{Requeue: true}, nil
			}
			log.Error(err, "failed to update public key credential in Warpgate")
			meta.SetStatusCondition(&cred.Status.Conditions, metav1.Condition{
				Type:               "Ready",
				Status:             metav1.ConditionFalse,
				Reason:             "UpdateFailed",
				Message:            fmt.Sprintf("Failed to update public key credential: %v", err),
				ObservedGeneration: cred.Generation,
			})
			_ = r.Status().Update(ctx, &cred)
			return ctrl.Result{}, err
		}
		log.Info("updated public key credential in Warpgate", "credentialID", cred.Status.CredentialID)
	}

	// Mark as ready.
	meta.SetStatusCondition(&cred.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            "Public key credential is in sync with Warpgate",
		ObservedGeneration: cred.Generation,
	})
	if err := r.Status().Update(ctx, &cred); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WarpgatePublicKeyCredentialReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&warpgatev1alpha1.WarpgatePublicKeyCredential{}).
		Named("warpgatepublickeycredential").
		Complete(r)
}
