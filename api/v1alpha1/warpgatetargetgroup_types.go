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

// WarpgateTargetGroupSpec defines the desired state of WarpgateTargetGroup.
type WarpgateTargetGroupSpec struct {
	// connectionRef is the name of the WarpgateConnection CR in the same namespace.
	// +required
	ConnectionRef string `json:"connectionRef"`
	// name is the target group name in Warpgate.
	// +required
	Name string `json:"name"`
	// description is an optional description for the target group.
	// +optional
	Description string `json:"description,omitempty"`
	// color is the visual color for the target group in the Warpgate UI.
	// +kubebuilder:validation:Enum=Primary;Secondary;Success;Danger;Warning;Info;Light;Dark
	// +optional
	Color string `json:"color,omitempty"`
}

// WarpgateTargetGroupStatus defines the observed state of WarpgateTargetGroup.
type WarpgateTargetGroupStatus struct {
	// externalID is the Warpgate-assigned UUID for this target group.
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
// +kubebuilder:printcolumn:name="Color",type=string,JSONPath=`.spec.color`
// +kubebuilder:printcolumn:name="ExternalID",type=string,JSONPath=`.status.externalID`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`

// WarpgateTargetGroup is the Schema for the warpgatetargetgroups API.
type WarpgateTargetGroup struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of WarpgateTargetGroup.
	// +required
	Spec WarpgateTargetGroupSpec `json:"spec"`

	// status defines the observed state of WarpgateTargetGroup.
	// +optional
	Status WarpgateTargetGroupStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// WarpgateTargetGroupList contains a list of WarpgateTargetGroup.
type WarpgateTargetGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []WarpgateTargetGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WarpgateTargetGroup{}, &WarpgateTargetGroupList{})
}
