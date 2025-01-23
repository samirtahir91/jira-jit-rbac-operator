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
	"fmt"
	"os"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	justintimev1 "jira-jit-rbac-operator/api/v1"
	"jira-jit-rbac-operator/test/utils"
	// TODO (user): Add any additional imports if needed
)

var (
	TestNamespace = os.Getenv("OPERATOR_NAMESPACE")
)

const (
	JitRequestName     = "e2e-jit-test"
	TestJitConfig      = "jira-jit-rbac-operator-int"
	ValidClusterRole   = "edit"
	InvalidClusterRole = "admin"
	InvalidNamespace   = "invalid-namespace"
)

// Function to initialise os vars
func init() {
	if TestNamespace == "" {
		panic(fmt.Errorf("OPERATOR_NAMESPACE environment variable(s) not set"))
	}
}

var _ = Describe("JitRequest Webhook", Ordered, func() {
	var (
		obj       *justintimev1.JitRequest
		validator JitRequestCustomValidator
	)

	BeforeAll(func() {
		By("removing manager config")
		cmd := exec.Command("kubectl", "delete", "jitcfg", TestJitConfig)
		_, _ = utils.Run(cmd)
	})

	BeforeEach(func() {
		obj = &justintimev1.JitRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name: "e2e-jit-test",
			},
			Spec: justintimev1.JitRequestSpec{
				ClusterRole: ValidClusterRole,
				Reporter:    "master-chief@unsc.com",
				Namespaces: []string{
					TestNamespace,
				},
				NamespaceLabels: map[string]string{},
				StartTime:       metav1.NewTime(metav1.Now().Add(10 * time.Second)),
				EndTime:         metav1.NewTime(metav1.Now().Add(20 * time.Second)),
				JiraFields: map[string]string{
					"Approver":      "cptKeyes",
					"ProductOwner":  "Oni",
					"Justification": "I need a weapon",
				},
			},
		}
		validator = JitRequestCustomValidator{}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")
	})

	AfterEach(func() {
	})

	AfterAll(func() {
		By("removing manager config")
		cmd := exec.Command("kubectl", "delete", "jitcfg", TestJitConfig)
		_, _ = utils.Run(cmd)
	})

	Context("When creating the JustInTime config object", func() {
		It("should successfully load the config and write the config file", func() {
			By("Creating the operator JustInTimeConfig")
			err := utils.CreateJitConfig(ctx, k8sClient, ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			By("Ensuring the config file is written")
			configFilePath := "/tmp/jit-test/config.json"
			Eventually(func() bool {
				_, err := os.Stat(configFilePath)
				return err == nil
			}, time.Second*5, time.Millisecond*100).Should(BeTrue(), "Config file was not created in time")
		})
	})

	Context("When creating or updating JitRequest under Validating Webhook", func() {

		It("Should admit deletion", func() {
			By("simulating a valid delete scenario")
			Expect(validator.ValidateDelete(ctx, obj)).To(BeNil())
		})

		It("Should admit creation if all required fields are present and correct", func() {
			By("simulating a valid creation scenario")
			Expect(validator.ValidateCreate(ctx, obj)).To(BeNil())
		})

		It("Should validate updates correctly", func() {
			By("simulating a valid update scenario")
			oldObj := obj
			obj.Spec.Reporter = "updated_value"
			Expect(validator.ValidateUpdate(ctx, oldObj, obj)).To(BeNil())
		})

		It("Should deny update if cluster role is invalid", func() {
			By("simulating a invalid cluster role update")
			oldObj := obj
			obj.Spec.ClusterRole = InvalidClusterRole
			Expect(validator.ValidateUpdate(ctx, oldObj, obj)).Error().To(
				MatchError(ContainSubstring("clusterRole must be one of")),
				"clusterRole to fail if not allowed in config")
		})

		It("Should deny creation if cluster role is invalid", func() {
			By("simulating a invalid cluster role")
			obj.Spec.ClusterRole = InvalidClusterRole
			Expect(validator.ValidateCreate(ctx, obj)).Error().To(
				MatchError(ContainSubstring("clusterRole must be one of")),
				"clusterRole to fail if not allowed in config")
		})

		It("Should deny creation if startTime is invalid", func() {
			By("simulating a invalid startTime")
			obj.Spec.StartTime = metav1.NewTime(metav1.Now().Add(-10 * time.Second))
			Expect(validator.ValidateCreate(ctx, obj)).Error().To(
				MatchError(ContainSubstring("start time must be after current time")),
				"startTime to fail if not after current time")
		})

		It("Should deny creation if endTime is invalid", func() {
			By("simulating a invalid endTime")
			obj.Spec.EndTime = metav1.NewTime(metav1.Now().Add(-10 * time.Second))
			Expect(validator.ValidateCreate(ctx, obj)).Error().To(
				MatchError(ContainSubstring("end time must be after startTime")),
				"endTime to fail if not after startTime")
		})

		It("Should deny creation if any namespace is invalid if using NamespaceAllowedRegex in config", func() {
			By("simulating a invalid namespace")
			obj.Spec.Namespaces = []string{
				InvalidNamespace,
			}
			Expect(validator.ValidateCreate(ctx, obj)).Error().To(
				MatchError(ContainSubstring("namespace does not match the allowed pattern")),
				"namespace to fail if not matching a regex")
		})

		It("Should deny creation if any namespace is not matching required labels if defined", func() {
			By("simulating a invalid namespace")
			label := "foo"
			labelValue := "bar"
			obj.Spec.NamespaceLabels = map[string]string{
				label: labelValue,
			}
			msg := fmt.Sprintf("the following namespaces do not match the specified labels (%s=%s): [%s]", label, labelValue, TestNamespace)
			Expect(validator.ValidateCreate(ctx, obj)).Error().To(
				MatchError(ContainSubstring(msg)),
				"namespace to fail if not matching defined labels")
		})

		It("Should deny creation if any JiraField is missing if it is a defined CustomField in config", func() {
			By("simulating a invalid endTime")
			obj.Spec.JiraFields = map[string]string{
				"Approver":     "cptKeyes",
				"ProductOwner": "Oni",
			}
			Expect(validator.ValidateCreate(ctx, obj)).Error().To(
				MatchError(ContainSubstring("missing custom field")),
				"jiraFields to fail if missing a field from customFields in config")
		})

	})

})
