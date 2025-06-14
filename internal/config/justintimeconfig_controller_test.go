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

package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	justintimev1 "jira-jit-rbac-operator/api/v1"
	"jira-jit-rbac-operator/test/utils"
	"os/exec"

	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const (
	ValidClusterRole string = "edit"
)

var (
	TestNamespace = os.Getenv("OPERATOR_NAMESPACE")
)

// init os vars
func init() {
	if TestNamespace == "" {
		panic(fmt.Errorf("OPERATOR_NAMESPACE environment variable(s) not set"))
	}
}

var _ = Describe("JustInTimeConfig Controller", Ordered, Label("integration"), func() {

	BeforeAll(func() {
		By("removing manager config")
		cmd := exec.Command("kubectl", "delete", "jitcfg", TestJitConfig)
		_, _ = utils.Run(cmd)

	})

	AfterAll(func() {
		By("removing manager config")
		cmd := exec.Command("kubectl", "delete", "jitcfg", TestJitConfig)
		_, _ = utils.Run(cmd)
	})

	Context("When initialising a context and K8s client", func() {
		It("should be successfully initialised", func() {
			By("Creating the ctx and client")
			ctx = context.TODO()
			err := justintimev1.AddToScheme(scheme.Scheme)
			Expect(err).NotTo(HaveOccurred())
			cfg, err := config.GetConfig()
			if err != nil {
				fmt.Printf("Failed to load kubeconfig: %v\n", err)
				return
			}
			k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient).NotTo(BeNil())
		})
	})

	Context("When creating the JustInTime config object", func() {
		It("should successfully load the config and write the config file", func() {
			By("Creating the operator JustInTimeConfig")
			err := utils.CreateJitConfig(ctx, k8sClient, ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			By("Checking the config json file matches expected config")
			expectedConfig := justintimev1.JustInTimeConfigSpec{
				AllowedClusterRoles:       []string{"edit"},
				JiraWorkflowApproveStatus: "Approved",
				RejectedTransitionID:      "21",
				JiraProject:               "IAM",
				JiraIssueType:             "Access Request",
				CompletedTransitionID:     "41",
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
				Labels: []string{
					"default-config",
				},
				Environment: &justintimev1.EnvironmentSpec{
					Environment: "dev-test",
					Cluster:     "minikube",
				},
				AdditionalCommentText: "config: default",
				NamespaceAllowedRegex: ".*",
				SelfApprovalEnabled:   false,
			}

			// Read the generated config file
			data, err := os.ReadFile(ConfigCacheFilePath + "/" + ConfigFile)
			Expect(err).NotTo(HaveOccurred())

			var generatedConfig justintimev1.JustInTimeConfigSpec
			err = json.Unmarshal(data, &generatedConfig)
			Expect(err).NotTo(HaveOccurred())

			// Compare the generated config with the expected config
			Expect(expectedConfig).To(Equal(generatedConfig))
		})
	})
})
