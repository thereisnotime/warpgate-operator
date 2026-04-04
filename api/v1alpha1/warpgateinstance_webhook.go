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

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// nolint:unused
// log is for logging in this package.
var warpgateinstancelog = logf.Log.WithName("warpgateinstance-resource")

// SetupWebhookWithManager registers the webhooks for WarpgateInstance.
func (r *WarpgateInstance) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, r).
		WithDefaulter(&WarpgateInstanceCustomDefaulter{}).
		WithValidator(&WarpgateInstanceCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-warpgate-warpgate-warp-tech-v1alpha1-warpgateinstance,mutating=true,failurePolicy=fail,sideEffects=None,groups=warpgate.warpgate.warp.tech,resources=warpgateinstances,verbs=create;update,versions=v1alpha1,name=mwarpgateinstance.kb.io,admissionReviewVersions=v1

// WarpgateInstanceCustomDefaulter handles defaulting for WarpgateInstance.
type WarpgateInstanceCustomDefaulter struct{}

// WarpgateInstanceCustomValidator handles validation for WarpgateInstance.
type WarpgateInstanceCustomValidator struct{}

var _ admission.Defaulter[*WarpgateInstance] = &WarpgateInstanceCustomDefaulter{}
var _ admission.Validator[*WarpgateInstance] = &WarpgateInstanceCustomValidator{}

// Default implements admission.Defaulter.
func (d *WarpgateInstanceCustomDefaulter) Default(ctx context.Context, inst *WarpgateInstance) error {
	warpgateinstancelog.Info("Defaulting", "name", inst.GetName())

	// Replicas defaults to 1.
	if inst.Spec.Replicas == nil {
		one := int32(1)
		inst.Spec.Replicas = &one
	}

	// Image defaults to ghcr.io/warp-tech/warpgate:<version>.
	if inst.Spec.Image == "" && inst.Spec.Version != "" {
		inst.Spec.Image = fmt.Sprintf("ghcr.io/warp-tech/warpgate:%s", inst.Spec.Version)
	}

	// HTTP defaults: enabled=true, port=8888, serviceType=ClusterIP.
	if inst.Spec.HTTP == nil {
		inst.Spec.HTTP = &HTTPListenerSpec{}
	}
	if inst.Spec.HTTP.Enabled == nil {
		t := true
		inst.Spec.HTTP.Enabled = &t
	}
	if inst.Spec.HTTP.Port == nil {
		p := int32(8888)
		inst.Spec.HTTP.Port = &p
	}
	if inst.Spec.HTTP.ServiceType == "" {
		inst.Spec.HTTP.ServiceType = "ClusterIP"
	}

	// SSH defaults (only if SSH struct is present): port=2222, serviceType=ClusterIP.
	if inst.Spec.SSH != nil {
		if inst.Spec.SSH.Port == nil {
			p := int32(2222)
			inst.Spec.SSH.Port = &p
		}
		if inst.Spec.SSH.ServiceType == "" {
			inst.Spec.SSH.ServiceType = "ClusterIP"
		}
	}

	// MySQL defaults (only if struct is present): port=33306.
	if inst.Spec.MySQL != nil {
		if inst.Spec.MySQL.Port == nil {
			p := int32(33306)
			inst.Spec.MySQL.Port = &p
		}
	}

	// PostgreSQL defaults (only if struct is present): port=55432.
	if inst.Spec.PostgreSQL != nil {
		if inst.Spec.PostgreSQL.Port == nil {
			p := int32(55432)
			inst.Spec.PostgreSQL.Port = &p
		}
	}

	// Storage defaults: size=1Gi.
	if inst.Spec.Storage == nil {
		inst.Spec.Storage = &StorageSpec{}
	}
	if inst.Spec.Storage.Size == "" {
		inst.Spec.Storage.Size = "1Gi"
	}

	// TLS defaults: certManager=true.
	if inst.Spec.TLS == nil {
		inst.Spec.TLS = &InstanceTLSSpec{}
	}
	if inst.Spec.TLS.CertManager == nil {
		t := true
		inst.Spec.TLS.CertManager = &t
	}

	// CreateConnection defaults to true.
	if inst.Spec.CreateConnection == nil {
		t := true
		inst.Spec.CreateConnection = &t
	}

	return nil
}

// +kubebuilder:webhook:path=/validate-warpgate-warpgate-warp-tech-v1alpha1-warpgateinstance,mutating=false,failurePolicy=fail,sideEffects=None,groups=warpgate.warpgate.warp.tech,resources=warpgateinstances,verbs=create;update;delete,versions=v1alpha1,name=vwarpgateinstance.kb.io,admissionReviewVersions=v1

// ValidateCreate implements admission.Validator.
func (v *WarpgateInstanceCustomValidator) ValidateCreate(ctx context.Context, inst *WarpgateInstance) (admission.Warnings, error) {
	warpgateinstancelog.Info("Validating create", "name", inst.GetName())
	return nil, validateWarpgateInstance(inst)
}

// ValidateUpdate implements admission.Validator.
func (v *WarpgateInstanceCustomValidator) ValidateUpdate(ctx context.Context, oldInst, inst *WarpgateInstance) (admission.Warnings, error) {
	warpgateinstancelog.Info("Validating update", "name", inst.GetName())
	return nil, validateWarpgateInstance(inst)
}

// ValidateDelete implements admission.Validator.
func (v *WarpgateInstanceCustomValidator) ValidateDelete(ctx context.Context, inst *WarpgateInstance) (admission.Warnings, error) {
	return nil, nil
}

// validateWarpgateInstance runs all field-level validation checks.
func validateWarpgateInstance(inst *WarpgateInstance) error {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	// version must not be empty.
	if inst.Spec.Version == "" {
		allErrs = append(allErrs, field.Required(specPath.Child("version"), "version must not be empty"))
	}

	// adminPasswordSecretRef.name must not be empty.
	if inst.Spec.AdminPasswordSecretRef.Name == "" {
		allErrs = append(allErrs, field.Required(specPath.Child("adminPasswordSecretRef", "name"), "adminPasswordSecretRef.name must not be empty"))
	}

	// At least one of HTTP or SSH must be enabled.
	httpEnabled := inst.Spec.HTTP != nil && inst.Spec.HTTP.Enabled != nil && *inst.Spec.HTTP.Enabled
	sshEnabled := inst.Spec.SSH != nil && inst.Spec.SSH.Enabled != nil && *inst.Spec.SSH.Enabled
	if !httpEnabled && !sshEnabled {
		allErrs = append(allErrs, field.Required(specPath, "at least one of http or ssh must be enabled"))
	}

	// Port validation for HTTP.
	if inst.Spec.HTTP != nil && inst.Spec.HTTP.Port != nil {
		if *inst.Spec.HTTP.Port < 1 || *inst.Spec.HTTP.Port > 65535 {
			allErrs = append(allErrs, field.Invalid(specPath.Child("http", "port"), *inst.Spec.HTTP.Port, "port must be between 1 and 65535"))
		}
	}

	// Port validation for SSH.
	if inst.Spec.SSH != nil && inst.Spec.SSH.Port != nil {
		if *inst.Spec.SSH.Port < 1 || *inst.Spec.SSH.Port > 65535 {
			allErrs = append(allErrs, field.Invalid(specPath.Child("ssh", "port"), *inst.Spec.SSH.Port, "port must be between 1 and 65535"))
		}
	}

	// Port validation for MySQL.
	if inst.Spec.MySQL != nil && inst.Spec.MySQL.Port != nil {
		if *inst.Spec.MySQL.Port < 1 || *inst.Spec.MySQL.Port > 65535 {
			allErrs = append(allErrs, field.Invalid(specPath.Child("mysql", "port"), *inst.Spec.MySQL.Port, "port must be between 1 and 65535"))
		}
	}

	// Port validation for PostgreSQL.
	if inst.Spec.PostgreSQL != nil && inst.Spec.PostgreSQL.Port != nil {
		if *inst.Spec.PostgreSQL.Port < 1 || *inst.Spec.PostgreSQL.Port > 65535 {
			allErrs = append(allErrs, field.Invalid(specPath.Child("postgresql", "port"), *inst.Spec.PostgreSQL.Port, "port must be between 1 and 65535"))
		}
	}

	// Storage size must be parseable as a resource quantity.
	if inst.Spec.Storage != nil && inst.Spec.Storage.Size != "" {
		if _, err := resource.ParseQuantity(inst.Spec.Storage.Size); err != nil {
			allErrs = append(allErrs, field.Invalid(specPath.Child("storage", "size"), inst.Spec.Storage.Size, "must be a valid resource quantity"))
		}
	}

	if len(allErrs) == 0 {
		return nil
	}
	return allErrs.ToAggregate()
}
