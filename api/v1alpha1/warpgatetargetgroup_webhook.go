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

// +kubebuilder:webhook:path=/mutate-warpgate-warpgate-warp-tech-v1alpha1-warpgatetargetgroup,mutating=true,failurePolicy=fail,sideEffects=None,groups=warpgate.warpgate.warp.tech,resources=warpgatetargetgroups,verbs=create;update,versions=v1alpha1,name=mwarpgatetargetgroup.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-warpgate-warpgate-warp-tech-v1alpha1-warpgatetargetgroup,mutating=false,failurePolicy=fail,sideEffects=None,groups=warpgate.warpgate.warp.tech,resources=warpgatetargetgroups,verbs=create;update;delete,versions=v1alpha1,name=vwarpgatetargetgroup.kb.io,admissionReviewVersions=v1

// WarpgateTargetGroupCustomDefaulter handles defaulting for WarpgateTargetGroup.
type WarpgateTargetGroupCustomDefaulter struct{}

// WarpgateTargetGroupCustomValidator handles validation for WarpgateTargetGroup.
type WarpgateTargetGroupCustomValidator struct{}

// SetupWebhookWithManager registers the webhooks for WarpgateTargetGroup.
func (r *WarpgateTargetGroup) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, r).
		WithDefaulter(&WarpgateTargetGroupCustomDefaulter{}).
		WithValidator(&WarpgateTargetGroupCustomValidator{}).
		Complete()
}

// Default is a no-op for WarpgateTargetGroup (no defaults needed).
func (d *WarpgateTargetGroupCustomDefaulter) Default(ctx context.Context, group *WarpgateTargetGroup) error {
	return nil
}

// ValidateCreate validates a new WarpgateTargetGroup.
func (v *WarpgateTargetGroupCustomValidator) ValidateCreate(ctx context.Context, group *WarpgateTargetGroup) (admission.Warnings, error) {
	return validateTargetGroup(group)
}

// ValidateUpdate validates an updated WarpgateTargetGroup.
func (v *WarpgateTargetGroupCustomValidator) ValidateUpdate(ctx context.Context, oldGroup, group *WarpgateTargetGroup) (admission.Warnings, error) {
	return validateTargetGroup(group)
}

// ValidateDelete is a no-op for WarpgateTargetGroup.
func (v *WarpgateTargetGroupCustomValidator) ValidateDelete(ctx context.Context, group *WarpgateTargetGroup) (admission.Warnings, error) {
	return nil, nil
}

func validateTargetGroup(group *WarpgateTargetGroup) (admission.Warnings, error) {
	if group.Spec.ConnectionRef == "" {
		return nil, fmt.Errorf("spec.connectionRef must not be empty")
	}
	if group.Spec.Name == "" {
		return nil, fmt.Errorf("spec.name must not be empty")
	}
	return nil, nil
}
