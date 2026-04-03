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

// WarpgatePasswordCredentialSpec defines the desired state of WarpgatePasswordCredential.
type WarpgatePasswordCredentialSpec struct {
	// connectionRef is the name of the WarpgateConnection CR in the same namespace.
	// +required
	ConnectionRef string `json:"connectionRef"`
	// username is the Warpgate username to add the credential to.
	// +required
	Username string `json:"username"`
	// passwordSecretRef references a Secret containing the password.
	// +required
	PasswordSecretRef SecretKeyRef `json:"passwordSecretRef"`
}

// WarpgatePasswordCredentialStatus defines the observed state of WarpgatePasswordCredential.
type WarpgatePasswordCredentialStatus struct {
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
// +kubebuilder:printcolumn:name="CredentialID",type=string,JSONPath=`.status.credentialID`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`

// WarpgatePasswordCredential is the Schema for the warpgatepasswordcredentials API.
type WarpgatePasswordCredential struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of WarpgatePasswordCredential.
	// +required
	Spec WarpgatePasswordCredentialSpec `json:"spec"`

	// status defines the observed state of WarpgatePasswordCredential.
	// +optional
	Status WarpgatePasswordCredentialStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// WarpgatePasswordCredentialList contains a list of WarpgatePasswordCredential.
type WarpgatePasswordCredentialList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []WarpgatePasswordCredential `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WarpgatePasswordCredential{}, &WarpgatePasswordCredentialList{})
}
