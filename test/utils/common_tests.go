package utils

import (
	"context"
	"fmt"

	justintimev1 "jira-jit-rbac-operator/api/v1"

	//lint:ignore ST1001 for ginko
	. "github.com/onsi/ginkgo/v2" //nolint:golint,revive
	//lint:ignore ST1001 for ginko
	. "github.com/onsi/gomega" //nolint:golint,revive
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	config "sigs.k8s.io/controller-runtime/pkg/client/config"
)

const (
	JitRequestName                    = "e2e-jit-test"
	RoleBindingName                   = JitRequestName + "-jit"
	ValidClusterRole           string = "edit"
	InvalidClusterRole                = "admin"
	TestJiraWorkflowToDoStatus        = "ToDo"
	TestJiraWorkflowApproved          = "Approved"
	EventValidationFailed             = "ValidationFailed"
	StatusRejected                    = "Rejected"
	StatusPreApproved                 = "Pre-Approved"
	StatusSucceeded                   = "Succeeded"
	Skipped                           = "Skipped"
)

var k8sClient client.Client
var ctx context.Context

func JitRequestTests(namespace string) {

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
			err := CreateJitConfig(ctx, k8sClient, ValidClusterRole, namespace)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When creating a new valid JitRequest with a start time 10s from now", func() {
		It("should successfully process as a new request and issue a rolebinding", func() {
			By("Creating and approving the JitRequest")
			IssueStatus = TestJiraWorkflowApproved
			jitRequest, err := CreateJitRequest(ctx, k8sClient, 10, ValidClusterRole, namespace)
			Expect(err).NotTo(HaveOccurred())

			By("Checking the status of the JitRequest for Pre-Approval")
			err = CheckJitStatus(ctx, k8sClient, jitRequest, StatusPreApproved)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for the JitRequest Pre-Approval event to be recorded")
			err = CheckEvent(
				ctx,
				k8sClient,
				JitRequestName,
				namespace,
				"Normal",
				StatusPreApproved,
				"ClusterRole 'edit' is allowed\nJira: IAM-1",
			)
			Expect(err).NotTo(HaveOccurred())

			By("Checking the status of the JitRequest for completed status")
			err = CheckJitStatus(ctx, k8sClient, jitRequest, StatusSucceeded)
			Expect(err).NotTo(HaveOccurred())

			By("Checking the RoleBinding exists")
			err = CheckRoleBindingExists(ctx, k8sClient, namespace, RoleBindingName)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should successfully remove the JitRequest on expiry and remove the RoleBinding", func() {
			By("Checking the RoleBinding is eventually removed")
			err := CheckRoleBindingRemoved(ctx, k8sClient, namespace, RoleBindingName)
			Expect(err).NotTo(HaveOccurred())

			By("Checking the JitRequest is eventually removed")
			err = CheckJitRemoved(ctx, k8sClient, JitRequestName)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When creating a new JitRequest with invalid start time from now", func() {
		It("should successfully process as a new request and reject the JitRequest", func() {
			By("Creating the JitRequest")
			IssueStatus = TestJiraWorkflowToDoStatus
			_, err := CreateJitRequest(ctx, k8sClient, -10, ValidClusterRole, namespace)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for the JitRequest Rejected event to be recorded")
			err = CheckEvent(
				ctx,
				k8sClient,
				JitRequestName,
				namespace,
				"Warning",
				EventValidationFailed,
				"must be after current time",
			)
			Expect(err).NotTo(HaveOccurred())

			By("Checking the JitRequest is eventually removed")
			err = CheckJitRemoved(ctx, k8sClient, JitRequestName)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When creating a new JitRequest with invalid cluster role", func() {
		It("should successfully process as a new request and reject the JitRequest", func() {
			By("Creating the JitRequest")
			IssueStatus = TestJiraWorkflowToDoStatus
			_, err := CreateJitRequest(ctx, k8sClient, 10, InvalidClusterRole, namespace)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for the JitRequest Rejected event to be recorded")
			err = CheckEvent(
				ctx,
				k8sClient,
				JitRequestName,
				namespace,
				"Warning",
				EventValidationFailed,
				fmt.Sprintf("ClusterRole '%s' is not allowed", InvalidClusterRole),
			)
			Expect(err).NotTo(HaveOccurred())

			By("Checking the JitRequest is eventually removed")
			err = CheckJitRemoved(ctx, k8sClient, JitRequestName)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When creating a new valid JitRequest with a start time 10s from now and not approving in Jira", func() {
		It("should successfully process as a new request and reject the Jira and JitRequest", func() {
			By("Creating and not approving the JitRequest")
			IssueStatus = TestJiraWorkflowToDoStatus
			jitRequest, err := CreateJitRequest(ctx, k8sClient, 10, ValidClusterRole, namespace)
			Expect(err).NotTo(HaveOccurred())

			By("Checking the status of the JitRequest for Pre-Approval")
			err = CheckJitStatus(ctx, k8sClient, jitRequest, StatusPreApproved)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for the JitRequest Rejected event to be recorded")
			err = CheckEvent(
				ctx,
				k8sClient,
				JitRequestName,
				namespace,
				"Warning",
				"JiraNotApproved",
				"Error: failed on jira approval",
			)
			Expect(err).NotTo(HaveOccurred())

			By("Checking the JitRequest is eventually removed")
			err = CheckJitRemoved(ctx, k8sClient, JitRequestName)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When creating a new JitRequest with a start time 10s from now "+
		"and for a Namespace with non-matching label", func() {
		It("should successfully process as a new request and reject the Jira and JitRequest", func() {
			By("Creating and approving the JitRequest with a label match")
			IssueStatus = TestJiraWorkflowApproved
			namespaceLabel := "bar"
			_, err := CreateJitRequest(ctx, k8sClient, 10, ValidClusterRole, namespace, namespaceLabel)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for the JitRequest Rejected event to be recorded")
			err = CheckEvent(
				ctx,
				k8sClient,
				JitRequestName,
				namespace,
				"Warning",
				EventValidationFailed,
				fmt.Sprintf(
					"Namespace(s) %s not validated | Error: the following namespaces do not match the specified labels (foo=%s): [%s]",
					namespace,
					namespaceLabel,
					namespace,
				))
			Expect(err).NotTo(HaveOccurred())

			By("Checking the JitRequest is eventually removed")
			err = CheckJitRemoved(ctx, k8sClient, JitRequestName)
			Expect(err).NotTo(HaveOccurred())
		})
	})
}
