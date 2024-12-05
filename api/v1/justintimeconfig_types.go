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

// JustInTimeConfigSpec defines the desired state of JustInTimeConfig.
type JustInTimeConfigSpec struct {
	// Configure allowed cluster roles to bind for a JitRequest
	AllowedClusterRoles []string `json:"allowedClusterRoles,omitempty"`
}

// JustInTimeConfigStatus defines the observed state of JustInTimeConfig.
type JustInTimeConfigStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=jitcfg

// JustInTimeConfig is the Schema for the justintimeconfigs API.
type JustInTimeConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   JustInTimeConfigSpec   `json:"spec,omitempty"`
	Status JustInTimeConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// JustInTimeConfigList contains a list of JustInTimeConfig.
type JustInTimeConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []JustInTimeConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&JustInTimeConfig{}, &JustInTimeConfigList{})
}
