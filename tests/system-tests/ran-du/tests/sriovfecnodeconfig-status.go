package ran_du_system_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	sriovfec "github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov-fec"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/ran-du/internal/randuinittools"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/ran-du/internal/randuparams"
)

var _ = Describe(
	"SriovFecNodeConfigStatus",
	Label("SriovFecNodeConfigStatus"),
	Ordered,
	ContinueOnFailure,
	func() {
		It("Asserts SriovFecNodeConfig resource is configured successfully", func() {
			klog.V(randuparams.RanDuLogLevel).Infof("Check SriovFecNodeConfig resource")

			nodeList, err := nodes.List(
				APIClient,
				metav1.ListOptions{},
			)
			Expect(err).ToNot(HaveOccurred(), "Error listing nodes.")

			for _, node := range nodeList {
				Eventually(func() bool {
					fecStatus, err := sriovfec.Pull(APIClient, node.Definition.Name, RanDuTestConfig.SriovFecOperatorNamespace)
					Expect(err).ToNot(HaveOccurred(), "Failed to get SriovFecNodeConfig")

					for _, condition := range fecStatus.Object.Status.Conditions {
						if condition.Type == "Configured" {
							if condition.Status == "True" {
								return true
							}
						}
					}

					return false

				}, 5*time.Minute, 30*time.Second).Should(BeTrue(), "SriovFecNodeConfig is not configured")
			}
		})
	},
)
