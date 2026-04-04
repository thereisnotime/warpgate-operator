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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WarpgateInstanceSpec defines the desired state of a Warpgate deployment.
type WarpgateInstanceSpec struct {
	// version is the Warpgate image tag to deploy (e.g. "0.21.1", "latest").
	// +required
	Version string `json:"version"`

	// image overrides the full image reference. Defaults to ghcr.io/warp-tech/warpgate:<version>.
	// +optional
	Image string `json:"image,omitempty"`

	// replicas is the number of Warpgate pods. Defaults to 1.
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	Replicas *int32 `json:"replicas,omitempty"`

	// adminPasswordSecretRef references a Secret containing the initial admin password.
	// +required
	AdminPasswordSecretRef SecretKeyRef `json:"adminPasswordSecretRef"`

	// http configures the HTTP/HTTPS protocol listener.
	// +optional
	HTTP *HTTPListenerSpec `json:"http,omitempty"`

	// ssh configures the SSH protocol listener.
	// +optional
	SSH *SSHListenerSpec `json:"ssh,omitempty"`

	// kubernetes configures the Kubernetes protocol proxy listener.
	// +optional
	Kubernetes *ProtocolListenerSpec `json:"kubernetes,omitempty"`

	// mysql configures the MySQL protocol proxy listener.
	// +optional
	MySQL *ProtocolListenerSpec `json:"mysql,omitempty"`

	// postgresql configures the PostgreSQL protocol proxy listener.
	// +optional
	PostgreSQL *ProtocolListenerSpec `json:"postgresql,omitempty"`

	// storage configures persistent storage for Warpgate data.
	// +optional
	Storage *StorageSpec `json:"storage,omitempty"`

	// tls configures TLS certificate provisioning.
	// +optional
	TLS *InstanceTLSSpec `json:"tls,omitempty"`

	// sshKeysSecretName references a Secret containing SSH host/client keys.
	// Expected keys: client-ed25519, client-rsa, host-ed25519, host-rsa.
	// If not set, Warpgate generates its own keys on first boot.
	// +optional
	SSHKeysSecretName string `json:"sshKeysSecretName,omitempty"`

	// configOverride allows providing a complete warpgate.yaml config that replaces
	// the auto-generated one. Mounted as a ConfigMap.
	// +optional
	ConfigOverride string `json:"configOverride,omitempty"`

	// databaseURL overrides the default SQLite database with a PostgreSQL connection string.
	// Example: "postgres://user:pass@host:5432/warpgate"
	// +optional
	DatabaseURL string `json:"databaseURL,omitempty"`

	// recordSessions enables session recording. Defaults to false.
	// +optional
	RecordSessions *bool `json:"recordSessions,omitempty"`

	// strategy overrides the default deployment update strategy.
	// Use "Recreate" when using RWO persistent volumes (default).
	// +optional
	// +kubebuilder:default="Recreate"
	// +kubebuilder:validation:Enum=Recreate;RollingUpdate
	Strategy string `json:"strategy,omitempty"`

	// ingress configures an Ingress resource for the HTTP listener.
	// +optional
	Ingress *IngressSpec `json:"ingress,omitempty"`

	// resources defines CPU/memory requests and limits for the Warpgate container.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// nodeSelector constrains which nodes the pod can be scheduled on.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// tolerations for scheduling.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// createConnection when true (default) auto-creates a WarpgateConnection CR
	// pointing to this instance, so other CRDs can reference it.
	// +optional
	// +kubebuilder:default=true
	CreateConnection *bool `json:"createConnection,omitempty"`

	// externalHost sets the external hostname for Warpgate cookie domain and URL generation.
	// +optional
	ExternalHost string `json:"externalHost,omitempty"`
}

// HTTPListenerSpec configures the HTTP/HTTPS protocol listener.
type HTTPListenerSpec struct {
	// enabled enables the HTTP listener. Defaults to true.
	// +optional
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty"`
	// port is the container port. Defaults to 8888.
	// +optional
	// +kubebuilder:default=8888
	Port *int32 `json:"port,omitempty"`
	// serviceType is the Kubernetes Service type (ClusterIP, NodePort, LoadBalancer). Defaults to ClusterIP.
	// +optional
	// +kubebuilder:default="ClusterIP"
	// +kubebuilder:validation:Enum=ClusterIP;NodePort;LoadBalancer
	ServiceType string `json:"serviceType,omitempty"`
}

// SSHListenerSpec configures the SSH protocol listener.
type SSHListenerSpec struct {
	// enabled enables the SSH listener.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
	// port is the container port. Defaults to 2222.
	// +optional
	// +kubebuilder:default=2222
	Port *int32 `json:"port,omitempty"`
	// serviceType for the SSH service. Defaults to ClusterIP.
	// +optional
	// +kubebuilder:default="ClusterIP"
	// +kubebuilder:validation:Enum=ClusterIP;NodePort;LoadBalancer
	ServiceType string `json:"serviceType,omitempty"`
}

// ProtocolListenerSpec configures a generic protocol proxy listener (MySQL, PostgreSQL).
type ProtocolListenerSpec struct {
	// enabled enables this protocol listener.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
	// port is the container port.
	// +optional
	Port *int32 `json:"port,omitempty"`
}

// StorageSpec configures persistent storage for Warpgate data.
type StorageSpec struct {
	// enabled enables persistent storage via PVC. When false, uses emptyDir.
	// +optional
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty"`
	// size is the PVC size. Defaults to 1Gi.
	// +optional
	// +kubebuilder:default="1Gi"
	Size string `json:"size,omitempty"`
	// storageClassName overrides the default StorageClass.
	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`
	// existingClaimName uses an existing PVC instead of creating one.
	// +optional
	ExistingClaimName string `json:"existingClaimName,omitempty"`
}

// InstanceTLSSpec configures TLS certificate provisioning.
type InstanceTLSSpec struct {
	// certManager enables automatic TLS cert provisioning via cert-manager.
	// +optional
	CertManager *bool `json:"certManager,omitempty"`
	// issuerRef references a custom cert-manager issuer.
	// +optional
	IssuerRef *CertIssuerRef `json:"issuerRef,omitempty"`
	// secretName references an existing TLS Secret (with tls.crt and tls.key).
	// If set, cert-manager is not used and the secret is mounted directly.
	// +optional
	SecretName string `json:"secretName,omitempty"`
}

// CertIssuerRef references a cert-manager Issuer or ClusterIssuer.
type CertIssuerRef struct {
	// name is the name of the issuer.
	// +required
	Name string `json:"name"`
	// kind is the issuer kind — Issuer or ClusterIssuer.
	// +optional
	Kind string `json:"kind,omitempty"`
}

// IngressSpec configures an Ingress resource for the HTTP listener.
type IngressSpec struct {
	Enabled     bool              `json:"enabled,omitempty"`
	ClassName   string            `json:"className,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Hosts       []IngressHost     `json:"hosts,omitempty"`
	TLS         []IngressTLS      `json:"tls,omitempty"`
}

// IngressHost defines a host entry for an Ingress resource.
type IngressHost struct {
	Host  string        `json:"host"`
	Paths []IngressPath `json:"paths,omitempty"`
}

// IngressPath defines a path entry for an Ingress host.
type IngressPath struct {
	Path     string `json:"path"`
	PathType string `json:"pathType,omitempty"`
}

// IngressTLS defines a TLS entry for an Ingress resource.
type IngressTLS struct {
	SecretName string   `json:"secretName,omitempty"`
	Hosts      []string `json:"hosts,omitempty"`
}

// WarpgateInstanceStatus defines the observed state of WarpgateInstance.
type WarpgateInstanceStatus struct {
	// readyReplicas is the number of ready pods.
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`
	// version is the currently deployed version.
	Version string `json:"version,omitempty"`
	// connectionRef is the name of the auto-created WarpgateConnection CR.
	ConnectionRef string `json:"connectionRef,omitempty"`
	// endpoint is the internal service URL for the Warpgate API.
	Endpoint string `json:"endpoint,omitempty"`
	// conditions represent the current state of the WarpgateInstance resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.readyReplicas
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.spec.replicas`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyReplicas`
// +kubebuilder:printcolumn:name="Endpoint",type=string,JSONPath=`.status.endpoint`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// WarpgateInstance is the Schema for the warpgateinstances API
type WarpgateInstance struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of WarpgateInstance
	// +required
	Spec WarpgateInstanceSpec `json:"spec"`

	// status defines the observed state of WarpgateInstance
	// +optional
	Status WarpgateInstanceStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// WarpgateInstanceList contains a list of WarpgateInstance
type WarpgateInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []WarpgateInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WarpgateInstance{}, &WarpgateInstanceList{})
}
