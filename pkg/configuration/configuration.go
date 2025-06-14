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

package configuration

import justintimev1 "jira-jit-rbac-operator/api/v1"

type Configuration interface {
	AllowedClusterRoles() []string
	JiraWorkflowApproveStatus() string
	RejectedTransitionID() string
	JiraProject() string
	JiraIssueType() string
	CompletedTransitionID() string
	CustomFields() map[string]justintimev1.CustomFieldSettings
	RequiredFields() *justintimev1.RequiredFieldsSpec
	Labels() []string
	AdditionalCommentText() string
	Environment() *justintimev1.EnvironmentSpec
	NamespaceAllowedRegex() string
	SelfApprovalEnabled() bool
}
