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
	// +kubebuilder:validation:Enum=Password;PublicKey
	AuthKind string `json:"authKind"`
	// passwordSecretRef references a Secret containing the SSH password (for Password auth).
	// +optional
	PasswordSecretRef *SecretKeyRef `json:"passwordSecretRef,omitempty"`
	// allowInsecureAlgos permits the use of insecure SSH algorithms.
	AllowInsecureAlgos bool `json:"allowInsecureAlgos,omitempty"`
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
	// passwordSecretRef references a Secret containing the MySQL password.
	// +optional
	PasswordSecretRef *SecretKeyRef `json:"passwordSecretRef,omitempty"`
	// tls configures TLS settings for the MySQL connection.
	// +optional
	TLS *TLSConfigSpec `json:"tls,omitempty"`
}

// PostgreSQLTargetSpec defines the configuration for a PostgreSQL target.
type PostgreSQLTargetSpec struct {
	// host is the hostname or IP of the PostgreSQL server.
	Host string `json:"host"`
	// port is the PostgreSQL port.
	Port int `json:"port"`
	// username is the PostgreSQL username.
	Username string `json:"username"`
	// passwordSecretRef references a Secret containing the PostgreSQL password.
	// +optional
	PasswordSecretRef *SecretKeyRef `json:"passwordSecretRef,omitempty"`
	// tls configures TLS settings for the PostgreSQL connection.
	// +optional
	TLS *TLSConfigSpec `json:"tls,omitempty"`
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
