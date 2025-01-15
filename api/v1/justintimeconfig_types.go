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
	AllowedClusterRoles  []string                       `json:"allowedClusterRoles" validate:"required"`
	RejectedTransitionID string                         `json:"rejectedTransitionID" validate:"required"`
	JiraProject          string                         `json:"jiraProject" validate:"required"`
	JiraIssueType        string                         `json:"jiraIssueType" validate:"required"`
	ApprovedTransitionID string                         `json:"approvedTransitionID" validate:"required"`
	RequiredFields       *RequiredFieldsSpec            `json:"requiredFields"`
	CustomFields         map[string]CustomFieldSettings `json:"customFields"`
}

// RequiredFieldsSpec defines the specification for required fields
type RequiredFieldsSpec struct {
	// Cluster role field in Jira
	ClusterRole CustomFieldSettings `json:"ClusterRole" validate:"required"`
	// StartTime field in Jira
	StartTime CustomFieldSettings `json:"StartTime" validate:"required"`
	// EndTime field in Jira
	EndTime CustomFieldSettings `json:"EndTime" validate:"required"`
}

// CustomFieldsSpec defines the specification for custom fields
// type CustomFieldsSpec struct {
// 	Reporter      CustomFieldSettings `json:"Reporter" validate:"required"`
// 	Approver      CustomFieldSettings `json:"Approver" validate:"required"`
// 	ProductOwner  CustomFieldSettings `json:"ProductOwner" validate:"required"`
// 	Justification CustomFieldSettings `json:"Justification" validate:"required"`
// }

// CustomField defines the custom Jira fields to use in a Jira create payload
type CustomFieldSettings struct {
	Type            string `json:"type" validate:"required"`
	JiraCustomField string `json:"jiraCustomField" validate:"required"`
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
