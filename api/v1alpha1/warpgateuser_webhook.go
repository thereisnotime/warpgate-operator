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

// +kubebuilder:webhook:path=/mutate-warpgate-warpgate-warp-tech-v1alpha1-warpgateuser,mutating=true,failurePolicy=fail,sideEffects=None,groups=warpgate.warpgate.warp.tech,resources=warpgateusers,verbs=create;update,versions=v1alpha1,name=mwarpgateuser.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-warpgate-warpgate-warp-tech-v1alpha1-warpgateuser,mutating=false,failurePolicy=fail,sideEffects=None,groups=warpgate.warpgate.warp.tech,resources=warpgateusers,verbs=create;update;delete,versions=v1alpha1,name=vwarpgateuser.kb.io,admissionReviewVersions=v1

// WarpgateUserCustomDefaulter handles defaulting for WarpgateUser.
type WarpgateUserCustomDefaulter struct{}

// WarpgateUserCustomValidator handles validation for WarpgateUser.
type WarpgateUserCustomValidator struct{}

// SetupWebhookWithManager registers the webhooks for WarpgateUser.
func (r *WarpgateUser) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, r).
		WithDefaulter(&WarpgateUserCustomDefaulter{}).
		WithValidator(&WarpgateUserCustomValidator{}).
		Complete()
}

// Default sets sensible defaults for WarpgateUser fields.
func (d *WarpgateUserCustomDefaulter) Default(ctx context.Context, user *WarpgateUser) error {
	if user.Spec.GeneratePassword == nil {
		t := true
		user.Spec.GeneratePassword = &t
	}
	if user.Spec.PasswordLength == nil {
		defaultLen := 32
		user.Spec.PasswordLength = &defaultLen
	}
	return nil
}

// ValidateCreate validates a new WarpgateUser.
func (v *WarpgateUserCustomValidator) ValidateCreate(ctx context.Context, user *WarpgateUser) (admission.Warnings, error) {
	return validateUser(user)
}

// ValidateUpdate validates an updated WarpgateUser.
func (v *WarpgateUserCustomValidator) ValidateUpdate(ctx context.Context, oldUser, user *WarpgateUser) (admission.Warnings, error) {
	return validateUser(user)
}

// ValidateDelete is a no-op for WarpgateUser.
func (v *WarpgateUserCustomValidator) ValidateDelete(ctx context.Context, user *WarpgateUser) (admission.Warnings, error) {
	return nil, nil
}

func validateUser(user *WarpgateUser) (admission.Warnings, error) {
	if user.Spec.ConnectionRef == "" {
		return nil, fmt.Errorf("spec.connectionRef must not be empty")
	}
	if user.Spec.Username == "" {
		return nil, fmt.Errorf("spec.username must not be empty")
	}
	if user.Spec.PasswordLength != nil {
		l := *user.Spec.PasswordLength
		if l < 16 || l > 128 {
			return nil, fmt.Errorf("spec.passwordLength must be between 16 and 128, got %d", l)
		}
	}
	return nil, nil
}
