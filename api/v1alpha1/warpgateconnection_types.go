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

// SecretKeyRef references a key within a Kubernetes Secret.
type SecretKeyRef struct {
	// name is the name of the Secret.
	// +required
	Name string `json:"name"`
	// key is the key within the Secret. Defaults to "password" for password refs.
	// Unused by tokenSecretRef (which always reads "username" and "password" keys).
	// +optional
	Key string `json:"key,omitempty"`
}

// CredentialsSecretRef references a Kubernetes Secret containing login credentials.
type CredentialsSecretRef struct {
	// name is the name of the Secret.
	// +required
	Name string `json:"name"`
	// usernameKey is the key in the Secret that holds the username. Defaults to "username".
	// +optional
	// +kubebuilder:default="username"
	UsernameKey string `json:"usernameKey,omitempty"`
	// passwordKey is the key in the Secret that holds the password. Defaults to "password".
	// +optional
	// +kubebuilder:default="password"
	PasswordKey string `json:"passwordKey,omitempty"`
}

// WarpgateConnectionSpec defines the desired state of WarpgateConnection
type WarpgateConnectionSpec struct {
	// host is the URL of the Warpgate instance (e.g. https://warpgate.example.com).
	// +required
	Host string `json:"host"`
	// credentialsSecretRef references a Kubernetes Secret containing Warpgate admin credentials.
	// The Secret must have keys for the username and password (configurable, defaults to "username" and "password").
	// +required
	CredentialsSecretRef CredentialsSecretRef `json:"credentialsSecretRef"`
	// insecureSkipVerify disables TLS certificate verification. Not recommended for production.
	// +optional
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
}

// WarpgateConnectionStatus defines the observed state of WarpgateConnection.
type WarpgateConnectionStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the WarpgateConnection resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// WarpgateConnection is the Schema for the warpgateconnections API
type WarpgateConnection struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of WarpgateConnection
	// +required
	Spec WarpgateConnectionSpec `json:"spec"`

	// status defines the observed state of WarpgateConnection
	// +optional
	Status WarpgateConnectionStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// WarpgateConnectionList contains a list of WarpgateConnection
type WarpgateConnectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []WarpgateConnection `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WarpgateConnection{}, &WarpgateConnectionList{})
}
