package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/olm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/internal/deploy"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/2upgrade/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/2upgrade/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/nfdconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/nfdhelpers"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/nfdparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	"k8s.io/klog/v2"
)

var _ = Describe("NFD", Ordered, Label(nfdparams.Label), func() {

	Context("Operator", Label(tsparams.NfdUpgradeLabel), func() {
		nfdConfig := nfdconfig.NewNfdConfig()

		var nfdInstaller *deploy.OperatorInstaller
		var nfdCRUtils *deploy.NFDCRUtils

		BeforeAll(func() {
			if nfdConfig.CatalogSource == "" {
				Skip("No CatalogSourceName defined. Skipping test")
			}

			var options *nfdhelpers.NFDInstallConfigOptions
			if nfdConfig.CatalogSource != "" {
				options = &nfdhelpers.NFDInstallConfigOptions{
					CatalogSource: nfdhelpers.StringPtr(nfdConfig.CatalogSource),
				}
			}

			installConfig := nfdhelpers.GetDefaultNFDInstallConfig(APIClient, options)
			nfdInstaller = deploy.NewOperatorInstaller(installConfig)
			nfdCRUtils = deploy.NewNFDCRUtils(APIClient, installConfig.Namespace, nfdparams.NfdInstance)

			By("Installing NFD operator")
			err := nfdInstaller.Install()
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error installing NFD operator: %s", err))

			By("Waiting for NFD operator CSV to be ready")
			ready, err := nfdhelpers.WaitForNFDOperatorReady(APIClient, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error waiting for NFD operator CSV")
			Expect(ready).To(BeTrue(), "NFD operator CSV not ready")

			By("Creating NFD CR")
			crConfig := deploy.NFDCRConfig{
				Image: nfdConfig.Image,
			}
			err = nfdCRUtils.DeployNFDCR(crConfig)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error creating NFD CR: %s", err))

			By("Waiting for NFD CR to be ready")
			crReady, err := nfdCRUtils.IsNFDCRReady(5 * time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error waiting for NFD CR")
			Expect(crReady).To(BeTrue(), "NFD CR not ready")
		})

		AfterAll(func() {
			By("Deleting NFD CR")
			err := nfdCRUtils.DeleteNFDCR()
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error deleting NFD CR: %s", err))

			By("Uninstalling NFD operator")
			uninstallConfig := nfdhelpers.GetDefaultNFDUninstallConfig(
				APIClient,
				"nfd-operator-group",
				"nfd-subscription")
			nfdUninstaller := deploy.NewOperatorUninstaller(uninstallConfig)
			err = nfdUninstaller.Uninstall()
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error uninstalling NFD: %s", err))
		})

		It("should upgrade successfully", reportxml.ID("54540"), func() {
			if nfdConfig.UpgradeTargetVersion == "" {
				Skip("No UpgradeTargetVersion defined. Skipping test")
			}
			if nfdConfig.CustomCatalogSource == "" {
				Skip("No CustomCatalogSource defined. Skipping test")
			}

			By("Getting NFD subscription")
			sub, err := olm.PullSubscription(APIClient, "nfd-subscription", nfdparams.NFDNamespace)
			Expect(err).ToNot(HaveOccurred(), "failed getting subscription")

			By("Update subscription to use new catalog source")
			klog.V(nfdparams.LogLevel).Infof("Current CatalogSource: %s", sub.Object.Spec.CatalogSource)
			sub.Definition.Spec.CatalogSource = nfdConfig.CustomCatalogSource
			_, err = sub.Update()
			Expect(err).ToNot(HaveOccurred(), "failed updating subscription")

			By("Await operator to be upgraded")
			err = await.OperatorUpgrade(APIClient, nfdConfig.UpgradeTargetVersion, 10*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "failed awaiting subscription upgrade")
		})
	})
})
