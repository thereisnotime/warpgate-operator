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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TLSConfigSpec defines TLS settings for a target connection.
type TLSConfigSpec struct {
	// mode is the TLS mode: Disabled, Preferred, or Required.
	// +kubebuilder:validation:Enum=Disabled;Preferred;Required
	Mode string `json:"mode"`
	// verify enables TLS certificate verification.
	Verify bool `json:"verify,omitempty"`
}

// SSHTargetSpec defines the configuration for an SSH target.
type SSHTargetSpec struct {
	// host is the hostname or IP of the SSH target.
	Host string `json:"host"`
	// port is the SSH port.
	Port int `json:"port"`
	// username is the SSH username.
	Username string `json:"username"`
	// authKind is the SSH authentication method.
	// +kubebuilder:validation:Enum=Password;PublicKey;IamRole
	AuthKind string `json:"authKind"`
	// passwordSecretRef references a Secret containing the SSH password (for Password auth).
	// +optional
	PasswordSecretRef *SecretKeyRef `json:"passwordSecretRef,omitempty"`
	// keyID is the UUID of a stored Warpgate SSH client key to authenticate with (for PublicKey auth).
	// Empty uses the keys marked default in Warpgate.
	// +optional
	KeyID string `json:"keyID,omitempty"`
	// allowInsecureAlgos permits the use of insecure SSH algorithms.
	AllowInsecureAlgos bool `json:"allowInsecureAlgos,omitempty"`
	// jumpHostRef is the name of another WarpgateTarget (in the same namespace) to use as an SSH jump host.
	// The referenced target must be an SSH target and must already be synced (have an ExternalID).
	// +optional
	JumpHostRef string `json:"jumpHostRef,omitempty"`
}

// HTTPTargetSpec defines the configuration for an HTTP target.
type HTTPTargetSpec struct {
	// url is the upstream URL of the HTTP target.
	URL string `json:"url"`
	// tls configures TLS settings for the HTTP connection.
	// +optional
	TLS *TLSConfigSpec `json:"tls,omitempty"`
	// headers are additional HTTP headers sent to the upstream.
	// +optional
	Headers map[string]string `json:"headers,omitempty"`
	// externalHost overrides the Host header sent to the upstream.
	// +optional
	ExternalHost string `json:"externalHost,omitempty"`
}

// MySQLTargetSpec defines the configuration for a MySQL target.
type MySQLTargetSpec struct {
	// host is the hostname or IP of the MySQL server.
	Host string `json:"host"`
	// port is the MySQL port.
	Port int `json:"port"`
	// username is the MySQL username.
	Username string `json:"username"`
	// authKind is the database authentication method. Defaults to Password.
	// +kubebuilder:validation:Enum=Password;IamRole
	// +optional
	AuthKind string `json:"authKind,omitempty"`
	// passwordSecretRef references a Secret containing the MySQL password.
	// +optional
	PasswordSecretRef *SecretKeyRef `json:"passwordSecretRef,omitempty"`
	// tls configures TLS settings for the MySQL connection.
	// +optional
	TLS *TLSConfigSpec `json:"tls,omitempty"`
	// defaultDatabaseName is the database shown in connection instructions.
	// +optional
	DefaultDatabaseName string `json:"defaultDatabaseName,omitempty"`
}

// PostgreSQLTargetSpec defines the configuration for a PostgreSQL target.
type PostgreSQLTargetSpec struct {
	// host is the hostname or IP of the PostgreSQL server.
	Host string `json:"host"`
	// port is the PostgreSQL port.
	Port int `json:"port"`
	// username is the PostgreSQL username.
	Username string `json:"username"`
	// protocolVersion is the PostgreSQL protocol version used for the target connection. Defaults to 3.2.
	// +kubebuilder:validation:Pattern=`^3\.(0|2)$`
	// +optional
	ProtocolVersion string `json:"protocolVersion,omitempty"`
	// authKind is the database authentication method. Defaults to Password.
	// +kubebuilder:validation:Enum=Password;IamRole
	// +optional
	AuthKind string `json:"authKind,omitempty"`
	// passwordSecretRef references a Secret containing the PostgreSQL password.
	// +optional
	PasswordSecretRef *SecretKeyRef `json:"passwordSecretRef,omitempty"`
	// tls configures TLS settings for the PostgreSQL connection.
	// +optional
	TLS *TLSConfigSpec `json:"tls,omitempty"`
	// idleTimeout closes idle target connections after this duration (e.g. "5m").
	// +optional
	IdleTimeout string `json:"idleTimeout,omitempty"`
	// defaultDatabaseName is the database shown in connection instructions.
	// +optional
	DefaultDatabaseName string `json:"defaultDatabaseName,omitempty"`
}

// KubernetesTargetSpec defines the configuration for a Kubernetes target.
type KubernetesTargetSpec struct {
	// clusterURL is the Kubernetes API server URL.
	ClusterURL string `json:"clusterURL"`
	// authKind is the cluster authentication method.
	// +kubebuilder:validation:Enum=Token;Certificate;IamRole
	AuthKind string `json:"authKind"`
	// tokenSecretRef references a Secret containing the bearer token (for Token auth).
	// +optional
	TokenSecretRef *SecretKeyRef `json:"tokenSecretRef,omitempty"`
	// certificateSecretRef references a Secret containing the PEM client certificate (for Certificate auth).
	// +optional
	CertificateSecretRef *SecretKeyRef `json:"certificateSecretRef,omitempty"`
	// privateKeySecretRef references a Secret containing the PEM client private key (for Certificate auth).
	// +optional
	PrivateKeySecretRef *SecretKeyRef `json:"privateKeySecretRef,omitempty"`
	// tls configures TLS settings for the API server connection.
	// +optional
	TLS *TLSConfigSpec `json:"tls,omitempty"`
}

// RDPTargetSpec defines the configuration for an RDP target.
type RDPTargetSpec struct {
	// host is the hostname or IP of the RDP server.
	Host string `json:"host"`
	// port is the RDP port.
	Port int `json:"port"`
	// username is the RDP username.
	Username string `json:"username"`
	// domain is the Windows logon domain.
	// +optional
	Domain string `json:"domain,omitempty"`
	// passwordSecretRef references a Secret containing the RDP password.
	// +optional
	PasswordSecretRef *SecretKeyRef `json:"passwordSecretRef,omitempty"`
	// verifyTLS verifies the RDP server certificate against the system root store.
	// +optional
	VerifyTLS bool `json:"verifyTLS,omitempty"`
	// compression is the codec: remotefx (default) or lossless.
	// +kubebuilder:validation:Enum=remotefx;lossless
	// +optional
	Compression string `json:"compression,omitempty"`
	// interactiveLogon shows the target's own sign-in screen instead of logging on automatically.
	// +optional
	InteractiveLogon bool `json:"interactiveLogon,omitempty"`
	// tlsSecurity is the TLS profile for the target connection. Defaults to Tls12.
	// +kubebuilder:validation:Enum=Tls12;Tls12WithLegacyCiphers;Tls10Unsafe
	// +optional
	TLSSecurity string `json:"tlsSecurity,omitempty"`
}

// VNCTargetSpec defines the configuration for a VNC target.
type VNCTargetSpec struct {
	// host is the hostname or IP of the VNC server.
	Host string `json:"host"`
	// port is the VNC port.
	Port int `json:"port"`
	// passwordSecretRef references a Secret containing the VNC password. Omit for no authentication.
	// +optional
	PasswordSecretRef *SecretKeyRef `json:"passwordSecretRef,omitempty"`
}

// WarpgateTargetSpec defines the desired state of WarpgateTarget.
type WarpgateTargetSpec struct {
	// connectionRef is the name of the WarpgateConnection to use.
	// +required
	ConnectionRef string `json:"connectionRef"`
	// name is the target name in Warpgate.
	// +required
	Name string `json:"name"`
	// description is a human-readable description of the target.
	// +optional
	Description string `json:"description,omitempty"`
	// groupRef is the name of a WarpgateTargetGroup CR in the same namespace.
	// The referenced group must already be synced (have an ExternalID).
	// +optional
	GroupRef string `json:"groupRef,omitempty"`
	// rateLimitBytesPerSecond limits transfer speed in bytes per second. Zero means unlimited.
	// +optional
	RateLimitBytesPerSecond *int64 `json:"rateLimitBytesPerSecond,omitempty"`
	// ticketMaxDurationSeconds limits how long a ticket session can last. Zero means unlimited.
	// +optional
	TicketMaxDurationSeconds *int64 `json:"ticketMaxDurationSeconds,omitempty"`
	// ticketRequestsDisabled disables ticket requests for this target.
	// +optional
	TicketRequestsDisabled *bool `json:"ticketRequestsDisabled,omitempty"`
	// ticketRequireApproval requires admin approval of ticket requests for this target.
	// +optional
	TicketRequireApproval *bool `json:"ticketRequireApproval,omitempty"`
	// requireApproval requires admin approval before any session to this target starts.
	// +optional
	RequireApproval *bool `json:"requireApproval,omitempty"`
	// ticketMaxUses limits how many times a ticket can be used. Zero means unlimited.
	// +optional
	TicketMaxUses *int64 `json:"ticketMaxUses,omitempty"`
	// ssh configures an SSH target. Exactly one target type must be set.
	// +optional
	SSH *SSHTargetSpec `json:"ssh,omitempty"`
	// http configures an HTTP target. Exactly one target type must be set.
	// +optional
	HTTP *HTTPTargetSpec `json:"http,omitempty"`
	// mysql configures a MySQL target. Exactly one target type must be set.
	// +optional
	MySQL *MySQLTargetSpec `json:"mysql,omitempty"`
	// postgresql configures a PostgreSQL target. Exactly one target type must be set.
	// +optional
	PostgreSQL *PostgreSQLTargetSpec `json:"postgresql,omitempty"`
	// kubernetes configures a Kubernetes target. Exactly one target type must be set.
	// +optional
	Kubernetes *KubernetesTargetSpec `json:"kubernetes,omitempty"`
	// rdp configures an RDP target. Exactly one target type must be set.
	// +optional
	RDP *RDPTargetSpec `json:"rdp,omitempty"`
	// vnc configures a VNC target. Exactly one target type must be set.
	// +optional
	VNC *VNCTargetSpec `json:"vnc,omitempty"`
}

// WarpgateTargetStatus defines the observed state of WarpgateTarget.
type WarpgateTargetStatus struct {
	// externalID is the target's ID in Warpgate.
	ExternalID string `json:"externalID,omitempty"`
	// conditions represent the current state of the WarpgateTarget resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Name",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`

// WarpgateTarget is the Schema for the warpgatetargets API.
type WarpgateTarget struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of WarpgateTarget.
	// +required
	Spec WarpgateTargetSpec `json:"spec"`

	// status defines the observed state of WarpgateTarget.
	// +optional
	Status WarpgateTargetStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// WarpgateTargetList contains a list of WarpgateTarget.
type WarpgateTargetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []WarpgateTarget `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WarpgateTarget{}, &WarpgateTargetList{})
}
