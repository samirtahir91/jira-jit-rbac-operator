package controller

import (
	v1 "jira-jit-rbac-operator/api/v1"
	testUtils "jira-jit-rbac-operator/test/utils"
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("JitRequestReconciler jira_client Unit Tests", Ordered, Label("unit", "jira_client"), func() {

	var reconciler *JitRequestReconciler
	l := log.FromContext(ctx)
	var fakeRecorder *record.FakeRecorder

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

	BeforeEach(func() {
		By("setting a jitRequest reconciler")
		fakeRecorder = record.NewFakeRecorder(10)
		reconciler = &JitRequestReconciler{
			Client:     k8sClient,
			Recorder:   fakeRecorder,
			Scheme:     scheme.Scheme,
			JiraClient: jiraClient,
		}
	})

	BeforeAll(func() {

		By("removing manager config")
		cmd := exec.Command("kubectl", "delete", "jitcfg", TestJitConfig)
		_, _ = testUtils.Run(cmd)

		By("removing jitRequest")
		cmd = exec.Command("kubectl", "delete", "jitreq", JitRequestName)
		_, _ = testUtils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", TestNamespace)
		_, _ = testUtils.Run(cmd)

		By("creating manager namespace")
		err := testUtils.CreateNamespace(ctx, k8sClient, TestNamespace)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		By("removing manager namespace")
		cmd := exec.Command("kubectl", "delete", "ns", TestNamespace)
		_, _ = testUtils.Run(cmd)

		By("removing manager config")
		cmd = exec.Command("kubectl", "delete", "jitcfg", TestJitConfig)
		_, _ = testUtils.Run(cmd)

		By("removing jitRequest")
		cmd = exec.Command("kubectl", "delete", "jitreq", JitRequestName)
		_, _ = testUtils.Run(cmd)
	})

	AfterEach(func() {
		By("removing jitRequest")
		cmd := exec.Command("kubectl", "delete", "jitreq", JitRequestName)
		_, _ = testUtils.Run(cmd)
	})

	Describe("preApproveRequest", func() {

		It("should reject if startTime has exceeded current time", func() {
			By("Simulating an expired start time")
			jitRequest, err := testUtils.CreateJitRequest(ctx, reconciler.Client, -1, testUtils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			By("Attempting to pre-approve with invlaid start time")
			result, err := reconciler.preApproveRequest(ctx, l, jitRequest, JiraTicket, additionalComments)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.IsZero()).To(BeTrue())

			By("Checking the jitRequest status is rejected")
			namespacedName := types.NamespacedName{
				Name: "e2e-jit-test",
			}
			err = reconciler.Get(ctx, namespacedName, jitRequest)
			message := "must be after current time"
			Expect(err).NotTo(HaveOccurred())
			Expect(jitRequest.Status.State).To(Equal(StatusRejected))
			Expect(jitRequest.Status.Message).To(ContainSubstring(message))
			Expect(jitRequest.Status.JiraTicket).To(Equal(JiraTicket))
		})

		It("should pre-approve valid JitRequests", func() {
			By("Simulating a valid JitRequest")
			jitRequest, err := testUtils.CreateJitRequest(ctx, reconciler.Client, 10, testUtils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			By("Creating a Jira ticket")
			ticket, err := reconciler.createJiraTicket(ctx, jitRequest, jiraProject, jiraIssueType, customFieldsConfig, requiredFieldsConfig, ticketLabels, targetEnvironment)
			Expect(err).NotTo(HaveOccurred())

			By("Attempting to pre-approve a valid JitRequest")
			result, err := reconciler.preApproveRequest(ctx, l, jitRequest, ticket, additionalComments)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.IsZero()).To(BeFalse())

			By("Checking the jitRequest status is pre-approved")
			namespacedName := types.NamespacedName{
				Name: "e2e-jit-test",
			}
			err = reconciler.Get(ctx, namespacedName, jitRequest)
			message := "Pre-approval - Access will be granted at start time pending human approval(s)"
			Expect(err).NotTo(HaveOccurred())
			Expect(jitRequest.Status.State).To(Equal(StatusPreApproved))
			Expect(jitRequest.Status.Message).To(ContainSubstring(message))
			Expect(jitRequest.Status.JiraTicket).To(Equal(JiraTicket))
		})
	})

	Describe("createJiraTicket", func() {

		It("should create a Jira Ticket", func() {
			By("Simulating a valid JitRequest")
			jitRequest, err := testUtils.CreateJitRequest(ctx, reconciler.Client, 10, testUtils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			By("Creating a Jira ticket")
			result, err := reconciler.createJiraTicket(ctx, jitRequest, jiraProject, jiraIssueType, customFieldsConfig, requiredFieldsConfig, ticketLabels, targetEnvironment)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(JiraTicket))
		})

		It("should return Skipped if missing jira field", func() {
			By("Simulating a valid JitRequest")
			jitRequest, err := testUtils.CreateJitRequest(ctx, reconciler.Client, 10, testUtils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			By("Checking removing a field required and creating a jira ticket")
			missingCustomFieldsConfig := map[string]v1.CustomFieldSettings{
				"MissingField": {Type: "user", JiraCustomField: "customfield_10114"},
			}
			result, err := reconciler.createJiraTicket(ctx, jitRequest, jiraProject, jiraIssueType, missingCustomFieldsConfig, requiredFieldsConfig, ticketLabels, targetEnvironment)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("Skipped"))

			By("Checking the jitRequest status is rejected")
			namespacedName := types.NamespacedName{
				Name: "e2e-jit-test",
			}
			err = reconciler.Get(ctx, namespacedName, jitRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(jitRequest.Status.State).To(Equal(StatusRejected))
			Expect(jitRequest.Status.Message).To(Equal("missing custom field: MissingField"))
			Expect(jitRequest.Status.JiraTicket).To(Equal(Skipped))
		})
	})

	Describe("rejectJiraTicket", func() {

		It("should reject a Jira Ticket", func() {
			By("Simulating a valid JitRequest")
			jitRequest, err := testUtils.CreateJitRequest(ctx, reconciler.Client, 10, testUtils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			By("Creating a jira ticket")
			ticket, err := reconciler.createJiraTicket(ctx, jitRequest, jiraProject, jiraIssueType, customFieldsConfig, requiredFieldsConfig, ticketLabels, targetEnvironment)
			Expect(err).NotTo(HaveOccurred())

			By("Simulating rejecting a ticket")
			jitRequest.Status.JiraTicket = ticket
			jitRequest.Status.Message = "test rejected"
			err = reconciler.rejectJiraTicket(ctx, jitRequest, "1")
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("updateJiraTicket", func() {

		It("should update a Jira Ticket", func() {
			By("Simulating a valid JitRequest")
			jitRequest, err := testUtils.CreateJitRequest(ctx, reconciler.Client, 10, testUtils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			By("Creating a jira ticket")
			ticket, err := reconciler.createJiraTicket(ctx, jitRequest, jiraProject, jiraIssueType, customFieldsConfig, requiredFieldsConfig, ticketLabels, targetEnvironment)
			Expect(err).NotTo(HaveOccurred())

			By("Simulating updating a ticket")
			jitRequest.Status.JiraTicket = ticket
			err = reconciler.updateJiraTicket(ctx, ticket, "test updated")
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("completeJiraTicket", func() {

		It("should complete a Jira Ticket", func() {
			By("Simulating a valid JitRequest")
			jitRequest, err := testUtils.CreateJitRequest(ctx, reconciler.Client, 10, testUtils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			By("Creating a jira ticket")
			ticket, err := reconciler.createJiraTicket(ctx, jitRequest, jiraProject, jiraIssueType, customFieldsConfig, requiredFieldsConfig, ticketLabels, targetEnvironment)
			Expect(err).NotTo(HaveOccurred())

			By("Simulating completing a ticket")
			jitRequest.Status.JiraTicket = ticket
			err = reconciler.completeJiraTicket(ctx, jitRequest, "1")
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("getJiraApproval", func() {

		It("should return nil for an approved jira ticket", func() {
			By("Simulating a valid JitRequest")
			jitRequest, err := testUtils.CreateJitRequest(ctx, reconciler.Client, 10, testUtils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			By("Creating a jira ticket")
			ticket, err := reconciler.createJiraTicket(ctx, jitRequest, jiraProject, jiraIssueType, customFieldsConfig, requiredFieldsConfig, ticketLabels, targetEnvironment)
			Expect(err).NotTo(HaveOccurred())

			By("Approving a jira ticket")
			jitRequest.Status.JiraTicket = ticket
			testUtils.IssueStatus = testUtils.TestJiraWorkflowApproved

			By("Checking getJiraApproval has no error")
			err = reconciler.getJiraApproval(ctx, jitRequest, testUtils.IssueStatus)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return err for a non-approved jira ticket", func() {
			By("Simulating a valid JitRequest")
			jitRequest, err := testUtils.CreateJitRequest(ctx, reconciler.Client, 10, testUtils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			By("Creating a jira ticket")
			ticket, err := reconciler.createJiraTicket(ctx, jitRequest, jiraProject, jiraIssueType, customFieldsConfig, requiredFieldsConfig, ticketLabels, targetEnvironment)
			Expect(err).NotTo(HaveOccurred())

			By("Checking getJiraApproval raises an error")
			jitRequest.Status.JiraTicket = ticket
			err = reconciler.getJiraApproval(ctx, jitRequest, "Not Approved")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed on jira approval"))
		})
	})
})
