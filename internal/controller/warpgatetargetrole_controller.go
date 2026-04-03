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

const warpgateTargetRoleFinalizer = "warpgate.warp.tech/finalizer"

// WarpgateTargetRoleReconciler reconciles a WarpgateTargetRole object
type WarpgateTargetRoleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgatetargetroles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgatetargetroles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgatetargetroles/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile handles reconciliation of WarpgateTargetRole resources.
func (r *WarpgateTargetRoleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the CR.
	var targetRole warpgatev1alpha1.WarpgateTargetRole
	if err := r.Get(ctx, req.NamespacedName, &targetRole); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Build the Warpgate API client from the referenced connection.
	wgClient, err := getWarpgateClient(ctx, r.Client, targetRole.Namespace, targetRole.Spec.ConnectionRef)
	if err != nil {
		log.Error(err, "unable to build warpgate client")
		setTargetRoleCondition(&targetRole, metav1.ConditionFalse, "ClientError", err.Error())
		if updateErr := r.Status().Update(ctx, &targetRole); updateErr != nil {
			log.Error(updateErr, "unable to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Handle deletion via finalizer.
	if !targetRole.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&targetRole, warpgateTargetRoleFinalizer) {
			if targetRole.Status.TargetID != "" && targetRole.Status.RoleID != "" {
				if err := wgClient.DeleteTargetRole(targetRole.Status.TargetID, targetRole.Status.RoleID); err != nil && !warpgate.IsNotFound(err) {
					log.Error(err, "unable to delete target-role binding from warpgate")
					return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
				}
			}
			controllerutil.RemoveFinalizer(&targetRole, warpgateTargetRoleFinalizer)
			if err := r.Update(ctx, &targetRole); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Ensure the finalizer is present.
	if !controllerutil.ContainsFinalizer(&targetRole, warpgateTargetRoleFinalizer) {
		controllerutil.AddFinalizer(&targetRole, warpgateTargetRoleFinalizer)
		if err := r.Update(ctx, &targetRole); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Resolve target ID from target name.
	target, err := wgClient.GetTargetByName(targetRole.Spec.TargetName)
	if err != nil {
		msg := fmt.Sprintf("unable to resolve target %q: %v", targetRole.Spec.TargetName, err)
		log.Error(err, "unable to resolve target", "targetName", targetRole.Spec.TargetName)
		setTargetRoleCondition(&targetRole, metav1.ConditionFalse, "TargetNotFound", msg)
		if updateErr := r.Status().Update(ctx, &targetRole); updateErr != nil {
			log.Error(updateErr, "unable to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Resolve role ID from role name.
	role, err := wgClient.GetRoleByName(targetRole.Spec.RoleName)
	if err != nil {
		msg := fmt.Sprintf("unable to resolve role %q: %v", targetRole.Spec.RoleName, err)
		log.Error(err, "unable to resolve role", "roleName", targetRole.Spec.RoleName)
		setTargetRoleCondition(&targetRole, metav1.ConditionFalse, "RoleNotFound", msg)
		if updateErr := r.Status().Update(ctx, &targetRole); updateErr != nil {
			log.Error(updateErr, "unable to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Store resolved IDs in status.
	targetRole.Status.TargetID = target.ID
	targetRole.Status.RoleID = role.ID

	// Create binding (idempotent).
	if err := wgClient.CreateTargetRole(target.ID, role.ID); err != nil {
		msg := fmt.Sprintf("unable to create target-role binding: %v", err)
		log.Error(err, "unable to create target-role binding")
		setTargetRoleCondition(&targetRole, metav1.ConditionFalse, "BindingFailed", msg)
		if updateErr := r.Status().Update(ctx, &targetRole); updateErr != nil {
			log.Error(updateErr, "unable to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// All good — mark Ready.
	setTargetRoleCondition(&targetRole, metav1.ConditionTrue, "Bound", "target-role binding is active")
	if err := r.Status().Update(ctx, &targetRole); err != nil {
		log.Error(err, "unable to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func setTargetRoleCondition(tr *warpgatev1alpha1.WarpgateTargetRole, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               "Ready",
		Status:             status,
		ObservedGeneration: tr.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
	for i, c := range tr.Status.Conditions {
		if c.Type == condition.Type {
			if c.Status != condition.Status {
				tr.Status.Conditions[i] = condition
			} else {
				tr.Status.Conditions[i].Reason = reason
				tr.Status.Conditions[i].Message = message
				tr.Status.Conditions[i].ObservedGeneration = tr.Generation
			}
			return
		}
	}
	tr.Status.Conditions = append(tr.Status.Conditions, condition)
}

// SetupWithManager sets up the controller with the Manager.
func (r *WarpgateTargetRoleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&warpgatev1alpha1.WarpgateTargetRole{}).
		Named("warpgatetargetrole").
		Complete(r)
}
