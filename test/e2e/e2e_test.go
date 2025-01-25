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

package e2e

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"jira-jit-rbac-operator/test/utils"
)

// namespace where the project is deployed in
const namespace = "jira-jit-rbac-operator-system"

// serviceAccountName created for the project
const serviceAccountName = "v1-jira-jit-rbac-operator-controller-manager"

// metricsServiceName is the name of the metrics service of the project
const metricsServiceName = "v1-jira-jit-rbac-operator-controller-manager-metrics-service"

// metricsRoleBindingName is the name of the RBAC that will be created to allow get the metrics data
const metricsRoleBindingName = "v1-jira-jit-rbac-operator-metrics-binding"

// releaseName of chart
const releaseName = "v1"
const chartPath = "./charts/jira-jit-rbac-operator"

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string

	// Before running the tests, set up the environment by creating the namespace,
	// installing CRDs, and deploying the controller.
	BeforeAll(func() {
		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

		By("creating manager jira secret")
		cmd = exec.Command(
			"kubectl",
			"create",
			"-n",
			namespace,
			"secret",
			"generic",
			"jira-credentials",
			"--from-literal=api-token=dummy",
		)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

		By("creating jira stubs service in k8s")
		cmd = exec.Command("kubectl", "-n", namespace, "create", "-f", "./test/manifests/stub_service.yaml")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create jira stub service in k8s")

		By("deploying the controller-manager with Helm")
		helmArgImg := fmt.Sprintf("controllerManager.manager.image.repository=%s", projectImageRepo)
		helmArgImgTag := fmt.Sprintf("controllerManager.manager.image.tag=%s", projectImageTag)
		var jiraBaseUrl string
		if os.Getenv("TEST_OS") == "mac" {
			// MAC OS
			jiraBaseUrl = "http://host.docker.internal:8082"
		} else {
			// Linux - use docker bridge service
			jiraBaseUrl = "http://dockerhost:8082"
		}
		helmArgJiraUrl := fmt.Sprintf("controllerManager.manager.env.jiraBaseUrl=%s", jiraBaseUrl)
		helmArgEnvWebhook := "controllerManager.manager.env.enableWebhooks=false"
		helmArgGlobalWebhook := "webhook.enabled=false"
		helmSetArg := "--set"
		cmd = exec.Command(
			"helm",
			"install",
			"-n",
			namespace,
			releaseName,
			chartPath,
			helmSetArg,
			helmArgImg,
			helmSetArg,
			helmArgImgTag,
			helmSetArg,
			helmArgJiraUrl,
			helmSetArg,
			helmArgEnvWebhook,
			helmSetArg,
			helmArgGlobalWebhook,
			helmSetArg,
			"controllerManager.manager.args[0]=--metrics-bind-address=:8443",
			helmSetArg,
			"controllerManager.manager.args[1]=--leader-elect",
			helmSetArg,
			"controllerManager.manager.args[2]=--health-probe-bind-address=:8081",
			helmSetArg,
			"controllerManager.manager.args[3]=--configuration-name=jira-jit-rbac-operator-default",
		)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")
	})

	// After all tests have been executed, clean up by undeploying the controller, uninstalling CRDs,
	// and deleting the namespace.
	AfterAll(func() {
		By("cleaning up the curl pod for metrics")
		cmd := exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", namespace)
		_, _ = utils.Run(cmd)

		// By("undeploying the controller-manager")
		// cmd = exec.Command("helm", "delete", "-n", namespace, releaseName)
		// _, _ = utils.Run(cmd)

		// By("removing manager namespace")
		// cmd = exec.Command("kubectl", "delete", "ns", namespace)
		// _, _ = utils.Run(cmd)
	})

	// After each test, check for failures and collect logs, events,
	// and pod descriptions for debugging.
	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			By("Fetching controller manager pod logs")
			cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
			controllerLogs, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerLogs)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Controller logs: %s", err)
			}

			By("Fetching Kubernetes events")
			cmd = exec.Command("kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
			eventsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events:\n%s", eventsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Kubernetes events: %s", err)
			}

			By("Fetching curl-metrics logs")
			cmd = exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
			metricsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Metrics logs:\n %s", metricsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get curl-metrics logs: %s", err)
			}

			By("Fetching controller manager pod description")
			cmd = exec.Command("kubectl", "describe", "pod", controllerPodName, "-n", namespace)
			podDescription, err := utils.Run(cmd)
			if err == nil {
				fmt.Println("Pod description:\n", podDescription)
			} else {
				fmt.Println("Failed to describe controller pod")
			}
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Manager", func() {
		It("should run successfully", func() {
			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func(g Gomega) {
				// Get the name of the controller-manager pod
				cmd := exec.Command("kubectl", "get",
					"pods", "-l", "control-plane=controller-manager",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", namespace,
				)

				podOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve controller-manager pod information")
				podNames := utils.GetNonEmptyLines(podOutput)
				g.Expect(podNames).To(HaveLen(1), "expected 1 controller pod running")
				controllerPodName = podNames[0]
				g.Expect(controllerPodName).To(ContainSubstring("controller-manager"))

				// Validate the pod's status
				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"), "Incorrect controller-manager pod status")
			}
			Eventually(verifyControllerUp).Should(Succeed())
		})

		It("should ensure the metrics endpoint is serving metrics", func() {
			By("creating a ClusterRoleBinding for the service account to allow access to metrics")
			cmd := exec.Command("kubectl", "create", "clusterrolebinding", metricsRoleBindingName,
				"--clusterrole=v1-jira-jit-rbac-operator-metrics-reader",
				fmt.Sprintf("--serviceaccount=%s:%s", namespace, serviceAccountName),
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ClusterRoleBinding")

			By("validating that the metrics service is available")
			cmd = exec.Command("kubectl", "get", "service", metricsServiceName, "-n", namespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Metrics service should exist")

			By("validating that the ServiceMonitor for Prometheus is applied in the namespace")
			cmd = exec.Command("kubectl", "get", "ServiceMonitor", "-n", namespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "ServiceMonitor should exist")

			By("getting the service account token")
			token, err := serviceAccountToken()
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())

			By("waiting for the metrics endpoint to be ready")
			verifyMetricsEndpointReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "endpoints", metricsServiceName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("8443"), "Metrics endpoint is not ready")
			}
			Eventually(verifyMetricsEndpointReady).Should(Succeed())

			By("verifying that the controller manager is serving the metrics server")
			verifyMetricsServerStarted := func(g Gomega) {
				cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("Serving metrics server"),
					"Metrics server not yet started")
			}
			Eventually(verifyMetricsServerStarted).Should(Succeed())

			By("creating the curl-metrics pod to access the metrics endpoint")
			cmd = exec.Command("kubectl", "run", "curl-metrics", "--restart=Never",
				"--namespace", namespace,
				"--image=curlimages/curl:7.78.0",
				"--", "/bin/sh", "-c", fmt.Sprintf(
					"curl -v -k -H 'Authorization: Bearer %s' https://%s.%s.svc.cluster.local:8443/metrics",
					token, metricsServiceName, namespace))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create curl-metrics pod")

			By("waiting for the curl-metrics pod to complete.")
			verifyCurlUp := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "curl-metrics",
					"-o", "jsonpath={.status.phase}",
					"-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Succeeded"), "curl pod in wrong status")
			}
			Eventually(verifyCurlUp, 5*time.Minute).Should(Succeed())

			By("getting the metrics by checking curl-metrics logs")
			metricsOutput := getMetricsOutput()
			Expect(metricsOutput).To(ContainSubstring(
				"controller_runtime_reconcile_total",
			))
		})

		// Run common controller test cases
		utils.JitRequestTests(namespace)

		// Webhook tests
		Context("When installing the operator with webhooks enabled", func() {
			It("should successfully run with webhooks working", func() {

				By("undeploying the controller-manager")
				cmd := exec.Command("helm", "delete", "-n", namespace, releaseName)
				_, _ = utils.Run(cmd)

				By("deploying the controller-manager with Helm and webhooks enabled")
				helmArgImg := fmt.Sprintf("controllerManager.manager.image.repository=%s", projectImageRepo)
				helmArgImgTag := fmt.Sprintf("controllerManager.manager.image.tag=%s", projectImageTag)
				// Get jiraStubsUrl and replace host for local Kind to connect on localhost
				parsedURL, _ := url.Parse(ts.URL)
				hostParts := strings.Split(parsedURL.Host, ":")
				parsedURL.Host = fmt.Sprintf("host.docker.internal:%s", hostParts[1])
				jiraBaseUrl := parsedURL.String()
				helmArgJiraUrl := fmt.Sprintf("controllerManager.manager.env.jiraBaseUrl=%s", jiraBaseUrl)
				helmSetArg := "--set"
				cmd = exec.Command(
					"helm",
					"install",
					"-n",
					namespace,
					releaseName,
					chartPath,
					helmSetArg,
					helmArgImg,
					helmSetArg,
					helmArgImgTag,
					helmSetArg,
					helmArgJiraUrl,
				)
				_, err := utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")
			})

			It("should provisioned cert-manager", func() {
				By("validating that cert-manager has the certificate Secret")
				verifyCertManager := func(g Gomega) {
					cmd := exec.Command("kubectl", "get", "secrets", "webhook-server-cert", "-n", namespace)
					_, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred())
				}
				Eventually(verifyCertManager).Should(Succeed())
			})

			It("should have CA injection for validating webhooks", func() {
				By("checking CA injection for validating webhooks")
				verifyCAInjection := func(g Gomega) {
					cmd := exec.Command("kubectl", "get",
						"validatingwebhookconfigurations.admissionregistration.k8s.io",
						"v1-jira-jit-rbac-operator-validating-webhook-configuration",
						"-o", "go-template={{ range .webhooks }}{{ .clientConfig.caBundle }}{{ end }}")
					vwhOutput, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(len(vwhOutput)).To(BeNumerically(">", 10))
				}
				Eventually(verifyCAInjection).Should(Succeed())
			})

		})

	})
})

// serviceAccountToken returns a token for the specified service account in the given namespace.
// It uses the Kubernetes TokenRequest API to generate a token by directly sending a request
// and parsing the resulting token from the API response.
func serviceAccountToken() (string, error) {
	const tokenRequestRawString = `{
		"apiVersion": "authentication.k8s.io/v1",
		"kind": "TokenRequest"
	}`

	// Temporary file to store the token request
	secretName := fmt.Sprintf("%s-token-request", serviceAccountName)
	tokenRequestFile := filepath.Join("/tmp", secretName)
	err := os.WriteFile(tokenRequestFile, []byte(tokenRequestRawString), os.FileMode(0o644))
	if err != nil {
		return "", err
	}

	var out string
	verifyTokenCreation := func(g Gomega) {
		// Execute kubectl command to create the token
		cmd := exec.Command("kubectl", "create", "--raw", fmt.Sprintf(
			"/api/v1/namespaces/%s/serviceaccounts/%s/token",
			namespace,
			serviceAccountName,
		), "-f", tokenRequestFile)

		output, err := cmd.CombinedOutput()
		g.Expect(err).NotTo(HaveOccurred())

		// Parse the JSON output to extract the token
		var token tokenRequest
		err = json.Unmarshal([]byte(output), &token) // nolint: unconvert
		g.Expect(err).NotTo(HaveOccurred())

		out = token.Status.Token
	}
	Eventually(verifyTokenCreation).Should(Succeed())

	return out, err
}

// getMetricsOutput retrieves and returns the logs from the curl pod used to access the metrics endpoint.
func getMetricsOutput() string {
	By("getting the curl-metrics logs")
	cmd := exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
	metricsOutput, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to retrieve logs from curl pod")
	Expect(metricsOutput).To(ContainSubstring("< HTTP/1.1 200 OK"))
	return metricsOutput
}

// tokenRequest is a simplified representation of the Kubernetes TokenRequest API response,
// containing only the token field that we need to extract.
type tokenRequest struct {
	Status struct {
		Token string `json:"token"`
	} `json:"status"`
}
