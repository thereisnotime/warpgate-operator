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

const roleFinalizer = "warpgate.warp.tech/finalizer"

// WarpgateRoleReconciler reconciles a WarpgateRole object.
type WarpgateRoleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgateroles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgateroles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgateroles/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile handles the reconciliation loop for WarpgateRole resources.
func (r *WarpgateRoleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the WarpgateRole CR.
	var role warpgatev1alpha1.WarpgateRole
	if err := r.Get(ctx, req.NamespacedName, &role); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Get the Warpgate API client from the referenced connection.
	wgClient, err := getWarpgateClient(ctx, r.Client, role.Namespace, role.Spec.ConnectionRef)
	if err != nil {
		log.Error(err, "failed to get warpgate client")
		meta.SetStatusCondition(&role.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "ClientError",
			Message:            fmt.Sprintf("Failed to get Warpgate client: %v", err),
			ObservedGeneration: role.Generation,
		})
		_ = r.Status().Update(ctx, &role)
		return ctrl.Result{}, err
	}

	// Handle deletion.
	if !role.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&role, roleFinalizer) {
			if role.Status.ExternalID != "" {
				if err := wgClient.DeleteRole(role.Status.ExternalID); err != nil && !warpgate.IsNotFound(err) {
					log.Error(err, "failed to delete role in Warpgate")
					return ctrl.Result{}, err
				}
				log.Info("deleted role in Warpgate", "externalID", role.Status.ExternalID)
			}
			controllerutil.RemoveFinalizer(&role, roleFinalizer)
			if err := r.Update(ctx, &role); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if missing.
	if !controllerutil.ContainsFinalizer(&role, roleFinalizer) {
		controllerutil.AddFinalizer(&role, roleFinalizer)
		if err := r.Update(ctx, &role); err != nil {
			return ctrl.Result{}, err
		}
	}

	roleReq := warpgate.RoleCreateRequest{
		Name:        role.Spec.Name,
		Description: role.Spec.Description,
	}

	if role.Status.ExternalID == "" {
		// Create the role in Warpgate.
		created, err := wgClient.CreateRole(roleReq)
		if err != nil {
			log.Error(err, "failed to create role in Warpgate")
			meta.SetStatusCondition(&role.Status.Conditions, metav1.Condition{
				Type:               "Ready",
				Status:             metav1.ConditionFalse,
				Reason:             "CreateFailed",
				Message:            fmt.Sprintf("Failed to create role: %v", err),
				ObservedGeneration: role.Generation,
			})
			_ = r.Status().Update(ctx, &role)
			return ctrl.Result{}, err
		}
		role.Status.ExternalID = created.ID
		log.Info("created role in Warpgate", "externalID", created.ID)
	} else {
		// Update the existing role in Warpgate.
		if _, err := wgClient.UpdateRole(role.Status.ExternalID, roleReq); err != nil {
			if warpgate.IsNotFound(err) {
				log.Info("role not found in Warpgate, will recreate", "externalID", role.Status.ExternalID)
				role.Status.ExternalID = ""
				meta.SetStatusCondition(&role.Status.Conditions, metav1.Condition{
					Type:               "Ready",
					Status:             metav1.ConditionFalse,
					Reason:             "NotFound",
					Message:            "Role was deleted externally, will recreate on next reconcile",
					ObservedGeneration: role.Generation,
				})
				_ = r.Status().Update(ctx, &role)
				return ctrl.Result{Requeue: true}, nil
			}
			log.Error(err, "failed to update role in Warpgate")
			meta.SetStatusCondition(&role.Status.Conditions, metav1.Condition{
				Type:               "Ready",
				Status:             metav1.ConditionFalse,
				Reason:             "UpdateFailed",
				Message:            fmt.Sprintf("Failed to update role: %v", err),
				ObservedGeneration: role.Generation,
			})
			_ = r.Status().Update(ctx, &role)
			return ctrl.Result{}, err
		}
		log.Info("updated role in Warpgate", "externalID", role.Status.ExternalID)
	}

	// Mark as ready.
	meta.SetStatusCondition(&role.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            "Role is in sync with Warpgate",
		ObservedGeneration: role.Generation,
	})
	if err := r.Status().Update(ctx, &role); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WarpgateRoleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&warpgatev1alpha1.WarpgateRole{}).
		Named("warpgaterole").
		Complete(r)
}
