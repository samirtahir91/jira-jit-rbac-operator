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

package controller

import (
	"fmt"
	"os"
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"jira-jit-rbac-operator/test/utils"
)

var (
	TestNamespace                 = os.Getenv("OPERATOR_NAMESPACE")
	TestJiraWorkflowApproveStatus string
)

const (
	JitRequestName             = "e2e-jit-test"
	RoleBindingName            = JitRequestName + "-jit"
	TestJitConfig              = "jira-jit-rbac-operator-int"
	ValidClusterRole           = "edit"
	InvalidClusterRole         = "admin"
	InvalidNamespace           = "invalid-namespace"
	TestJiraWorkflowToDoStatus = "ToDo"
	TestJiraWorkflowApproved   = "Approved"
)

// Function to initialise os vars
func init() {
	if TestNamespace == "" {
		panic(fmt.Errorf("OPERATOR_NAMESPACE environment variable(s) not set"))
	}
}

var _ = Describe("JitRequest Controller", Ordered, func() {

	BeforeAll(func() {

		By("removing manager config")
		cmd := exec.Command("kubectl", "delete", "jitcfg", TestJitConfig)
		_, _ = utils.Run(cmd)

		By("removing jitRequest")
		cmd = exec.Command("kubectl", "delete", "jitreq", JitRequestName)
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", TestNamespace)
		_, _ = utils.Run(cmd)

		By("creating manager namespace")
		err := utils.CreateNamespace(ctx, k8sClient, TestNamespace)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		By("removing manager namespace")
		cmd := exec.Command("kubectl", "delete", "ns", TestNamespace)
		_, _ = utils.Run(cmd)

		By("removing manager config")
		cmd = exec.Command("kubectl", "delete", "jitcfg", TestJitConfig)
		_, _ = utils.Run(cmd)

		By("removing jitRequest")
		cmd = exec.Command("kubectl", "delete", "jitreq", JitRequestName)
		_, _ = utils.Run(cmd)
	})

	Context("When creating the JustInTime config object", func() {
		It("should successfully load the config and write the config file", func() {
			By("Creating the operator JustInTimeConfig")
			err := utils.CreateJitConfig(ctx, k8sClient, ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When creating a new valid JitRequest with a start time 10s from now", func() {
		It("should successfully process as a new request and issue a rolebinding", func() {
			By("Creating and approving the JitRequest")
			TestJiraWorkflowApproveStatus = TestJiraWorkflowApproved
			jitRequest, err := utils.CreateJitRequest(ctx, k8sClient, 10, ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			By("Checking the status of the JitRequest for Pre-Approval")
			err = utils.CheckJitStatus(ctx, k8sClient, jitRequest, StatusPreApproved)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for the JitRequest Pre-Approval event to be recorded")
			err = utils.CheckEvent(
				ctx,
				k8sClient,
				JitRequestName,
				TestNamespace,
				"Normal",
				StatusPreApproved,
				"ClusterRole 'edit' is allowed\nJira: IAM-1",
			)
			Expect(err).NotTo(HaveOccurred())

			By("Checking the status of the JitRequest for completed status")
			err = utils.CheckJitStatus(ctx, k8sClient, jitRequest, StatusSucceeded)
			Expect(err).NotTo(HaveOccurred())

			By("Checking the RoleBinding exists")
			err = utils.CheckRoleBindingExists(ctx, k8sClient, TestNamespace, RoleBindingName)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should successfully remove the JitRequest on expiry and remove the RoleBinding", func() {
			By("Checking the RoleBinding is eventually removed")
			err := utils.CheckRoleBindingRemoved(ctx, k8sClient, TestNamespace, RoleBindingName)
			Expect(err).NotTo(HaveOccurred())

			By("Checking the JitRequest is eventually removed")
			err = utils.CheckJitRemoved(ctx, k8sClient, JitRequestName)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When creating a new JitRequest with invalid start time from now", func() {
		It("should successfully process as a new request and reject the JitRequest", func() {
			By("Creating the JitRequest")
			TestJiraWorkflowApproveStatus = TestJiraWorkflowToDoStatus
			_, err := utils.CreateJitRequest(ctx, k8sClient, -10, ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for the JitRequest Rejected event to be recorded")
			err = utils.CheckEvent(
				ctx,
				k8sClient,
				JitRequestName,
				TestNamespace,
				"Warning",
				EventValidationFailed,
				"must be after current time",
			)
			Expect(err).NotTo(HaveOccurred())

			By("Checking the JitRequest is eventually removed")
			err = utils.CheckJitRemoved(ctx, k8sClient, JitRequestName)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When creating a new JitRequest with invalid cluster role", func() {
		It("should successfully process as a new request and reject the JitRequest", func() {
			By("Creating the JitRequest")
			TestJiraWorkflowApproveStatus = TestJiraWorkflowToDoStatus
			_, err := utils.CreateJitRequest(ctx, k8sClient, 10, InvalidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for the JitRequest Rejected event to be recorded")
			err = utils.CheckEvent(
				ctx,
				k8sClient,
				JitRequestName,
				TestNamespace,
				"Warning",
				EventValidationFailed,
				fmt.Sprintf("ClusterRole '%s' is not allowed", InvalidClusterRole),
			)
			Expect(err).NotTo(HaveOccurred())

			By("Checking the JitRequest is eventually removed")
			err = utils.CheckJitRemoved(ctx, k8sClient, JitRequestName)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When creating a new valid JitRequest with a start time 10s from now and not approving in Jira", func() {
		It("should successfully process as a new request and reject the Jira and JitRequest", func() {
			By("Creating and not approving the JitRequest")
			TestJiraWorkflowApproveStatus = TestJiraWorkflowToDoStatus
			jitRequest, err := utils.CreateJitRequest(ctx, k8sClient, 10, ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			By("Checking the status of the JitRequest for Pre-Approval")
			err = utils.CheckJitStatus(ctx, k8sClient, jitRequest, StatusPreApproved)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for the JitRequest Rejected event to be recorded")
			err = utils.CheckEvent(
				ctx,
				k8sClient,
				JitRequestName,
				TestNamespace,
				"Warning",
				"JiraNotApproved",
				"Error: failed on jira approval",
			)
			Expect(err).NotTo(HaveOccurred())

			By("Checking the JitRequest is eventually removed")
			err = utils.CheckJitRemoved(ctx, k8sClient, JitRequestName)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When creating a new JitRequest with a start time 10s from now and for a non-existing Namespace", func() {
		It("should successfully process as a new request and reject the Jira and JitRequest", func() {
			By("Creating and approving the JitRequest")
			TestJiraWorkflowApproveStatus = TestJiraWorkflowApproved
			_, err := utils.CreateJitRequest(ctx, k8sClient, 10, ValidClusterRole, InvalidNamespace)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for the JitRequest Rejected event to be recorded")
			err = utils.CheckEvent(
				ctx,
				k8sClient,
				JitRequestName,
				TestNamespace,
				"Warning",
				EventValidationFailed,
				fmt.Sprintf("Namespace %s is not validated | Error: failed to get namespace %s: Namespace \"%s\" not found", InvalidNamespace, InvalidNamespace, InvalidNamespace),
			)
			Expect(err).NotTo(HaveOccurred())

			By("Checking the JitRequest is eventually removed")
			err = utils.CheckJitRemoved(ctx, k8sClient, JitRequestName)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When creating a new JitRequest with a start time 10s from now and for a Namespace with non-matching label", func() {
		It("should successfully process as a new request and reject the Jira and JitRequest", func() {
			By("Creating and approving the JitRequest with a label match")
			TestJiraWorkflowApproveStatus = TestJiraWorkflowApproved
			namespaceLabel := "bar"
			_, err := utils.CreateJitRequest(ctx, k8sClient, 10, ValidClusterRole, TestNamespace, namespaceLabel)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for the JitRequest Rejected event to be recorded")
			err = utils.CheckEvent(
				ctx,
				k8sClient,
				JitRequestName,
				TestNamespace,
				"Warning",
				EventValidationFailed,
				fmt.Sprintf("Namespace %s is not validated | Error: namespace %s does not have the label foo=%s", TestNamespace, TestNamespace, namespaceLabel),
			)
			Expect(err).NotTo(HaveOccurred())

			By("Checking the JitRequest is eventually removed")
			err = utils.CheckJitRemoved(ctx, k8sClient, JitRequestName)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
