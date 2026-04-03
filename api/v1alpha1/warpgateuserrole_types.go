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

// WarpgateUserRoleSpec defines the desired state of WarpgateUserRole.
type WarpgateUserRoleSpec struct {
	// connectionRef is the name of the WarpgateConnection resource to use.
	// +required
	ConnectionRef string `json:"connectionRef"`

	// username is the Warpgate username to bind.
	// +required
	Username string `json:"username"`

	// roleName is the Warpgate role name to bind.
	// +required
	RoleName string `json:"roleName"`
}

// WarpgateUserRoleStatus defines the observed state of WarpgateUserRole.
type WarpgateUserRoleStatus struct {
	// userID is the resolved Warpgate user UUID.
	UserID string `json:"userID,omitempty"`

	// roleID is the resolved Warpgate role UUID.
	RoleID string `json:"roleID,omitempty"`

	// conditions represent the current state of the WarpgateUserRole resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Username",type=string,JSONPath=`.spec.username`
// +kubebuilder:printcolumn:name="RoleName",type=string,JSONPath=`.spec.roleName`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`

// WarpgateUserRole is the Schema for the warpgateuserroles API
type WarpgateUserRole struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of WarpgateUserRole
	// +required
	Spec WarpgateUserRoleSpec `json:"spec"`

	// status defines the observed state of WarpgateUserRole
	// +optional
	Status WarpgateUserRoleStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// WarpgateUserRoleList contains a list of WarpgateUserRole
type WarpgateUserRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []WarpgateUserRole `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WarpgateUserRole{}, &WarpgateUserRoleList{})
}
