/*
Copyright 2024.

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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// GhostSpec defines the desired state of Ghost
type GhostSpec struct {
	//+kubebuilder:validation:Pattern=`^[-a-z0-9]*$`
	ImageTag string `json:"imageTag"`
}

// GhostStatus defines the observed state of Ghost
type GhostStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Ghost is the Schema for the ghosts API
type Ghost struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GhostSpec   `json:"spec,omitempty"`
	Status GhostStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GhostList contains a list of Ghost
type GhostList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Ghost `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Ghost{}, &GhostList{})
}
