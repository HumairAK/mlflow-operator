/*
Copyright 2025.

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
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/opendatahub-io/mlflow-operator/test/utils"
)

// namespace where the project is deployed in
const namespace = "opendatahub"

// serviceAccountName created for the project
const serviceAccountName = "mlflow-operator-controller-manager"

// metricsServiceName is the name of the metrics service of the project
const metricsServiceName = "mlflow-operator-controller-manager-metrics-service"

// metricsRoleBindingName is the name of the RBAC that will be created to allow get the metrics data
const metricsRoleBindingName = "mlflow-operator-metrics-binding"

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string

	// Before running the tests, set up the environment by creating the namespace,
	// enforce the restricted security policy to the namespace, installing CRDs,
	// and deploying the controller.
	BeforeAll(func() {
		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

		By("labeling the namespace to enforce the restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

		By("installing CRDs")
		cmd = exec.Command("make", "install")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the controller-manager")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")
	})

	// After all tests have been executed, clean up by undeploying the controller, uninstalling CRDs,
	// and deleting the namespace.
	AfterAll(func() {
		By("cleaning up the curl pod for metrics")
		cmd := exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", namespace, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)

		By("cleaning up the ClusterRoleBinding for metrics")
		cmd = exec.Command("kubectl", "delete", "clusterrolebinding", metricsRoleBindingName, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)

		By("cleaning up any MLflow resources")
		cmd = exec.Command("kubectl", "delete", "mlflow", "--all", "--ignore-not-found=true")
		_, _ = utils.Run(cmd)

		By("undeploying the controller-manager")
		cmd = exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace)
		_, _ = utils.Run(cmd)
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
			By("cleaning up any existing ClusterRoleBinding for metrics")
			cmd := exec.Command("kubectl", "delete", "clusterrolebinding", metricsRoleBindingName, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)

			By("creating a ClusterRoleBinding for the service account to allow access to metrics")
			cmd = exec.Command("kubectl", "create", "clusterrolebinding", metricsRoleBindingName,
				"--clusterrole=mlflow-operator-metrics-reader",
				fmt.Sprintf("--serviceaccount=%s:%s", namespace, serviceAccountName),
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ClusterRoleBinding")

			By("validating that the metrics service is available")
			cmd = exec.Command("kubectl", "get", "service", metricsServiceName, "-n", namespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Metrics service should exist")

			By("getting the service account token")
			token, err := serviceAccountToken()
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())

			By("ensuring the controller pod is ready")
			verifyControllerPodReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod", controllerPodName, "-n", namespace,
					"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"), "Controller pod not ready")
			}
			Eventually(verifyControllerPodReady, 3*time.Minute, time.Second).Should(Succeed())

			By("verifying that the controller manager is serving the metrics server")
			verifyMetricsServerStarted := func(g Gomega) {
				cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("Serving metrics server"),
					"Metrics server not yet started")
			}
			Eventually(verifyMetricsServerStarted, 3*time.Minute, time.Second).Should(Succeed())

			// +kubebuilder:scaffold:e2e-metrics-webhooks-readiness

			By("cleaning up any existing curl-metrics pod")
			cmd = exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", namespace, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)

			By("creating the curl-metrics pod to access the metrics endpoint")
			cmd = exec.Command("kubectl", "run", "curl-metrics", "--restart=Never",
				"--namespace", namespace,
				"--image=curlimages/curl:latest",
				"--overrides",
				fmt.Sprintf(`{
					"spec": {
						"containers": [{
							"name": "curl",
							"image": "curlimages/curl:latest",
							"command": ["/bin/sh", "-c"],
							"args": ["curl -v -k -H 'Authorization: Bearer %s' https://%s.%s.svc.cluster.local:8443/metrics"],
							"securityContext": {
								"readOnlyRootFilesystem": true,
								"allowPrivilegeEscalation": false,
								"capabilities": {
									"drop": ["ALL"]
								},
								"runAsNonRoot": true,
								"runAsUser": 1000,
								"seccompProfile": {
									"type": "RuntimeDefault"
								}
							}
						}],
						"serviceAccountName": "%s"
					}
				}`, token, metricsServiceName, namespace, serviceAccountName))
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
			verifyMetricsAvailable := func(g Gomega) {
				metricsOutput, err := getMetricsOutput()
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve logs from curl pod")
				g.Expect(metricsOutput).NotTo(BeEmpty())
				g.Expect(metricsOutput).To(ContainSubstring("< HTTP/1.1 200 OK"))
			}
			Eventually(verifyMetricsAvailable, 2*time.Minute).Should(Succeed())
		})

		// +kubebuilder:scaffold:e2e-webhooks-checks

		It("should validate CEL constraint for singleton MLflow resource", func() {
			By("creating an MLflow resource with the correct name 'mlflow'")
			mlflowYAML := `apiVersion: mlflow.opendatahub.io/v1
kind: MLflow
metadata:
  name: mlflow
spec: {}`

			mlflowFile := filepath.Join("/tmp", "mlflow-valid.yaml")
			err := os.WriteFile(mlflowFile, []byte(mlflowYAML), os.FileMode(0o644))
			Expect(err).NotTo(HaveOccurred(), "Failed to write valid MLflow manifest")
			defer func() {
				if removeErr := os.Remove(mlflowFile); removeErr != nil {
					_, _ = fmt.Fprintf(GinkgoWriter, "failed to remove %s: %v\n", mlflowFile, removeErr)
				}
			}()

			cmd := exec.Command("kubectl", "apply", "-f", mlflowFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create MLflow resource with name 'mlflow'")

			By("verifying the MLflow resource was created successfully")
			cmd = exec.Command("kubectl", "get", "mlflow", "mlflow", "-o", "jsonpath={.metadata.name}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("mlflow"), "MLflow resource should exist with name 'mlflow'")

			By("attempting to create an MLflow resource with an invalid name")
			invalidYAML := `apiVersion: mlflow.opendatahub.io/v1
kind: MLflow
metadata:
  name: invalid-name
spec: {}`

			invalidFile := filepath.Join("/tmp", "mlflow-invalid.yaml")
			err = os.WriteFile(invalidFile, []byte(invalidYAML), os.FileMode(0o644))
			Expect(err).NotTo(HaveOccurred(), "Failed to write invalid MLflow manifest")
			defer func() {
				if removeErr := os.Remove(invalidFile); removeErr != nil {
					_, _ = fmt.Fprintf(GinkgoWriter, "failed to remove %s: %v\n", invalidFile, removeErr)
				}
			}()

			cmd = exec.Command("kubectl", "apply", "-f", invalidFile)
			output, err = utils.Run(cmd)
			Expect(err).To(HaveOccurred(), "Should fail to create MLflow with invalid name")
			Expect(output).To(ContainSubstring("MLflow resource name must be 'mlflow'"),
				"Error message should indicate name validation failure")

			By("cleaning up the valid MLflow resource")
			cmd = exec.Command("kubectl", "delete", "mlflow", "mlflow")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to delete MLflow resource")

			By("verifying the MLflow resource was deleted")
			verifyDeleted := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "mlflow", "mlflow")
				_, err := utils.Run(cmd)
				g.Expect(err).To(HaveOccurred(), "MLflow resource should not exist after deletion")
			}
			Eventually(verifyDeleted, 30*time.Second).Should(Succeed())
		})

		It("should deploy a minimal MLflow instance and verify functionality", func() {
			const (
				mlflowName      = "mlflow"
				targetNamespace = "opendatahub"
				timeout         = 5 * time.Minute
			)

			By("creating a minimal MLflow instance")
			projectDir, err := utils.GetProjectDir()
			Expect(err).NotTo(HaveOccurred())

			mlflowManifestPath := filepath.Join(projectDir, "test/e2e/manifests/mlflow-minimal.yaml")

			// Check if MLFLOW_IMAGE environment variable is set to override the image
			mlflowImage := os.Getenv("MLFLOW_IMAGE")
			if mlflowImage != "" {
				By(fmt.Sprintf("Using custom MLflow image: %s", mlflowImage))
				// Read the manifest
				manifestBytes, err := os.ReadFile(mlflowManifestPath)
				Expect(err).NotTo(HaveOccurred(), "Failed to read MLflow manifest")

				// Replace the image in the YAML
				// The structure is:
				//   image:
				//     image: quay.io/opendatahub/mlflow:latest
				// We need to replace the nested "image:" line that has a value
				manifestStr := string(manifestBytes)
				lines := strings.Split(manifestStr, "\n")
				for i, line := range lines {
					// Look for "image: <value>" where value is not empty
					// Skip lines with "imagePullPolicy"
					if strings.Contains(line, "image:") && !strings.Contains(line, "imagePullPolicy") {
						// Check if this line has a value after "image:"
						parts := strings.SplitN(line, "image:", 2)
						if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
							// This is the line with the actual image value
							indent := parts[0] // Preserve original indentation
							lines[i] = indent + "image: " + mlflowImage
							break
						}
					}
				}
				manifestStr = strings.Join(lines, "\n")

				// Write to temp file
				tmpManifest := filepath.Join("/tmp", "mlflow-custom-image.yaml")
				err = os.WriteFile(tmpManifest, []byte(manifestStr), os.FileMode(0o644))
				Expect(err).NotTo(HaveOccurred(), "Failed to write modified manifest")
				defer func() {
					if removeErr := os.Remove(tmpManifest); removeErr != nil {
						_, _ = fmt.Fprintf(GinkgoWriter, "failed to remove %s: %v\n", tmpManifest, removeErr)
					}
				}()

				mlflowManifestPath = tmpManifest
			}

			cmd := exec.Command("kubectl", "apply", "-f", mlflowManifestPath)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create MLflow resource")

			By("waiting for MLflow resource to be created")
			verifyMLflowCreated := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "mlflow", mlflowName, "-o", "jsonpath={.metadata.name}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal(mlflowName))
			}
			Eventually(verifyMLflowCreated, 30*time.Second).Should(Succeed())

			By("waiting for MLflow deployment to be created")
			verifyDeploymentCreated := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "deployment", mlflowName,
					"-n", targetNamespace, "-o", "jsonpath={.metadata.name}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal(mlflowName))
			}
			Eventually(verifyDeploymentCreated, timeout).Should(Succeed())

			By("waiting for MLflow pods to be ready")
			verifyPodsReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "deployment", mlflowName,
					"-n", targetNamespace,
					"-o", "jsonpath={.status.readyReplicas}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"), "Expected 1 ready replica")
			}
			Eventually(verifyPodsReady, timeout, 5*time.Second).Should(Succeed())

			By("verifying MLflow status conditions")
			verifyStatusConditions := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "mlflow", mlflowName,
					"-o", "jsonpath={.status.conditions[?(@.type=='Available')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"), "MLflow should be Available")
			}
			Eventually(verifyStatusConditions, timeout, 5*time.Second).Should(Succeed())

			By("getting the MLflow service endpoint")
			var mlflowURL string
			getServiceEndpoint := func(g Gomega) {
				// Get service port
				cmd := exec.Command("kubectl", "get", "service", mlflowName,
					"-n", targetNamespace,
					"-o", "jsonpath={.spec.ports[0].port}")
				portOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(portOutput).NotTo(BeEmpty())

				mlflowURL = fmt.Sprintf("http://%s.%s.svc.cluster.local:%s",
					mlflowName, targetNamespace, portOutput)
			}
			Eventually(getServiceEndpoint, 30*time.Second).Should(Succeed())

			By("creating a test pod with MLflow client to test API")
			// Clean up any existing test pod
			cmd = exec.Command("kubectl", "delete", "pod", "mlflow-api-test",
				"-n", targetNamespace, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)

			// Wait for pod to be fully deleted
			time.Sleep(5 * time.Second)

			// Read and render test pod template with dynamic values
			// Note: We need a temp file here because we're rendering a template
			testPodTemplatePath := filepath.Join(projectDir, "test/e2e/manifests/test-pod.yaml.tmpl")
			testPodTemplateContent, err := os.ReadFile(testPodTemplatePath)
			Expect(err).NotTo(HaveOccurred(), "Failed to read test pod template")

			tmpl, err := template.New("test-pod").Parse(string(testPodTemplateContent))
			Expect(err).NotTo(HaveOccurred(), "Failed to parse test pod template")

			var testPodManifest bytes.Buffer
			templateData := map[string]string{
				"Namespace":   targetNamespace,
				"TrackingURI": mlflowURL,
			}
			err = tmpl.Execute(&testPodManifest, templateData)
			Expect(err).NotTo(HaveOccurred(), "Failed to render test pod template")

			testPodFile := filepath.Join("/tmp", "mlflow-test-pod.yaml")
			err = os.WriteFile(testPodFile, testPodManifest.Bytes(), os.FileMode(0o644))
			Expect(err).NotTo(HaveOccurred(), "Failed to write rendered test pod manifest")
			defer func() {
				if removeErr := os.Remove(testPodFile); removeErr != nil {
					_, _ = fmt.Fprintf(GinkgoWriter, "failed to remove %s: %v\n", testPodFile, removeErr)
				}
			}()

			By("creating ConfigMap with test script")
			scriptPath := filepath.Join(projectDir, "test/scripts/test_mlflow_api.py")

			cmd = exec.Command("kubectl", "create", "configmap", "mlflow-test-script",
				"-n", targetNamespace,
				"--from-file=test_mlflow_api.py="+scriptPath)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ConfigMap")

			By("running the test pod")
			cmd = exec.Command("kubectl", "apply", "-f", testPodFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create test pod")

			By("waiting for test pod to complete")
			verifyTestPodComplete := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod", "mlflow-api-test",
					"-n", targetNamespace,
					"-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Or(Equal("Succeeded"), Equal("Failed")),
					"Test pod should have completed")
			}
			Eventually(verifyTestPodComplete, 5*time.Minute, 10*time.Second).Should(Succeed())

			By("checking test pod logs for success")
			cmd = exec.Command("kubectl", "logs", "mlflow-api-test", "-n", targetNamespace)
			testOutput, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to get test pod logs")
			_, _ = fmt.Fprintf(GinkgoWriter, "Test output:\n%s\n", testOutput)

			Expect(testOutput).To(ContainSubstring("All tests passed successfully!"),
				"MLflow API tests should pass")
			Expect(testOutput).To(ContainSubstring("Successfully connected to MLflow"),
				"Should connect to MLflow")
			Expect(testOutput).To(ContainSubstring("Created experiment"),
				"Should create experiment")
			Expect(testOutput).To(ContainSubstring("Created run"),
				"Should create run")
			Expect(testOutput).To(And(ContainSubstring("Logged"), ContainSubstring("parameters")),
				"Should log parameters")
			Expect(testOutput).To(And(ContainSubstring("Logged"), ContainSubstring("metrics")),
				"Should log metrics")

			By("verifying test pod succeeded")
			cmd = exec.Command("kubectl", "get", "pod", "mlflow-api-test",
				"-n", targetNamespace,
				"-o", "jsonpath={.status.phase}")
			podPhase, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(podPhase).To(Equal("Succeeded"), "Test pod should have succeeded")

			By("cleaning up MLflow test resources")
			// Clean up test pod
			cmd = exec.Command("kubectl", "delete", "pod", "mlflow-api-test",
				"-n", targetNamespace, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)

			// Clean up ConfigMap
			cmd = exec.Command("kubectl", "delete", "configmap", "mlflow-test-script",
				"-n", targetNamespace, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)

			// Clean up MLflow instance
			cmd = exec.Command("kubectl", "delete", "mlflow", mlflowName, "--ignore-not-found=true")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to delete MLflow resource")

			By("verifying MLflow resource was deleted")
			verifyMLflowDeleted := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "mlflow", mlflowName)
				_, err := utils.Run(cmd)
				g.Expect(err).To(HaveOccurred(), "MLflow resource should be deleted")
			}
			Eventually(verifyMLflowDeleted, timeout).Should(Succeed())

			By("verifying MLflow deployment was cleaned up")
			verifyDeploymentDeleted := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "deployment", mlflowName, "-n", targetNamespace)
				_, err := utils.Run(cmd)
				g.Expect(err).To(HaveOccurred(), "Deployment should be deleted")
			}
			Eventually(verifyDeploymentDeleted, timeout).Should(Succeed())
		})

		It("should deploy MLflow with kube-rbac-proxy and cert-manager", Label("rbac-proxy"), func() {
			// Skip this test unless TEST_RBAC_PROXY is set to true
			testRbacProxy := os.Getenv("TEST_RBAC_PROXY")
			if testRbacProxy != "true" {
				Skip("Skipping kube-rbac-proxy test (set TEST_RBAC_PROXY=true to enable)")
			}

			const (
				mlflowName      = "mlflow"
				targetNamespace = "opendatahub"
				timeout         = 5 * time.Minute
			)

			By("creating the Certificate resource for TLS")
			projectDir, err := utils.GetProjectDir()
			Expect(err).NotTo(HaveOccurred())

			certificatePath := filepath.Join(projectDir, "test/e2e/manifests/mlflow-certificate.yaml")
			cmd := exec.Command("kubectl", "apply", "-f", certificatePath)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create Certificate resource")

			By("waiting for certificate to be ready")
			verifyCertReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "certificate", "mlflow-tls",
					"-n", targetNamespace,
					"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"), "Certificate should be ready")
			}
			Eventually(verifyCertReady, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying TLS secret was created by cert-manager")
			cmd = exec.Command("kubectl", "get", "secret", "mlflow-tls", "-n", targetNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "TLS secret should exist")

			By("creating MLflow instance with kube-rbac-proxy enabled")
			mlflowManifestPath := filepath.Join(projectDir, "test/e2e/manifests/mlflow-rbac-proxy.yaml")

			// Check if MLFLOW_IMAGE environment variable is set to override the image
			mlflowImage := os.Getenv("MLFLOW_IMAGE")
			if mlflowImage != "" {
				By(fmt.Sprintf("Using custom MLflow image: %s", mlflowImage))
				manifestBytes, err := os.ReadFile(mlflowManifestPath)
				Expect(err).NotTo(HaveOccurred(), "Failed to read MLflow manifest")

				manifestStr := string(manifestBytes)
				lines := strings.Split(manifestStr, "\n")
				for i, line := range lines {
					if strings.Contains(line, "image:") && !strings.Contains(line, "imagePullPolicy") {
						parts := strings.SplitN(line, "image:", 2)
						if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
							indent := parts[0]
							lines[i] = indent + "image: " + mlflowImage
							break
						}
					}
				}
				manifestStr = strings.Join(lines, "\n")

				tmpManifest := filepath.Join("/tmp", "mlflow-rbac-proxy-custom.yaml")
				err = os.WriteFile(tmpManifest, []byte(manifestStr), os.FileMode(0o644))
				Expect(err).NotTo(HaveOccurred(), "Failed to write modified manifest")
				defer func() {
					if removeErr := os.Remove(tmpManifest); removeErr != nil {
						_, _ = fmt.Fprintf(GinkgoWriter, "failed to remove %s: %v\n", tmpManifest, removeErr)
					}
				}()

				mlflowManifestPath = tmpManifest
			}

			cmd = exec.Command("kubectl", "apply", "-f", mlflowManifestPath)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create MLflow resource with kube-rbac-proxy")

			By("waiting for MLflow deployment to be created")
			verifyDeploymentCreated := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "deployment", mlflowName,
					"-n", targetNamespace, "-o", "jsonpath={.metadata.name}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal(mlflowName))
			}
			Eventually(verifyDeploymentCreated, timeout).Should(Succeed())

			By("verifying deployment has kube-rbac-proxy sidecar")
			verifyRbacProxySidecar := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "deployment", mlflowName,
					"-n", targetNamespace,
					"-o", "jsonpath={.spec.template.spec.containers[*].name}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("kube-rbac-proxy"), "Deployment should have kube-rbac-proxy container")
			}
			Eventually(verifyRbacProxySidecar, 30*time.Second).Should(Succeed())

			By("waiting for MLflow pods to be ready")
			verifyPodsReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "deployment", mlflowName,
					"-n", targetNamespace,
					"-o", "jsonpath={.status.readyReplicas}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"), "Expected 1 ready replica")
			}
			Eventually(verifyPodsReady, timeout, 5*time.Second).Should(Succeed())

			By("verifying TLS secret is mounted in kube-rbac-proxy container")
			verifyTLSMount := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "deployment", mlflowName,
					"-n", targetNamespace,
					"-o", "jsonpath={.spec.template.spec.volumes[?(@.name=='tls')].secret.secretName}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("mlflow-tls"), "TLS secret should be mounted")
			}
			Eventually(verifyTLSMount, 30*time.Second).Should(Succeed())

			By("getting service account token for authenticated access")
			token, err := serviceAccountToken()
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())

			By("testing authenticated access through kube-rbac-proxy")
			// Clean up any existing test pod
			cmd = exec.Command("kubectl", "delete", "pod", "rbac-proxy-test",
				"-n", targetNamespace, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)

			// Wait for pod to be fully deleted
			time.Sleep(5 * time.Second)

			// Create a test pod to access MLflow through kube-rbac-proxy
			cmd = exec.Command("kubectl", "run", "rbac-proxy-test", "--restart=Never",
				"--namespace", targetNamespace,
				"--image=curlimages/curl:latest",
				"--overrides",
				fmt.Sprintf(`{
					"spec": {
						"containers": [{
							"name": "curl",
							"image": "curlimages/curl:latest",
							"command": ["/bin/sh", "-c"],
							"args": ["curl -v -k -H 'Authorization: Bearer %s' https://mlflow.%s.svc.cluster.local:8443/ && echo 'SUCCESS: Authenticated access works'"],
							"securityContext": {
								"readOnlyRootFilesystem": true,
								"allowPrivilegeEscalation": false,
								"capabilities": {
									"drop": ["ALL"]
								},
								"runAsNonRoot": true,
								"runAsUser": 1000,
								"seccompProfile": {
									"type": "RuntimeDefault"
								}
							}
						}],
						"serviceAccountName": "%s"
					}
				}`, token, targetNamespace, serviceAccountName))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create rbac-proxy-test pod")

			By("waiting for test pod to complete")
			verifyTestPodComplete := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod", "rbac-proxy-test",
					"-n", targetNamespace,
					"-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Or(Equal("Succeeded"), Equal("Failed")),
					"Test pod should have completed")
			}
			Eventually(verifyTestPodComplete, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying authenticated access succeeded")
			cmd = exec.Command("kubectl", "logs", "rbac-proxy-test", "-n", targetNamespace)
			testOutput, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to get test pod logs")
			_, _ = fmt.Fprintf(GinkgoWriter, "RBAC Proxy test output:\n%s\n", testOutput)

			Expect(testOutput).To(ContainSubstring("SUCCESS: Authenticated access works"),
				"Should successfully access MLflow through kube-rbac-proxy")

			By("verifying test pod succeeded")
			cmd = exec.Command("kubectl", "get", "pod", "rbac-proxy-test",
				"-n", targetNamespace,
				"-o", "jsonpath={.status.phase}")
			podPhase, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(podPhase).To(Equal("Succeeded"), "Test pod should have succeeded")

			By("cleaning up test resources")
			// Clean up test pod
			cmd = exec.Command("kubectl", "delete", "pod", "rbac-proxy-test",
				"-n", targetNamespace, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)

			// Clean up MLflow instance
			cmd = exec.Command("kubectl", "delete", "mlflow", mlflowName, "--ignore-not-found=true")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to delete MLflow resource")

			// Clean up Certificate
			cmd = exec.Command("kubectl", "delete", "certificate", "mlflow-tls",
				"-n", targetNamespace, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)

			// Clean up TLS secret (if not auto-deleted)
			cmd = exec.Command("kubectl", "delete", "secret", "mlflow-tls",
				"-n", targetNamespace, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)

			By("verifying MLflow resource was deleted")
			verifyMLflowDeleted := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "mlflow", mlflowName)
				_, err := utils.Run(cmd)
				g.Expect(err).To(HaveOccurred(), "MLflow resource should be deleted")
			}
			Eventually(verifyMLflowDeleted, timeout).Should(Succeed())

			By("verifying MLflow deployment was cleaned up")
			verifyDeploymentDeleted := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "deployment", mlflowName, "-n", targetNamespace)
				_, err := utils.Run(cmd)
				g.Expect(err).To(HaveOccurred(), "Deployment should be deleted")
			}
			Eventually(verifyDeploymentDeleted, timeout).Should(Succeed())
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

	defer func(name string) {
		err := os.Remove(name)
		if err != nil {
			_ = fmt.Sprintf("Failed to remove file %s", name)
		}
	}(tokenRequestFile) // Clean up temp file

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
		err = json.Unmarshal(output, &token)
		g.Expect(err).NotTo(HaveOccurred())

		out = token.Status.Token
	}
	if !Eventually(verifyTokenCreation).Should(Succeed()) {
		return "", fmt.Errorf("failed to create service account token")
	}

	return out, nil
}

// getMetricsOutput retrieves and returns the logs from the curl pod used to access the metrics endpoint.
func getMetricsOutput() (string, error) {
	By("getting the curl-metrics logs")
	cmd := exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
	return utils.Run(cmd)
}

// tokenRequest is a simplified representation of the Kubernetes TokenRequest API response,
// containing only the token field that we need to extract.
type tokenRequest struct {
	Status struct {
		Token string `json:"token"`
	} `json:"status"`
}
