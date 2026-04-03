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

// WarpgateTicketSpec defines the desired state of WarpgateTicket.
type WarpgateTicketSpec struct {
	// connectionRef is the name of the WarpgateConnection CR in the same namespace.
	// +required
	ConnectionRef string `json:"connectionRef"`
	// username is the Warpgate username the ticket is for.
	// +optional
	Username string `json:"username,omitempty"`
	// targetName is the Warpgate target the ticket grants access to.
	// +optional
	TargetName string `json:"targetName,omitempty"`
	// expiry is the ticket expiration time (RFC3339 format).
	// +optional
	Expiry string `json:"expiry,omitempty"`
	// numberOfUses limits how many times the ticket can be used.
	// +optional
	NumberOfUses *int `json:"numberOfUses,omitempty"`
	// description is an optional description for the ticket.
	// +optional
	Description string `json:"description,omitempty"`
}

// WarpgateTicketStatus defines the observed state of WarpgateTicket.
type WarpgateTicketStatus struct {
	// ticketID is the Warpgate-assigned ticket UUID.
	TicketID string `json:"ticketID,omitempty"`
	// secretRef is the name of the auto-created Secret containing the ticket secret.
	SecretRef string `json:"secretRef,omitempty"`
	// conditions represent the current state.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Username",type=string,JSONPath=`.spec.username`
// +kubebuilder:printcolumn:name="Target",type=string,JSONPath=`.spec.targetName`
// +kubebuilder:printcolumn:name="TicketID",type=string,JSONPath=`.status.ticketID`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`

// WarpgateTicket is the Schema for the warpgatetickets API.
type WarpgateTicket struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of WarpgateTicket.
	// +required
	Spec WarpgateTicketSpec `json:"spec"`

	// status defines the observed state of WarpgateTicket.
	// +optional
	Status WarpgateTicketStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// WarpgateTicketList contains a list of WarpgateTicket.
type WarpgateTicketList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []WarpgateTicket `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WarpgateTicket{}, &WarpgateTicketList{})
}
