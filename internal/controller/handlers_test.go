package controller

import (
	"errors"
	"fmt"
	v1 "jira-jit-rbac-operator/api/v1"
	"jira-jit-rbac-operator/internal/config"
	testUtils "jira-jit-rbac-operator/test/utils"
	"os/exec"
	"regexp"
	"time"

	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const JiraTicket = "IAM-1"

var _ = Describe("JitRequestReconciler handlers Unit Tests", Ordered, Label("unit", "handlers"), func() {

	var reconciler *JitRequestReconciler
	l := log.FromContext(ctx)
	var fakeRecorder *record.FakeRecorder

	// test jitConfig
	allowedClusterRoles := []string{"edit"}
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

	Describe("handleNewRequest", func() {

		It("should handle and pre-approve a new valid JitRequest", func() {
			// Create JitRequest
			jitRequest, err := testUtils.CreateJitRequest(ctx, reconciler.Client, 10, testUtils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			By("Checking the jitRequest is re-queued for startTime")
			result, err := reconciler.handleNewRequest(ctx, l, jitRequest, allowedClusterRoles, jiraProject, jiraIssueType, customFieldsConfig, requiredFieldsConfig, ticketLabels, targetEnvironment, additionalComments)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.IsZero()).To(BeFalse())

			By("Checking the jitRequest status is pre-approved")
			namespacedName := types.NamespacedName{
				Name: "e2e-jit-test",
			}
			err = reconciler.Get(ctx, namespacedName, jitRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(jitRequest.Status.State).To(Equal(StatusPreApproved))
			Expect(jitRequest.Status.Message).To(Equal("Pre-approval - Access will be granted at start time pending human approval(s)"))
			Expect(jitRequest.Status.JiraTicket).To(Equal(JiraTicket))
		})

		It("should return if missing jira field", func() {
			// Create JitRequest
			jitRequest, err := testUtils.CreateJitRequest(ctx, reconciler.Client, 10, testUtils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			By("Checking controller returns with no error")
			missingCustomFieldsConfig := map[string]v1.CustomFieldSettings{
				"MissingField": {Type: "user", JiraCustomField: "customfield_10114"},
			}
			result, err := reconciler.handleNewRequest(ctx, l, jitRequest, allowedClusterRoles, jiraProject, jiraIssueType, missingCustomFieldsConfig, requiredFieldsConfig, ticketLabels, targetEnvironment, additionalComments)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.IsZero()).To(BeTrue())

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

		It("should return rejectInvalidRole if invalid cluster role", func() {
			// Create JitRequest
			jitRequest, err := testUtils.CreateJitRequest(ctx, reconciler.Client, 10, testUtils.InvalidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			By("Checking controller returns with no error")
			result, err := reconciler.handleNewRequest(ctx, l, jitRequest, allowedClusterRoles, jiraProject, jiraIssueType, customFieldsConfig, requiredFieldsConfig, ticketLabels, targetEnvironment, additionalComments)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.IsZero()).To(BeTrue())

			By("Checking the jitRequest status is rejected")
			namespacedName := types.NamespacedName{
				Name: "e2e-jit-test",
			}
			err = reconciler.Get(ctx, namespacedName, jitRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(jitRequest.Status.State).To(Equal(StatusRejected))
			Expect(jitRequest.Status.Message).To(Equal("ClusterRole 'admin' is not allowed"))
			Expect(jitRequest.Status.JiraTicket).To(Equal(JiraTicket))
		})

		It("should return rejectInvalidNamespace if invalid namespace labels", func() {
			// Create JitRequest
			jitRequest, err := testUtils.CreateJitRequest(ctx, reconciler.Client, 10, testUtils.ValidClusterRole, TestNamespace, "bar")
			Expect(err).NotTo(HaveOccurred())

			By("Checking controller returns with no error")
			result, err := reconciler.handleNewRequest(ctx, l, jitRequest, allowedClusterRoles, jiraProject, jiraIssueType, customFieldsConfig, requiredFieldsConfig, ticketLabels, targetEnvironment, additionalComments)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.IsZero()).To(BeTrue())

			By("Checking the jitRequest status is rejected")
			namespacedName := types.NamespacedName{
				Name: "e2e-jit-test",
			}
			err = reconciler.Get(ctx, namespacedName, jitRequest)
			message := "Namespace(s) jira-jit-int-test not validated | Error: the following namespaces do not match the specified labels (foo=bar): [jira-jit-int-test]"
			Expect(err).NotTo(HaveOccurred())
			Expect(jitRequest.Status.State).To(Equal(StatusRejected))
			Expect(jitRequest.Status.Message).To(Equal(message))
			Expect(jitRequest.Status.JiraTicket).To(Equal(JiraTicket))
		})

		It("should return rejectInvalidNamespace namespace(s) do(es) not match regex defined in config", func() {
			config.NamespaceAllowedRegex = regexp.MustCompile(`^valid-.*`)

			// Create JitRequest
			jitRequest, err := testUtils.CreateJitRequest(ctx, reconciler.Client, 10, testUtils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			By("Checking controller returns with no error")
			result, err := reconciler.handleNewRequest(ctx, l, jitRequest, allowedClusterRoles, jiraProject, jiraIssueType, customFieldsConfig, requiredFieldsConfig, ticketLabels, targetEnvironment, additionalComments)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.IsZero()).To(BeTrue())

			By("Checking the jitRequest status is rejected")
			namespacedName := types.NamespacedName{
				Name: "e2e-jit-test",
			}
			err = reconciler.Get(ctx, namespacedName, jitRequest)
			message := "Namespace(s) jira-jit-int-test not validated | Error: spec.namespace: Invalid value: \"jira-jit-int-test\": namespace does not match the allowed pattern: ^valid-.*"
			Expect(err).NotTo(HaveOccurred())
			Expect(jitRequest.Status.State).To(Equal(StatusRejected))
			Expect(jitRequest.Status.Message).To(Equal(message))
			Expect(jitRequest.Status.JiraTicket).To(Equal(JiraTicket))
		})
	})

	Describe("handlePreApproved", func() {

		It("should re-queue JitRequest if startTime is not met", func() {
			// Create JitRequest
			jitRequest, err := testUtils.CreateJitRequest(ctx, reconciler.Client, 100, testUtils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			By("Checking the jitRequest is re-queued for startTime")
			jitRequest.Status.StartTime.Time = jitRequest.Spec.StartTime.Time
			result, err := reconciler.handlePreApproved(ctx, l, jitRequest, "", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.IsZero()).To(BeFalse())
		})

		It("should return nil if call to getJiraApproval fails", func() {
			// Create JitRequest
			jitRequest, err := testUtils.CreateJitRequest(ctx, reconciler.Client, 10, testUtils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			By("Checking controller returns with no error")
			result, err := reconciler.handlePreApproved(ctx, l, jitRequest, "", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.IsZero()).To(BeTrue())

			By("Checking the jitRequest status is rejected")
			namespacedName := types.NamespacedName{
				Name: "e2e-jit-test",
			}
			err = reconciler.Get(ctx, namespacedName, jitRequest)
			message := "Jira ticket has not been approved"
			Expect(err).NotTo(HaveOccurred())
			Expect(jitRequest.Status.State).To(Equal(StatusRejected))
			Expect(jitRequest.Status.Message).To(Equal(message))
			Expect(jitRequest.Status.JiraTicket).To(Equal(""))
		})

		It("should handle a valid pre-approved JitRequest", func() {
			// Create JitRequest
			jitRequest, err := testUtils.CreateJitRequest(ctx, reconciler.Client, 0, testUtils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			By("Approving the JitRequest")
			jitRequest.Status.StartTime.Time = jitRequest.Spec.StartTime.Time
			jitRequest.Status.JiraTicket = JiraTicket
			jiraWorkflowApproved := "Approved"
			testUtils.IssueStatus = jiraWorkflowApproved

			By("Checking the jitRequest is re-queued for clean-up")
			result, err := reconciler.handlePreApproved(ctx, l, jitRequest, "10", jiraWorkflowApproved)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.IsZero()).To(BeFalse())

			By("Checking the jitRequest status is completed")
			namespacedName := types.NamespacedName{
				Name: "e2e-jit-test",
			}
			err = reconciler.Get(ctx, namespacedName, jitRequest)
			message := "Access granted until end time"
			Expect(err).NotTo(HaveOccurred())
			Expect(jitRequest.Status.State).To(Equal(StatusSucceeded))
			Expect(jitRequest.Status.Message).To(Equal(message))
			Expect(jitRequest.Status.JiraTicket).To(Equal(JiraTicket))

			By("checking role binding exists")
			rbName := fmt.Sprintf("%s-jit", jitRequest.Name)
			rb := &rbacv1.RoleBinding{}
			rbNamespacedName := types.NamespacedName{
				Namespace: TestNamespace,
				Name:      rbName,
			}
			err = reconciler.Get(ctx, rbNamespacedName, rb)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("handleRejected", func() {

		It("should fail to rejected an invalid Jira Ticket", func() {
			// Create JitRequest
			jitRequest, err := testUtils.CreateJitRequest(ctx, reconciler.Client, 10, testUtils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			jitRequest.Status.State = "Rejected"
			jitRequest.Status.JiraTicket = "IAM-BAD"

			result, err := reconciler.handleRejected(ctx, l, jitRequest, "1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no atlassian resource found"))
			Expect(result.IsZero()).To(BeTrue())
		})

		It("should handle a rejected JitRequest", func() {
			// Create JitRequest
			jitRequest, err := testUtils.CreateJitRequest(ctx, reconciler.Client, 10, testUtils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			By("Rejecting the JitRequest")
			jitRequest.Status.State = "Rejected"
			jitRequest.Status.JiraTicket = JiraTicket
			result, err := reconciler.handleRejected(ctx, l, jitRequest, "1")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.IsZero()).To(BeTrue())

			By("Checking the JitRequest is eventually removed")
			err = testUtils.CheckJitRemoved(ctx, k8sClient, JitRequestName)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("handleCleaup", func() {

		It("should requeue a non-expired JitRequest", func() {
			// Create JitRequest
			jitRequest, err := testUtils.CreateJitRequest(ctx, reconciler.Client, 10, testUtils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			jitRequest.Status.EndTime = metav1.NewTime(metav1.Now().Add(10 * time.Second))
			result, err := reconciler.handleCleaup(ctx, l, jitRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.IsZero()).To(BeFalse())
		})

		It("should handle a expired JitRequest", func() {
			// Create JitRequest
			jitRequest, err := testUtils.CreateJitRequest(ctx, reconciler.Client, 10, testUtils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			By("Simulating an expired JitRequest")
			jitRequest.Status.EndTime = metav1.NewTime(metav1.Now().Add(-1 * time.Second))
			result, err := reconciler.handleCleaup(ctx, l, jitRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.IsZero()).To(BeTrue())

			By("Checking the JitRequest is eventually removed")
			err = testUtils.CheckJitRemoved(ctx, k8sClient, JitRequestName)
			Expect(err).NotTo(HaveOccurred())
		})
	})
	Describe("handleFetchError", func() {
		jitRequest := &v1.JitRequest{}
		notFoundErr := apierrors.NewNotFound(schema.GroupResource{Group: "justintimev1", Resource: "JitRequest"}, "test")
		otherErr := errors.New("some other error")

		It("should ignore NotFound error", func() {
			result, err := reconciler.handleFetchError(ctx, l, notFoundErr, jitRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})

		It("should return other errors", func() {
			result, err := reconciler.handleFetchError(ctx, l, otherErr, jitRequest)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal(otherErr.Error()))
			Expect(result).To(Equal(ctrl.Result{}))
		})
	})
})
