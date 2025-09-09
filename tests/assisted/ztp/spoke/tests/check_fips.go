package spoke_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/installconfig"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/ztpinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/spoke/internal/tsparams"
)

var _ = Describe(
	"Openshift Spoke cluster deployed with FIPS mode enabled",
	Ordered,
	ContinueOnFailure,
	Label(tsparams.LabelFipsVerificationTestCases), func() {
		var fipsEnabledOnSpoke bool
		When("on MCE 2.0 and above", func() {
			BeforeAll(func() {
				installConfigData, err := installconfig.NewInstallConfigFromString(
					ZTPConfig.HubInstallConfig.Object.Data["install-config"])
				Expect(err).NotTo(HaveOccurred(), "error reading in install-config as yaml")
				if !installConfigData.FIPS {
					Skip("Hub cluster is not FIPS enabled")
				}

				By("Checking agentclusterinstall has fips:true annotation")
				fipsEnabledOnSpoke = false
				if override,
					ok := ZTPConfig.SpokeAgentClusterInstall.
					Object.Annotations["agent-install.openshift.io/install-config-overrides"]; ok {
					agentClusterInstallOverrideConfig, err := installconfig.NewInstallConfigFromString(override)
					Expect(err).ToNot(HaveOccurred(), "error getting installconfig from spoke override")

					if agentClusterInstallOverrideConfig.FIPS {
						fipsEnabledOnSpoke = true
					}
				}
				if !fipsEnabledOnSpoke {
					Skip("spoke should be installed with fips enabled")
				}
			})

			It("Assert Spoke cluster was deployed with FIPS", reportxml.ID("65865"), func() {
				By("Getting configmap")
				Expect(ZTPConfig.SpokeInstallConfig.Object.Data["install-config"]).ToNot(BeEmpty(),
					"error pulling install-config from spoke cluster")
				installConfigData, err := installconfig.NewInstallConfigFromString(
					ZTPConfig.SpokeInstallConfig.Object.Data["install-config"])
				Expect(err).NotTo(HaveOccurred(), "error reading in install-config as yaml")
				Expect(installConfigData.FIPS).To(BeTrue(),
					"spoke Cluster is not installed with fips")
			})
		})
	})
