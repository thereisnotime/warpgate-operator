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

// CredentialPolicySpec defines allowed credential types per protocol.
type CredentialPolicySpec struct {
	HTTP     []string `json:"http,omitempty"`
	SSH      []string `json:"ssh,omitempty"`
	MySQL    []string `json:"mysql,omitempty"`
	Postgres []string `json:"postgres,omitempty"`
}

// WarpgateUserSpec defines the desired state of WarpgateUser.
type WarpgateUserSpec struct {
	// connectionRef is the name of the WarpgateConnection CR in the same namespace.
	// +required
	ConnectionRef string `json:"connectionRef"`
	// username is the Warpgate username.
	// +required
	Username string `json:"username"`
	// description is an optional description.
	// +optional
	Description string `json:"description,omitempty"`
	// credentialPolicy defines allowed credential types per protocol.
	// +optional
	CredentialPolicy *CredentialPolicySpec `json:"credentialPolicy,omitempty"`
	// generatePassword when true (the default) auto-generates a random password
	// credential for this user and stores it in a Kubernetes Secret named
	// <cr-name>-password in the same namespace. Set to false to skip.
	// +optional
	// +kubebuilder:default=true
	GeneratePassword *bool `json:"generatePassword,omitempty"`
	// passwordLength is the length of the auto-generated password. Defaults to 32.
	// +optional
	// +kubebuilder:default=32
	// +kubebuilder:validation:Minimum=16
	// +kubebuilder:validation:Maximum=128
	PasswordLength *int `json:"passwordLength,omitempty"`
}

// WarpgateUserStatus defines the observed state of WarpgateUser.
type WarpgateUserStatus struct {
	// externalID is the Warpgate-assigned UUID for this user.
	ExternalID string `json:"externalID,omitempty"`
	// passwordCredentialID is the Warpgate-assigned UUID for the auto-generated password credential.
	PasswordCredentialID string `json:"passwordCredentialID,omitempty"`
	// passwordSecretRef is the name of the auto-created Secret containing the generated password.
	PasswordSecretRef string `json:"passwordSecretRef,omitempty"`
	// conditions represent the current state.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Username",type=string,JSONPath=`.spec.username`
// +kubebuilder:printcolumn:name="ExternalID",type=string,JSONPath=`.status.externalID`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`

// WarpgateUser is the Schema for the warpgateusers API.
type WarpgateUser struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of WarpgateUser.
	// +required
	Spec WarpgateUserSpec `json:"spec"`

	// status defines the observed state of WarpgateUser.
	// +optional
	Status WarpgateUserStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// WarpgateUserList contains a list of WarpgateUser.
type WarpgateUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []WarpgateUser `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WarpgateUser{}, &WarpgateUserList{})
}
