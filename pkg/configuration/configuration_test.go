package configuration

import (
	"context"

	justintimev1 "jira-jit-rbac-operator/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("JitRbacOperatorConfiguration", func() {
	var (
		ctx        context.Context
		k8sClient  client.Client
		configName string
		scheme     *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.TODO()
		configName = "test-config"
		scheme = runtime.NewScheme()
		Expect(justintimev1.AddToScheme(scheme)).To(Succeed())
	})

	It("should return a default configuration if the config is not found", func() {
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
		config := NewJitRbacOperatorConfiguration(ctx, k8sClient, configName)

		Expect(config.AllowedClusterRoles()).To(Equal([]string{"edit"}))
		Expect(config.JiraWorkflowApproveStatus()).To(Equal("Approved"))
		Expect(config.JiraProject()).To(Equal("IAM"))
		Expect(config.JiraIssueType()).To(Equal("Access Request"))
		Expect(config.CompletedTransitionID()).To(Equal("41"))
		Expect(config.AdditionalCommentText()).To(Equal("config: default"))
		Expect(config.NamespaceAllowedRegex()).To(Equal(".*"))
		Expect(config.Labels()).To(Equal([]string{"default-config"}))
		Expect(config.Environment()).To(Equal(&justintimev1.EnvironmentSpec{
			Environment: "dev-test",
			Cluster:     "minikube",
		}))
		Expect(config.RequiredFields()).To(Equal(&justintimev1.RequiredFieldsSpec{
			StartTime:   justintimev1.CustomFieldSettings{Type: "date", JiraCustomField: "customfield_10118"},
			EndTime:     justintimev1.CustomFieldSettings{Type: "date", JiraCustomField: "customfield_10119"},
			ClusterRole: justintimev1.CustomFieldSettings{Type: "date", JiraCustomField: "customfield_10117"},
		}))
		Expect(config.CustomFields()).To(Equal(map[string]justintimev1.CustomFieldSettings{
			"Approver":      {Type: "user", JiraCustomField: "customfield_10114"},
			"ProductOwner":  {Type: "user", JiraCustomField: "customfield_10115"},
			"Justification": {Type: "text", JiraCustomField: "customfield_10116"},
		}))
		Expect(config.SelfApprovalEnabled()).To(BeFalse())
	})

	It("should return the retrieved configuration if found", func() {
		expectedConfig := &justintimev1.JustInTimeConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: configName,
			},
			Spec: justintimev1.JustInTimeConfigSpec{
				AllowedClusterRoles:       []string{"admin"},
				JiraWorkflowApproveStatus: "Approved",
				RejectedTransitionID:      "22",
				JiraProject:               "IAM",
				JiraIssueType:             "Access Request",
				CompletedTransitionID:     "42",
				AdditionalCommentText:     "config: custom",
				NamespaceAllowedRegex:     ".*",
				Labels: []string{
					"custom-config",
				},
				Environment: &justintimev1.EnvironmentSpec{
					Environment: "prod",
					Cluster:     "k8s",
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
				SelfApprovalEnabled: true,
			},
		}

		k8sClient = fake.NewClientBuilder().WithScheme(scheme).WithObjects(expectedConfig).Build()
		config := NewJitRbacOperatorConfiguration(ctx, k8sClient, configName)

		Expect(config.AllowedClusterRoles()).To(Equal(expectedConfig.Spec.AllowedClusterRoles))
		Expect(config.JiraWorkflowApproveStatus()).To(Equal(expectedConfig.Spec.JiraWorkflowApproveStatus))
		Expect(config.RejectedTransitionID()).To(Equal(expectedConfig.Spec.RejectedTransitionID))
		Expect(config.JiraProject()).To(Equal(expectedConfig.Spec.JiraProject))
		Expect(config.JiraIssueType()).To(Equal(expectedConfig.Spec.JiraIssueType))
		Expect(config.CompletedTransitionID()).To(Equal(expectedConfig.Spec.CompletedTransitionID))
		Expect(config.AdditionalCommentText()).To(Equal(expectedConfig.Spec.AdditionalCommentText))
		Expect(config.NamespaceAllowedRegex()).To(Equal(expectedConfig.Spec.NamespaceAllowedRegex))
		Expect(config.Labels()).To(Equal(expectedConfig.Spec.Labels))
		Expect(config.Environment()).To(Equal(expectedConfig.Spec.Environment))
		Expect(config.RequiredFields()).To(Equal(expectedConfig.Spec.RequiredFields))
		Expect(config.CustomFields()).To(Equal(expectedConfig.Spec.CustomFields))
		Expect(config.SelfApprovalEnabled()).To(BeTrue())
	})
})
