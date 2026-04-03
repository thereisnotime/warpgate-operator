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
	"k8s.io/apimachinery/pkg/api/errors"
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

const ticketFinalizer = "warpgate.warp.tech/finalizer"

// WarpgateTicketReconciler reconciles a WarpgateTicket object.
type WarpgateTicketReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgatetickets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgatetickets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgatetickets/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;delete

// Reconcile handles the reconciliation loop for WarpgateTicket resources.
func (r *WarpgateTicketReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the CR.
	var ticket warpgatev1alpha1.WarpgateTicket
	if err := r.Get(ctx, req.NamespacedName, &ticket); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Get the Warpgate API client from the referenced connection.
	wgClient, err := getWarpgateClient(ctx, r.Client, ticket.Namespace, ticket.Spec.ConnectionRef)
	if err != nil {
		log.Error(err, "failed to get warpgate client")
		meta.SetStatusCondition(&ticket.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "ClientError",
			Message:            fmt.Sprintf("Failed to get Warpgate client: %v", err),
			ObservedGeneration: ticket.Generation,
		})
		_ = r.Status().Update(ctx, &ticket)
		return ctrl.Result{}, err
	}

	// Handle deletion.
	if !ticket.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&ticket, ticketFinalizer) {
			if ticket.Status.TicketID != "" {
				if err := wgClient.DeleteTicket(ticket.Status.TicketID); err != nil && !warpgate.IsNotFound(err) {
					log.Error(err, "failed to delete ticket in Warpgate")
					return ctrl.Result{}, err
				}
				log.Info("deleted ticket in Warpgate", "ticketID", ticket.Status.TicketID)
			}
			// Clean up the auto-created Secret.
			if ticket.Status.SecretRef != "" {
				var secret corev1.Secret
				if err := r.Get(ctx, types.NamespacedName{
					Name:      ticket.Status.SecretRef,
					Namespace: ticket.Namespace,
				}, &secret); err == nil {
					if err := r.Delete(ctx, &secret); err != nil && !errors.IsNotFound(err) {
						log.Error(err, "failed to delete ticket secret", "secret", ticket.Status.SecretRef)
						return ctrl.Result{}, err
					}
					log.Info("deleted ticket secret", "secret", ticket.Status.SecretRef)
				}
			}
			controllerutil.RemoveFinalizer(&ticket, ticketFinalizer)
			if err := r.Update(ctx, &ticket); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if missing.
	if !controllerutil.ContainsFinalizer(&ticket, ticketFinalizer) {
		controllerutil.AddFinalizer(&ticket, ticketFinalizer)
		if err := r.Update(ctx, &ticket); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Tickets are immutable — only create if we don't have one yet.
	if ticket.Status.TicketID == "" {
		createReq := warpgate.TicketCreateRequest{
			Username:     ticket.Spec.Username,
			TargetName:   ticket.Spec.TargetName,
			Expiry:       ticket.Spec.Expiry,
			NumberOfUses: ticket.Spec.NumberOfUses,
			Description:  ticket.Spec.Description,
		}
		result, err := wgClient.CreateTicket(createReq)
		if err != nil {
			log.Error(err, "failed to create ticket in Warpgate")
			meta.SetStatusCondition(&ticket.Status.Conditions, metav1.Condition{
				Type:               "Ready",
				Status:             metav1.ConditionFalse,
				Reason:             "CreateFailed",
				Message:            fmt.Sprintf("Failed to create ticket: %v", err),
				ObservedGeneration: ticket.Generation,
			})
			_ = r.Status().Update(ctx, &ticket)
			return ctrl.Result{}, err
		}
		ticket.Status.TicketID = result.Ticket.ID
		log.Info("created ticket in Warpgate", "ticketID", result.Ticket.ID)

		// Create a Secret containing the ticket secret value.
		secretName := ticket.Name + "-secret"
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: ticket.Namespace,
			},
			StringData: map[string]string{
				"secret": result.Secret,
			},
		}
		if err := ctrl.SetControllerReference(&ticket, secret, r.Scheme); err != nil {
			log.Error(err, "failed to set owner reference on ticket secret")
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, secret); err != nil {
			log.Error(err, "failed to create ticket secret")
			meta.SetStatusCondition(&ticket.Status.Conditions, metav1.Condition{
				Type:               "Ready",
				Status:             metav1.ConditionFalse,
				Reason:             "SecretCreateFailed",
				Message:            fmt.Sprintf("Failed to create ticket secret: %v", err),
				ObservedGeneration: ticket.Generation,
			})
			_ = r.Status().Update(ctx, &ticket)
			return ctrl.Result{}, err
		}
		ticket.Status.SecretRef = secretName
		log.Info("created ticket secret", "secret", secretName)
	}

	// Mark as ready.
	meta.SetStatusCondition(&ticket.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            "Ticket is in sync with Warpgate",
		ObservedGeneration: ticket.Generation,
	})
	if err := r.Status().Update(ctx, &ticket); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WarpgateTicketReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&warpgatev1alpha1.WarpgateTicket{}).
		Named("warpgateticket").
		Complete(r)
}
