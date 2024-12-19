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

type Config struct {
	AllowedClusterRolesField  []string                       `json:"allowedClusterRoles"`
	RejectedTransitionIDField string                         `json:"rejectedTransitionID"`
	JiraProjectField          string                         `json:"jiraProject"`
	JiraIssueTypeField        string                         `json:"jiraIssueType"`
	ApprovedTransitionIDField string                         `json:"approvedTransitionID"`
	CustomFieldsField         *justintimev1.CustomFieldsSpec `json:"customFields"`
}

func (c *Config) AllowedClusterRoles() []string {
	return c.AllowedClusterRolesField
}

func (c *Config) RejectedTransitionID() string {
	return c.RejectedTransitionIDField
}

func (c *Config) JiraProject() string {
	return c.JiraProjectField
}

func (c *Config) JiraIssueType() string {
	return c.JiraIssueTypeField
}

func (c *Config) ApprovedTransitionID() string {
	return c.ApprovedTransitionIDField
}

func (c *Config) CustomFields() *justintimev1.CustomFieldsSpec {
	return c.CustomFieldsField
}

type Configuration interface {
	AllowedClusterRoles() []string
	RejectedTransitionID() string
	JiraProject() string
	JiraIssueType() string
	ApprovedTransitionID() string
	CustomFields() *justintimev1.CustomFieldsSpec
}
