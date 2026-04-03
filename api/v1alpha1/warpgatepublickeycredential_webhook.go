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
	"strings"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/mutate-warpgate-warpgate-warp-tech-v1alpha1-warpgatepublickeycredential,mutating=true,failurePolicy=fail,sideEffects=None,groups=warpgate.warpgate.warp.tech,resources=warpgatepublickeycredentials,verbs=create;update,versions=v1alpha1,name=mwarpgatepublickeycredential.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-warpgate-warpgate-warp-tech-v1alpha1-warpgatepublickeycredential,mutating=false,failurePolicy=fail,sideEffects=None,groups=warpgate.warpgate.warp.tech,resources=warpgatepublickeycredentials,verbs=create;update;delete,versions=v1alpha1,name=vwarpgatepublickeycredential.kb.io,admissionReviewVersions=v1

// WarpgatePublicKeyCredentialCustomDefaulter handles defaulting for WarpgatePublicKeyCredential.
type WarpgatePublicKeyCredentialCustomDefaulter struct{}

// WarpgatePublicKeyCredentialCustomValidator handles validation for WarpgatePublicKeyCredential.
type WarpgatePublicKeyCredentialCustomValidator struct{}

// SetupWebhookWithManager registers the webhooks for WarpgatePublicKeyCredential.
func (r *WarpgatePublicKeyCredential) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, r).
		WithDefaulter(&WarpgatePublicKeyCredentialCustomDefaulter{}).
		WithValidator(&WarpgatePublicKeyCredentialCustomValidator{}).
		Complete()
}

// Default is a no-op for WarpgatePublicKeyCredential (no defaults needed).
func (d *WarpgatePublicKeyCredentialCustomDefaulter) Default(_ context.Context, _ *WarpgatePublicKeyCredential) error {
	return nil
}

// ValidateCreate validates a new WarpgatePublicKeyCredential.
func (v *WarpgatePublicKeyCredentialCustomValidator) ValidateCreate(_ context.Context, pkc *WarpgatePublicKeyCredential) (admission.Warnings, error) {
	return nil, validateWarpgatePublicKeyCredentialSpec(&pkc.Spec)
}

// ValidateUpdate validates an updated WarpgatePublicKeyCredential.
func (v *WarpgatePublicKeyCredentialCustomValidator) ValidateUpdate(_ context.Context, _, pkc *WarpgatePublicKeyCredential) (admission.Warnings, error) {
	return nil, validateWarpgatePublicKeyCredentialSpec(&pkc.Spec)
}

// ValidateDelete is a no-op for WarpgatePublicKeyCredential.
func (v *WarpgatePublicKeyCredentialCustomValidator) ValidateDelete(_ context.Context, _ *WarpgatePublicKeyCredential) (admission.Warnings, error) {
	return nil, nil
}

func validateWarpgatePublicKeyCredentialSpec(spec *WarpgatePublicKeyCredentialSpec) error {
	if spec.ConnectionRef == "" {
		return fmt.Errorf("spec.connectionRef must not be empty")
	}
	if spec.Username == "" {
		return fmt.Errorf("spec.username must not be empty")
	}
	if spec.Label == "" {
		return fmt.Errorf("spec.label must not be empty")
	}
	if spec.OpenSSHPublicKey == "" {
		return fmt.Errorf("spec.opensshPublicKey must not be empty")
	}
	if !strings.HasPrefix(spec.OpenSSHPublicKey, "ssh-") {
		return fmt.Errorf("spec.opensshPublicKey must start with \"ssh-\" (e.g. ssh-rsa, ssh-ed25519)")
	}
	return nil
}
