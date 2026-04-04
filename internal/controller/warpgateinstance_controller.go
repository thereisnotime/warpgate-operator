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
	"crypto/sha256"
	"fmt"
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
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	warpgatev1alpha1 "github.com/thereisnotime/warpgate-operator/api/v1alpha1"
)

const instanceFinalizer = "warpgate.warp.tech/instance-finalizer"

// WarpgateInstanceReconciler reconciles a WarpgateInstance object.
type WarpgateInstanceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
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

	// 1. Get CR, IgnoreNotFound.
	var inst warpgatev1alpha1.WarpgateInstance
	if err := r.Get(ctx, req.NamespacedName, &inst); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 2. Handle deletion with finalizer.
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

	// 3. Add finalizer if missing.
	if !controllerutil.ContainsFinalizer(&inst, instanceFinalizer) {
		controllerutil.AddFinalizer(&inst, instanceFinalizer)
		if err := r.Update(ctx, &inst); err != nil {
			return ctrl.Result{}, err
		}
	}

	// 4. Ensure ConfigMap (operator-generated config + optional config override).
	if err := r.ensureConfigMap(ctx, &inst); err != nil {
		log.Error(err, "failed to ensure ConfigMap")
		r.setCondition(&inst, metav1.ConditionFalse, "ConfigMapFailed", err.Error())
		_ = r.Status().Update(ctx, &inst)
		return ctrl.Result{}, err
	}

	// 4b. Ensure config override ConfigMap if needed.
	if inst.Spec.ConfigOverride != "" {
		if err := r.ensureConfigOverrideConfigMap(ctx, &inst); err != nil {
			log.Error(err, "failed to ensure config override ConfigMap")
			r.setCondition(&inst, metav1.ConditionFalse, "ConfigOverrideFailed", err.Error())
			_ = r.Status().Update(ctx, &inst)
			return ctrl.Result{}, err
		}
	}

	// 5. Ensure PVC (create only, never update) — only when storage is enabled
	//    and no existing claim is referenced.
	if storageEnabled(&inst) && !hasExistingClaim(&inst) {
		if err := r.ensurePVC(ctx, &inst); err != nil {
			log.Error(err, "failed to ensure PVC")
			r.setCondition(&inst, metav1.ConditionFalse, "PVCFailed", err.Error())
			_ = r.Status().Update(ctx, &inst)
			return ctrl.Result{}, err
		}
	}

	// 6. Ensure Deployment (create/update).
	if err := r.ensureDeployment(ctx, &inst); err != nil {
		log.Error(err, "failed to ensure Deployment")
		r.setCondition(&inst, metav1.ConditionFalse, "DeploymentFailed", err.Error())
		_ = r.Status().Update(ctx, &inst)
		return ctrl.Result{}, err
	}

	// 7. Ensure Service(s).
	if err := r.ensureServices(ctx, &inst); err != nil {
		log.Error(err, "failed to ensure Services")
		r.setCondition(&inst, metav1.ConditionFalse, "ServiceFailed", err.Error())
		_ = r.Status().Update(ctx, &inst)
		return ctrl.Result{}, err
	}

	// 8. If cert-manager enabled, ensure Issuer + Certificate.
	if certManagerEnabled(&inst) {
		if err := r.ensureCertManagerResources(ctx, &inst); err != nil {
			log.Error(err, "failed to ensure cert-manager resources")
			r.setCondition(&inst, metav1.ConditionFalse, "CertManagerFailed", err.Error())
			_ = r.Status().Update(ctx, &inst)
			return ctrl.Result{}, err
		}
	}

	// 9. If createConnection, ensure WarpgateConnection CR.
	if shouldCreateConnection(&inst) {
		if err := r.ensureWarpgateConnection(ctx, &inst); err != nil {
			log.Error(err, "failed to ensure WarpgateConnection")
			r.setCondition(&inst, metav1.ConditionFalse, "ConnectionFailed", err.Error())
			_ = r.Status().Update(ctx, &inst)
			return ctrl.Result{}, err
		}
	}

	// 10. Update status from Deployment.
	r.refreshStatus(ctx, &inst)

	// 11. Set Ready condition.
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
	tag := inst.Spec.Version
	if !strings.HasPrefix(tag, "v") {
		tag = "v" + tag
	}
	return fmt.Sprintf("ghcr.io/warp-tech/warpgate:%s", tag)
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
	h.Write([]byte(inst.Spec.ConfigOverride))
	h.Write([]byte(inst.Spec.DatabaseURL))
	h.Write([]byte(inst.Spec.SSHKeysSecretName))
	return fmt.Sprintf("%x", h.Sum(nil))[:12]
}

// ---------------------------------------------------------------------------
// 4. ConfigMap — warpgate.yaml
// ---------------------------------------------------------------------------

func (r *WarpgateInstanceReconciler) buildWarpgateConfig(inst *warpgatev1alpha1.WarpgateInstance) string {
	var b strings.Builder

	b.WriteString("store:\n")
	if inst.Spec.DatabaseURL != "" {
		fmt.Fprintf(&b, "  database_url:\n")
		fmt.Fprintf(&b, "    postgres: \"%s\"\n", inst.Spec.DatabaseURL)
	} else {
		b.WriteString("  database_url:\n")
		b.WriteString("    sqlite:\n")
		b.WriteString("      path: /data/db\n")
	}

	// HTTP
	b.WriteString("http:\n")
	if httpEnabled(inst) {
		b.WriteString("  enable: true\n")
	} else {
		b.WriteString("  enable: false\n")
	}
	fmt.Fprintf(&b, "  listen: \"0.0.0.0:%d\"\n", instanceHTTPPort(inst))
	b.WriteString("  certificate: /data/tls.certificate.pem\n")
	b.WriteString("  key: /data/tls.key.pem\n")

	// SSH
	b.WriteString("ssh:\n")
	if sshEnabled(inst) {
		b.WriteString("  enable: true\n")
	} else {
		b.WriteString("  enable: false\n")
	}
	fmt.Fprintf(&b, "  listen: \"0.0.0.0:%d\"\n", instanceSSHPort(inst))

	// MySQL
	if mysqlEnabled(inst) {
		b.WriteString("mysql:\n")
		b.WriteString("  enable: true\n")
		fmt.Fprintf(&b, "  listen: \"0.0.0.0:%d\"\n", instanceMySQLPort(inst))
	}

	// PostgreSQL
	if pgEnabled(inst) {
		b.WriteString("postgres:\n")
		b.WriteString("  enable: true\n")
		fmt.Fprintf(&b, "  listen: \"0.0.0.0:%d\"\n", instancePGPort(inst))
	}

	// Kubernetes
	if kubernetesEnabled(inst) {
		b.WriteString("kubernetes:\n")
		b.WriteString("  enable: true\n")
		fmt.Fprintf(&b, "  listen: \"0.0.0.0:%d\"\n", instanceKubernetesPort(inst))
	}

	// External host
	if inst.Spec.ExternalHost != "" {
		fmt.Fprintf(&b, "external_host: %s\n", inst.Spec.ExternalHost)
	}

	// Session recording
	if inst.Spec.RecordSessions != nil && *inst.Spec.RecordSessions {
		b.WriteString("recordings:\n")
		b.WriteString("  enable: true\n")
		b.WriteString("  path: /data/recordings\n")
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
	// 2. Copy TLS certs from secret mount (tls.crt -> tls.certificate.pem, tls.key -> tls.key.pem)
	// 3. Run unattended-setup if /data/warpgate.yaml doesn't exist
	// 4. Copy config override if provided
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

	// 2. Copy TLS certs from secret mount if provided
	if tlsSecretProvided(inst) {
		scriptParts = append(scriptParts,
			`echo "Copying TLS certificates from secret..."`,
			`if [ -f /tls-secret/tls.crt ]; then`,
			`  cp /tls-secret/tls.crt /data/tls.certificate.pem`,
			`  cp /tls-secret/tls.key /data/tls.key.pem`,
			`  chmod 600 /data/tls.certificate.pem /data/tls.key.pem`,
			`fi`,
		)
	}

	// 3. Run unattended-setup if warpgate.yaml doesn't exist
	setupCmd := fmt.Sprintf(
		`warpgate --skip-securing-files unattended-setup --data-path /data --http-port %d --admin-password "${ADMIN_PASSWORD}"`,
		instanceHTTPPort(inst),
	)
	if kubernetesEnabled(inst) {
		setupCmd += fmt.Sprintf(` --kubernetes-port %d`, instanceKubernetesPort(inst))
	}
	if inst.Spec.DatabaseURL != "" {
		setupCmd += fmt.Sprintf(` --database-url "%s"`, inst.Spec.DatabaseURL)
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

	// 4. Copy config override if provided
	if inst.Spec.ConfigOverride != "" {
		scriptParts = append(scriptParts,
			`echo "Applying config override..."`,
			`cp /override/warpgate.yaml /data/warpgate.yaml`,
		)
	} else {
		// Apply the operator-generated config
		scriptParts = append(scriptParts,
			`echo "Applying operator config..."`,
			`cp /config/warpgate.yaml /data/warpgate.yaml`,
		)
	}

	// 5. Generate self-signed TLS if no certs exist yet (and no TLS secret / cert-manager)
	if !tlsSecretProvided(inst) && !certManagerEnabled(inst) {
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
	if tlsSecretProvided(inst) {
		initVolumeMounts = append(initVolumeMounts, corev1.VolumeMount{
			Name: "tls-secret", MountPath: "/tls-secret", ReadOnly: true,
		})
	}
	if inst.Spec.ConfigOverride != "" {
		initVolumeMounts = append(initVolumeMounts, corev1.VolumeMount{
			Name: "config-override", MountPath: "/override", ReadOnly: true,
		})
	}

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

	hash := configHash(inst)

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

	// Optional TLS secret volume (spec.tls.secretName)
	if tlsSecretProvided(inst) {
		volumes = append(volumes, corev1.Volume{
			Name: "tls-secret",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: inst.Spec.TLS.SecretName,
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
						"warpgate.warp.tech/config-hash": hash,
					},
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name:         "init-setup",
							Image:        image,
							Command:      []string{"/bin/sh", "-c", initScript},
							VolumeMounts: initVolumeMounts,
							Env: []corev1.EnvVar{
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
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "warpgate",
							Image: image,
							Args:  []string{"--config", "/data/warpgate.yaml", "run", "--skip-securing-files"},
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

func (r *WarpgateInstanceReconciler) ensureDeployment(ctx context.Context, inst *warpgatev1alpha1.WarpgateInstance) error {
	desired := r.buildDeployment(inst)
	if err := controllerutil.SetControllerReference(inst, desired, r.Scheme); err != nil {
		return fmt.Errorf("setting owner reference on Deployment: %w", err)
	}

	var existing appsv1.Deployment
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, &existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	// Check if update is needed by comparing image, config hash, and replicas.
	existingImage := existing.Spec.Template.Spec.Containers[0].Image
	desiredImage := desired.Spec.Template.Spec.Containers[0].Image
	existingHash := existing.Spec.Template.Annotations["warpgate.warp.tech/config-hash"]
	desiredHash := desired.Spec.Template.Annotations["warpgate.warp.tech/config-hash"]

	if existingImage != desiredImage || existingHash != desiredHash || *existing.Spec.Replicas != *desired.Spec.Replicas {
		existing.Spec.Template = desired.Spec.Template
		existing.Spec.Replicas = desired.Spec.Replicas
		existing.Spec.Strategy = desired.Spec.Strategy
		return r.Update(ctx, &existing)
	}
	return nil
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

func (r *WarpgateInstanceReconciler) ensureCertificate(ctx context.Context, inst *warpgatev1alpha1.WarpgateInstance, issuerRefName string) error {
	name := inst.Name + "-tls"

	issuerKind := "Issuer"
	if inst.Spec.TLS != nil && inst.Spec.TLS.IssuerRef != nil && inst.Spec.TLS.IssuerRef.Kind != "" {
		issuerKind = inst.Spec.TLS.IssuerRef.Kind
	}

	dnsNames := []any{
		fmt.Sprintf("%s-http.%s.svc", inst.Name, inst.Namespace),
		fmt.Sprintf("%s-http.%s.svc.cluster.local", inst.Name, inst.Namespace),
	}
	if inst.Spec.ExternalHost != "" {
		dnsNames = append(dnsNames, inst.Spec.ExternalHost)
	}

	desired := &unstructured.Unstructured{}
	desired.SetGroupVersionKind(certificateGVK)
	desired.SetName(name)
	desired.SetNamespace(inst.Namespace)

	spec := map[string]any{
		"secretName": inst.Name + "-tls",
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
			AuthSecretRef:      warpgatev1alpha1.AuthSecretRef{Name: authSecretName},
			InsecureSkipVerify: true, // self-signed cert within cluster
		}
		return nil
	})
	if err != nil {
		return err
	}

	inst.Status.ConnectionRef = connName
	return nil
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
		Named("warpgateinstance").
		Complete(r)
}
