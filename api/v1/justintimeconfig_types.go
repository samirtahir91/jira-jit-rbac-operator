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
	AllowedClusterRoles []string `json:"allowedClusterRoles" validate:"required"`
	// The value of the approved state for a Jira ticket, i.e. "Approved"
	JiraWorkflowApproveStatus string `json:"workflowApprovedStatus" validate:"required"`
	// The workflow transition ID for rejecting a ticket
	RejectedTransitionID string `json:"rejectedTransitionID" validate:"required"`
	// The Jira project key
	JiraProject string `json:"jiraProject" validate:"required"`
	// The Jira issue type
	JiraIssueType string `json:"jiraIssueType" validate:"required"`
	// The workflow transition ID for an approved ticket
	CompletedTransitionID string `json:"completedTransitionID" validate:"required"`
	// Required fields for the Jira ticket
	RequiredFields *RequiredFieldsSpec `json:"requiredFields"`
	// Optional additional fields to map to the ticket and enforce on a JitRequest's jiraFields
	CustomFields map[string]CustomFieldSettings `json:"customFields"`
	// Optional labels to add to jira tickets
	Labels []string `json:"labels,omitempty"`
	// Environment and cluster name to add as label to jira tickets
	Environment *EnvironmentSpec `json:"environment"`
	// Optional text to add to jira ticket comment
	AdditionalCommentText string `json:"additionalCommentText"`
	// Optional regex to only allow namespace names matching the regular expression
	NamespaceAllowedRegex string `json:"namespaceAllowedRegex,omitempty"`
}

// EnvironmentSpec defines the specification for the environment
type EnvironmentSpec struct {
	// Environmnt name
	Environment string `json:"environment"`
	// StartTime field in Jira
	Cluster string `json:"cluster"`
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
