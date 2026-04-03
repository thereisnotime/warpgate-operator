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

// WarpgateRoleSpec defines the desired state of WarpgateRole.
type WarpgateRoleSpec struct {
	// connectionRef is the name of the WarpgateConnection CR in the same namespace.
	// +required
	ConnectionRef string `json:"connectionRef"`
	// name is the role name in Warpgate.
	// +required
	Name string `json:"name"`
	// description is an optional description for the role.
	// +optional
	Description string `json:"description,omitempty"`
}

// WarpgateRoleStatus defines the observed state of WarpgateRole.
type WarpgateRoleStatus struct {
	// externalID is the Warpgate-assigned UUID for this role.
	ExternalID string `json:"externalID,omitempty"`
	// conditions represent the current state.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Name",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="ExternalID",type=string,JSONPath=`.status.externalID`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`

// WarpgateRole is the Schema for the warpgateroles API.
type WarpgateRole struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of WarpgateRole.
	// +required
	Spec WarpgateRoleSpec `json:"spec"`

	// status defines the observed state of WarpgateRole.
	// +optional
	Status WarpgateRoleStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// WarpgateRoleList contains a list of WarpgateRole.
type WarpgateRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []WarpgateRole `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WarpgateRole{}, &WarpgateRoleList{})
}
