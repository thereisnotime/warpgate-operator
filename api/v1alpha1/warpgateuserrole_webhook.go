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

// +kubebuilder:webhook:path=/mutate-warpgate-warpgate-warp-tech-v1alpha1-warpgateuserrole,mutating=true,failurePolicy=fail,sideEffects=None,groups=warpgate.warpgate.warp.tech,resources=warpgateuserroles,verbs=create;update,versions=v1alpha1,name=mwarpgateuserrole.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-warpgate-warpgate-warp-tech-v1alpha1-warpgateuserrole,mutating=false,failurePolicy=fail,sideEffects=None,groups=warpgate.warpgate.warp.tech,resources=warpgateuserroles,verbs=create;update;delete,versions=v1alpha1,name=vwarpgateuserrole.kb.io,admissionReviewVersions=v1

// WarpgateUserRoleCustomDefaulter handles defaulting for WarpgateUserRole.
type WarpgateUserRoleCustomDefaulter struct{}

// WarpgateUserRoleCustomValidator handles validation for WarpgateUserRole.
type WarpgateUserRoleCustomValidator struct{}

// SetupWebhookWithManager registers the webhooks for WarpgateUserRole.
func (r *WarpgateUserRole) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, r).
		WithDefaulter(&WarpgateUserRoleCustomDefaulter{}).
		WithValidator(&WarpgateUserRoleCustomValidator{}).
		Complete()
}

// Default is a no-op for WarpgateUserRole (no defaults needed).
func (d *WarpgateUserRoleCustomDefaulter) Default(_ context.Context, _ *WarpgateUserRole) error {
	return nil
}

// ValidateCreate validates a new WarpgateUserRole.
func (v *WarpgateUserRoleCustomValidator) ValidateCreate(_ context.Context, ur *WarpgateUserRole) (admission.Warnings, error) {
	return nil, validateWarpgateUserRoleSpec(&ur.Spec)
}

// ValidateUpdate validates an updated WarpgateUserRole.
func (v *WarpgateUserRoleCustomValidator) ValidateUpdate(_ context.Context, _, ur *WarpgateUserRole) (admission.Warnings, error) {
	return nil, validateWarpgateUserRoleSpec(&ur.Spec)
}

// ValidateDelete is a no-op for WarpgateUserRole.
func (v *WarpgateUserRoleCustomValidator) ValidateDelete(_ context.Context, _ *WarpgateUserRole) (admission.Warnings, error) {
	return nil, nil
}

func validateWarpgateUserRoleSpec(spec *WarpgateUserRoleSpec) error {
	if spec.ConnectionRef == "" {
		return fmt.Errorf("spec.connectionRef must not be empty")
	}
	if spec.Username == "" {
		return fmt.Errorf("spec.username must not be empty")
	}
	if spec.RoleName == "" {
		return fmt.Errorf("spec.roleName must not be empty")
	}
	return nil
}
