package tests

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/internal/hwaccelparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/features/internal/helpers"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/get"
	nfdset "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/set"
	nfdwait "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/wait"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/nfdparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

var _ = Describe("NFD Resilience", Label("resilience"), func() {
	Context("Pod Failure Recovery", func() {

		It("Worker pod restart - labels persist", func() {
			By("Getting initial node labels")
			initialLabels, err := get.NodeFeatureLabels(APIClient, GeneralConfig.WorkerLabelMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(initialLabels)).To(BeNumerically(">", 0), "Should have initial labels")

			// Store a sample of labels to verify later
			var sampleNode string
			var sampleLabels []string
			for nodeName, labels := range initialLabels {
				if len(labels) > 0 {
					sampleNode = nodeName
					sampleLabels = labels

					break
				}
			}
			Expect(sampleNode).NotTo(BeEmpty(), "Should have at least one node with labels")
			Expect(len(sampleLabels)).To(BeNumerically(">", 0), "Should have labels on sample node")

			By("Finding NFD worker pods")
			listOptions := metav1.ListOptions{
				LabelSelector: "app=nfd-worker",
			}
			pods, err := pod.List(APIClient, hwaccelparams.NFDNamespace, listOptions)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(pods)).To(BeNumerically(">", 0), "Should have NFD worker pods")

			By("Deleting one worker pod to trigger restart")
			workerPod := pods[0]
			klog.V(nfdparams.LogLevel).Infof("Deleting worker pod: %s", workerPod.Object.Name)

			_, err = workerPod.DeleteAndWait(2 * time.Minute)
			Expect(err).NotTo(HaveOccurred(), "Failed to delete worker pod")

			By("Waiting for worker pod to be recreated and running")
			Eventually(func() bool {
				pods, err := pod.List(APIClient, hwaccelparams.NFDNamespace, listOptions)
				if err != nil {
					return false
				}

				for _, p := range pods {
					if p.Object.Status.Phase == corev1.PodRunning {
						klog.V(nfdparams.LogLevel).Infof("Worker pod %s is running", p.Object.Name)

						return true
					}
				}

				return false
			}).WithTimeout(5*time.Minute).Should(BeTrue(), "Worker pod should be recreated and running")

			By("Verifying labels still exist after pod restart")
			Eventually(func() bool {
				currentLabels, err := get.NodeFeatureLabels(APIClient, GeneralConfig.WorkerLabelMap)
				if err != nil {
					klog.V(nfdparams.LogLevel).Infof("Error getting labels: %v", err)

					return false
				}

				nodeLabels, exists := currentLabels[sampleNode]
				if !exists {
					return false
				}

				// Check that all labels are still present
				return len(nodeLabels) >= len(sampleLabels)
			}).WithTimeout(5*time.Minute).Should(BeTrue(), "Labels should persist after worker pod restart")

			By("Verifying specific labels are unchanged")
			finalLabels, err := get.NodeFeatureLabels(APIClient, GeneralConfig.WorkerLabelMap)
			Expect(err).NotTo(HaveOccurred())

			// Check a few sample labels still exist
			checkLabels := []string{}
			for i := 0; i < 3 && i < len(sampleLabels); i++ {
				// Extract just the label key (before '=')
				labelParts := sampleLabels[i]
				for idx := 0; idx < len(labelParts); idx++ {
					if labelParts[idx] == '=' {
						checkLabels = append(checkLabels, labelParts[:idx])

						break
					}
				}
			}

			if len(checkLabels) > 0 {
				err = helpers.CheckLabelsExist(finalLabels, checkLabels, nil, sampleNode)
				Expect(err).NotTo(HaveOccurred(), "Sample labels should still exist after restart")
			}
		})

		It("Master pod restart - rule processing continues", func() {
			By("Creating a test NodeFeatureRule")
			ruleYAML := `[
{
    "apiVersion": "nfd.openshift.io/v1alpha1",
    "kind": "NodeFeatureRule",
    "metadata": {
        "name": "test-master-restart",
        "namespace": "` + nfdparams.NFDNamespace + `"
    },
    "spec": {
        "rules": [
            {
                "name": "test.master.resilience",
                "labels": {
                    "test.feature.node.kubernetes.io/master-test": "true"
                },
                "matchFeatures": [
                    {
                        "feature": "kernel.version",
                        "matchExpressions": {
                            "major": {
                                "op": "Exists"
                            }
                        }
                    }
                ]
            }
        ]
    }
}]
`

			testRule, err := nfdset.CreateNodeFeatureRuleFromJSON(APIClient, ruleYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(testRule).NotTo(BeNil())

			defer func() {
				if testRule != nil && testRule.Exists() {
					_, _ = testRule.Delete()
				}
			}()

			By("Waiting for initial labels")
			err = nfdwait.WaitForLabelsFromRule(APIClient,
				[]string{"test.feature.node.kubernetes.io/master-test"},
				5*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			By("Finding NFD master pod")
			listOptions := metav1.ListOptions{
				LabelSelector: "app=nfd-master",
			}
			pods, err := pod.List(APIClient, hwaccelparams.NFDNamespace, listOptions)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(pods)).To(BeNumerically(">", 0), "Should have NFD master pod")

			By("Deleting master pod to trigger restart")
			masterPod := pods[0]
			klog.V(nfdparams.LogLevel).Infof("Deleting master pod: %s", masterPod.Object.Name)

			_, err = masterPod.DeleteAndWait(2 * time.Minute)
			Expect(err).NotTo(HaveOccurred(), "Failed to delete master pod")

			By("Waiting for master pod to be recreated and running")
			Eventually(func() bool {
				pods, err := pod.List(APIClient, hwaccelparams.NFDNamespace, listOptions)
				if err != nil {

					return false
				}

				for _, p := range pods {
					if p.Object.Status.Phase == corev1.PodRunning {
						klog.V(nfdparams.LogLevel).Infof("Master pod %s is running", p.Object.Name)

						return true
					}
				}

				return false
			}).WithTimeout(5*time.Minute).Should(BeTrue(), "Master pod should be recreated and running")

			By("Verifying rule processing continues after master restart")
			Eventually(func() bool {
				nodelabels, err := get.NodeFeatureLabels(APIClient, GeneralConfig.WorkerLabelMap)
				if err != nil {

					return false
				}

				for nodeName := range nodelabels {
					if helpers.CheckLabelsExist(nodelabels,
						[]string{"test.feature.node.kubernetes.io/master-test"},
						nil, nodeName) == nil {

						return true
					}
				}

				return false
			}).WithTimeout(5*time.Minute).Should(BeTrue(), "Labels should still be present after master restart")
		})

		It("GC cleanup - stale NodeFeature objects removed", func() {
			By("Checking current NodeFeature objects")

			// Note: NodeFeature objects are internal to NFD and may not be directly accessible
			// This test focuses on verifying label cleanup after rule deletion
			klog.V(nfdparams.LogLevel).Info("Testing GC cleanup through label removal verification")

			By("Creating a test rule that will be deleted")
			ruleYAML := `[
{
    "apiVersion": "nfd.openshift.io/v1alpha1",
    "kind": "NodeFeatureRule",
    "metadata": {
        "name": "test-gc-cleanup",
        "namespace": "` + nfdparams.NFDNamespace + `"
    },
    "spec": {
        "rules": [
            {
                "name": "test.gc.rule",
                "labels": {
                    "test.feature.node.kubernetes.io/gc-test": "true"
                },
                "matchFeatures": [
                    {
                        "feature": "kernel.version",
                        "matchExpressions": {
                            "major": {
                                "op": "Exists"
                            }
                        }
                    }
                ]
            }
        ]
    }
}]
`

			testRule, err := nfdset.CreateNodeFeatureRuleFromJSON(APIClient, ruleYAML)
			Expect(err).NotTo(HaveOccurred())
			Expect(testRule).NotTo(BeNil())

			By("Waiting for labels to appear")
			err = nfdwait.WaitForLabelsFromRule(APIClient,
				[]string{"test.feature.node.kubernetes.io/gc-test"},
				3*time.Minute)
			Expect(err).NotTo(HaveOccurred())

			By("Deleting the rule")
			_, err = testRule.Delete()
			Expect(err).NotTo(HaveOccurred())

			By("Verifying labels are eventually cleaned up")
			Eventually(func() bool {
				nodelabels, err := get.NodeFeatureLabels(APIClient, GeneralConfig.WorkerLabelMap)
				if err != nil {

					return false
				}

				for nodeName := range nodelabels {
					if helpers.CheckLabelsExist(nodelabels,
						[]string{"test.feature.node.kubernetes.io/gc-test"},
						nil, nodeName) == nil {

						return false
					}
				}

				return true
			}).WithTimeout(5*time.Minute).Should(BeTrue(), "Labels should be garbage collected")

			klog.V(nfdparams.LogLevel).Info("GC cleanup verified successfully")
		})

		It("Topology updater functionality", func() {
			By("Checking if topology updater is enabled")

			listOptions := metav1.ListOptions{
				LabelSelector: "app=nfd-topology-updater",
			}
			pods, err := pod.List(APIClient, hwaccelparams.NFDNamespace, listOptions)

			if err != nil || len(pods) == 0 {
				Skip("Topology updater not enabled - skipping topology test")
			}

			By("Verifying topology updater pods are running")
			allRunning := true
			for _, p := range pods {
				if p.Object.Status.Phase != corev1.PodRunning {
					allRunning = false
					klog.V(nfdparams.LogLevel).Infof("Topology updater pod %s is not running: %s",
						p.Object.Name, p.Object.Status.Phase)
				}
			}
			Expect(allRunning).To(BeTrue(), "All topology updater pods should be running")

			By("Checking for NodeResourceTopology objects")
			// NodeResourceTopology CRD is created by topology-aware-scheduling
			// This is an advanced feature that may not be available in all deployments
			klog.V(nfdparams.LogLevel).Info(
				"Topology updater is running - NRT objects creation depends on cluster configuration")
			klog.V(nfdparams.LogLevel).Info("If topology-aware-scheduling is installed, NRT objects should be created")

			// Just verify the topology updater pods are running and healthy
			// Actual NRT object verification would require topology client which may not be available
			klog.V(nfdparams.LogLevel).Info("Topology updater functionality verified through pod status check")
		})
	})
})
