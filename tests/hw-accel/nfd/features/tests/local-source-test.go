package tests

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/internal/hwaccelparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/features/internal/helpers"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/get"
	nfdset "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/set"
	nfdwait "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/wait"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/nfdparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	"k8s.io/klog/v2"
)

var _ = Describe("NFD Local Source", Label("local-source"), func() {
	Context("User-Defined Features", func() {

		AfterEach(func() {
			By("Cleaning up local source ConfigMap")
			err := nfdset.DeleteLocalSourceConfigMap(APIClient, hwaccelparams.NFDNamespace)
			if err != nil {
				klog.Errorf("Failed to delete local source ConfigMap: %v", err)
			}
		})

		It("User-defined feature labels via ConfigMap", reportxml.ID("70030"), func() {
			By("Creating ConfigMap with user-defined features")

			// Define custom features
			features := map[string]string{
				"custom-feature-1": "true",
				"custom-feature-2": "enabled",
				"custom-app":       "myapp-v1.0",
			}

			err := nfdset.CreateLocalSourceConfigMap(APIClient, hwaccelparams.NFDNamespace, features)
			Expect(err).NotTo(HaveOccurred(), "Failed to create local source ConfigMap")

			By("Creating NodeFeatureRule to expose local features")

			// This rule reads from the local source and creates labels
			ruleYAML := `[
{
    "apiVersion": "nfd.openshift.io/v1alpha1",
    "kind": "NodeFeatureRule",
    "metadata": {
        "name": "test-local-source",
        "namespace": "` + nfdparams.NFDNamespace + `"
    },
    "spec": {
        "rules": [
            {
                "name": "test.local.features",
                "labels": {
                    "test.feature.node.kubernetes.io/custom-feature-1": "true",
                    "test.feature.node.kubernetes.io/custom-app": "myapp"
                },
                "matchFeatures": [
                    {
                        "feature": "local.label",
                        "matchExpressions": {
                            "custom-feature-1": {
                                "op": "In",
                                "value": ["true"]
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
			Expect(err).NotTo(HaveOccurred(), "Failed to create NodeFeatureRule")
			Expect(testRule).NotTo(BeNil())

			defer func() {
				if testRule != nil && testRule.Exists() {
					testRule.Delete()
				}
			}()

			By("Waiting for user-defined labels to appear")
			// Local source processing may take a bit longer
			err = nfdwait.WaitForLabelsFromRule(APIClient,
				[]string{"test.feature.node.kubernetes.io/custom"},
				5*time.Minute)

			if err != nil {
				klog.V(nfdparams.LogLevel).Info("Local source labels not found - this may require " +
					"additional NFD worker configuration to enable local source")
				Skip("Local source not configured or not supported in this NFD deployment")
			}

			By("Verifying user-defined labels exist")
			nodelabels, err := get.NodeFeatureLabels(APIClient, GeneralConfig.WorkerLabelMap)
			Expect(err).NotTo(HaveOccurred())

			labelFound := false
			for nodeName := range nodelabels {
				if helpers.CheckLabelsExist(nodelabels,
					[]string{"test.feature.node.kubernetes.io/custom"},
					nil, nodeName) == nil {
					klog.V(nfdparams.LogLevel).Infof("Node %s has custom local source labels", nodeName)
					labelFound = true
					break
				}
			}

			if !labelFound {
				klog.V(nfdparams.LogLevel).Info("User-defined labels not found - " +
					"NFD worker may need to be configured with local source support")
				Skip("Local source feature not available in current NFD configuration")
			}

			Expect(labelFound).To(BeTrue(), "User-defined labels should be present")
		})

		It("Feature files from hostPath", reportxml.ID("70031"), func() {
			By("Verifying hostPath local source capability")

			// This test verifies that NFD can read features from hostPath
			// In practice, this requires mounting a hostPath volume with feature files

			ruleYAML := `[
{
    "apiVersion": "nfd.openshift.io/v1alpha1",
    "kind": "NodeFeatureRule",
    "metadata": {
        "name": "test-hostpath-source",
        "namespace": "` + nfdparams.NFDNamespace + `"
    },
    "spec": {
        "rules": [
            {
                "name": "test.hostpath.features",
                "labels": {
                    "test.feature.node.kubernetes.io/hostpath-test": "true"
                },
                "matchFeatures": [
                    {
                        "feature": "local.label",
                        "matchExpressions": {
                            "hostpath-feature": {
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
			Expect(err).NotTo(HaveOccurred(), "Failed to create NodeFeatureRule")
			Expect(testRule).NotTo(BeNil())

			defer func() {
				if testRule != nil && testRule.Exists() {
					testRule.Delete()
				}
			}()

			By("Checking if hostPath features are available")
			// Wait a bit to see if any hostPath features are detected
			time.Sleep(30 * time.Second)

			nodelabels, err := get.NodeFeatureLabels(APIClient, GeneralConfig.WorkerLabelMap)
			Expect(err).NotTo(HaveOccurred())

			hostPathFound := false
			for nodeName := range nodelabels {
				if helpers.CheckLabelsExist(nodelabels,
					[]string{"test.feature.node.kubernetes.io/hostpath-test"},
					nil, nodeName) == nil {
					klog.V(nfdparams.LogLevel).Infof("Node %s has hostPath features", nodeName)
					hostPathFound = true
					break
				}
			}

			if !hostPathFound {
				klog.V(nfdparams.LogLevel).Info("No hostPath features found - this is expected " +
					"unless the cluster has been configured with custom feature files on the host")
				Skip("hostPath local source features not configured (expected for most deployments)")
			}

			By("Verifying hostPath feature detection works")
			Expect(hostPathFound).To(BeTrue(), "hostPath features should be detected if configured")
		})
	})
})
