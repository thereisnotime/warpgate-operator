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

// +kubebuilder:webhook:path=/mutate-warpgate-warpgate-warp-tech-v1alpha1-warpgaterole,mutating=true,failurePolicy=fail,sideEffects=None,groups=warpgate.warpgate.warp.tech,resources=warpgateroles,verbs=create;update,versions=v1alpha1,name=mwarpgaterole.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-warpgate-warpgate-warp-tech-v1alpha1-warpgaterole,mutating=false,failurePolicy=fail,sideEffects=None,groups=warpgate.warpgate.warp.tech,resources=warpgateroles,verbs=create;update;delete,versions=v1alpha1,name=vwarpgaterole.kb.io,admissionReviewVersions=v1

// WarpgateRoleCustomDefaulter handles defaulting for WarpgateRole.
type WarpgateRoleCustomDefaulter struct{}

// WarpgateRoleCustomValidator handles validation for WarpgateRole.
type WarpgateRoleCustomValidator struct{}

// SetupWebhookWithManager registers the webhooks for WarpgateRole.
func (r *WarpgateRole) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, r).
		WithDefaulter(&WarpgateRoleCustomDefaulter{}).
		WithValidator(&WarpgateRoleCustomValidator{}).
		Complete()
}

// Default is a no-op for WarpgateRole (no defaults needed).
func (d *WarpgateRoleCustomDefaulter) Default(ctx context.Context, role *WarpgateRole) error {
	return nil
}

// ValidateCreate validates a new WarpgateRole.
func (v *WarpgateRoleCustomValidator) ValidateCreate(ctx context.Context, role *WarpgateRole) (admission.Warnings, error) {
	return validateRole(role)
}

// ValidateUpdate validates an updated WarpgateRole.
func (v *WarpgateRoleCustomValidator) ValidateUpdate(ctx context.Context, oldRole, role *WarpgateRole) (admission.Warnings, error) {
	return validateRole(role)
}

// ValidateDelete is a no-op for WarpgateRole.
func (v *WarpgateRoleCustomValidator) ValidateDelete(ctx context.Context, role *WarpgateRole) (admission.Warnings, error) {
	return nil, nil
}

func validateRole(role *WarpgateRole) (admission.Warnings, error) {
	if role.Spec.ConnectionRef == "" {
		return nil, fmt.Errorf("spec.connectionRef must not be empty")
	}
	if role.Spec.Name == "" {
		return nil, fmt.Errorf("spec.name must not be empty")
	}
	return nil, nil
}
