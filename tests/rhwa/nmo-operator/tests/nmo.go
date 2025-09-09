package tests

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/internal/rhwainittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/internal/rhwaparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/nmo-operator/internal/nmoparams"
)

var _ = Describe(
	"NMO tests",
	Ordered,
	ContinueOnFailure,
	Label(nmoparams.Label), func() {
		BeforeAll(func() {
			By("Get NMO deployment object")
			nmoDeployment, err := deployment.Pull(
				APIClient, nmoparams.OperatorDeploymentName, rhwaparams.RhwaOperatorNs)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to get NMO deployment %s", err))

			By("Verify NMO deployment is Ready")
			Expect(nmoDeployment.IsReady(rhwaparams.DefaultTimeout)).To(BeTrue(), "NMO deployment is not Ready")
		})
		It("Verify Node Maintenance Operator pod is running", reportxml.ID("46315"), func() {
			_, err := pod.WaitForAllPodsInNamespaceRunning(
				APIClient,
				rhwaparams.RhwaOperatorNs,
				rhwaparams.DefaultTimeout,
			)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Pod is not ready %s", err))
		})
	})
