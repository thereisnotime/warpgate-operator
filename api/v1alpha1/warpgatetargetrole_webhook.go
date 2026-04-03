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

// +kubebuilder:webhook:path=/mutate-warpgate-warpgate-warp-tech-v1alpha1-warpgatetargetrole,mutating=true,failurePolicy=fail,sideEffects=None,groups=warpgate.warpgate.warp.tech,resources=warpgatetargetroles,verbs=create;update,versions=v1alpha1,name=mwarpgatetargetrole.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-warpgate-warpgate-warp-tech-v1alpha1-warpgatetargetrole,mutating=false,failurePolicy=fail,sideEffects=None,groups=warpgate.warpgate.warp.tech,resources=warpgatetargetroles,verbs=create;update;delete,versions=v1alpha1,name=vwarpgatetargetrole.kb.io,admissionReviewVersions=v1

// WarpgateTargetRoleCustomDefaulter handles defaulting for WarpgateTargetRole.
type WarpgateTargetRoleCustomDefaulter struct{}

// WarpgateTargetRoleCustomValidator handles validation for WarpgateTargetRole.
type WarpgateTargetRoleCustomValidator struct{}

// SetupWebhookWithManager registers the webhooks for WarpgateTargetRole.
func (r *WarpgateTargetRole) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, r).
		WithDefaulter(&WarpgateTargetRoleCustomDefaulter{}).
		WithValidator(&WarpgateTargetRoleCustomValidator{}).
		Complete()
}

// Default is a no-op for WarpgateTargetRole (no defaults needed).
func (d *WarpgateTargetRoleCustomDefaulter) Default(_ context.Context, _ *WarpgateTargetRole) error {
	return nil
}

// ValidateCreate validates a new WarpgateTargetRole.
func (v *WarpgateTargetRoleCustomValidator) ValidateCreate(_ context.Context, tr *WarpgateTargetRole) (admission.Warnings, error) {
	return nil, validateWarpgateTargetRoleSpec(&tr.Spec)
}

// ValidateUpdate validates an updated WarpgateTargetRole.
func (v *WarpgateTargetRoleCustomValidator) ValidateUpdate(_ context.Context, _, tr *WarpgateTargetRole) (admission.Warnings, error) {
	return nil, validateWarpgateTargetRoleSpec(&tr.Spec)
}

// ValidateDelete is a no-op for WarpgateTargetRole.
func (v *WarpgateTargetRoleCustomValidator) ValidateDelete(_ context.Context, _ *WarpgateTargetRole) (admission.Warnings, error) {
	return nil, nil
}

func validateWarpgateTargetRoleSpec(spec *WarpgateTargetRoleSpec) error {
	if spec.ConnectionRef == "" {
		return fmt.Errorf("spec.connectionRef must not be empty")
	}
	if spec.TargetName == "" {
		return fmt.Errorf("spec.targetName must not be empty")
	}
	if spec.RoleName == "" {
		return fmt.Errorf("spec.roleName must not be empty")
	}
	return nil
}
