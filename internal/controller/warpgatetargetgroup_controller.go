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

const targetGroupFinalizer = "warpgate.warp.tech/finalizer"

// WarpgateTargetGroupReconciler reconciles a WarpgateTargetGroup object.
type WarpgateTargetGroupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgatetargetgroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgatetargetgroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgatetargetgroups/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile handles the reconciliation loop for WarpgateTargetGroup resources.
func (r *WarpgateTargetGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the WarpgateTargetGroup CR.
	var group warpgatev1alpha1.WarpgateTargetGroup
	if err := r.Get(ctx, req.NamespacedName, &group); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Get the Warpgate API client from the referenced connection.
	wgClient, err := getWarpgateClient(ctx, r.Client, group.Namespace, group.Spec.ConnectionRef)
	if err != nil {
		log.Error(err, "Failed to get Warpgate client")
		meta.SetStatusCondition(&group.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "ClientError",
			Message:            fmt.Sprintf("Failed to get Warpgate client: %v", err),
			ObservedGeneration: group.Generation,
		})
		_ = r.Status().Update(ctx, &group)
		return ctrl.Result{}, err
	}

	// Handle deletion.
	if !group.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&group, targetGroupFinalizer) {
			if group.Status.ExternalID != "" {
				if err := wgClient.DeleteTargetGroup(group.Status.ExternalID); err != nil && !warpgate.IsNotFound(err) {
					log.Error(err, "Failed to delete TargetGroup in Warpgate")
					return ctrl.Result{}, err
				}
				log.Info("Deleted TargetGroup in Warpgate", "externalID", group.Status.ExternalID)
			}
			controllerutil.RemoveFinalizer(&group, targetGroupFinalizer)
			if err := r.Update(ctx, &group); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if missing.
	if !controllerutil.ContainsFinalizer(&group, targetGroupFinalizer) {
		controllerutil.AddFinalizer(&group, targetGroupFinalizer)
		if err := r.Update(ctx, &group); err != nil {
			return ctrl.Result{}, err
		}
	}

	groupReq := warpgate.TargetGroupRequest{
		Name:        group.Spec.Name,
		Description: group.Spec.Description,
		Color:       group.Spec.Color,
	}

	if group.Status.ExternalID == "" {
		// Create the target group in Warpgate.
		created, err := wgClient.CreateTargetGroup(groupReq)
		if err != nil {
			log.Error(err, "Failed to create TargetGroup in Warpgate")
			meta.SetStatusCondition(&group.Status.Conditions, metav1.Condition{
				Type:               "Ready",
				Status:             metav1.ConditionFalse,
				Reason:             "CreateFailed",
				Message:            fmt.Sprintf("Failed to create target group: %v", err),
				ObservedGeneration: group.Generation,
			})
			_ = r.Status().Update(ctx, &group)
			return ctrl.Result{}, err
		}
		group.Status.ExternalID = created.ID
		log.Info("Created TargetGroup in Warpgate", "externalID", created.ID)
	} else {
		// Update the existing target group in Warpgate.
		if _, err := wgClient.UpdateTargetGroup(group.Status.ExternalID, groupReq); err != nil {
			if warpgate.IsNotFound(err) {
				log.Info("TargetGroup not found in Warpgate, will recreate", "externalID", group.Status.ExternalID)
				group.Status.ExternalID = ""
				meta.SetStatusCondition(&group.Status.Conditions, metav1.Condition{
					Type:               "Ready",
					Status:             metav1.ConditionFalse,
					Reason:             "NotFound",
					Message:            "Target group was deleted externally, will recreate on next reconcile",
					ObservedGeneration: group.Generation,
				})
				_ = r.Status().Update(ctx, &group)
				return ctrl.Result{Requeue: true}, nil
			}
			log.Error(err, "Failed to update TargetGroup in Warpgate")
			meta.SetStatusCondition(&group.Status.Conditions, metav1.Condition{
				Type:               "Ready",
				Status:             metav1.ConditionFalse,
				Reason:             "UpdateFailed",
				Message:            fmt.Sprintf("Failed to update target group: %v", err),
				ObservedGeneration: group.Generation,
			})
			_ = r.Status().Update(ctx, &group)
			return ctrl.Result{}, err
		}
		log.Info("Updated TargetGroup in Warpgate", "externalID", group.Status.ExternalID)
	}

	// Mark as ready.
	meta.SetStatusCondition(&group.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            "Target group is in sync with Warpgate",
		ObservedGeneration: group.Generation,
	})
	if err := r.Status().Update(ctx, &group); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WarpgateTargetGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&warpgatev1alpha1.WarpgateTargetGroup{}).
		Named("warpgatetargetgroup").
		Complete(r)
}
