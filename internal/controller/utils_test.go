package controller

import (
	v1 "jira-jit-rbac-operator/api/v1"
	test_utils "jira-jit-rbac-operator/test/utils"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("JitRequestReconciler utils Unit Tests", Ordered, Label("unit", "utils"), func() {

	var reconciler *JitRequestReconciler
	var l = log.FromContext(ctx)
	var globalJitRequest = &v1.JitRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name: "e2e-jit-test",
			UID:  "foo",
		},
		Spec: v1.JitRequestSpec{
			ClusterRole: test_utils.ValidClusterRole,
			Reporter:    "master-chief@unsc.com",
			Namespaces: []string{
				TestNamespace,
			},
			AdditionUserEmails: []string{
				"foo@foo.com",
			},
			NamespaceLabels: make(map[string]string),
			StartTime:       metav1.NewTime(metav1.Now().Add(10 * time.Second)),
			EndTime:         metav1.NewTime(metav1.Now().Add(20 * time.Second)),
			JiraFields: map[string]string{
				"Approver":      "cptKeyes",
				"ProductOwner":  "Oni",
				"Justification": "I need a weapon",
			},
		},
	}

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

	Describe("fetchJitRequest", func() {

		It("should fetch a JitRequest successfully", func() {
			// Create JitRequest
			_, err := test_utils.CreateJitRequest(ctx, reconciler.Client, 10, test_utils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			namespacedName := types.NamespacedName{
				Name: "e2e-jit-test",
			}
			result, err := reconciler.fetchJitRequest(ctx, namespacedName)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Name).To(Equal("e2e-jit-test"))
		})
	})

	Describe("updateStatus", func() {

		It("should update a JitRequest status successfully", func() {
			// Create JitRequest
			jitRequest, err := test_utils.CreateJitRequest(ctx, reconciler.Client, 10, test_utils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			// Check updateStatus
			status := "Completed"
			message := "Test updateStatus"
			jiraTicket := "IAM-1"
			err = reconciler.updateStatus(ctx, jitRequest, status, message, jiraTicket)
			Expect(err).NotTo(HaveOccurred())

			namespacedName := types.NamespacedName{
				Name: "e2e-jit-test",
			}
			updatedJitRequest := &v1.JitRequest{}
			err = reconciler.Get(ctx, namespacedName, updatedJitRequest)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedJitRequest.Status.State).To(Equal(status))
			Expect(updatedJitRequest.Status.Message).To(Equal(message))
			Expect(updatedJitRequest.Status.JiraTicket).To(Equal(jiraTicket))
		})
	})

	Describe("deleteJitRequest", func() {

		It("should delete a JitRequest successfully", func() {
			// Create JitRequest
			jitRequest, err := test_utils.CreateJitRequest(ctx, reconciler.Client, 10, test_utils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			err = reconciler.deleteJitRequest(ctx, jitRequest)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should error if failed to delete a JitRequest", func() {
			jitRequest := &v1.JitRequest{}
			err := reconciler.deleteJitRequest(ctx, jitRequest)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("resource name may not be empty"))
		})
	})

	Describe("raiseEvent", func() {

		It("should record an event successfully", func() {
			reconciler.raiseEvent(globalJitRequest, "Warning", "JiraNotApproved", "raiseEvent test")
		})
	})

	Describe("rejectInvalidNamespace", func() {

		It("should reject invalid namespaces", func() {
			// Create JitRequest
			jitRequest, err := test_utils.CreateJitRequest(ctx, reconciler.Client, 10, test_utils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			_, err = reconciler.rejectInvalidNamespace(ctx, l, jitRequest, "jiraIssueKey", "namespace", "error")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should error if failed status update in rejectInvalidNamespace", func() {
			jitRequest := &v1.JitRequest{}
			_, err := reconciler.rejectInvalidNamespace(ctx, l, jitRequest, "jiraIssueKey", "namespace", "error")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("failed to update JitRequest status: resource name may not be empty"))
		})
	})

	Describe("rejectInvalidRole", func() {

		It("should reject invalid cluster role", func() {

			// Create JitRequest
			jitRequest, err := test_utils.CreateJitRequest(ctx, reconciler.Client, 10, test_utils.ValidClusterRole, TestNamespace)
			Expect(err).NotTo(HaveOccurred())

			_, err = reconciler.rejectInvalidRole(ctx, l, jitRequest, "jiraIssueKey")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should error if failed status update in rejectInvalidRole", func() {
			jitRequest := &v1.JitRequest{}
			_, err := reconciler.rejectInvalidRole(ctx, l, jitRequest, "jiraIssueKey")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("failed to update JitRequest status: resource name may not be empty"))
		})
	})

	Describe("createRoleBinding", func() {

		It("should create role bindings", func() {
			// Create role binding
			err := reconciler.createRoleBinding(ctx, globalJitRequest)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("deleteOwnedObjects", func() {

		It("should delete role bindings", func() {
			// Create role binding
			jitRequest := globalJitRequest
			jitRequest.ObjectMeta.UID = "deleteOwnedObjects"
			jitRequest.ObjectMeta.Name = "deleteOwnedObjects"
			err := reconciler.createRoleBinding(ctx, jitRequest)
			Expect(err).NotTo(HaveOccurred())

			// deleteOwnedObjects
			err = reconciler.deleteOwnedObjects(ctx, jitRequest)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
