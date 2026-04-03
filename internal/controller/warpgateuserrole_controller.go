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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	warpgatev1alpha1 "github.com/thereisnotime/warpgate-operator/api/v1alpha1"
	"github.com/thereisnotime/warpgate-operator/internal/warpgate"
)

const warpgateUserRoleFinalizer = "warpgate.warp.tech/finalizer"

// WarpgateUserRoleReconciler reconciles a WarpgateUserRole object
type WarpgateUserRoleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgateuserroles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgateuserroles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgateuserroles/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile handles reconciliation of WarpgateUserRole resources.
func (r *WarpgateUserRoleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the CR.
	var userRole warpgatev1alpha1.WarpgateUserRole
	if err := r.Get(ctx, req.NamespacedName, &userRole); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Build the Warpgate API client from the referenced connection.
	wgClient, err := getWarpgateClient(ctx, r.Client, userRole.Namespace, userRole.Spec.ConnectionRef)
	if err != nil {
		log.Error(err, "unable to build warpgate client")
		setUserRoleCondition(&userRole, metav1.ConditionFalse, "ClientError", err.Error())
		if updateErr := r.Status().Update(ctx, &userRole); updateErr != nil {
			log.Error(updateErr, "unable to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Handle deletion via finalizer.
	if !userRole.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&userRole, warpgateUserRoleFinalizer) {
			if userRole.Status.UserID != "" && userRole.Status.RoleID != "" {
				if err := wgClient.DeleteUserRole(userRole.Status.UserID, userRole.Status.RoleID); err != nil && !warpgate.IsNotFound(err) {
					log.Error(err, "unable to delete user-role binding from warpgate")
					return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
				}
			}
			controllerutil.RemoveFinalizer(&userRole, warpgateUserRoleFinalizer)
			if err := r.Update(ctx, &userRole); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Ensure the finalizer is present.
	if !controllerutil.ContainsFinalizer(&userRole, warpgateUserRoleFinalizer) {
		controllerutil.AddFinalizer(&userRole, warpgateUserRoleFinalizer)
		if err := r.Update(ctx, &userRole); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Resolve user ID from username.
	user, err := wgClient.GetUserByUsername(userRole.Spec.Username)
	if err != nil {
		msg := fmt.Sprintf("unable to resolve user %q: %v", userRole.Spec.Username, err)
		log.Error(err, "unable to resolve user", "username", userRole.Spec.Username)
		setUserRoleCondition(&userRole, metav1.ConditionFalse, "UserNotFound", msg)
		if updateErr := r.Status().Update(ctx, &userRole); updateErr != nil {
			log.Error(updateErr, "unable to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Resolve role ID from role name.
	role, err := wgClient.GetRoleByName(userRole.Spec.RoleName)
	if err != nil {
		msg := fmt.Sprintf("unable to resolve role %q: %v", userRole.Spec.RoleName, err)
		log.Error(err, "unable to resolve role", "roleName", userRole.Spec.RoleName)
		setUserRoleCondition(&userRole, metav1.ConditionFalse, "RoleNotFound", msg)
		if updateErr := r.Status().Update(ctx, &userRole); updateErr != nil {
			log.Error(updateErr, "unable to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Store resolved IDs in status.
	userRole.Status.UserID = user.ID
	userRole.Status.RoleID = role.ID

	// Create binding (idempotent).
	if err := wgClient.CreateUserRole(user.ID, role.ID); err != nil {
		msg := fmt.Sprintf("unable to create user-role binding: %v", err)
		log.Error(err, "unable to create user-role binding")
		setUserRoleCondition(&userRole, metav1.ConditionFalse, "BindingFailed", msg)
		if updateErr := r.Status().Update(ctx, &userRole); updateErr != nil {
			log.Error(updateErr, "unable to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// All good — mark Ready.
	setUserRoleCondition(&userRole, metav1.ConditionTrue, "Bound", "user-role binding is active")
	if err := r.Status().Update(ctx, &userRole); err != nil {
		log.Error(err, "unable to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func setUserRoleCondition(ur *warpgatev1alpha1.WarpgateUserRole, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               "Ready",
		Status:             status,
		ObservedGeneration: ur.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
	for i, c := range ur.Status.Conditions {
		if c.Type == condition.Type {
			if c.Status != condition.Status {
				ur.Status.Conditions[i] = condition
			} else {
				ur.Status.Conditions[i].Reason = reason
				ur.Status.Conditions[i].Message = message
				ur.Status.Conditions[i].ObservedGeneration = ur.Generation
			}
			return
		}
	}
	ur.Status.Conditions = append(ur.Status.Conditions, condition)
}

// SetupWithManager sets up the controller with the Manager.
func (r *WarpgateUserRoleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&warpgatev1alpha1.WarpgateUserRole{}).
		Named("warpgateuserrole").
		Complete(r)
}
