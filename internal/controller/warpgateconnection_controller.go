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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	warpgatev1alpha1 "github.com/thereisnotime/warpgate-operator/api/v1alpha1"
)

// WarpgateConnectionReconciler reconciles a WarpgateConnection object
type WarpgateConnectionReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	ReconcileInterval time.Duration
}

const warpgateFinalizer = "warpgate.warp.tech/finalizer"

// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgateconnections,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgateconnections/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgateconnections/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile validates the WarpgateConnection by attempting to connect to the
// Warpgate instance and updates the status conditions accordingly.
func (r *WarpgateConnectionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the WarpgateConnection CR.
	var conn warpgatev1alpha1.WarpgateConnection
	if err := r.Get(ctx, req.NamespacedName, &conn); err != nil {
		if apierrors.IsNotFound(err) {
			warpgateClients.evict(req.NamespacedName)
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion.
	if !conn.DeletionTimestamp.IsZero() {
		warpgateClients.evict(req.NamespacedName)
		if controllerutil.ContainsFinalizer(&conn, warpgateFinalizer) {
			controllerutil.RemoveFinalizer(&conn, warpgateFinalizer)
			if err := r.Update(ctx, &conn); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present.
	if !controllerutil.ContainsFinalizer(&conn, warpgateFinalizer) {
		controllerutil.AddFinalizer(&conn, warpgateFinalizer)
		if err := r.Update(ctx, &conn); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Build a Warpgate client from the referenced Secret.
	wgClient, err := clientForConnection(ctx, r.Client, &conn)
	if err != nil {
		log.Error(err, "failed to build Warpgate client")
		r.setReadyCondition(&conn, metav1.ConditionFalse, "ConnectionFailed", fmt.Sprintf("Failed to build client: %v", err))
		if updateErr := r.Status().Update(ctx, &conn); updateErr != nil {
			log.Error(updateErr, "failed to update status")
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{RequeueAfter: r.ReconcileInterval}, nil
	}

	// Validate the connection by listing roles.
	if _, err := wgClient.ListRoles(""); err != nil {
		log.Error(err, "Warpgate connection check failed")
		r.setReadyCondition(&conn, metav1.ConditionFalse, "ConnectionFailed", fmt.Sprintf("Connection check failed: %v", err))
		if updateErr := r.Status().Update(ctx, &conn); updateErr != nil {
			log.Error(updateErr, "failed to update status")
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{RequeueAfter: r.ReconcileInterval}, nil
	}

	// Connection is healthy.
	log.Info("Warpgate connection validated successfully")
	r.setReadyCondition(&conn, metav1.ConditionTrue, "Connected", "Successfully connected to Warpgate")
	if err := r.Status().Update(ctx, &conn); err != nil {
		log.Error(err, "failed to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: r.ReconcileInterval}, nil
}

// setReadyCondition updates the Ready condition on the WarpgateConnection status.
func (r *WarpgateConnectionReconciler) setReadyCondition(conn *warpgatev1alpha1.WarpgateConnection, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&conn.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             status,
		ObservedGeneration: conn.Generation,
		Reason:             reason,
		Message:            message,
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *WarpgateConnectionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&warpgatev1alpha1.WarpgateConnection{}).
		Named("warpgateconnection").
		Complete(r)
}
