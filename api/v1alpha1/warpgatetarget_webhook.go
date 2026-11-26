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
	"strings"
	"unicode"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	defaultSSHAuthKind = "PublicKey"
	defaultTLSMode     = "Preferred"
)

// nolint:unused
// log is for logging in this package.
var warpgatetargetlog = logf.Log.WithName("warpgatetarget-resource")

// SetupWebhookWithManager registers the webhooks for WarpgateTarget.
func (r *WarpgateTarget) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, r).
		WithDefaulter(&WarpgateTargetCustomDefaulter{}).
		WithValidator(&WarpgateTargetCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-warpgate-warpgate-warp-tech-v1alpha1-warpgatetarget,mutating=true,failurePolicy=fail,sideEffects=None,groups=warpgate.warpgate.warp.tech,resources=warpgatetargets,verbs=create;update,versions=v1alpha1,name=mwarpgatetarget.kb.io,admissionReviewVersions=v1

// WarpgateTargetCustomDefaulter handles defaulting for WarpgateTarget.
type WarpgateTargetCustomDefaulter struct{}

// WarpgateTargetCustomValidator handles validation for WarpgateTarget.
type WarpgateTargetCustomValidator struct{}

var _ admission.Defaulter[*WarpgateTarget] = &WarpgateTargetCustomDefaulter{}
var _ admission.Validator[*WarpgateTarget] = &WarpgateTargetCustomValidator{}

// Default implements admission.Defaulter.
func (d *WarpgateTargetCustomDefaulter) Default(ctx context.Context, target *WarpgateTarget) error {
	warpgatetargetlog.Info("Defaulting", "name", target.GetName())

	if target.Spec.SSH != nil {
		if target.Spec.SSH.Port == 0 {
			target.Spec.SSH.Port = 22
		}
		if target.Spec.SSH.AuthKind == "" {
			target.Spec.SSH.AuthKind = defaultSSHAuthKind
		}
	}

	if target.Spec.MySQL != nil {
		if target.Spec.MySQL.Port == 0 {
			target.Spec.MySQL.Port = 3306
		}
	}

	if target.Spec.PostgreSQL != nil {
		if target.Spec.PostgreSQL.Port == 0 {
			target.Spec.PostgreSQL.Port = 5432
		}
	}

	if target.Spec.RDP != nil && target.Spec.RDP.Port == 0 {
		target.Spec.RDP.Port = 3389
	}

	if target.Spec.VNC != nil && target.Spec.VNC.Port == 0 {
		target.Spec.VNC.Port = 5900
	}

	// Default TLS mode and verification for HTTP, MySQL, PostgreSQL, and Kubernetes targets.
	if target.Spec.HTTP != nil {
		defaultTLS(target.Spec.HTTP.TLS)
	}
	if target.Spec.MySQL != nil {
		defaultTLS(target.Spec.MySQL.TLS)
	}
	if target.Spec.PostgreSQL != nil {
		defaultTLS(target.Spec.PostgreSQL.TLS)
	}
	if target.Spec.Kubernetes != nil {
		defaultTLS(target.Spec.Kubernetes.TLS)
		if sat := target.Spec.Kubernetes.ServiceAccountToken; sat != nil && sat.ExpirationSeconds == nil {
			v := ServiceAccountTokenDefaultExpirationSeconds
			sat.ExpirationSeconds = &v
		}
	}

	return nil
}

// defaultTLS fills in an explicit tls block: mode defaults to Preferred and
// certificate verification defaults to on.
func defaultTLS(tls *TLSConfigSpec) {
	if tls == nil {
		return
	}
	if tls.Mode == "" {
		tls.Mode = defaultTLSMode
	}
	if tls.Verify == nil {
		v := true
		tls.Verify = &v
	}
}

// +kubebuilder:webhook:path=/validate-warpgate-warpgate-warp-tech-v1alpha1-warpgatetarget,mutating=false,failurePolicy=fail,sideEffects=None,groups=warpgate.warpgate.warp.tech,resources=warpgatetargets,verbs=create;update;delete,versions=v1alpha1,name=vwarpgatetarget.kb.io,admissionReviewVersions=v1

// ValidateCreate implements admission.Validator.
func (v *WarpgateTargetCustomValidator) ValidateCreate(ctx context.Context, target *WarpgateTarget) (admission.Warnings, error) {
	warpgatetargetlog.Info("Validating create", "name", target.GetName())
	return nil, validateWarpgateTarget(target)
}

// ValidateUpdate implements webhook.CustomValidator.
func (v *WarpgateTargetCustomValidator) ValidateUpdate(ctx context.Context, oldTarget, target *WarpgateTarget) (admission.Warnings, error) {
	warpgatetargetlog.Info("Validating update", "name", target.GetName())
	// Ratchet: an update that leaves the spec alone (finalizers, labels, annotations) or
	// lands on an object being deleted is always allowed, so objects admitted under older,
	// looser rules can still have their finalizer removed and finish deleting.
	if !target.DeletionTimestamp.IsZero() || (oldTarget != nil && equality.Semantic.DeepEqual(oldTarget.Spec, target.Spec)) {
		return nil, nil
	}
	return nil, validateWarpgateTarget(target)
}

// ValidateDelete implements webhook.CustomValidator.
func (v *WarpgateTargetCustomValidator) ValidateDelete(ctx context.Context, target *WarpgateTarget) (admission.Warnings, error) {
	return nil, nil
}

// validateWarpgateTarget runs all field-level validation checks.
func validateWarpgateTarget(target *WarpgateTarget) error {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	if target.Spec.ConnectionRef == "" {
		allErrs = append(allErrs, field.Required(specPath.Child("connectionRef"), "connectionRef must not be empty"))
	}

	if target.Spec.Name == "" {
		allErrs = append(allErrs, field.Required(specPath.Child("name"), "name must not be empty"))
	}

	// Exactly one target type must be set.
	count := 0
	if target.Spec.SSH != nil {
		count++
	}
	if target.Spec.HTTP != nil {
		count++
	}
	if target.Spec.MySQL != nil {
		count++
	}
	if target.Spec.PostgreSQL != nil {
		count++
	}
	if target.Spec.Kubernetes != nil {
		count++
	}
	if target.Spec.RDP != nil {
		count++
	}
	if target.Spec.VNC != nil {
		count++
	}
	if count == 0 {
		allErrs = append(allErrs, field.Required(specPath, "exactly one of ssh, http, mysql, postgresql, kubernetes, rdp, or vnc must be set"))
	} else if count > 1 {
		allErrs = append(allErrs, field.Forbidden(specPath, "only one of ssh, http, mysql, postgresql, kubernetes, rdp, or vnc may be set"))
	}

	// Type-specific validation.
	if target.Spec.SSH != nil {
		allErrs = append(allErrs, validateSSHTarget(target.Spec.SSH, specPath.Child("ssh"))...)
	}
	if target.Spec.HTTP != nil {
		allErrs = append(allErrs, validateHTTPTarget(target.Spec.HTTP, specPath.Child("http"))...)
	}
	if target.Spec.MySQL != nil {
		allErrs = append(allErrs, validateMySQLTarget(target.Spec.MySQL, specPath.Child("mysql"))...)
	}
	if target.Spec.PostgreSQL != nil {
		allErrs = append(allErrs, validatePostgreSQLTarget(target.Spec.PostgreSQL, specPath.Child("postgresql"))...)
	}
	if target.Spec.Kubernetes != nil {
		allErrs = append(allErrs, validateKubernetesTarget(target.Spec.Kubernetes, specPath.Child("kubernetes"))...)
	}
	if target.Spec.RDP != nil {
		allErrs = append(allErrs, validateHostPortUsername(target.Spec.RDP.Host, target.Spec.RDP.Port, target.Spec.RDP.Username, specPath.Child("rdp"))...)
	}
	if target.Spec.VNC != nil {
		allErrs = append(allErrs, validateTargetPort(target.Spec.VNC.Port, specPath.Child("vnc"))...)
		if target.Spec.VNC.Host == "" {
			allErrs = append(allErrs, field.Required(specPath.Child("vnc", "host"), "host must not be empty"))
		}
	}

	if len(allErrs) == 0 {
		return nil
	}
	return allErrs.ToAggregate()
}

func validateSSHTarget(ssh *SSHTargetSpec, fldPath *field.Path) field.ErrorList {
	var errs field.ErrorList
	if ssh.Host == "" {
		errs = append(errs, field.Required(fldPath.Child("host"), "host must not be empty"))
	}
	if ssh.Port < 1 || ssh.Port > 65535 {
		errs = append(errs, field.Invalid(fldPath.Child("port"), ssh.Port, "port must be between 1 and 65535"))
	}
	if ssh.Username == "" {
		errs = append(errs, field.Required(fldPath.Child("username"), "username must not be empty"))
	}
	if ssh.AuthKind != "Password" && ssh.AuthKind != defaultSSHAuthKind && ssh.AuthKind != "IamRole" {
		errs = append(errs, field.NotSupported(fldPath.Child("authKind"), ssh.AuthKind, []string{"Password", defaultSSHAuthKind, "IamRole"}))
	}
	return errs
}

func validateKubernetesTarget(k8s *KubernetesTargetSpec, fldPath *field.Path) field.ErrorList {
	var errs field.ErrorList
	if k8s.ClusterURL == "" {
		errs = append(errs, field.Required(fldPath.Child("clusterURL"), "clusterURL must not be empty"))
	}
	if k8s.AuthKind != "Token" && k8s.AuthKind != "Certificate" && k8s.AuthKind != "IamRole" {
		errs = append(errs, field.NotSupported(fldPath.Child("authKind"), k8s.AuthKind, []string{"Token", "Certificate", "IamRole"}))
	}
	if k8s.TLS != nil {
		errs = append(errs, validateTLS(k8s.TLS, fldPath.Child("tls"))...)
	}
	switch k8s.AuthKind {
	case "Token":
		hasSecret, hasSA := k8s.TokenSecretRef != nil, k8s.ServiceAccountToken != nil
		switch {
		case hasSecret && hasSA:
			errs = append(errs, field.Forbidden(fldPath.Child("serviceAccountToken"), "tokenSecretRef and serviceAccountToken are mutually exclusive"))
		case !hasSecret && !hasSA:
			errs = append(errs, field.Required(fldPath, "authKind Token needs tokenSecretRef or serviceAccountToken"))
		}
		if hasSecret {
			errs = append(errs, validateSecretKeyRef(k8s.TokenSecretRef, fldPath.Child("tokenSecretRef"))...)
		}
		if hasSA {
			errs = append(errs, validateServiceAccountToken(k8s.ServiceAccountToken, fldPath.Child("serviceAccountToken"))...)
		}
	case "Certificate":
		if k8s.CertificateSecretRef == nil {
			errs = append(errs, field.Required(fldPath.Child("certificateSecretRef"), "authKind Certificate needs certificateSecretRef"))
		} else {
			errs = append(errs, validateSecretKeyRef(k8s.CertificateSecretRef, fldPath.Child("certificateSecretRef"))...)
		}
		if k8s.PrivateKeySecretRef == nil {
			errs = append(errs, field.Required(fldPath.Child("privateKeySecretRef"), "authKind Certificate needs privateKeySecretRef"))
		} else {
			errs = append(errs, validateSecretKeyRef(k8s.PrivateKeySecretRef, fldPath.Child("privateKeySecretRef"))...)
		}
	}
	if k8s.AuthKind != "Token" && k8s.ServiceAccountToken != nil {
		errs = append(errs, field.Forbidden(fldPath.Child("serviceAccountToken"), "serviceAccountToken is only valid with authKind Token"))
	}
	return errs
}

func validateSecretKeyRef(ref *SecretKeyRef, fldPath *field.Path) field.ErrorList {
	if ref.Name == "" {
		return field.ErrorList{field.Required(fldPath.Child("name"), "name must not be empty")}
	}
	return nil
}

// validateServiceAccountToken checks serviceAccountToken. Audiences may not contain
// commas or whitespace, so they can be matched exactly against the comma-separated
// warpgate.warp.tech/token-audiences annotation.
func validateServiceAccountToken(sat *ServiceAccountTokenSpec, fldPath *field.Path) field.ErrorList {
	var errs field.ErrorList
	if sat.Name == "" {
		errs = append(errs, field.Required(fldPath.Child("name"), "name must not be empty"))
	} else {
		for _, msg := range validation.IsDNS1123Subdomain(sat.Name) {
			errs = append(errs, field.Invalid(fldPath.Child("name"), sat.Name, msg))
		}
	}
	audPath := fldPath.Child("audiences")
	switch {
	case len(sat.Audiences) == 0:
		errs = append(errs, field.Required(audPath, "at least one audience is required"))
	case len(sat.Audiences) > 8:
		errs = append(errs, field.TooMany(audPath, len(sat.Audiences), 8))
	}
	seen := sets.New[string]()
	for i, aud := range sat.Audiences {
		p := audPath.Index(i)
		switch {
		case aud == "":
			errs = append(errs, field.Required(p, "audience must not be empty"))
		case len(aud) > 256:
			errs = append(errs, field.TooLong(p, aud, 256))
		case strings.ContainsFunc(aud, func(r rune) bool { return r == ',' || unicode.IsSpace(r) || unicode.IsControl(r) }):
			errs = append(errs, field.Invalid(p, aud, "audience must not contain commas, whitespace or control characters"))
		case seen.Has(aud):
			errs = append(errs, field.Duplicate(p, aud))
		}
		seen.Insert(aud)
	}
	if exp := sat.ExpirationSeconds; exp != nil &&
		(*exp < ServiceAccountTokenMinExpirationSeconds || *exp > ServiceAccountTokenMaxExpirationSeconds) {
		errs = append(errs, field.Invalid(fldPath.Child("expirationSeconds"), *exp,
			"expirationSeconds must be between 7200 (2h) and 172800 (48h)"))
	}
	return errs
}

func validateTargetPort(port int, fldPath *field.Path) field.ErrorList {
	if port < 1 || port > 65535 {
		return field.ErrorList{field.Invalid(fldPath.Child("port"), port, "port must be between 1 and 65535")}
	}
	return nil
}

func validateHostPortUsername(host string, port int, username string, fldPath *field.Path) field.ErrorList {
	var errs field.ErrorList
	if host == "" {
		errs = append(errs, field.Required(fldPath.Child("host"), "host must not be empty"))
	}
	errs = append(errs, validateTargetPort(port, fldPath)...)
	if username == "" {
		errs = append(errs, field.Required(fldPath.Child("username"), "username must not be empty"))
	}
	return errs
}

func validateHTTPTarget(http *HTTPTargetSpec, fldPath *field.Path) field.ErrorList {
	var errs field.ErrorList
	if http.URL == "" {
		errs = append(errs, field.Required(fldPath.Child("url"), "url must not be empty"))
	}
	if http.TLS != nil {
		errs = append(errs, validateTLS(http.TLS, fldPath.Child("tls"))...)
	}
	return errs
}

func validateMySQLTarget(mysql *MySQLTargetSpec, fldPath *field.Path) field.ErrorList {
	var errs field.ErrorList
	if mysql.Host == "" {
		errs = append(errs, field.Required(fldPath.Child("host"), "host must not be empty"))
	}
	if mysql.Port < 1 || mysql.Port > 65535 {
		errs = append(errs, field.Invalid(fldPath.Child("port"), mysql.Port, "port must be between 1 and 65535"))
	}
	if mysql.Username == "" {
		errs = append(errs, field.Required(fldPath.Child("username"), "username must not be empty"))
	}
	if mysql.TLS != nil {
		errs = append(errs, validateTLS(mysql.TLS, fldPath.Child("tls"))...)
	}
	return errs
}

func validatePostgreSQLTarget(pg *PostgreSQLTargetSpec, fldPath *field.Path) field.ErrorList {
	var errs field.ErrorList
	if pg.Host == "" {
		errs = append(errs, field.Required(fldPath.Child("host"), "host must not be empty"))
	}
	if pg.Port < 1 || pg.Port > 65535 {
		errs = append(errs, field.Invalid(fldPath.Child("port"), pg.Port, "port must be between 1 and 65535"))
	}
	if pg.Username == "" {
		errs = append(errs, field.Required(fldPath.Child("username"), "username must not be empty"))
	}
	if pg.ProtocolVersion != "" && pg.ProtocolVersion != "3.0" && pg.ProtocolVersion != "3.2" {
		errs = append(errs, field.NotSupported(fldPath.Child("protocolVersion"), pg.ProtocolVersion, []string{"3.0", "3.2"}))
	}
	if pg.TLS != nil {
		errs = append(errs, validateTLS(pg.TLS, fldPath.Child("tls"))...)
	}
	return errs
}

func validateTLS(tls *TLSConfigSpec, fldPath *field.Path) field.ErrorList {
	var errs field.ErrorList
	if tls.Mode != "" && tls.Mode != "Disabled" && tls.Mode != defaultTLSMode && tls.Mode != "Required" {
		errs = append(errs, field.NotSupported(fldPath.Child("mode"), tls.Mode, []string{"Disabled", defaultTLSMode, "Required"}))
	}
	return errs
}
