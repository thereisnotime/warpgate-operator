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

// +kubebuilder:webhook:path=/mutate-warpgate-warpgate-warp-tech-v1alpha1-warpgatepasswordcredential,mutating=true,failurePolicy=fail,sideEffects=None,groups=warpgate.warpgate.warp.tech,resources=warpgatepasswordcredentials,verbs=create;update,versions=v1alpha1,name=mwarpgatepasswordcredential.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-warpgate-warpgate-warp-tech-v1alpha1-warpgatepasswordcredential,mutating=false,failurePolicy=fail,sideEffects=None,groups=warpgate.warpgate.warp.tech,resources=warpgatepasswordcredentials,verbs=create;update;delete,versions=v1alpha1,name=vwarpgatepasswordcredential.kb.io,admissionReviewVersions=v1

// WarpgatePasswordCredentialCustomDefaulter handles defaulting for WarpgatePasswordCredential.
type WarpgatePasswordCredentialCustomDefaulter struct{}

// WarpgatePasswordCredentialCustomValidator handles validation for WarpgatePasswordCredential.
type WarpgatePasswordCredentialCustomValidator struct{}

// SetupWebhookWithManager registers the webhooks for WarpgatePasswordCredential.
func (r *WarpgatePasswordCredential) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, r).
		WithDefaulter(&WarpgatePasswordCredentialCustomDefaulter{}).
		WithValidator(&WarpgatePasswordCredentialCustomValidator{}).
		Complete()
}

// Default sets sensible defaults for WarpgatePasswordCredential fields.
func (d *WarpgatePasswordCredentialCustomDefaulter) Default(_ context.Context, pc *WarpgatePasswordCredential) error {
	if pc.Spec.PasswordSecretRef.Key == "" {
		pc.Spec.PasswordSecretRef.Key = "password"
	}
	return nil
}

// ValidateCreate validates a new WarpgatePasswordCredential.
func (v *WarpgatePasswordCredentialCustomValidator) ValidateCreate(_ context.Context, pc *WarpgatePasswordCredential) (admission.Warnings, error) {
	return nil, validateWarpgatePasswordCredentialSpec(&pc.Spec)
}

// ValidateUpdate validates an updated WarpgatePasswordCredential.
func (v *WarpgatePasswordCredentialCustomValidator) ValidateUpdate(_ context.Context, _, pc *WarpgatePasswordCredential) (admission.Warnings, error) {
	return nil, validateWarpgatePasswordCredentialSpec(&pc.Spec)
}

// ValidateDelete is a no-op for WarpgatePasswordCredential.
func (v *WarpgatePasswordCredentialCustomValidator) ValidateDelete(_ context.Context, _ *WarpgatePasswordCredential) (admission.Warnings, error) {
	return nil, nil
}

func validateWarpgatePasswordCredentialSpec(spec *WarpgatePasswordCredentialSpec) error {
	if spec.ConnectionRef == "" {
		return fmt.Errorf("spec.connectionRef must not be empty")
	}
	if spec.Username == "" {
		return fmt.Errorf("spec.username must not be empty")
	}
	if spec.PasswordSecretRef.Name == "" {
		return fmt.Errorf("spec.passwordSecretRef.name must not be empty")
	}
	return nil
}
