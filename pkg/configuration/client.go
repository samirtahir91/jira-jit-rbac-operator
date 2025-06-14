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

import (
	"context"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	justintimev1 "jira-jit-rbac-operator/api/v1"
)

// jitRbacOperatorConfiguration
type jitRbacOperatorConfiguration struct {
	retrievalFn func() *justintimev1.JustInTimeConfig
}

// NewJitRbacOperatorConfiguration returns new JustInTimeConfig or default config if not found in the cluster
func NewJitRbacOperatorConfiguration(ctx context.Context, client client.Client, name string) Configuration {
	return &jitRbacOperatorConfiguration{retrievalFn: func() *justintimev1.JustInTimeConfig {
		config := &justintimev1.JustInTimeConfig{}

		if err := client.Get(ctx, types.NamespacedName{Name: name}, config); err != nil {
			if apierrors.IsNotFound(err) {
				return &justintimev1.JustInTimeConfig{
					Spec: justintimev1.JustInTimeConfigSpec{
						AllowedClusterRoles:       []string{"edit"},
						JiraWorkflowApproveStatus: "Approved",
						RejectedTransitionID:      "21",
						JiraProject:               "IAM",
						JiraIssueType:             "Access Request",
						CompletedTransitionID:     "41",
						AdditionalCommentText:     "config: default",
						NamespaceAllowedRegex:     ".*",
						Labels: []string{
							"default-config",
						},
						Environment: &justintimev1.EnvironmentSpec{
							Environment: "dev-test",
							Cluster:     "minikube",
						},
						RequiredFields: &justintimev1.RequiredFieldsSpec{
							StartTime:   justintimev1.CustomFieldSettings{Type: "date", JiraCustomField: "customfield_10118"},
							EndTime:     justintimev1.CustomFieldSettings{Type: "date", JiraCustomField: "customfield_10119"},
							ClusterRole: justintimev1.CustomFieldSettings{Type: "date", JiraCustomField: "customfield_10117"},
						},
						CustomFields: map[string]justintimev1.CustomFieldSettings{
							"Approver":      {Type: "user", JiraCustomField: "customfield_10114"},
							"ProductOwner":  {Type: "user", JiraCustomField: "customfield_10115"},
							"Justification": {Type: "text", JiraCustomField: "customfield_10116"},
						},
						SelfApprovalEnabled: false,
					},
				}
			}
			panic(errors.Wrap(err, "Cannot retrieve configuration with name "+name))
		}

		return config
	}}
}

func (c *jitRbacOperatorConfiguration) SelfApprovalEnabled() bool {
	return c.retrievalFn().Spec.SelfApprovalEnabled
}

func (c *jitRbacOperatorConfiguration) NamespaceAllowedRegex() string {
	return c.retrievalFn().Spec.NamespaceAllowedRegex
}

func (c *jitRbacOperatorConfiguration) Environment() *justintimev1.EnvironmentSpec {
	return c.retrievalFn().Spec.Environment
}

func (c *jitRbacOperatorConfiguration) AdditionalCommentText() string {
	return c.retrievalFn().Spec.AdditionalCommentText
}

func (c *jitRbacOperatorConfiguration) Labels() []string {
	return c.retrievalFn().Spec.Labels
}

func (c *jitRbacOperatorConfiguration) AllowedClusterRoles() []string {
	return c.retrievalFn().Spec.AllowedClusterRoles
}

func (c *jitRbacOperatorConfiguration) JiraWorkflowApproveStatus() string {
	return c.retrievalFn().Spec.JiraWorkflowApproveStatus
}

func (c *jitRbacOperatorConfiguration) RejectedTransitionID() string {
	return c.retrievalFn().Spec.RejectedTransitionID
}

func (c *jitRbacOperatorConfiguration) JiraProject() string {
	return c.retrievalFn().Spec.JiraProject
}

func (c *jitRbacOperatorConfiguration) JiraIssueType() string {
	return c.retrievalFn().Spec.JiraIssueType
}

func (c *jitRbacOperatorConfiguration) CompletedTransitionID() string {
	return c.retrievalFn().Spec.CompletedTransitionID
}

func (c *jitRbacOperatorConfiguration) CustomFields() map[string]justintimev1.CustomFieldSettings {
	return c.retrievalFn().Spec.CustomFields
}

func (c *jitRbacOperatorConfiguration) RequiredFields() *justintimev1.RequiredFieldsSpec {
	return c.retrievalFn().Spec.RequiredFields
}
