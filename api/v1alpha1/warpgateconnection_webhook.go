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

// +kubebuilder:webhook:path=/mutate-warpgate-warpgate-warp-tech-v1alpha1-warpgateconnection,mutating=true,failurePolicy=fail,sideEffects=None,groups=warpgate.warpgate.warp.tech,resources=warpgateconnections,verbs=create;update,versions=v1alpha1,name=mwarpgateconnection.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-warpgate-warpgate-warp-tech-v1alpha1-warpgateconnection,mutating=false,failurePolicy=fail,sideEffects=None,groups=warpgate.warpgate.warp.tech,resources=warpgateconnections,verbs=create;update;delete,versions=v1alpha1,name=vwarpgateconnection.kb.io,admissionReviewVersions=v1

// WarpgateConnectionCustomDefaulter handles defaulting for WarpgateConnection.
type WarpgateConnectionCustomDefaulter struct{}

// WarpgateConnectionCustomValidator handles validation for WarpgateConnection.
type WarpgateConnectionCustomValidator struct{}

// SetupWebhookWithManager registers the webhooks for WarpgateConnection.
func (r *WarpgateConnection) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, r).
		WithDefaulter(&WarpgateConnectionCustomDefaulter{}).
		WithValidator(&WarpgateConnectionCustomValidator{}).
		Complete()
}

const (
	defaultTokenKey    = "token"
	defaultUsernameKey = "username"
	defaultPasswordKey = "password"
)

// Default sets sensible defaults for WarpgateConnection fields.
func (d *WarpgateConnectionCustomDefaulter) Default(ctx context.Context, conn *WarpgateConnection) error {
	if conn.Spec.AuthSecretRef.TokenKey == "" {
		conn.Spec.AuthSecretRef.TokenKey = defaultTokenKey
	}
	if conn.Spec.AuthSecretRef.UsernameKey == "" {
		conn.Spec.AuthSecretRef.UsernameKey = defaultUsernameKey
	}
	if conn.Spec.AuthSecretRef.PasswordKey == "" {
		conn.Spec.AuthSecretRef.PasswordKey = defaultPasswordKey
	}
	return nil
}

// ValidateCreate validates a new WarpgateConnection.
func (v *WarpgateConnectionCustomValidator) ValidateCreate(ctx context.Context, conn *WarpgateConnection) (admission.Warnings, error) {
	return validateConnection(conn)
}

// ValidateUpdate validates an updated WarpgateConnection.
func (v *WarpgateConnectionCustomValidator) ValidateUpdate(ctx context.Context, oldConn, conn *WarpgateConnection) (admission.Warnings, error) {
	return validateConnection(conn)
}

// ValidateDelete is a no-op for WarpgateConnection.
func (v *WarpgateConnectionCustomValidator) ValidateDelete(ctx context.Context, conn *WarpgateConnection) (admission.Warnings, error) {
	return nil, nil
}

// validateConnection checks that the connection spec has the required fields.
// The referenced Secret must contain "username" and "password" keys for
// session-based authentication against Warpgate.
func validateConnection(conn *WarpgateConnection) (admission.Warnings, error) {
	if conn.Spec.Host == "" {
		return nil, fmt.Errorf("spec.host must not be empty")
	}
	if !strings.HasPrefix(conn.Spec.Host, "http://") && !strings.HasPrefix(conn.Spec.Host, "https://") {
		return nil, fmt.Errorf("spec.host must start with http:// or https://")
	}
	if conn.Spec.AuthSecretRef.Name == "" {
		return nil, fmt.Errorf("spec.authSecretRef.name must not be empty")
	}
	return nil, nil
}
