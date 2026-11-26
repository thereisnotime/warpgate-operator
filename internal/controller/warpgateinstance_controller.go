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

package controller

import (
	"context"
	"crypto/pbkdf2"
	"crypto/sha256"
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	warpgatev1alpha1 "github.com/thereisnotime/warpgate-operator/api/v1alpha1"
)

const instanceFinalizer = "warpgate.warp.tech/instance-finalizer"

const (
	configHashAnnotation = "warpgate.warp.tech/config-hash"
	// tlsHashAnnotation tracks the served certificate so pods restart (and
	// copy the new pair into /data) when the TLS Secret changes, for example
	// on a cert-manager renewal.
	tlsHashAnnotation = "warpgate.warp.tech/tls-hash"

	// certificatePendingRequeue is a fallback in case the Secret watch misses
	// the cert-manager Secret being created.
	certificatePendingRequeue = 10 * time.Second
)

// WarpgateInstanceReconciler reconciles a WarpgateInstance object.
type WarpgateInstanceReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	ReconcileInterval time.Duration
}

// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgateinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgateinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgateinstances/finalizers,verbs=update
// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgateinstances/scale,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=configmaps;persistentvolumeclaims;services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cert-manager.io,resources=issuers;certificates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=warpgate.warpgate.warp.tech,resources=warpgateconnections,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile handles the reconciliation loop for WarpgateInstance resources.
func (r *WarpgateInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var inst warpgatev1alpha1.WarpgateInstance
	if err := r.Get(ctx, req.NamespacedName, &inst); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !inst.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&inst, instanceFinalizer) {
			// Explicitly clean up the auto-created WarpgateConnection (owned resources
			// are already GC'd via owner references, but belt-and-suspenders).
			if inst.Status.ConnectionRef != "" {
				var conn warpgatev1alpha1.WarpgateConnection
				if err := r.Get(ctx, types.NamespacedName{
					Name: inst.Status.ConnectionRef, Namespace: inst.Namespace,
				}, &conn); err == nil {
					if err := r.Delete(ctx, &conn); err != nil && !apierrors.IsNotFound(err) {
						log.Error(err, "failed to delete auto-created WarpgateConnection")
						return ctrl.Result{}, err
					}
				}
			}

			controllerutil.RemoveFinalizer(&inst, instanceFinalizer)
			if err := r.Update(ctx, &inst); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(&inst, instanceFinalizer) {
		controllerutil.AddFinalizer(&inst, instanceFinalizer)
		if err := r.Update(ctx, &inst); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.ensureConfigMap(ctx, &inst); err != nil {
		log.Error(err, "failed to ensure ConfigMap")
		r.setCondition(&inst, metav1.ConditionFalse, "ConfigMapFailed", err.Error())
		_ = r.Status().Update(ctx, &inst)
		return ctrl.Result{}, err
	}

	if inst.Spec.ConfigOverride != "" {
		if err := r.ensureConfigOverrideConfigMap(ctx, &inst); err != nil {
			log.Error(err, "failed to ensure config override ConfigMap")
			r.setCondition(&inst, metav1.ConditionFalse, "ConfigOverrideFailed", err.Error())
			_ = r.Status().Update(ctx, &inst)
			return ctrl.Result{}, err
		}
	}

	// create only, never update — Kubernetes doesn't allow PVC spec changes
	if storageEnabled(&inst) && !hasExistingClaim(&inst) {
		if err := r.ensurePVC(ctx, &inst); err != nil {
			log.Error(err, "failed to ensure PVC")
			r.setCondition(&inst, metav1.ConditionFalse, "PVCFailed", err.Error())
			_ = r.Status().Update(ctx, &inst)
			return ctrl.Result{}, err
		}
	}

	if certManagerEnabled(&inst) {
		if err := r.ensureCertManagerResources(ctx, &inst); err != nil {
			log.Error(err, "failed to ensure cert-manager resources")
			r.setCondition(&inst, metav1.ConditionFalse, "CertManagerFailed", err.Error())
			_ = r.Status().Update(ctx, &inst)
			return ctrl.Result{}, err
		}
	}

	// The pod mounts the cert-manager Secret, so hold the Deployment back until
	// cert-manager has issued it instead of leaving the pod stuck on a missing
	// volume. The Secret watch requeues as soon as it shows up.
	if usesCertManagerSecret(&inst) {
		issued, err := r.tlsSecretIssued(ctx, &inst)
		if err != nil {
			log.Error(err, "failed to get cert-manager TLS Secret")
			r.setCondition(&inst, metav1.ConditionFalse, "CertManagerFailed", err.Error())
			_ = r.Status().Update(ctx, &inst)
			return ctrl.Result{}, err
		}
		if !issued {
			name := certManagerSecretName(&inst)
			log.Info("Waiting for cert-manager to issue TLS Secret", "secret", name)
			r.setCondition(&inst, metav1.ConditionFalse, "CertificatePending",
				fmt.Sprintf("waiting for cert-manager to issue Secret %q", name))
			if err := r.Status().Update(ctx, &inst); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: certificatePendingRequeue}, nil
		}
	}

	if err := r.ensureDeployment(ctx, &inst); err != nil {
		log.Error(err, "failed to ensure Deployment")
		r.setCondition(&inst, metav1.ConditionFalse, "DeploymentFailed", err.Error())
		_ = r.Status().Update(ctx, &inst)
		return ctrl.Result{}, err
	}

	if err := r.ensureServices(ctx, &inst); err != nil {
		log.Error(err, "failed to ensure Services")
		r.setCondition(&inst, metav1.ConditionFalse, "ServiceFailed", err.Error())
		_ = r.Status().Update(ctx, &inst)
		return ctrl.Result{}, err
	}

	if shouldCreateConnection(&inst) {
		if err := r.ensureWarpgateConnection(ctx, &inst); err != nil {
			log.Error(err, "failed to ensure WarpgateConnection")
			r.setCondition(&inst, metav1.ConditionFalse, secretErrorReason(err, "ConnectionFailed"), err.Error())
			_ = r.Status().Update(ctx, &inst)
			return ctrl.Result{}, err
		}
	}

	r.refreshStatus(ctx, &inst)

	if inst.Status.ReadyReplicas > 0 {
		r.setCondition(&inst, metav1.ConditionTrue, "Available", "Warpgate instance is running")
	} else {
		r.setCondition(&inst, metav1.ConditionFalse, "Unavailable", "Deployment has no ready replicas")
	}
	if err := r.Status().Update(ctx, &inst); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// ---------------------------------------------------------------------------
// Helper accessors
// ---------------------------------------------------------------------------

func resolveImage(inst *warpgatev1alpha1.WarpgateInstance) string {
	if inst.Spec.Image != "" {
		return inst.Spec.Image
	}
	// Upstream ghcr tags carry no leading "v" (ghcr.io/warp-tech/warpgate:0.26.0
	// exists, :v0.26.0 does not), so normalise a user-supplied "v" away.
	tag := strings.TrimPrefix(inst.Spec.Version, "v")
	if tag == "" {
		tag = warpgatev1alpha1.DefaultWarpgateVersion
	}
	return fmt.Sprintf("ghcr.io/warp-tech/warpgate:%s", tag)
}

// seedImage is the utility image for the config-seed init container on shell-less
// Warpgate deployments. An explicit spec.seedImage wins; otherwise a pinned
// busybox is used.
func seedImage(inst *warpgatev1alpha1.WarpgateInstance) string {
	if inst.Spec.SeedImage != "" {
		return inst.Spec.SeedImage
	}
	return warpgatev1alpha1.DefaultSeedImage
}

// warpgateTag returns the Warpgate version tag the instance resolves to, with any
// leading "v" stripped. It prefers spec.version, else parses the tag from
// spec.image, else falls back to the operator default.
func warpgateTag(inst *warpgatev1alpha1.WarpgateInstance) string {
	if inst.Spec.Version != "" {
		return strings.TrimPrefix(inst.Spec.Version, "v")
	}
	img := inst.Spec.Image
	if img == "" {
		return warpgatev1alpha1.DefaultWarpgateVersion
	}
	// Strip a digest, then take the tag after the last ':' that isn't a registry
	// port (a port is followed by a '/').
	if i := strings.IndexByte(img, '@'); i >= 0 {
		img = img[:i]
	}
	if i := strings.LastIndexByte(img, ':'); i >= 0 {
		if tag := img[i+1:]; !strings.Contains(tag, "/") {
			return strings.TrimPrefix(tag, "v")
		}
	}
	return ""
}

// imageHasShell reports whether the resolved Warpgate image bundles a POSIX shell
// and coreutils. Warpgate shipped a Debian-based image with a shell through
// 0.26; 0.27 switched to distroless (no /bin/sh, no cp). An unparseable tag
// (e.g. "latest" or a custom image) is treated as shell-less, the modern
// default, so the operator never assumes a shell it cannot rely on.
func imageHasShell(inst *warpgatev1alpha1.WarpgateInstance) bool {
	major, minor, ok := parseMajorMinor(warpgateTag(inst))
	if !ok {
		return false
	}
	return major == 0 && minor <= 26
}

// parseMajorMinor parses the leading "major.minor" of a version tag. It reports
// ok=false for anything it cannot read as two integers (e.g. "latest").
func parseMajorMinor(tag string) (major, minor int, ok bool) {
	parts := strings.SplitN(tag, ".", 3)
	if len(parts) < 2 {
		return 0, 0, false
	}
	maj, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	// Minor may carry a pre-release/build suffix (e.g. "27-rc1"); read its digits.
	minStr := parts[1]
	end := 0
	for end < len(minStr) && minStr[end] >= '0' && minStr[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, 0, false
	}
	min, err := strconv.Atoi(minStr[:end])
	if err != nil {
		return 0, 0, false
	}
	return maj, min, true
}

// warpgateImageUID and warpgateImageGID are the uid/gid the upstream Warpgate
// image runs as (the "warpgate" user, 1000:1000). The distroless init
// containers default to this so the files they seed into /data are owned by the
// runtime user; they also match the default fsGroup.
const (
	warpgateImageUID int64 = 1000
	warpgateImageGID int64 = 1000
)

// defaultFSGroup is applied to the pod so Warpgate (which runs non-root) can
// write its data directory on CSI volumes that provision root-owned
// filesystems: fsGroup is added to the process as a supplementary group and the
// volume is chowned to it.
const defaultFSGroup int64 = 1000

// podSecurityContext returns the pod-level security context to apply. An
// explicit spec.podSecurityContext wins; otherwise a default fsGroup
// (OnRootMismatch) is set so the data PVC is writable by the non-root process.
func podSecurityContext(inst *warpgatev1alpha1.WarpgateInstance) *corev1.PodSecurityContext {
	if inst.Spec.PodSecurityContext != nil {
		return inst.Spec.PodSecurityContext
	}
	fsGroup := defaultFSGroup
	policy := corev1.FSGroupChangeOnRootMismatch
	return &corev1.PodSecurityContext{
		FSGroup:             &fsGroup,
		FSGroupChangePolicy: &policy,
	}
}

func instanceReplicas(inst *warpgatev1alpha1.WarpgateInstance) int32 {
	if inst.Spec.Replicas != nil {
		return *inst.Spec.Replicas
	}
	return 1
}

func httpEnabled(inst *warpgatev1alpha1.WarpgateInstance) bool {
	if inst.Spec.HTTP == nil || inst.Spec.HTTP.Enabled == nil {
		return true // default on
	}
	return *inst.Spec.HTTP.Enabled
}

func sshEnabled(inst *warpgatev1alpha1.WarpgateInstance) bool {
	if inst.Spec.SSH == nil || inst.Spec.SSH.Enabled == nil {
		return false
	}
	return *inst.Spec.SSH.Enabled
}

func mysqlEnabled(inst *warpgatev1alpha1.WarpgateInstance) bool {
	if inst.Spec.MySQL == nil || inst.Spec.MySQL.Enabled == nil {
		return false
	}
	return *inst.Spec.MySQL.Enabled
}

func pgEnabled(inst *warpgatev1alpha1.WarpgateInstance) bool {
	if inst.Spec.PostgreSQL == nil || inst.Spec.PostgreSQL.Enabled == nil {
		return false
	}
	return *inst.Spec.PostgreSQL.Enabled
}

func kubernetesEnabled(inst *warpgatev1alpha1.WarpgateInstance) bool {
	if inst.Spec.Kubernetes == nil || inst.Spec.Kubernetes.Enabled == nil {
		return false
	}
	return *inst.Spec.Kubernetes.Enabled
}

func instanceHTTPPort(inst *warpgatev1alpha1.WarpgateInstance) int32 {
	if inst.Spec.HTTP != nil && inst.Spec.HTTP.Port != nil {
		return *inst.Spec.HTTP.Port
	}
	return 8888
}

// instanceHTTPExternalPort returns the public port Warpgate advertises in
// external URLs (notably the SSO redirect_uri) and whether it should be written
// to the config. An explicit http.externalPort wins; otherwise a set
// externalHost implies public HTTPS exposure on 443 (so url::Url omits the
// default port and the redirect_uri matches a portless OIDC registration);
// otherwise it is omitted and Warpgate falls back to the listener port.
func instanceHTTPExternalPort(inst *warpgatev1alpha1.WarpgateInstance) (int32, bool) {
	if inst.Spec.HTTP != nil && inst.Spec.HTTP.ExternalPort != nil {
		return *inst.Spec.HTTP.ExternalPort, true
	}
	if inst.Spec.ExternalHost != "" {
		return 443, true
	}
	return 0, false
}

func instanceSSHPort(inst *warpgatev1alpha1.WarpgateInstance) int32 {
	if inst.Spec.SSH != nil && inst.Spec.SSH.Port != nil {
		return *inst.Spec.SSH.Port
	}
	return 2222
}

func instanceMySQLPort(inst *warpgatev1alpha1.WarpgateInstance) int32 {
	if inst.Spec.MySQL != nil && inst.Spec.MySQL.Port != nil {
		return *inst.Spec.MySQL.Port
	}
	return 33306
}

func instancePGPort(inst *warpgatev1alpha1.WarpgateInstance) int32 {
	if inst.Spec.PostgreSQL != nil && inst.Spec.PostgreSQL.Port != nil {
		return *inst.Spec.PostgreSQL.Port
	}
	return 55432
}

func instanceKubernetesPort(inst *warpgatev1alpha1.WarpgateInstance) int32 {
	if inst.Spec.Kubernetes != nil && inst.Spec.Kubernetes.Port != nil {
		return *inst.Spec.Kubernetes.Port
	}
	return 8443
}

func instanceStorageSize(inst *warpgatev1alpha1.WarpgateInstance) string {
	if inst.Spec.Storage != nil && inst.Spec.Storage.Size != "" {
		return inst.Spec.Storage.Size
	}
	return "1Gi"
}

func storageEnabled(inst *warpgatev1alpha1.WarpgateInstance) bool {
	if inst.Spec.Storage == nil || inst.Spec.Storage.Enabled == nil {
		return true // default on
	}
	return *inst.Spec.Storage.Enabled
}

func hasExistingClaim(inst *warpgatev1alpha1.WarpgateInstance) bool {
	return inst.Spec.Storage != nil && inst.Spec.Storage.ExistingClaimName != ""
}

func certManagerEnabled(inst *warpgatev1alpha1.WarpgateInstance) bool {
	return inst.Spec.TLS != nil && inst.Spec.TLS.CertManager != nil && *inst.Spec.TLS.CertManager
}

func tlsSecretProvided(inst *warpgatev1alpha1.WarpgateInstance) bool {
	return inst.Spec.TLS != nil && inst.Spec.TLS.SecretName != ""
}

// certManagerSecretName is the Secret cert-manager writes the instance's
// Certificate to.
func certManagerSecretName(inst *warpgatev1alpha1.WarpgateInstance) string {
	return inst.Name + "-tls"
}

// usesCertManagerSecret reports whether the pod serves the cert-manager
// certificate. A user-provided tls.secretName takes precedence.
func usesCertManagerSecret(inst *warpgatev1alpha1.WarpgateInstance) bool {
	return certManagerEnabled(inst) && !tlsSecretProvided(inst)
}

// servedTLSSecret returns the Secret whose certificate Warpgate serves:
// tls.secretName when set, otherwise the cert-manager Certificate's Secret.
// An empty name means the pod serves a self-signed certificate it generates
// itself.
func servedTLSSecret(inst *warpgatev1alpha1.WarpgateInstance) string {
	switch {
	case tlsSecretProvided(inst):
		return inst.Spec.TLS.SecretName
	case certManagerEnabled(inst):
		return certManagerSecretName(inst)
	default:
		return ""
	}
}

func shouldCreateConnection(inst *warpgatev1alpha1.WarpgateInstance) bool {
	if inst.Spec.CreateConnection == nil {
		return true // default
	}
	return *inst.Spec.CreateConnection
}

func adminPasswordKey(inst *warpgatev1alpha1.WarpgateInstance) string {
	if inst.Spec.AdminPasswordSecretRef.Key != "" {
		return inst.Spec.AdminPasswordSecretRef.Key
	}
	return "password"
}

// databaseURLFromSecret reports whether the database URL comes from a Secret.
// databaseURLSecretRef wins over the deprecated inline databaseURL.
func databaseURLFromSecret(inst *warpgatev1alpha1.WarpgateInstance) bool {
	return inst.Spec.DatabaseURLSecretRef != nil && inst.Spec.DatabaseURLSecretRef.Name != ""
}

// externalDatabase reports whether Warpgate should use a database other than
// the bundled SQLite file.
func externalDatabase(inst *warpgatev1alpha1.WarpgateInstance) bool {
	return databaseURLFromSecret(inst) || inst.Spec.DatabaseURL != ""
}

func databaseURLKey(inst *warpgatev1alpha1.WarpgateInstance) string {
	if inst.Spec.DatabaseURLSecretRef != nil && inst.Spec.DatabaseURLSecretRef.Key != "" {
		return inst.Spec.DatabaseURLSecretRef.Key
	}
	return "url"
}

// databaseURLEnv returns the DATABASE_URL variable for the init container,
// sourced from the Secret when one is referenced.
func databaseURLEnv(inst *warpgatev1alpha1.WarpgateInstance) corev1.EnvVar {
	if databaseURLFromSecret(inst) {
		return corev1.EnvVar{
			Name: "DATABASE_URL",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: inst.Spec.DatabaseURLSecretRef.Name,
					},
					Key: databaseURLKey(inst),
				},
			},
		}
	}
	return corev1.EnvVar{Name: "DATABASE_URL", Value: inst.Spec.DatabaseURL}
}

func instanceLabels(inst *warpgatev1alpha1.WarpgateInstance) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "warpgate",
		"app.kubernetes.io/instance":   inst.Name,
		"app.kubernetes.io/managed-by": "warpgate-operator",
	}
}

// configHash returns a short hash of fields that should trigger a pod rollout
// when they change (version, image, ports, config-affecting settings).
func configHash(inst *warpgatev1alpha1.WarpgateInstance) string {
	h := sha256.New()
	h.Write([]byte(resolveImage(inst)))
	h.Write(fmt.Appendf(nil, "%d%d%d%d%d",
		instanceHTTPPort(inst), instanceSSHPort(inst),
		instanceMySQLPort(inst), instancePGPort(inst),
		instanceKubernetesPort(inst)))
	h.Write(fmt.Appendf(nil, "%v%v%v%v%v",
		httpEnabled(inst), sshEnabled(inst),
		mysqlEnabled(inst), pgEnabled(inst),
		kubernetesEnabled(inst)))
	h.Write([]byte(inst.Spec.ExternalHost))
	if ep, ok := instanceHTTPExternalPort(inst); ok {
		h.Write(fmt.Appendf(nil, "extport=%d", ep))
	}
	h.Write([]byte(inst.Spec.ConfigOverride))
	if sc := podSecurityContext(inst); sc != nil {
		if sc.FSGroup != nil {
			h.Write(fmt.Appendf(nil, "fsgroup=%d", *sc.FSGroup))
		}
		if sc.RunAsUser != nil {
			h.Write(fmt.Appendf(nil, "runasuser=%d", *sc.RunAsUser))
		}
	}
	h.Write(databaseURLFingerprint(inst))
	if databaseURLFromSecret(inst) {
		h.Write([]byte(inst.Spec.DatabaseURLSecretRef.Name + "/" + databaseURLKey(inst)))
	}
	h.Write([]byte(inst.Spec.SSHKeysSecretName))
	if ssoEnabled(inst) {
		// Hash the non-secret SSO config plus the secret's ref (name/key), so a
		// changed mapping or a rotated-to Secret rolls the pods. The secret value
		// itself is never written to the rollout hash.
		for _, p := range inst.Spec.SSO.Providers {
			h.Write(fmt.Appendf(nil, "sso|%s|%s|%s|%s|%v|%v|%s/%s|%v|%v",
				p.Name, p.Label, p.ClientID, p.IssuerURL,
				p.Scopes, p.AutoCreateUsers,
				p.ClientSecretSecretRef.Name, p.ClientSecretSecretRef.Key,
				p.AdditionalTrustedAudiences, p.TrustUnknownAudiences))
			rmKeys := make([]string, 0, len(p.RoleMappings))
			for k := range p.RoleMappings {
				rmKeys = append(rmKeys, k)
			}
			sort.Strings(rmKeys)
			for _, k := range rmKeys {
				h.Write(fmt.Appendf(nil, "rm|%s=%s", k, p.RoleMappings[k]))
			}
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:12]
}

// databaseURLFingerprintIterations makes guessing a database password from the
// published rollout hash expensive; the URL usually embeds a credential.
const databaseURLFingerprintIterations = 100_000

// databaseURLFingerprint derives a change-detection value for spec.databaseURL
// with PBKDF2 instead of feeding the raw URL into the fast rollout hash, which
// ends up in a pod template annotation anyone with pod read access can see.
func databaseURLFingerprint(inst *warpgatev1alpha1.WarpgateInstance) []byte {
	if inst.Spec.DatabaseURL == "" {
		return nil
	}
	key, err := pbkdf2.Key(sha256.New, inst.Spec.DatabaseURL,
		[]byte("warpgate-operator/database-url/"+string(inst.UID)),
		databaseURLFingerprintIterations, 32)
	if err != nil {
		// Only fails for invalid parameters; still change on URL changes
		// without exposing the raw value.
		return []byte("database-url-set")
	}
	return key
}

// ---------------------------------------------------------------------------
// 4. ConfigMap — warpgate.yaml
// ---------------------------------------------------------------------------

// ssoEnabled reports whether SSO providers are configured and should be
// rendered. configOverride wins: when it is set the whole config comes from the
// override, so sso is ignored.
func ssoEnabled(inst *warpgatev1alpha1.WarpgateInstance) bool {
	return inst.Spec.ConfigOverride == "" &&
		inst.Spec.SSO != nil && len(inst.Spec.SSO.Providers) > 0
}

// ssoEnvSuffix turns a provider name into an upper-case, env-var-safe token
// (every non-alphanumeric becomes an underscore).
func ssoEnvSuffix(name string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(name) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// ssoClientSecretEnv is the env var name that carries a provider's client secret
// into the init container.
func ssoClientSecretEnv(name string) string {
	return "WARPGATE_SSO_" + ssoEnvSuffix(name) + "_CLIENT_SECRET"
}

// ssoClientSecretSentinel is the placeholder written into the ConfigMap in place
// of a provider's client secret; the init script swaps it for the real value
// from the environment, so the secret never lands in the ConfigMap.
func ssoClientSecretSentinel(name string) string {
	return "__WG_SSO_SECRET_" + ssoEnvSuffix(name) + "__"
}

// yamlScalar renders s as a single safely-quoted YAML scalar (no trailing
// newline), so operator-supplied values cannot break out of their field.
func yamlScalar(s string) string {
	out, err := yaml.Marshal(s)
	if err != nil {
		return `""`
	}
	return strings.TrimRight(string(out), "\n")
}

// writeSSOProviders renders the sso_providers block. Each client_secret is a
// sentinel the init script replaces at runtime.
func writeSSOProviders(b *strings.Builder, inst *warpgatev1alpha1.WarpgateInstance) {
	b.WriteString("sso_providers:\n")
	for _, p := range inst.Spec.SSO.Providers {
		fmt.Fprintf(b, "  - name: %s\n", yamlScalar(p.Name))
		label := p.Label
		if label == "" {
			label = p.Name
		}
		fmt.Fprintf(b, "    label: %s\n", yamlScalar(label))
		if p.AutoCreateUsers {
			b.WriteString("    auto_create_users: true\n")
		}
		b.WriteString("    provider:\n")
		b.WriteString("      type: custom\n")
		fmt.Fprintf(b, "      client_id: %s\n", yamlScalar(p.ClientID))
		fmt.Fprintf(b, "      client_secret: '%s'\n", ssoClientSecretSentinel(p.Name))
		fmt.Fprintf(b, "      issuer_url: %s\n", yamlScalar(p.IssuerURL))
		b.WriteString("      scopes:\n")
		for _, s := range p.Scopes {
			fmt.Fprintf(b, "        - %s\n", yamlScalar(s))
		}
		if len(p.RoleMappings) > 0 {
			b.WriteString("      role_mappings:\n")
			keys := make([]string, 0, len(p.RoleMappings))
			for k := range p.RoleMappings {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Fprintf(b, "        %s: %s\n", yamlScalar(k), yamlScalar(p.RoleMappings[k]))
			}
		}
		if len(p.AdditionalTrustedAudiences) > 0 {
			b.WriteString("      additional_trusted_audiences:\n")
			for _, a := range p.AdditionalTrustedAudiences {
				fmt.Fprintf(b, "        - %s\n", yamlScalar(a))
			}
		}
		if p.TrustUnknownAudiences {
			b.WriteString("      trust_unknown_audiences: true\n")
		}
	}
}

// ssoClientSecretEnvVars returns the init-container env vars that carry each SSO
// provider's client secret from its referenced Secret.
func ssoClientSecretEnvVars(inst *warpgatev1alpha1.WarpgateInstance) []corev1.EnvVar {
	if !ssoEnabled(inst) {
		return nil
	}
	envs := make([]corev1.EnvVar, 0, len(inst.Spec.SSO.Providers))
	for _, p := range inst.Spec.SSO.Providers {
		key := p.ClientSecretSecretRef.Key
		if key == "" {
			key = "client-secret"
		}
		envs = append(envs, corev1.EnvVar{
			Name: ssoClientSecretEnv(p.Name),
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: p.ClientSecretSecretRef.Name},
					Key:                  key,
				},
			},
		})
	}
	return envs
}

// ssoInitScriptLines returns the init-script lines that replace each provider's
// client_secret sentinel in /data/warpgate.yaml with the value from its env var.
// The whole client_secret line is rebuilt (fixed indent) rather than
// pattern-substituted, so no special char in the secret can misfire: the quote
// char arrives as an awk var and the value through ENVIRON, pre-escaped for a
// single-quoted YAML scalar.
func ssoInitScriptLines(inst *warpgatev1alpha1.WarpgateInstance) []string {
	if !ssoEnabled(inst) {
		return nil
	}
	var lines []string
	for _, p := range inst.Spec.SSO.Providers {
		escVar := "WG_SSO_ESC_" + ssoEnvSuffix(p.Name)
		lines = append(lines,
			escVar+`="$(printf '%s' "${`+ssoClientSecretEnv(p.Name)+`}" | sed "s/'/''/g")"`,
			`export `+escVar,
			`awk -v ph="`+ssoClientSecretSentinel(p.Name)+`" -v envk="`+escVar+`" -v q="'" 'index($0,ph)>0{print "      client_secret: " q ENVIRON[envk] q; next}{print}' /data/warpgate.yaml > /data/warpgate.yaml.sso && mv /data/warpgate.yaml.sso /data/warpgate.yaml`,
		)
	}
	return lines
}

func (r *WarpgateInstanceReconciler) buildWarpgateConfig(inst *warpgatev1alpha1.WarpgateInstance) string {
	var b strings.Builder

	// An external database URL usually carries credentials, so it never goes
	// into this ConfigMap. The init container appends it from DATABASE_URL.
	if !externalDatabase(inst) {
		b.WriteString("database_url: sqlite:/data/db\n")
	}

	// listener writes one protocol block. Every TLS-terminating listener reads
	// its certificate from the same pair the init container puts in /data.
	listener := func(name string, enabled bool, port int32, withTLS bool) {
		fmt.Fprintf(&b, "%s:\n", name)
		fmt.Fprintf(&b, "  enable: %t\n", enabled)
		fmt.Fprintf(&b, "  listen: \"0.0.0.0:%d\"\n", port)
		if withTLS {
			b.WriteString("  certificate: /data/tls.certificate.pem\n")
			b.WriteString("  key: /data/tls.key.pem\n")
		}
	}

	// HTTP has no enable switch in Warpgate; the admin UI always listens.
	b.WriteString("http:\n")
	fmt.Fprintf(&b, "  listen: \"0.0.0.0:%d\"\n", instanceHTTPPort(inst))
	if ep, ok := instanceHTTPExternalPort(inst); ok {
		fmt.Fprintf(&b, "  external_port: %d\n", ep)
	}
	b.WriteString("  certificate: /data/tls.certificate.pem\n")
	b.WriteString("  key: /data/tls.key.pem\n")

	listener("ssh", sshEnabled(inst), instanceSSHPort(inst), false)
	if mysqlEnabled(inst) {
		listener("mysql", true, instanceMySQLPort(inst), true)
	}
	if pgEnabled(inst) {
		listener("postgres", true, instancePGPort(inst), true)
	}
	if kubernetesEnabled(inst) {
		listener("kubernetes", true, instanceKubernetesPort(inst), true)
	}

	if inst.Spec.ExternalHost != "" {
		// Marshal instead of printf so the value can't break out of its scalar.
		out, err := yaml.Marshal(map[string]string{"external_host": inst.Spec.ExternalHost})
		if err == nil {
			b.Write(out)
		}
	}

	if sb := inst.Spec.SecretBackends; sb != nil && sb.Vault != nil {
		v := sb.Vault
		b.WriteString("secret_backends:\n")
		b.WriteString("  - type: vault\n")
		fmt.Fprintf(&b, "    url: \"%s\"\n", v.URL)
		if v.Path != "" {
			fmt.Fprintf(&b, "    path: \"%s\"\n", v.Path)
		}
		if v.TLSSkipVerify {
			b.WriteString("    tls_skip_verify: true\n")
		}
		if v.TokenSecretRef != nil {
			b.WriteString("    auth:\n")
			b.WriteString("      type: token\n")
			b.WriteString("      # token injected at runtime via VAULT_TOKEN env var\n")
		} else if v.AppRole != nil {
			b.WriteString("    auth:\n")
			b.WriteString("      type: approle\n")
			fmt.Fprintf(&b, "      role_id: \"%s\"\n", v.AppRole.RoleID)
			b.WriteString("      # secret_id injected at runtime via VAULT_SECRET_ID env var\n")
		}
	}

	// Session recording lives in Warpgate's database parameters, not the config
	// file; it is seeded through unattended-setup (see the init script).

	// SSO providers. Each client_secret is a sentinel the init script swaps for
	// the real value from the environment, so it never sits in the ConfigMap.
	if ssoEnabled(inst) {
		writeSSOProviders(&b, inst)
	}

	return b.String()
}

func (r *WarpgateInstanceReconciler) ensureConfigMap(ctx context.Context, inst *warpgatev1alpha1.WarpgateInstance) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      inst.Name + "-config",
			Namespace: inst.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		if err := controllerutil.SetControllerReference(inst, cm, r.Scheme); err != nil {
			return err
		}
		cm.Data = map[string]string{
			"warpgate.yaml": r.buildWarpgateConfig(inst),
		}
		return nil
	})
	return err
}

func (r *WarpgateInstanceReconciler) ensureConfigOverrideConfigMap(ctx context.Context, inst *warpgatev1alpha1.WarpgateInstance) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      inst.Name + "-config-override",
			Namespace: inst.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		if err := controllerutil.SetControllerReference(inst, cm, r.Scheme); err != nil {
			return err
		}
		cm.Data = map[string]string{
			"warpgate.yaml": inst.Spec.ConfigOverride,
		}
		return nil
	})
	return err
}

// ---------------------------------------------------------------------------
// 5. PVC — create only, never update
// ---------------------------------------------------------------------------

func (r *WarpgateInstanceReconciler) ensurePVC(ctx context.Context, inst *warpgatev1alpha1.WarpgateInstance) error {
	pvcName := inst.Name + "-data"

	var existing corev1.PersistentVolumeClaim
	err := r.Get(ctx, types.NamespacedName{Name: pvcName, Namespace: inst.Namespace}, &existing)
	if err == nil {
		return nil // already exists, don't update
	}
	if !apierrors.IsNotFound(err) {
		return err
	}

	qty, err := resource.ParseQuantity(instanceStorageSize(inst))
	if err != nil {
		return fmt.Errorf("parsing storage size %q: %w", instanceStorageSize(inst), err)
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: inst.Namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: qty,
				},
			},
		},
	}

	if inst.Spec.Storage != nil && inst.Spec.Storage.StorageClassName != nil {
		pvc.Spec.StorageClassName = inst.Spec.Storage.StorageClassName
	}

	if err := controllerutil.SetControllerReference(inst, pvc, r.Scheme); err != nil {
		return fmt.Errorf("setting owner reference on PVC: %w", err)
	}

	return r.Create(ctx, pvc)
}

// ---------------------------------------------------------------------------
// 6. Deployment — with init container matching the official Helm chart pattern
// ---------------------------------------------------------------------------

func (r *WarpgateInstanceReconciler) buildDeployment(inst *warpgatev1alpha1.WarpgateInstance) *appsv1.Deployment {
	labels := instanceLabels(inst)
	rep := instanceReplicas(inst)
	image := resolveImage(inst)

	// Container ports
	var ports []corev1.ContainerPort
	if httpEnabled(inst) {
		ports = append(ports, corev1.ContainerPort{
			Name: "http", ContainerPort: instanceHTTPPort(inst), Protocol: corev1.ProtocolTCP,
		})
	}
	if sshEnabled(inst) {
		ports = append(ports, corev1.ContainerPort{
			Name: "ssh", ContainerPort: instanceSSHPort(inst), Protocol: corev1.ProtocolTCP,
		})
	}
	if mysqlEnabled(inst) {
		ports = append(ports, corev1.ContainerPort{
			Name: "mysql", ContainerPort: instanceMySQLPort(inst), Protocol: corev1.ProtocolTCP,
		})
	}
	if pgEnabled(inst) {
		ports = append(ports, corev1.ContainerPort{
			Name: "postgresql", ContainerPort: instancePGPort(inst), Protocol: corev1.ProtocolTCP,
		})
	}
	if kubernetesEnabled(inst) {
		ports = append(ports, corev1.ContainerPort{
			Name: "kubernetes", ContainerPort: instanceKubernetesPort(inst), Protocol: corev1.ProtocolTCP,
		})
	}

	// Build the init script matching the official Helm chart pattern:
	// 1. Copy SSH keys from secret mount to /data/ssh-keys/ (if mounted)
	// 2. Run unattended-setup if /data/warpgate.yaml doesn't exist
	// 3. Copy config override if provided
	// 4. Copy TLS certs from the secret mount (tls.crt -> tls.certificate.pem,
	//    tls.key -> tls.key.pem), or generate a self-signed pair
	// 5. Set file permissions to 600
	var scriptParts []string
	scriptParts = append(scriptParts, "#!/bin/sh", "set -e")

	// 1. Copy SSH keys if mounted
	if inst.Spec.SSHKeysSecretName != "" {
		scriptParts = append(scriptParts,
			`echo "Copying SSH keys..."`,
			`mkdir -p /data/ssh-keys`,
			`if [ -d /ssh-keys ]; then`,
			`  cp /ssh-keys/* /data/ssh-keys/ 2>/dev/null || true`,
			`  chmod 600 /data/ssh-keys/* 2>/dev/null || true`,
			`fi`,
		)
	}

	// 2. Run unattended-setup if warpgate.yaml doesn't exist. Setup owns the
	// one-time state that lives outside the config file: the admin user, SSH
	// keys, and the recording parameter.
	setupCmd := fmt.Sprintf(
		`warpgate --skip-securing-files unattended-setup --data-path /data --http-port %d --admin-password "${ADMIN_PASSWORD}"`,
		instanceHTTPPort(inst),
	)
	if kubernetesEnabled(inst) {
		setupCmd += fmt.Sprintf(` --kubernetes-port %d`, instanceKubernetesPort(inst))
	}
	if externalDatabase(inst) {
		// The URL reaches the script through the environment so the shell never
		// parses it as code.
		setupCmd += ` --database-url "${DATABASE_URL}"`
	}
	if inst.Spec.RecordSessions != nil && *inst.Spec.RecordSessions {
		// ponytail: seeded at first setup only; later toggles need the admin UI or parameters API
		setupCmd += ` --record-sessions`
	}
	if inst.Spec.SSHKeysSecretName != "" {
		setupCmd += ` --import-ssh-host-keys /data/ssh-keys --import-ssh-client-keys /data/ssh-keys`
	}

	scriptParts = append(scriptParts,
		`if [ ! -f /data/warpgate.yaml ]; then`,
		`  echo "Running initial Warpgate setup..."`,
		fmt.Sprintf("  %s", setupCmd),
		`  echo "Initial setup complete."`,
		`else`,
		`  echo "Warpgate already initialized, skipping setup."`,
		`fi`,
	)

	// 3. Copy config override if provided. Remove any existing file first so a
	// /data left root-owned by an older seed (which this non-root user could not
	// overwrite in place) is replaced rather than hit with "Permission denied";
	// unlinking only needs write on the fsGroup-owned /data directory.
	if inst.Spec.ConfigOverride != "" {
		scriptParts = append(scriptParts,
			`echo "Applying config override..."`,
			`rm -f /data/warpgate.yaml`,
			`cp /override/warpgate.yaml /data/warpgate.yaml`,
		)
	} else {
		// Apply the operator-generated config
		scriptParts = append(scriptParts,
			`echo "Applying operator config..."`,
			`rm -f /data/warpgate.yaml`,
			`cp /config/warpgate.yaml /data/warpgate.yaml`,
		)
		if externalDatabase(inst) {
			// Write database_url from the environment as a single-quoted YAML
			// scalar ('' is the only escape needed inside one).
			scriptParts = append(scriptParts,
				`sed -i '/^database_url:/d' /data/warpgate.yaml`,
				`printf "database_url: '%s'\n" "$(printf '%s' "${DATABASE_URL}" | sed "s/'/''/g")" >> /data/warpgate.yaml`,
			)
		}
		// Swap each SSO provider's client_secret sentinel for the real value from
		// its env var (see ssoInitScriptLines).
		scriptParts = append(scriptParts, ssoInitScriptLines(inst)...)
	}

	// 4. Copy the served TLS certificate (tls.secretName or the cert-manager
	// Secret) on every start, after setup so nothing setup generates can
	// replace it. Without one, generate a self-signed pair once.
	if servedTLSSecret(inst) != "" {
		scriptParts = append(scriptParts,
			`echo "Copying TLS certificates from secret..."`,
			`if [ -f /tls-secret/tls.crt ]; then`,
			`  rm -f /data/tls.certificate.pem /data/tls.key.pem`,
			`  cp /tls-secret/tls.crt /data/tls.certificate.pem`,
			`  cp /tls-secret/tls.key /data/tls.key.pem`,
			`  chmod 600 /data/tls.certificate.pem /data/tls.key.pem`,
			`fi`,
		)
	} else {
		scriptParts = append(scriptParts,
			`if [ ! -f /data/tls.certificate.pem ]; then`,
			`  echo "Generating self-signed TLS certificate..."`,
			`  apk add --no-cache openssl >/dev/null 2>&1 || true`,
			`  openssl req -x509 -newkey rsa:4096 \`,
			`    -keyout /data/tls.key.pem -out /data/tls.certificate.pem \`,
			`    -days 3650 -nodes -subj "/CN=warpgate"`,
			`fi`,
		)
	}

	// Set permissions on all sensitive files
	scriptParts = append(scriptParts,
		`chmod 600 /data/*.pem 2>/dev/null || true`,
		`chmod 600 /data/warpgate.yaml 2>/dev/null || true`,
		`echo "Init complete."`,
	)

	initScript := strings.Join(scriptParts, "\n")

	initEnv := []corev1.EnvVar{
		{
			Name: "ADMIN_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: inst.Spec.AdminPasswordSecretRef.Name,
					},
					Key: adminPasswordKey(inst),
				},
			},
		},
	}
	if externalDatabase(inst) {
		initEnv = append(initEnv, databaseURLEnv(inst))
	}
	initEnv = append(initEnv, ssoClientSecretEnvVars(inst)...)

	// Init container volume mounts
	initVolumeMounts := []corev1.VolumeMount{
		{Name: "data", MountPath: "/data"},
		{Name: "config", MountPath: "/config", ReadOnly: true},
	}
	if inst.Spec.SSHKeysSecretName != "" {
		initVolumeMounts = append(initVolumeMounts, corev1.VolumeMount{
			Name: "ssh-keys", MountPath: "/ssh-keys", ReadOnly: true,
		})
	}
	if servedTLSSecret(inst) != "" {
		initVolumeMounts = append(initVolumeMounts, corev1.VolumeMount{
			Name: "tls-secret", MountPath: "/tls-secret", ReadOnly: true,
		})
	}
	if inst.Spec.ConfigOverride != "" {
		initVolumeMounts = append(initVolumeMounts, corev1.VolumeMount{
			Name: "config-override", MountPath: "/override", ReadOnly: true,
		})
	}

	// Init containers. Shell-bearing Warpgate images (<= 0.26) seed the config in
	// a single in-image init container that runs the script above. Shell-less
	// distroless images (>= 0.27) have no /bin/sh or cp, so the script cannot run
	// in the Warpgate image: seeding is split into a shell-free Warpgate setup
	// container and a busybox config-seed container. See buildInitContainers.
	initContainers, rolloutMaterial := buildInitContainers(inst, image, initScript, initEnv, initVolumeMounts)

	// Probes — use HTTP health check on the Warpgate admin API when HTTP is enabled.
	var livenessProbe, readinessProbe *corev1.Probe
	if httpEnabled(inst) {
		livenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:   "/@warpgate/api/info",
					Port:   intstr.FromInt32(instanceHTTPPort(inst)),
					Scheme: corev1.URISchemeHTTPS,
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       30,
			TimeoutSeconds:      5,
			FailureThreshold:    3,
		}
		readinessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:   "/@warpgate/api/info",
					Port:   intstr.FromInt32(instanceHTTPPort(inst)),
					Scheme: corev1.URISchemeHTTPS,
				},
			},
			InitialDelaySeconds: 15,
			PeriodSeconds:       10,
			TimeoutSeconds:      5,
			FailureThreshold:    3,
		}
	}

	// Fold the init script into the rollout hash so pods pick up changes to
	// the generated script (not just to the CR spec). This is a change-detection
	// key, not a security hash: the script only references secrets by env var
	// name and configHash already derives the database URL with PBKDF2.
	rollout := fnv.New64a()
	_, _ = rollout.Write([]byte(configHash(inst))) // hash writes never fail
	_, _ = rollout.Write([]byte(rolloutMaterial))
	hash := fmt.Sprintf("%016x", rollout.Sum64())[:12]

	// Deployment strategy — default Recreate for RWO PVC compatibility.
	strategy := appsv1.DeploymentStrategy{
		Type: appsv1.RecreateDeploymentStrategyType,
	}
	if inst.Spec.Strategy == "RollingUpdate" {
		strategy = appsv1.DeploymentStrategy{
			Type: appsv1.RollingUpdateDeploymentStrategyType,
		}
	}

	// Volumes
	volumes := []corev1.Volume{
		{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: inst.Name + "-config",
					},
				},
			},
		},
	}

	// Data volume — PVC (operator-created or existing) vs emptyDir
	if storageEnabled(inst) {
		claimName := inst.Name + "-data"
		if hasExistingClaim(inst) {
			claimName = inst.Spec.Storage.ExistingClaimName
		}
		volumes = append(volumes, corev1.Volume{
			Name: "data",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: claimName,
				},
			},
		})
	} else {
		volumes = append(volumes, corev1.Volume{
			Name: "data",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	// Optional SSH keys secret volume
	if inst.Spec.SSHKeysSecretName != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "ssh-keys",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: inst.Spec.SSHKeysSecretName,
				},
			},
		})
	}

	// Optional TLS secret volume (spec.tls.secretName or the cert-manager Secret)
	if name := servedTLSSecret(inst); name != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "tls-secret",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: name,
				},
			},
		})
	}

	// Optional config override volume
	if inst.Spec.ConfigOverride != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "config-override",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: inst.Name + "-config-override",
					},
				},
			},
		})
	}

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      inst.Name,
			Namespace: inst.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &rep,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Strategy: strategy,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
					Annotations: map[string]string{
						configHashAnnotation: hash,
					},
				},
				Spec: corev1.PodSpec{
					SecurityContext: podSecurityContext(inst),
					InitContainers:  initContainers,
					Containers: []corev1.Container{
						{
							Name:  "warpgate",
							Image: image,
							Args:  []string{"--config", "/data/warpgate.yaml", "--skip-securing-files", "run"},
							Ports: ports,
							VolumeMounts: []corev1.VolumeMount{
								{Name: "data", MountPath: "/data"},
							},
							Resources:      inst.Spec.Resources,
							LivenessProbe:  livenessProbe,
							ReadinessProbe: readinessProbe,
						},
					},
					Volumes:      volumes,
					NodeSelector: inst.Spec.NodeSelector,
					Tolerations:  inst.Spec.Tolerations,
				},
			},
		},
	}

	return deploy
}

// buildInitContainers returns the pod's init containers and the string to fold
// into the rollout hash so a change to any generated init step restarts the pod.
//
// Shell-bearing Warpgate images (<= 0.26) keep the original single in-image init
// container that runs shellInitScript via /bin/sh. Shell-less distroless images
// (>= 0.27, and anything whose tag the operator cannot parse) have no shell or
// cp, so the Warpgate image cannot run the script: seeding is split into a
// Warpgate setup container that runs the warpgate binary directly (shell-free)
// and a busybox config-seed container that performs the file operations. Both
// paths reapply the operator config on every boot and leave the admin bootstrap
// as a one-time step.
func buildInitContainers(
	inst *warpgatev1alpha1.WarpgateInstance,
	image, shellInitScript string,
	shellEnv []corev1.EnvVar,
	shellMounts []corev1.VolumeMount,
) ([]corev1.Container, string) {
	if imageHasShell(inst) {
		return []corev1.Container{
			{
				Name:         "init-setup",
				Image:        image,
				Command:      []string{"/bin/sh", "-c", shellInitScript},
				VolumeMounts: shellMounts,
				Env:          shellEnv,
			},
		}, shellInitScript
	}

	// Distroless path. Container 1 runs the Warpgate binary (the image entrypoint)
	// directly to bootstrap the admin user, keys and self-signed TLS cert once,
	// writing /data/warpgate.yaml. Warpgate's own --if-config-exists skip makes
	// re-runs a no-op, which replaces the shell "[ -f ]" guard. Container 2 then
	// overwrites that config with the operator-generated one on every boot.
	setupArgs := warpgateSetupArgs(inst)

	setupEnv := []corev1.EnvVar{adminPasswordEnvVar(inst)}
	setupMounts := []corev1.VolumeMount{{Name: "data", MountPath: "/data"}}
	if externalDatabase(inst) {
		setupEnv = append(setupEnv, databaseURLEnv(inst))
	}
	if inst.Spec.SSHKeysSecretName != "" {
		// Setup imports the keys straight from the read-only Secret mount, so no
		// pre-copy step (and thus no shell) is needed before it runs.
		setupMounts = append(setupMounts, corev1.VolumeMount{
			Name: "ssh-keys", MountPath: "/ssh-keys", ReadOnly: true,
		})
	}

	seedScript := buildSeedScript(inst)

	seedEnv := []corev1.EnvVar{}
	if externalDatabase(inst) {
		seedEnv = append(seedEnv, databaseURLEnv(inst))
	}
	seedEnv = append(seedEnv, ssoClientSecretEnvVars(inst)...)

	seedMounts := []corev1.VolumeMount{
		{Name: "data", MountPath: "/data"},
		{Name: "config", MountPath: "/config", ReadOnly: true},
	}
	if servedTLSSecret(inst) != "" {
		seedMounts = append(seedMounts, corev1.VolumeMount{
			Name: "tls-secret", MountPath: "/tls-secret", ReadOnly: true,
		})
	}
	if inst.Spec.ConfigOverride != "" {
		seedMounts = append(seedMounts, corev1.VolumeMount{
			Name: "config-override", MountPath: "/override", ReadOnly: true,
		})
	}

	// Both init containers must create files in /data as the same user that the
	// main container runs as, so the non-root runtime can read them. The seed
	// image (busybox) defaults to the root user, which would otherwise write a
	// root-owned config the Warpgate user cannot read under a non-root
	// securityContext. See seedSecurityContext.
	initSC := seedSecurityContext(inst)

	containers := []corev1.Container{
		{
			Name:            "init-setup",
			Image:           image,
			Args:            setupArgs,
			VolumeMounts:    setupMounts,
			Env:             setupEnv,
			SecurityContext: initSC,
		},
		{
			Name:            "init-config",
			Image:           seedImage(inst),
			Command:         []string{"/bin/sh", "-c", seedScript},
			VolumeMounts:    seedMounts,
			Env:             seedEnv,
			SecurityContext: initSC,
		},
	}
	return containers, strings.Join(setupArgs, " ") + "\n" + seedScript
}

// seedRuntimeUser returns the uid and gid the Warpgate pod runs as, which the
// distroless init containers must also use so the files they seed into /data are
// owned by (and readable by) the runtime user. An explicit
// spec.podSecurityContext RunAsUser/RunAsGroup wins; otherwise the default is
// 1000:1000, the uid/gid baked into the upstream Warpgate image and the default
// fsGroup.
func seedRuntimeUser(inst *warpgatev1alpha1.WarpgateInstance) (uid, gid int64) {
	uid, gid = warpgateImageUID, warpgateImageGID
	if sc := inst.Spec.PodSecurityContext; sc != nil {
		if sc.RunAsUser != nil {
			uid = *sc.RunAsUser
		}
		if sc.RunAsGroup != nil {
			gid = *sc.RunAsGroup
		}
	}
	return uid, gid
}

// seedSecurityContext pins the distroless init containers to the runtime
// uid/gid. Without it the busybox seed container runs as root and writes a
// root-owned /data/warpgate.yaml that the non-root Warpgate user cannot read
// ("Could not load config: Permission denied"). RunAsNonRoot is asserted unless
// the operator was explicitly told to run as uid 0.
func seedSecurityContext(inst *warpgatev1alpha1.WarpgateInstance) *corev1.SecurityContext {
	uid, gid := seedRuntimeUser(inst)
	sc := &corev1.SecurityContext{
		RunAsUser:  &uid,
		RunAsGroup: &gid,
	}
	if uid != 0 {
		nonRoot := true
		sc.RunAsNonRoot = &nonRoot
	}
	return sc
}

// adminPasswordEnvVar returns the ADMIN_PASSWORD env var sourced from the admin
// password Secret.
func adminPasswordEnvVar(inst *warpgatev1alpha1.WarpgateInstance) corev1.EnvVar {
	return corev1.EnvVar{
		Name: "ADMIN_PASSWORD",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: inst.Spec.AdminPasswordSecretRef.Name,
				},
				Key: adminPasswordKey(inst),
			},
		},
	}
}

// warpgateSetupArgs returns the argv for the shell-free Warpgate setup init
// container (distroless path). The image entrypoint is the warpgate binary, so
// these are its arguments. Secret values arrive by Kubernetes $(VAR) expansion
// of the container env, never literally in the manifest. --config points the
// built-in "config already exists" check at the same /data/warpgate.yaml the
// shell path guards on, and --if-config-exists skip makes a re-run a no-op.
func warpgateSetupArgs(inst *warpgatev1alpha1.WarpgateInstance) []string {
	args := []string{
		"--config", "/data/warpgate.yaml",
		"--skip-securing-files",
		"unattended-setup",
		"--data-path", "/data",
		"--http-port", strconv.Itoa(int(instanceHTTPPort(inst))),
		"--admin-password", "$(ADMIN_PASSWORD)",
		"--if-config-exists", "skip",
	}
	if kubernetesEnabled(inst) {
		args = append(args, "--kubernetes-port", strconv.Itoa(int(instanceKubernetesPort(inst))))
	}
	if externalDatabase(inst) {
		args = append(args, "--database-url", "$(DATABASE_URL)")
	}
	if inst.Spec.RecordSessions != nil && *inst.Spec.RecordSessions {
		args = append(args, "--record-sessions")
	}
	if inst.Spec.SSHKeysSecretName != "" {
		args = append(args, "--import-ssh-host-keys", "/ssh-keys", "--import-ssh-client-keys", "/ssh-keys")
	}
	return args
}

// buildSeedScript returns the shell script run by the busybox config-seed init
// container (distroless path). It reapplies the operator config to
// /data/warpgate.yaml on every boot, mirroring steps 3-5 of the shell path but
// without the admin bootstrap (done by the setup container), the SSH pre-copy
// (setup imports keys from the Secret mount) and the openssl self-signed branch
// (Warpgate's setup generates that certificate). busybox provides sh, cp, sed
// and awk, so the script never touches the Warpgate image.
func buildSeedScript(inst *warpgatev1alpha1.WarpgateInstance) string {
	var parts []string
	parts = append(parts, "#!/bin/sh", "set -e")

	// Remove any existing config first, then write a fresh copy owned by this
	// (runtime) user. A /data poisoned by an older seed that ran as root leaves
	// /data/warpgate.yaml root-owned; an in-place `cp` would fail with
	// "Permission denied", so unlink it first (write on the fsGroup-owned /data
	// directory is enough) and recreate it. This self-heals a bricked volume on
	// the next reconcile without manual PVC surgery.
	if inst.Spec.ConfigOverride != "" {
		parts = append(parts,
			`echo "Applying config override..."`,
			`rm -f /data/warpgate.yaml`,
			`cp /override/warpgate.yaml /data/warpgate.yaml`,
		)
	} else {
		parts = append(parts,
			`echo "Applying operator config..."`,
			`rm -f /data/warpgate.yaml`,
			`cp /config/warpgate.yaml /data/warpgate.yaml`,
		)
		if externalDatabase(inst) {
			parts = append(parts,
				`sed -i '/^database_url:/d' /data/warpgate.yaml`,
				`printf "database_url: '%s'\n" "$(printf '%s' "${DATABASE_URL}" | sed "s/'/''/g")" >> /data/warpgate.yaml`,
			)
		}
		parts = append(parts, ssoInitScriptLines(inst)...)
	}

	// Copy the served TLS certificate when a Secret provides one. Without one,
	// the setup container already generated a self-signed pair in /data.
	if servedTLSSecret(inst) != "" {
		parts = append(parts,
			`echo "Copying TLS certificates from secret..."`,
			`if [ -f /tls-secret/tls.crt ]; then`,
			`  rm -f /data/tls.certificate.pem /data/tls.key.pem`,
			`  cp /tls-secret/tls.crt /data/tls.certificate.pem`,
			`  cp /tls-secret/tls.key /data/tls.key.pem`,
			`  chmod 0640 /data/tls.certificate.pem /data/tls.key.pem`,
			`fi`,
		)
	}

	// Mode 0640 (not 0600) so the files stay readable by the pod's group as well
	// as the owner. This container runs as the runtime uid (see
	// seedSecurityContext), and the fsGroup owns /data, so the non-root Warpgate
	// process can always read what the seed writes, on every CSI driver,
	// regardless of which uid created the file.
	parts = append(parts,
		`chmod 0640 /data/*.pem 2>/dev/null || true`,
		`chmod 0640 /data/warpgate.yaml 2>/dev/null || true`,
		`echo "Config seed complete."`,
	)

	return strings.Join(parts, "\n")
}

func (r *WarpgateInstanceReconciler) ensureDeployment(ctx context.Context, inst *warpgatev1alpha1.WarpgateInstance) error {
	desired := r.buildDeployment(inst)
	if err := controllerutil.SetControllerReference(inst, desired, r.Scheme); err != nil {
		return fmt.Errorf("setting owner reference on Deployment: %w", err)
	}
	tlsHash, err := r.tlsSecretHash(ctx, inst)
	if err != nil {
		return err
	}
	if tlsHash != "" {
		desired.Spec.Template.Annotations[tlsHashAnnotation] = tlsHash
	}

	var existing appsv1.Deployment
	err = r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, &existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	// Check if update is needed by comparing image, config and TLS hashes, and replicas.
	existingImage := existing.Spec.Template.Spec.Containers[0].Image
	desiredImage := desired.Spec.Template.Spec.Containers[0].Image
	existingAnn := existing.Spec.Template.Annotations
	desiredAnn := desired.Spec.Template.Annotations
	hashChanged := existingAnn[configHashAnnotation] != desiredAnn[configHashAnnotation] ||
		existingAnn[tlsHashAnnotation] != desiredAnn[tlsHashAnnotation]

	if existingImage != desiredImage || hashChanged || *existing.Spec.Replicas != *desired.Spec.Replicas {
		existing.Spec.Template = desired.Spec.Template
		existing.Spec.Replicas = desired.Spec.Replicas
		existing.Spec.Strategy = desired.Spec.Strategy
		return r.Update(ctx, &existing)
	}
	return nil
}

// tlsSecretIssued reports whether the cert-manager Secret exists and holds a
// certificate and key.
func (r *WarpgateInstanceReconciler) tlsSecretIssued(ctx context.Context, inst *warpgatev1alpha1.WarpgateInstance) (bool, error) {
	var secret corev1.Secret
	err := r.Get(ctx, types.NamespacedName{Name: certManagerSecretName(inst), Namespace: inst.Namespace}, &secret)
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return len(secret.Data[corev1.TLSCertKey]) > 0 && len(secret.Data[corev1.TLSPrivateKeyKey]) > 0, nil
}

// tlsSecretHash fingerprints the served certificate (public data only) so a
// renewed or rotated TLS Secret rolls the pods. It returns "" when there is no
// TLS Secret or it can't be found yet.
func (r *WarpgateInstanceReconciler) tlsSecretHash(ctx context.Context, inst *warpgatev1alpha1.WarpgateInstance) (string, error) {
	name := servedTLSSecret(inst)
	if name == "" {
		return "", nil
	}
	var secret corev1.Secret
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: inst.Namespace}, &secret)
	if apierrors.IsNotFound(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("getting TLS secret %q: %w", name, err)
	}
	h := fnv.New64a()
	_, _ = h.Write(secret.Data[corev1.TLSCertKey]) // hash writes never fail
	return fmt.Sprintf("%016x", h.Sum64())[:12], nil
}

// ---------------------------------------------------------------------------
// 7. Services — one per enabled protocol (HTTP, SSH)
// ---------------------------------------------------------------------------

func (r *WarpgateInstanceReconciler) ensureServices(ctx context.Context, inst *warpgatev1alpha1.WarpgateInstance) error {
	selector := map[string]string{
		"app.kubernetes.io/name":     "warpgate",
		"app.kubernetes.io/instance": inst.Name,
	}

	// HTTP Service
	if httpEnabled(inst) {
		svcType := corev1.ServiceTypeClusterIP
		if inst.Spec.HTTP != nil && inst.Spec.HTTP.ServiceType != "" {
			svcType = corev1.ServiceType(inst.Spec.HTTP.ServiceType)
		}
		if err := r.ensureService(ctx, inst, inst.Name+"-http", selector,
			corev1.ServicePort{
				Name: "http", Port: instanceHTTPPort(inst),
				TargetPort: intstr.FromString("http"), Protocol: corev1.ProtocolTCP,
			}, svcType); err != nil {
			return err
		}
	} else {
		r.deleteIfExists(ctx, &corev1.Service{}, inst.Namespace, inst.Name+"-http")
	}

	// SSH Service
	if sshEnabled(inst) {
		svcType := corev1.ServiceTypeClusterIP
		if inst.Spec.SSH != nil && inst.Spec.SSH.ServiceType != "" {
			svcType = corev1.ServiceType(inst.Spec.SSH.ServiceType)
		}
		if err := r.ensureService(ctx, inst, inst.Name+"-ssh", selector,
			corev1.ServicePort{
				Name: "ssh", Port: instanceSSHPort(inst),
				TargetPort: intstr.FromString("ssh"), Protocol: corev1.ProtocolTCP,
			}, svcType); err != nil {
			return err
		}
	} else {
		r.deleteIfExists(ctx, &corev1.Service{}, inst.Namespace, inst.Name+"-ssh")
	}

	return nil
}

func (r *WarpgateInstanceReconciler) ensureService(
	ctx context.Context,
	inst *warpgatev1alpha1.WarpgateInstance,
	name string,
	selector map[string]string,
	port corev1.ServicePort,
	svcType corev1.ServiceType,
) error {
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: inst.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type:     svcType,
			Selector: selector,
			Ports:    []corev1.ServicePort{port},
		},
	}
	if err := controllerutil.SetControllerReference(inst, desired, r.Scheme); err != nil {
		return fmt.Errorf("setting owner reference on Service %s: %w", name, err)
	}

	var existing corev1.Service
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: inst.Namespace}, &existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	// Preserve ClusterIP across updates.
	if !equality.Semantic.DeepEqual(existing.Spec.Ports, desired.Spec.Ports) ||
		existing.Spec.Type != desired.Spec.Type ||
		!equality.Semantic.DeepEqual(existing.Spec.Selector, desired.Spec.Selector) {
		existing.Spec.Ports = desired.Spec.Ports
		existing.Spec.Type = desired.Spec.Type
		existing.Spec.Selector = desired.Spec.Selector
		return r.Update(ctx, &existing)
	}
	return nil
}

func (r *WarpgateInstanceReconciler) deleteIfExists(ctx context.Context, obj client.Object, namespace, name string) {
	key := types.NamespacedName{Name: name, Namespace: namespace}
	if err := r.Get(ctx, key, obj); err == nil {
		_ = r.Delete(ctx, obj)
	}
}

// ---------------------------------------------------------------------------
// 8. Cert-manager resources — Issuer + Certificate (unstructured to avoid
//    a hard import dependency on cert-manager types)
// ---------------------------------------------------------------------------

var (
	issuerGVK = schema.GroupVersionKind{
		Group:   "cert-manager.io",
		Version: "v1",
		Kind:    "Issuer",
	}
	certificateGVK = schema.GroupVersionKind{
		Group:   "cert-manager.io",
		Version: "v1",
		Kind:    "Certificate",
	}
)

func (r *WarpgateInstanceReconciler) ensureCertManagerResources(ctx context.Context, inst *warpgatev1alpha1.WarpgateInstance) error {
	// If the user provided an external issuer, skip creating one.
	issuerRefName := inst.Name + "-selfsigned"
	if inst.Spec.TLS != nil && inst.Spec.TLS.IssuerRef != nil && inst.Spec.TLS.IssuerRef.Name != "" {
		issuerRefName = inst.Spec.TLS.IssuerRef.Name
	} else {
		if err := r.ensureSelfSignedIssuer(ctx, inst); err != nil {
			return err
		}
	}

	return r.ensureCertificate(ctx, inst, issuerRefName)
}

func (r *WarpgateInstanceReconciler) ensureSelfSignedIssuer(ctx context.Context, inst *warpgatev1alpha1.WarpgateInstance) error {
	name := inst.Name + "-selfsigned"

	desired := &unstructured.Unstructured{}
	desired.SetGroupVersionKind(issuerGVK)
	desired.SetName(name)
	desired.SetNamespace(inst.Namespace)
	if err := unstructured.SetNestedMap(desired.Object, map[string]any{}, "spec", "selfSigned"); err != nil {
		return err
	}

	if err := controllerutil.SetControllerReference(inst, desired, r.Scheme); err != nil {
		return fmt.Errorf("setting owner reference on Issuer: %w", err)
	}

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(issuerGVK)
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: inst.Namespace}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	return err
}

// certificateDNSNames returns the SANs for the instance's serving Certificate.
// With a user-supplied (public CA) issuer, only public names are included: a
// public CA rejects an order that carries internal Service SANs ("not a valid
// public suffix"). The internal Service SANs are used only for the default
// self-signed issuer.
func certificateDNSNames(inst *warpgatev1alpha1.WarpgateInstance) []string {
	userIssuer := inst.Spec.TLS != nil && inst.Spec.TLS.IssuerRef != nil && inst.Spec.TLS.IssuerRef.Name != ""
	var names []string
	if !userIssuer {
		names = append(names,
			fmt.Sprintf("%s-http.%s.svc", inst.Name, inst.Namespace),
			fmt.Sprintf("%s-http.%s.svc.cluster.local", inst.Name, inst.Namespace),
		)
	}
	if inst.Spec.ExternalHost != "" {
		names = append(names, inst.Spec.ExternalHost)
	}
	return names
}

func (r *WarpgateInstanceReconciler) ensureCertificate(ctx context.Context, inst *warpgatev1alpha1.WarpgateInstance, issuerRefName string) error {
	name := inst.Name + "-tls"

	issuerKind := "Issuer"
	if inst.Spec.TLS != nil && inst.Spec.TLS.IssuerRef != nil && inst.Spec.TLS.IssuerRef.Kind != "" {
		issuerKind = inst.Spec.TLS.IssuerRef.Kind
	}

	names := certificateDNSNames(inst)
	// Nothing public to certify (public issuer but no externalHost): skip the
	// Certificate rather than request an invalid empty-SAN order.
	if len(names) == 0 {
		return nil
	}
	dnsNames := make([]any, len(names))
	for i, n := range names {
		dnsNames[i] = n
	}

	desired := &unstructured.Unstructured{}
	desired.SetGroupVersionKind(certificateGVK)
	desired.SetName(name)
	desired.SetNamespace(inst.Namespace)

	spec := map[string]any{
		"secretName": certManagerSecretName(inst),
		"issuerRef": map[string]any{
			"name": issuerRefName,
			"kind": issuerKind,
		},
		"dnsNames": dnsNames,
	}
	if err := unstructured.SetNestedField(desired.Object, spec, "spec"); err != nil {
		return err
	}

	if err := controllerutil.SetControllerReference(inst, desired, r.Scheme); err != nil {
		return fmt.Errorf("setting owner reference on Certificate: %w", err)
	}

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(certificateGVK)
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: inst.Namespace}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	return err
}

// ---------------------------------------------------------------------------
// 9. WarpgateConnection CR
// ---------------------------------------------------------------------------

func (r *WarpgateInstanceReconciler) ensureWarpgateConnection(ctx context.Context, inst *warpgatev1alpha1.WarpgateInstance) error {
	connName := inst.Name + "-connection"

	// Build an auth Secret from the admin password secret ref.
	authSecretName := inst.Name + "-admin-auth"
	authSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      authSecretName,
			Namespace: inst.Namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, authSecret, func() error {
		if err := controllerutil.SetControllerReference(inst, authSecret, r.Scheme); err != nil {
			return err
		}

		var pwSecret corev1.Secret
		if err := r.Get(ctx, types.NamespacedName{
			Name: inst.Spec.AdminPasswordSecretRef.Name, Namespace: inst.Namespace,
		}, &pwSecret); err != nil {
			return fmt.Errorf("getting admin password secret %q: %w", inst.Spec.AdminPasswordSecretRef.Name, err)
		}

		key := inst.Spec.AdminPasswordSecretRef.Key
		if key == "" {
			key = "password"
		}

		pw, ok := pwSecret.Data[key]
		if !ok {
			return fmt.Errorf("key %q not found in admin password secret %q", key, inst.Spec.AdminPasswordSecretRef.Name)
		}

		authSecret.Data = map[string][]byte{
			"username": []byte("admin"),
			"password": pw,
		}
		return nil
	})
	if err != nil {
		return err
	}

	insecure, caRef, err := r.connectionTLS(ctx, inst)
	if err != nil {
		return err
	}

	// Create/update the WarpgateConnection CR.
	host := fmt.Sprintf("https://%s-http.%s.svc:%d",
		inst.Name, inst.Namespace, instanceHTTPPort(inst))

	conn := &warpgatev1alpha1.WarpgateConnection{
		ObjectMeta: metav1.ObjectMeta{
			Name:      connName,
			Namespace: inst.Namespace,
		},
	}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, conn, func() error {
		if err := controllerutil.SetControllerReference(inst, conn, r.Scheme); err != nil {
			return err
		}
		conn.Spec = warpgatev1alpha1.WarpgateConnectionSpec{
			Host:               host,
			HostHeader:         inst.Spec.ExternalHost,
			AuthSecretRef:      warpgatev1alpha1.AuthSecretRef{Name: authSecretName},
			InsecureSkipVerify: insecure,
			CASecretRef:        caRef,
		}
		return nil
	})
	if err != nil {
		return err
	}

	inst.Status.ConnectionRef = connName
	return nil
}

// connectionTLS decides how the auto-created WarpgateConnection verifies the
// instance. With tls.secretName or cert-manager the operator knows the served
// certificate and verifies against its Secret (ca.crt, falling back to
// tls.crt). Otherwise the pod serves a self-signed certificate generated at
// startup that the operator never sees, so verification is skipped unless
// tls.verifyConnection demands it.
func (r *WarpgateInstanceReconciler) connectionTLS(
	ctx context.Context, inst *warpgatev1alpha1.WarpgateInstance,
) (insecure bool, caRef *warpgatev1alpha1.SecretKeyRef, err error) {
	var verify *bool
	if inst.Spec.TLS != nil {
		verify = inst.Spec.TLS.VerifyConnection
	}
	if verify != nil && !*verify {
		return true, nil, nil
	}

	if name := servedTLSSecret(inst); name != "" {
		var secret corev1.Secret
		if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: inst.Namespace}, &secret); err != nil {
			return false, nil, fmt.Errorf("getting TLS secret %q for connection verification: %w", name, err)
		}
		for _, key := range []string{"ca.crt", corev1.TLSCertKey} {
			if len(secret.Data[key]) > 0 {
				return false, &warpgatev1alpha1.SecretKeyRef{Name: name, Key: key}, nil
			}
		}
		return false, nil, fmt.Errorf("TLS secret %q has neither ca.crt nor tls.crt to verify the connection against", name)
	}

	if verify != nil && *verify {
		return false, nil, fmt.Errorf("tls.verifyConnection is true but there is no CA to verify against; enable tls.certManager or set tls.secretName")
	}
	// Self-signed certificate generated inside the pod.
	return true, nil, nil
}

// ---------------------------------------------------------------------------
// 10. Status update
// ---------------------------------------------------------------------------

func (r *WarpgateInstanceReconciler) refreshStatus(ctx context.Context, inst *warpgatev1alpha1.WarpgateInstance) {
	var deploy appsv1.Deployment
	if err := r.Get(ctx, types.NamespacedName{
		Name: inst.Name, Namespace: inst.Namespace,
	}, &deploy); err == nil {
		inst.Status.ReadyReplicas = deploy.Status.ReadyReplicas
	} else {
		inst.Status.ReadyReplicas = 0
	}

	inst.Status.Version = inst.Spec.Version

	if httpEnabled(inst) {
		inst.Status.Endpoint = fmt.Sprintf("https://%s-http.%s.svc:%d",
			inst.Name, inst.Namespace, instanceHTTPPort(inst))
	}
}

func (r *WarpgateInstanceReconciler) setCondition(inst *warpgatev1alpha1.WarpgateInstance, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&inst.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             status,
		ObservedGeneration: inst.Generation,
		Reason:             reason,
		Message:            message,
	})
}

// ---------------------------------------------------------------------------
// SetupWithManager
// ---------------------------------------------------------------------------

// SetupWithManager sets up the controller with the Manager.
// It watches Deployments and Services owned by WarpgateInstance.
func (r *WarpgateInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&warpgatev1alpha1.WarpgateInstance{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&warpgatev1alpha1.WarpgateConnection{}).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(r.instancesForTLSSecret)).
		Named("warpgateinstance").
		Complete(r)
}

// instancesForTLSSecret maps a Secret to the instances that serve it, so an
// instance waiting on cert-manager reconciles as soon as the Secret is issued
// and a renewed certificate rolls the pods.
func (r *WarpgateInstanceReconciler) instancesForTLSSecret(ctx context.Context, obj client.Object) []reconcile.Request {
	var list warpgatev1alpha1.WarpgateInstanceList
	if err := r.List(ctx, &list, client.InNamespace(obj.GetNamespace())); err != nil {
		return nil
	}
	var reqs []reconcile.Request
	for i := range list.Items {
		if servedTLSSecret(&list.Items[i]) == obj.GetName() {
			reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&list.Items[i])})
		}
	}
	return reqs
}
