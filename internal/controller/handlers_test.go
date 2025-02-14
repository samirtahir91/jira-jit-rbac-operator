package controller

import (
	"errors"
	v1 "jira-jit-rbac-operator/api/v1"
	"jira-jit-rbac-operator/internal/config"
	test_utils "jira-jit-rbac-operator/test/utils"
	"os/exec"
	"regexp"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

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

	// test config
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

	Describe("handleNewRequest", func() {

		It("should handle a new JitRequest", func() {
			// Create JitRequest
			jitRequest, err := test_utils.CreateJitRequest(ctx, reconciler.Client, 10, test_utils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			result, err := reconciler.handleNewRequest(ctx, l, jitRequest, allowedClusterRoles, jiraProject, jiraIssueType, customFieldsConfig, requiredFieldsConfig, ticketLabels, targetEnvironment, additionalComments)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.IsZero()).To(BeFalse())
		})

		It("should return if missing jira field", func() {
			// Create JitRequest
			jitRequest, err := test_utils.CreateJitRequest(ctx, reconciler.Client, 10, test_utils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			missingCustomFieldsConfig := map[string]v1.CustomFieldSettings{
				"MissingField": {Type: "user", JiraCustomField: "customfield_10114"},
			}
			result, err := reconciler.handleNewRequest(ctx, l, jitRequest, allowedClusterRoles, jiraProject, jiraIssueType, missingCustomFieldsConfig, requiredFieldsConfig, ticketLabels, targetEnvironment, additionalComments)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.IsZero()).To(BeTrue())
		})

		It("should return rejectInvalidRole if invalid cluster role", func() {
			// Create JitRequest
			jitRequest, err := test_utils.CreateJitRequest(ctx, reconciler.Client, 10, test_utils.InvalidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			result, err := reconciler.handleNewRequest(ctx, l, jitRequest, allowedClusterRoles, jiraProject, jiraIssueType, customFieldsConfig, requiredFieldsConfig, ticketLabels, targetEnvironment, additionalComments)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.IsZero()).To(BeTrue())
		})

		It("should return rejectInvalidNamespace if invalid namespace labels", func() {
			// Create JitRequest
			jitRequest, err := test_utils.CreateJitRequest(ctx, reconciler.Client, 10, test_utils.ValidClusterRole, TestNamespace, "bar")
			Expect(err).NotTo(HaveOccurred())

			result, err := reconciler.handleNewRequest(ctx, l, jitRequest, allowedClusterRoles, jiraProject, jiraIssueType, customFieldsConfig, requiredFieldsConfig, ticketLabels, targetEnvironment, additionalComments)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.IsZero()).To(BeTrue())
		})

		It("should return rejectInvalidNamespace namespace(s) do(es) not match regex defined in config", func() {
			config.NamespaceAllowedRegex = regexp.MustCompile(`^valid-.*`)

			// Create JitRequest
			jitRequest, err := test_utils.CreateJitRequest(ctx, reconciler.Client, 10, test_utils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			result, err := reconciler.handleNewRequest(ctx, l, jitRequest, allowedClusterRoles, jiraProject, jiraIssueType, customFieldsConfig, requiredFieldsConfig, ticketLabels, targetEnvironment, additionalComments)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.IsZero()).To(BeTrue())
		})
	})

	Describe("handlePreApproved", func() {

		It("should requeue JitRequest if startTime is not met", func() {
			// Create JitRequest
			jitRequest, err := test_utils.CreateJitRequest(ctx, reconciler.Client, 100, test_utils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			jitRequest.Status.StartTime.Time = jitRequest.Spec.StartTime.Time
			result, err := reconciler.handlePreApproved(ctx, l, jitRequest, "", "")

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.IsZero()).To(BeFalse())
		})

		It("should return nil if call to getJiraApproval fails", func() {
			// Create JitRequest
			jitRequest, err := test_utils.CreateJitRequest(ctx, reconciler.Client, 10, test_utils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			result, err := reconciler.handlePreApproved(ctx, l, jitRequest, "", "")

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.IsZero()).To(BeTrue())
		})

		It("should handle a valid pre-approved JitRequest", func() {
			// Create JitRequest
			jitRequest, err := test_utils.CreateJitRequest(ctx, reconciler.Client, 0, test_utils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			jitRequest.Status.StartTime.Time = jitRequest.Spec.StartTime.Time
			jitRequest.Status.JiraTicket = JiraTicket
			jiraWorkflowApproved := "Approved"
			test_utils.IssueStatus = jiraWorkflowApproved

			result, err := reconciler.handlePreApproved(ctx, l, jitRequest, "10", jiraWorkflowApproved)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.IsZero()).To(BeFalse())
		})
	})

	Describe("handleRejected", func() {

		It("should fail to rejected an invalid Jira Ticket", func() {
			// Create JitRequest
			jitRequest, err := test_utils.CreateJitRequest(ctx, reconciler.Client, 10, test_utils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			jitRequest.Status.State = "Rejected"
			jitRequest.Status.JiraTicket = "IAM-BAD"

			result, err := reconciler.handleRejected(ctx, l, jitRequest, "1")
			Expect(err).To(HaveOccurred())
			Expect(result.IsZero()).To(BeTrue())
		})

		It("should handle a rejected JitRequest", func() {
			// Create JitRequest
			jitRequest, err := test_utils.CreateJitRequest(ctx, reconciler.Client, 10, test_utils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			jitRequest.Status.State = "Rejected"
			jitRequest.Status.JiraTicket = JiraTicket

			result, err := reconciler.handleRejected(ctx, l, jitRequest, "1")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.IsZero()).To(BeTrue())

			By("Checking the JitRequest is eventually removed")
			err = test_utils.CheckJitRemoved(ctx, k8sClient, JitRequestName)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("handleCleaup", func() {

		It("should requeue a non-expired JitRequest", func() {
			// Create JitRequest
			jitRequest, err := test_utils.CreateJitRequest(ctx, reconciler.Client, 10, test_utils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			jitRequest.Status.EndTime = metav1.NewTime(metav1.Now().Add(10 * time.Second))

			result, err := reconciler.handleCleaup(ctx, l, jitRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.IsZero()).To(BeFalse())
		})

		It("should handle a expired JitRequest", func() {
			// Create JitRequest
			jitRequest, err := test_utils.CreateJitRequest(ctx, reconciler.Client, 10, test_utils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			jitRequest.Status.EndTime = metav1.NewTime(metav1.Now().Add(-1 * time.Second))

			result, err := reconciler.handleCleaup(ctx, l, jitRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.IsZero()).To(BeTrue())

			By("Checking the JitRequest is eventually removed")
			err = test_utils.CheckJitRemoved(ctx, k8sClient, JitRequestName)
			Expect(err).NotTo(HaveOccurred())
		})
	})
	Describe("handleCleaup", func() {
		jitRequest := &v1.JitRequest{}
		notFoundErr := apierrors.NewNotFound(schema.GroupResource{Group: "justintimev1", Resource: "JitRequest"}, "test")
		otherErr := errors.New("some other error")

		It("should handle NotFound error", func() {
			result, err := reconciler.handleFetchError(ctx, l, notFoundErr, jitRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})

		It("should handle other errors", func() {
			result, err := reconciler.handleFetchError(ctx, l, otherErr, jitRequest)
			Expect(err).To(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})
	})
})
