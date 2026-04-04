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

	// Strategy defaults to Recreate.
	if inst.Spec.Strategy == "" {
		inst.Spec.Strategy = "Recreate"
	}

	// Kubernetes port defaults to 0 (disabled).
	if inst.Spec.Kubernetes != nil && inst.Spec.Kubernetes.Port == nil {
		p := int32(0)
		inst.Spec.Kubernetes.Port = &p
	}

	// Storage enabled defaults to true.
	if inst.Spec.Storage != nil && inst.Spec.Storage.Enabled == nil {
		t := true
		inst.Spec.Storage.Enabled = &t
	}

	// RecordSessions defaults to false.
	if inst.Spec.RecordSessions == nil {
		f := false
		inst.Spec.RecordSessions = &f
	}

	return nil
}

// +kubebuilder:webhook:path=/validate-warpgate-warpgate-warp-tech-v1alpha1-warpgateinstance,mutating=false,failurePolicy=fail,sideEffects=None,groups=warpgate.warpgate.warp.tech,resources=warpgateinstances,verbs=create;update;delete,versions=v1alpha1,name=vwarpgateinstance.kb.io,admissionReviewVersions=v1

// ValidateCreate implements admission.Validator.
func (v *WarpgateInstanceCustomValidator) ValidateCreate(ctx context.Context, inst *WarpgateInstance) (admission.Warnings, error) {
	warpgateinstancelog.Info("Validating create", "name", inst.GetName())
	warnings, err := validateWarpgateInstance(inst)
	return warnings, err
}

// ValidateUpdate implements admission.Validator.
func (v *WarpgateInstanceCustomValidator) ValidateUpdate(ctx context.Context, oldInst, inst *WarpgateInstance) (admission.Warnings, error) {
	warpgateinstancelog.Info("Validating update", "name", inst.GetName())
	warnings, err := validateWarpgateInstance(inst)
	return warnings, err
}

// ValidateDelete implements admission.Validator.
func (v *WarpgateInstanceCustomValidator) ValidateDelete(ctx context.Context, inst *WarpgateInstance) (admission.Warnings, error) {
	return nil, nil
}

// validatePort checks that a port pointer is within 1-65535 (or 0-65535 if allowZero).
func validatePort(port *int32, fldPath *field.Path, allowZero bool) *field.Error {
	if port == nil {
		return nil
	}
	min := int32(1)
	if allowZero {
		min = 0
	}
	if *port < min || *port > 65535 {
		msg := "port must be between 1 and 65535"
		if allowZero {
			msg = "port must be between 0 and 65535"
		}
		return field.Invalid(fldPath, *port, msg)
	}
	return nil
}

// validateWarpgateInstance runs all field-level validation checks.
func validateWarpgateInstance(inst *WarpgateInstance) (admission.Warnings, error) {
	var allErrs field.ErrorList
	var warnings admission.Warnings
	specPath := field.NewPath("spec")

	if inst.Spec.Version == "" {
		allErrs = append(allErrs, field.Required(specPath.Child("version"), "version must not be empty"))
	}
	if inst.Spec.AdminPasswordSecretRef.Name == "" {
		allErrs = append(allErrs, field.Required(specPath.Child("adminPasswordSecretRef", "name"), "adminPasswordSecretRef.name must not be empty"))
	}

	httpOn := inst.Spec.HTTP != nil && inst.Spec.HTTP.Enabled != nil && *inst.Spec.HTTP.Enabled
	sshOn := inst.Spec.SSH != nil && inst.Spec.SSH.Enabled != nil && *inst.Spec.SSH.Enabled
	if !httpOn && !sshOn {
		allErrs = append(allErrs, field.Required(specPath, "at least one of http or ssh must be enabled"))
	}

	// Validate all protocol ports.
	for _, p := range []struct {
		spec *ProtocolListenerSpec
		http *HTTPListenerSpec
		ssh  *SSHListenerSpec
		name string
		zero bool
	}{
		{http: inst.Spec.HTTP, name: "http"},
		{ssh: inst.Spec.SSH, name: "ssh"},
		{spec: inst.Spec.MySQL, name: "mysql"},
		{spec: inst.Spec.PostgreSQL, name: "postgresql"},
		{spec: inst.Spec.Kubernetes, name: "kubernetes", zero: true},
	} {
		var port *int32
		switch {
		case p.http != nil:
			port = p.http.Port
		case p.ssh != nil:
			port = p.ssh.Port
		case p.spec != nil:
			port = p.spec.Port
		}
		if e := validatePort(port, specPath.Child(p.name, "port"), p.zero); e != nil {
			allErrs = append(allErrs, e)
		}
	}

	if s := inst.Spec.Strategy; s != "" && s != "Recreate" && s != "RollingUpdate" {
		allErrs = append(allErrs, field.Invalid(specPath.Child("strategy"), s, "must be Recreate or RollingUpdate"))
	}
	if inst.Spec.Storage != nil && inst.Spec.Storage.Size != "" {
		if _, err := resource.ParseQuantity(inst.Spec.Storage.Size); err != nil {
			allErrs = append(allErrs, field.Invalid(specPath.Child("storage", "size"), inst.Spec.Storage.Size, "must be a valid resource quantity"))
		}
	}

	if inst.Spec.DatabaseURL != "" {
		warnings = append(warnings, "databaseURL is set — SQLite persistence via PVC is not needed")
	}
	noStorage := inst.Spec.Storage != nil && inst.Spec.Storage.Enabled != nil && !*inst.Spec.Storage.Enabled
	if noStorage && inst.Spec.DatabaseURL == "" {
		warnings = append(warnings, "storage is disabled and no databaseURL is set — data will be lost on pod restart")
	}

	if len(allErrs) == 0 {
		return warnings, nil
	}
	return warnings, allErrs.ToAggregate()
}
