package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/olm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	nfdDeploy "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/internal/deploy"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/internal/hwaccelparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/2upgrade/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/2upgrade/internal/tsparams"
	NfdConfig "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/nfdconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/nfdparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	"k8s.io/klog/v2"
)

var _ = Describe("NFD", Ordered, Label(nfdparams.Label), func() {

	Context("Operator", Label(tsparams.NfdUpgradeLabel), func() {
		nfdConfig := NfdConfig.NewNfdConfig()

		nfdManager := nfdDeploy.NewNfdAPIResource(APIClient,
			hwaccelparams.NFDNamespace,
			"op-nfd",
			"nfd",
			nfdConfig.CatalogSource,
			"openshift-marketplace",
			"nfd",
			"stable")

		BeforeAll(func() {
			if nfdConfig.CatalogSource == "" {
				Skip("No CatalogSourceName defined. Skipping test")
			}
			By("Creating nfd")
			err := nfdManager.DeployNfd(25*int(time.Minute), false, "")
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error in deploy NFD %s", err))
		})
		AfterAll(func() {
			By("Undeploy NFD instance")
			err := nfdManager.UndeployNfd(tsparams.NfdInstance)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error in undeploy NFD %s", err))

		})
		It("should upgrade successfully", reportxml.ID("54540"), func() {

			if nfdConfig.UpgradeTargetVersion == "" {
				Skip("No UpgradeTargetVersion defined. Skipping test ")
			}
			if nfdConfig.CustomCatalogSource == "" {
				Skip("No CustomCatalogSource defined. Skipping test ")
			}
			By("Getting NFD subscription")
			sub, err := olm.PullSubscription(APIClient, "nfd", nfdparams.NFDNamespace)
			Expect(err).ToNot(HaveOccurred(), "failed getting subscription")

			By("Update subscription to use new catalog source")
			klog.V(nfdparams.LogLevel).Infof("SUB: %s", sub.Object.Spec.CatalogSource)
			sub.Definition.Spec.CatalogSource = nfdConfig.CustomCatalogSource
			_, err = sub.Update()
			Expect(err).ToNot(HaveOccurred(), "failed updating subscription")

			By("Await operator to be upgraded")
			versionRegexPattern := fmt.Sprintf(`(%s)\S+`, nfdConfig.UpgradeTargetVersion)
			err = await.OperatorUpgrade(APIClient, versionRegexPattern, 10*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "failed awaiting subscription upgrade")
		})
	})
})
