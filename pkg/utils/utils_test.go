package utils

import (
	"context"
	"fmt"
	v1 "jira-jit-rbac-operator/api/v1"
	"jira-jit-rbac-operator/internal/config"
	"os"
	"regexp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Utils", func() {

	Describe("ReadConfigFromFile", func() {
		var (
			configFilePath string
			configFileName string
		)

		BeforeEach(func() {
			configFilePath = "testdata"
			configFileName = "config.json"
			config.ConfigCacheFilePath = configFilePath
			config.ConfigFile = configFileName
			// Create the testdata directory if it doesn't exist
			err := os.MkdirAll(configFilePath, os.ModePerm)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should read a valid configuration file", func() {
			data := `{"jiraProject": "IAM"}`
			err := os.WriteFile(fmt.Sprintf("%s/%s", configFilePath, configFileName), []byte(data), 0644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := ReadConfigFromFile()
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.JiraProject).To(Equal("IAM"))
		})

		It("should return an error for a non-existent configuration file", func() {
			config.ConfigFile = "non_existent.json"
			_, err := ReadConfigFromFile()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to read configuration file"))
		})

		It("should return an error for an invalid JSON configuration file", func() {
			data := `invalidJSON`
			err := os.WriteFile(fmt.Sprintf("%s/%s", configFilePath, configFileName), []byte(data), 0644)
			Expect(err).NotTo(HaveOccurred())

			_, err = ReadConfigFromFile()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse configuration file"))
		})
	})

	Describe("Contains", func() {
		It("should return true if the slice contains the item", func() {
			slice := []string{"a", "b", "c"}
			item := "b"
			Expect(Contains(slice, item)).To(BeTrue())
		})

		It("should return false if the slice does not contain the item", func() {
			slice := []string{"a", "b", "c"}
			item := "d"
			Expect(Contains(slice, item)).To(BeFalse())
		})

		It("should return false if the slice is empty", func() {
			slice := []string{}
			item := "a"
			Expect(Contains(slice, item)).To(BeFalse())
		})
	})

	Describe("ValidateNamespaceRegex", func() {
		var (
			namespaces []string
		)

		BeforeEach(func() {
			namespaces = []string{"valid-namespace", "invalid-namespace"}
			config.NamespaceAllowedRegex = nil
		})

		It("should return an error if a namespace does not match the regex", func() {
			config.NamespaceAllowedRegex = regexp.MustCompile(`^valid-.*`)
			invalidNamespace, err := ValidateNamespaceRegex(namespaces)
			Expect(err).To(HaveOccurred())
			Expect(invalidNamespace).To(Equal("invalid-namespace"))
		})

		It("should return no error if all namespaces match the regex", func() {
			config.NamespaceAllowedRegex = regexp.MustCompile(`^valid-.*`)
			namespaces = []string{"valid-namespace"}
			invalidNamespace, err := ValidateNamespaceRegex(namespaces)
			Expect(err).NotTo(HaveOccurred())
			Expect(invalidNamespace).To(BeEmpty())
		})

		It("should return no error if no regex is provided", func() {
			invalidNamespace, err := ValidateNamespaceRegex(namespaces)
			Expect(err).NotTo(HaveOccurred())
			Expect(invalidNamespace).To(BeEmpty())
		})
	})

	Describe("ValidateNamespaceLabels", func() {
		var (
			ctx        context.Context
			jitRequest *v1.JitRequest
			k8sClient  client.Client
		)

		BeforeEach(func() {
			ctx = context.TODO()
			jitRequest = &v1.JitRequest{
				Spec: v1.JitRequestSpec{
					NamespaceLabels: map[string]string{"key": "value"},
					Namespaces:      []string{"namespace1", "namespace2"},
				},
			}
			k8sClient = fake.NewClientBuilder().WithObjects(
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "namespace1",
						Labels: map[string]string{"key": "value"},
					},
				},
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "namespace2",
						Labels: map[string]string{"key": "value"},
					},
				},
			).Build()
		})

		It("should return no error if there are no namespace labels", func() {
			jitRequest.Spec.NamespaceLabels = nil
			invalidNamespaces, err := ValidateNamespaceLabels(ctx, jitRequest, k8sClient)
			Expect(err).NotTo(HaveOccurred())
			Expect(invalidNamespaces).To(BeNil())
		})

		It("should return an error if namespaces do not match the labels", func() {
			k8sClient = fake.NewClientBuilder().WithObjects(
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "namespace1",
						Labels: map[string]string{"key": "value"},
					},
				},
			).Build()

			invalidNamespaces, err := ValidateNamespaceLabels(ctx, jitRequest, k8sClient)
			Expect(err).To(HaveOccurred())
			Expect(invalidNamespaces).To(ContainElement("namespace2"))
		})

		It("should return no error if all namespaces match the labels", func() {
			invalidNamespaces, err := ValidateNamespaceLabels(ctx, jitRequest, k8sClient)
			Expect(err).NotTo(HaveOccurred())
			Expect(invalidNamespaces).To(BeNil())
		})
	})

})
