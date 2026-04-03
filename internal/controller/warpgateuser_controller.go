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
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

const userFinalizer = "warpgate.warp.tech/finalizer"

// WarpgateUserReconciler reconciles a WarpgateUser object.
type WarpgateUserReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgateusers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgateusers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgateusers/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;delete

// Reconcile handles the reconciliation loop for WarpgateUser resources.
func (r *WarpgateUserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the WarpgateUser CR.
	var user warpgatev1alpha1.WarpgateUser
	if err := r.Get(ctx, req.NamespacedName, &user); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Get the Warpgate API client from the referenced connection.
	wgClient, err := getWarpgateClient(ctx, r.Client, user.Namespace, user.Spec.ConnectionRef)
	if err != nil {
		log.Error(err, "failed to get warpgate client")
		meta.SetStatusCondition(&user.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "ClientError",
			Message:            fmt.Sprintf("Failed to get Warpgate client: %v", err),
			ObservedGeneration: user.Generation,
		})
		_ = r.Status().Update(ctx, &user)
		return ctrl.Result{}, err
	}

	// Handle deletion.
	if !user.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&user, userFinalizer) {
			// Clean up generated password credential.
			if user.Status.PasswordCredentialID != "" && user.Status.ExternalID != "" {
				if err := wgClient.DeletePasswordCredential(user.Status.ExternalID, user.Status.PasswordCredentialID); err != nil && !warpgate.IsNotFound(err) {
					log.Error(err, "failed to delete password credential in Warpgate")
					return ctrl.Result{}, err
				}
			}
			// Clean up the auto-created password secret.
			if user.Status.PasswordSecretRef != "" {
				secret := &corev1.Secret{}
				if err := r.Get(ctx, types.NamespacedName{Name: user.Status.PasswordSecretRef, Namespace: user.Namespace}, secret); err == nil {
					if err := r.Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
						log.Error(err, "failed to delete password secret")
						return ctrl.Result{}, err
					}
				}
			}
			if user.Status.ExternalID != "" {
				if err := wgClient.DeleteUser(user.Status.ExternalID); err != nil && !warpgate.IsNotFound(err) {
					log.Error(err, "failed to delete user in Warpgate")
					return ctrl.Result{}, err
				}
				log.Info("deleted user in Warpgate", "externalID", user.Status.ExternalID)
			}
			controllerutil.RemoveFinalizer(&user, userFinalizer)
			if err := r.Update(ctx, &user); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if missing.
	if !controllerutil.ContainsFinalizer(&user, userFinalizer) {
		controllerutil.AddFinalizer(&user, userFinalizer)
		if err := r.Update(ctx, &user); err != nil {
			return ctrl.Result{}, err
		}
	}

	if user.Status.ExternalID == "" {
		// Create the user in Warpgate.
		createReq := warpgate.UserCreateRequest{
			Username:    user.Spec.Username,
			Description: user.Spec.Description,
		}
		created, err := wgClient.CreateUser(createReq)
		if err != nil {
			log.Error(err, "failed to create user in Warpgate")
			meta.SetStatusCondition(&user.Status.Conditions, metav1.Condition{
				Type:               "Ready",
				Status:             metav1.ConditionFalse,
				Reason:             "CreateFailed",
				Message:            fmt.Sprintf("Failed to create user: %v", err),
				ObservedGeneration: user.Generation,
			})
			_ = r.Status().Update(ctx, &user)
			return ctrl.Result{}, err
		}
		user.Status.ExternalID = created.ID
		log.Info("created user in Warpgate", "externalID", created.ID)
	} else {
		// Update the existing user in Warpgate.
		updateReq := warpgate.UserUpdateRequest{
			Username:         user.Spec.Username,
			Description:      user.Spec.Description,
			CredentialPolicy: toWarpgateCredentialPolicy(user.Spec.CredentialPolicy),
		}
		if _, err := wgClient.UpdateUser(user.Status.ExternalID, updateReq); err != nil {
			if warpgate.IsNotFound(err) {
				log.Info("user not found in Warpgate, will recreate", "externalID", user.Status.ExternalID)
				user.Status.ExternalID = ""
				meta.SetStatusCondition(&user.Status.Conditions, metav1.Condition{
					Type:               "Ready",
					Status:             metav1.ConditionFalse,
					Reason:             "NotFound",
					Message:            "User was deleted externally, will recreate on next reconcile",
					ObservedGeneration: user.Generation,
				})
				_ = r.Status().Update(ctx, &user)
				return ctrl.Result{Requeue: true}, nil
			}
			log.Error(err, "failed to update user in Warpgate")
			meta.SetStatusCondition(&user.Status.Conditions, metav1.Condition{
				Type:               "Ready",
				Status:             metav1.ConditionFalse,
				Reason:             "UpdateFailed",
				Message:            fmt.Sprintf("Failed to update user: %v", err),
				ObservedGeneration: user.Generation,
			})
			_ = r.Status().Update(ctx, &user)
			return ctrl.Result{}, err
		}
		log.Info("updated user in Warpgate", "externalID", user.Status.ExternalID)
	}

	// Auto-generate password credential if enabled and not yet created.
	if user.Spec.GeneratePassword == nil || *user.Spec.GeneratePassword {
		if user.Status.PasswordCredentialID == "" && user.Status.ExternalID != "" {
			if err := r.ensurePasswordCredential(ctx, &user, wgClient); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	// Mark as ready.
	meta.SetStatusCondition(&user.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            "User is in sync with Warpgate",
		ObservedGeneration: user.Generation,
	})
	if err := r.Status().Update(ctx, &user); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// toWarpgateCredentialPolicy converts the CRD spec to the Warpgate client type.
func toWarpgateCredentialPolicy(spec *warpgatev1alpha1.CredentialPolicySpec) *warpgate.CredentialPolicy {
	if spec == nil {
		return nil
	}
	return &warpgate.CredentialPolicy{
		HTTP:     spec.HTTP,
		SSH:      spec.SSH,
		MySQL:    spec.MySQL,
		Postgres: spec.Postgres,
	}
}

// ensurePasswordCredential generates a random password, creates the credential in Warpgate,
// and stores the password in a Kubernetes Secret.
func (r *WarpgateUserReconciler) ensurePasswordCredential(ctx context.Context, user *warpgatev1alpha1.WarpgateUser, wgClient *warpgate.Client) error {
	log := logf.FromContext(ctx)

	pwLen := 32
	if user.Spec.PasswordLength != nil {
		pwLen = *user.Spec.PasswordLength
	}
	password, err := generateRandomPassword(pwLen)
	if err != nil {
		log.Error(err, "failed to generate random password")
		return err
	}

	cred, err := wgClient.CreatePasswordCredential(user.Status.ExternalID, password)
	if err != nil {
		log.Error(err, "failed to create password credential in Warpgate")
		meta.SetStatusCondition(&user.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "PasswordCredentialFailed",
			Message:            fmt.Sprintf("Failed to create password credential: %v", err),
			ObservedGeneration: user.Generation,
		})
		_ = r.Status().Update(ctx, user)
		return err
	}

	// Store the password in a Kubernetes Secret.
	secretName := user.Name + "-password"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: user.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "warpgate-operator",
				"app.kubernetes.io/name":       "warpgate-user-password",
				"app.kubernetes.io/instance":   user.Name,
			},
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"password": password,
			"username": user.Spec.Username,
		},
	}
	if err := ctrl.SetControllerReference(user, secret, r.Scheme); err != nil {
		log.Error(err, "failed to set owner reference on password secret")
		return err
	}
	if err := r.Create(ctx, secret); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			log.Error(err, "failed to create password secret")
			return err
		}
	}

	user.Status.PasswordCredentialID = cred.ID
	user.Status.PasswordSecretRef = secretName
	log.Info("created auto-generated password credential", "credentialID", cred.ID, "secretName", secretName)
	return nil
}

// generateRandomPassword generates a cryptographically random password of the given length.
func generateRandomPassword(length int) (string, error) {
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("reading random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf)[:length], nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WarpgateUserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&warpgatev1alpha1.WarpgateUser{}).
		Named("warpgateuser").
		Complete(r)
}
