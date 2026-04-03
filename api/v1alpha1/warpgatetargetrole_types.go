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

// WarpgateTargetRoleSpec defines the desired state of WarpgateTargetRole.
type WarpgateTargetRoleSpec struct {
	// connectionRef is the name of the WarpgateConnection resource to use.
	// +required
	ConnectionRef string `json:"connectionRef"`

	// targetName is the Warpgate target name.
	// +required
	TargetName string `json:"targetName"`

	// roleName is the Warpgate role name.
	// +required
	RoleName string `json:"roleName"`
}

// WarpgateTargetRoleStatus defines the observed state of WarpgateTargetRole.
type WarpgateTargetRoleStatus struct {
	// targetID is the resolved Warpgate target UUID.
	TargetID string `json:"targetID,omitempty"`

	// roleID is the resolved Warpgate role UUID.
	RoleID string `json:"roleID,omitempty"`

	// conditions represent the current state of the WarpgateTargetRole resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="TargetName",type=string,JSONPath=`.spec.targetName`
// +kubebuilder:printcolumn:name="RoleName",type=string,JSONPath=`.spec.roleName`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`

// WarpgateTargetRole is the Schema for the warpgatetargetroles API
type WarpgateTargetRole struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of WarpgateTargetRole
	// +required
	Spec WarpgateTargetRoleSpec `json:"spec"`

	// status defines the observed state of WarpgateTargetRole
	// +optional
	Status WarpgateTargetRoleStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// WarpgateTargetRoleList contains a list of WarpgateTargetRole
type WarpgateTargetRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []WarpgateTargetRole `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WarpgateTargetRole{}, &WarpgateTargetRoleList{})
}
