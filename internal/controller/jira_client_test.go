package controller

import (
	v1 "jira-jit-rbac-operator/api/v1"
	test_utils "jira-jit-rbac-operator/test/utils"
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("JitRequestReconciler jira_client Unit Tests", Ordered, Label("unit", "jira_client"), func() {

	var reconciler *JitRequestReconciler
	l := log.FromContext(ctx)

	// test config
	jiraProject := "IAM"
	jiraIssueType := "Access Request"
	customFieldsConfig := map[string]v1.CustomFieldSettings{
		"Approver":      {Type: "user", JiraCustomField: "customfield_10114"},
		"ProductOwner":  {Type: "user", JiraCustomField: "customfield_10115"},
		"Justification": {Type: "text", JiraCustomField: "customfield_10116"},
	}
	requiredFieldsConfig := &v1.RequiredFieldsSpec{
		StartTime:   v1.CustomFieldSettings{Type: "date", JiraCustomField: "customfield_10118"},
		EndTime:     v1.CustomFieldSettings{Type: "date", JiraCustomField: "customfield_10119"},
		ClusterRole: v1.CustomFieldSettings{Type: "date", JiraCustomField: "customfield_10117"},
	}
	ticketLabels := []string{"label1", "label2"}
	targetEnvironment := &v1.EnvironmentSpec{
		Environment: "dev-test",
		Cluster:     "minikube",
	}
	additionalComments := "This is a test comment."

	BeforeAll(func() {
		By("setting a jitRequest reconciler")
		reconciler = &JitRequestReconciler{
			Client:     k8sClient,
			Recorder:   record.NewFakeRecorder(10),
			Scheme:     scheme.Scheme,
			JiraClient: jiraClient,
		}

		By("removing manager config")
		cmd := exec.Command("kubectl", "delete", "jitcfg", TestJitConfig)
		_, _ = test_utils.Run(cmd)

		By("removing jitRequest")
		cmd = exec.Command("kubectl", "delete", "jitreq", JitRequestName)
		_, _ = test_utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", TestNamespace)
		_, _ = test_utils.Run(cmd)

		By("creating manager namespace")
		err := test_utils.CreateNamespace(ctx, k8sClient, TestNamespace)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		By("removing manager namespace")
		cmd := exec.Command("kubectl", "delete", "ns", TestNamespace)
		_, _ = test_utils.Run(cmd)

		By("removing manager config")
		cmd = exec.Command("kubectl", "delete", "jitcfg", TestJitConfig)
		_, _ = test_utils.Run(cmd)

		By("removing jitRequest")
		cmd = exec.Command("kubectl", "delete", "jitreq", JitRequestName)
		_, _ = test_utils.Run(cmd)
	})

	AfterEach(func() {
		By("removing jitRequest")
		cmd := exec.Command("kubectl", "delete", "jitreq", JitRequestName)
		_, _ = test_utils.Run(cmd)
	})

	Describe("preApproveRequest", func() {

		It("should reject is startTime has exceeded current time", func() {
			// Create JitRequest
			jitRequest, err := test_utils.CreateJitRequest(ctx, reconciler.Client, -1, test_utils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			result, err := reconciler.preApproveRequest(ctx, l, jitRequest, "IAM-1,", additionalComments)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.IsZero()).To(BeTrue())
		})

		It("should pre-approve valid JitRequests", func() {
			// Create JitRequest
			jitRequest, err := test_utils.CreateJitRequest(ctx, reconciler.Client, 10, test_utils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			// Create jira ticket
			ticket, err := reconciler.createJiraTicket(ctx, jitRequest, jiraProject, jiraIssueType, customFieldsConfig, requiredFieldsConfig, ticketLabels, targetEnvironment)
			Expect(err).NotTo(HaveOccurred())

			result, err := reconciler.preApproveRequest(ctx, l, jitRequest, ticket, additionalComments)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.IsZero()).To(BeFalse())
		})
	})

	Describe("createJiraTicket", func() {

		It("should create a Jira Ticket", func() {
			// Create JitRequest
			jitRequest, err := test_utils.CreateJitRequest(ctx, reconciler.Client, 10, test_utils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			result, err := reconciler.createJiraTicket(ctx, jitRequest, jiraProject, jiraIssueType, customFieldsConfig, requiredFieldsConfig, ticketLabels, targetEnvironment)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("IAM-1"))
		})

		It("should return Skipped if missing jira field", func() {
			// Create JitRequest
			jitRequest, err := test_utils.CreateJitRequest(ctx, reconciler.Client, 10, test_utils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			missingCustomFieldsConfig := map[string]v1.CustomFieldSettings{
				"MissingField": {Type: "user", JiraCustomField: "customfield_10114"},
			}
			result, err := reconciler.createJiraTicket(ctx, jitRequest, jiraProject, jiraIssueType, missingCustomFieldsConfig, requiredFieldsConfig, ticketLabels, targetEnvironment)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("Skipped"))
		})
	})

	Describe("rejectJiraTicket", func() {

		It("should reject a Jira Ticket", func() {
			// Create JitRequest
			jitRequest, err := test_utils.CreateJitRequest(ctx, reconciler.Client, 10, test_utils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			// Create jira ticket
			ticket, err := reconciler.createJiraTicket(ctx, jitRequest, jiraProject, jiraIssueType, customFieldsConfig, requiredFieldsConfig, ticketLabels, targetEnvironment)
			Expect(err).NotTo(HaveOccurred())

			jitRequest.Status.JiraTicket = ticket
			jitRequest.Status.Message = "test rejected"

			err = reconciler.rejectJiraTicket(ctx, jitRequest, "1")

			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("updateJiraTicket", func() {

		It("should update a Jira Ticket", func() {
			// Create JitRequest
			jitRequest, err := test_utils.CreateJitRequest(ctx, reconciler.Client, 10, test_utils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			// Create jira ticket
			ticket, err := reconciler.createJiraTicket(ctx, jitRequest, jiraProject, jiraIssueType, customFieldsConfig, requiredFieldsConfig, ticketLabels, targetEnvironment)
			Expect(err).NotTo(HaveOccurred())

			jitRequest.Status.JiraTicket = ticket

			err = reconciler.updateJiraTicket(ctx, ticket, "test updated")

			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("completeJiraTicket", func() {

		It("should complete a Jira Ticket", func() {
			// Create JitRequest
			jitRequest, err := test_utils.CreateJitRequest(ctx, reconciler.Client, 10, test_utils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			// Create jira ticket
			ticket, err := reconciler.createJiraTicket(ctx, jitRequest, jiraProject, jiraIssueType, customFieldsConfig, requiredFieldsConfig, ticketLabels, targetEnvironment)
			Expect(err).NotTo(HaveOccurred())

			jitRequest.Status.JiraTicket = ticket

			err = reconciler.completeJiraTicket(ctx, jitRequest, "1")

			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("getJiraApproval", func() {

		It("should return nil for a approved jira ticket", func() {
			// Create JitRequest
			jitRequest, err := test_utils.CreateJitRequest(ctx, reconciler.Client, 10, test_utils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			// Create jira ticket
			ticket, err := reconciler.createJiraTicket(ctx, jitRequest, jiraProject, jiraIssueType, customFieldsConfig, requiredFieldsConfig, ticketLabels, targetEnvironment)
			Expect(err).NotTo(HaveOccurred())

			// Approve ticket
			jitRequest.Status.JiraTicket = ticket
			test_utils.IssueStatus = test_utils.TestJiraWorkflowApproved

			err = reconciler.getJiraApproval(ctx, jitRequest, test_utils.IssueStatus)

			Expect(err).NotTo(HaveOccurred())
		})

		It("should return err for a non-approved jira ticket", func() {
			// Create JitRequest
			jitRequest, err := test_utils.CreateJitRequest(ctx, reconciler.Client, 10, test_utils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			// Create jira ticket
			ticket, err := reconciler.createJiraTicket(ctx, jitRequest, jiraProject, jiraIssueType, customFieldsConfig, requiredFieldsConfig, ticketLabels, targetEnvironment)
			Expect(err).NotTo(HaveOccurred())

			jitRequest.Status.JiraTicket = ticket
			err = reconciler.getJiraApproval(ctx, jitRequest, "Not Approved")

			Expect(err).To(HaveOccurred())
		})
	})
})
