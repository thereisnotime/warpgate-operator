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

package v1alpha1

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/mutate-warpgate-warpgate-warp-tech-v1alpha1-warpgateticket,mutating=true,failurePolicy=fail,sideEffects=None,groups=warpgate.warpgate.warp.tech,resources=warpgatetickets,verbs=create;update,versions=v1alpha1,name=mwarpgateticket.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-warpgate-warpgate-warp-tech-v1alpha1-warpgateticket,mutating=false,failurePolicy=fail,sideEffects=None,groups=warpgate.warpgate.warp.tech,resources=warpgatetickets,verbs=create;update;delete,versions=v1alpha1,name=vwarpgateticket.kb.io,admissionReviewVersions=v1

// WarpgateTicketCustomDefaulter handles defaulting for WarpgateTicket.
type WarpgateTicketCustomDefaulter struct{}

// WarpgateTicketCustomValidator handles validation for WarpgateTicket.
type WarpgateTicketCustomValidator struct{}

// SetupWebhookWithManager registers the webhooks for WarpgateTicket.
func (r *WarpgateTicket) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, r).
		WithDefaulter(&WarpgateTicketCustomDefaulter{}).
		WithValidator(&WarpgateTicketCustomValidator{}).
		Complete()
}

// Default is a no-op for WarpgateTicket (no defaults needed).
func (d *WarpgateTicketCustomDefaulter) Default(_ context.Context, _ *WarpgateTicket) error {
	return nil
}

// ValidateCreate validates a new WarpgateTicket.
func (v *WarpgateTicketCustomValidator) ValidateCreate(_ context.Context, t *WarpgateTicket) (admission.Warnings, error) {
	return validateWarpgateTicketSpec(&t.Spec)
}

// ValidateUpdate validates an updated WarpgateTicket.
// Tickets are immutable: reject any spec change.
func (v *WarpgateTicketCustomValidator) ValidateUpdate(_ context.Context, oldTicket, newTicket *WarpgateTicket) (admission.Warnings, error) {
	if !ticketSpecEqual(&oldTicket.Spec, &newTicket.Spec) {
		return nil, fmt.Errorf("WarpgateTicket spec is immutable; delete and recreate the ticket instead")
	}
	return nil, nil
}

// ValidateDelete is a no-op for WarpgateTicket.
func (v *WarpgateTicketCustomValidator) ValidateDelete(_ context.Context, _ *WarpgateTicket) (admission.Warnings, error) {
	return nil, nil
}

func validateWarpgateTicketSpec(spec *WarpgateTicketSpec) (admission.Warnings, error) {
	if spec.ConnectionRef == "" {
		return nil, fmt.Errorf("spec.connectionRef must not be empty")
	}
	if spec.NumberOfUses != nil && *spec.NumberOfUses <= 0 {
		return nil, fmt.Errorf("spec.numberOfUses must be > 0 when set")
	}

	var warnings admission.Warnings
	if spec.Username == "" && spec.TargetName == "" {
		warnings = append(warnings, "neither spec.username nor spec.targetName is set; the ticket may not be very useful")
	}
	return warnings, nil
}

// ticketSpecEqual compares two WarpgateTicketSpec values for equality.
func ticketSpecEqual(a, b *WarpgateTicketSpec) bool {
	if a.ConnectionRef != b.ConnectionRef {
		return false
	}
	if a.Username != b.Username {
		return false
	}
	if a.TargetName != b.TargetName {
		return false
	}
	if a.Expiry != b.Expiry {
		return false
	}
	if a.Description != b.Description {
		return false
	}
	// Compare numberOfUses pointers.
	if a.NumberOfUses == nil && b.NumberOfUses == nil {
		return true
	}
	if a.NumberOfUses == nil || b.NumberOfUses == nil {
		return false
	}
	return *a.NumberOfUses == *b.NumberOfUses
}
