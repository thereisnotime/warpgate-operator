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

// WarpgatePublicKeyCredentialSpec defines the desired state of WarpgatePublicKeyCredential.
type WarpgatePublicKeyCredentialSpec struct {
	// connectionRef is the name of the WarpgateConnection CR in the same namespace.
	// +required
	ConnectionRef string `json:"connectionRef"`
	// username is the Warpgate username to add the credential to.
	// +required
	Username string `json:"username"`
	// label is a human-readable label for the key.
	// +required
	Label string `json:"label"`
	// opensshPublicKey is the SSH public key in OpenSSH format.
	// +required
	OpenSSHPublicKey string `json:"opensshPublicKey"`
}

// WarpgatePublicKeyCredentialStatus defines the observed state of WarpgatePublicKeyCredential.
type WarpgatePublicKeyCredentialStatus struct {
	// userID is the resolved Warpgate user UUID.
	UserID string `json:"userID,omitempty"`
	// credentialID is the Warpgate-assigned credential UUID.
	CredentialID string `json:"credentialID,omitempty"`
	// conditions represent the current state.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Username",type=string,JSONPath=`.spec.username`
// +kubebuilder:printcolumn:name="Label",type=string,JSONPath=`.spec.label`
// +kubebuilder:printcolumn:name="CredentialID",type=string,JSONPath=`.status.credentialID`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`

// WarpgatePublicKeyCredential is the Schema for the warpgatepublickeycredentials API.
type WarpgatePublicKeyCredential struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of WarpgatePublicKeyCredential.
	// +required
	Spec WarpgatePublicKeyCredentialSpec `json:"spec"`

	// status defines the observed state of WarpgatePublicKeyCredential.
	// +optional
	Status WarpgatePublicKeyCredentialStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// WarpgatePublicKeyCredentialList contains a list of WarpgatePublicKeyCredential.
type WarpgatePublicKeyCredentialList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []WarpgatePublicKeyCredential `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WarpgatePublicKeyCredential{}, &WarpgatePublicKeyCredentialList{})
}
