package spoke_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	installerTypes "github.com/openshift/installer/pkg/types"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	hiveextV1Beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/api/hiveextension/v1beta1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/installconfig"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/ztpinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/spoke/internal/tsparams"
)

var _ = Describe(
	"PlatformTypeVerification",
	Ordered,
	ContinueOnFailure,
	Label(tsparams.LabelPlatformVerificationTestCases), func() {

		var (
			platformType         string
			spokeClusterPlatform installerTypes.Platform
		)

		BeforeAll(func() {
			By("Get spoke cluster platformType from agentclusterinstall")
			platformType = string(ZTPConfig.SpokeAgentClusterInstall.Object.Status.PlatformType)

			By("Get spoke cluster-config configmap")
			Expect(ZTPConfig.SpokeInstallConfig.Object.Data["install-config"]).ToNot(
				BeEmpty(), "error pulling install-config from spoke cluster",
			)

			By("Read install-config data from configmap")
			installConfigData, err := installconfig.NewInstallConfigFromString(
				ZTPConfig.SpokeInstallConfig.Object.Data["install-config"])
			Expect(err).NotTo(HaveOccurred(), "error reading in install-config as yaml")

			spokeClusterPlatform = installConfigData.Platform

		})

		DescribeTable("none platform checks", func(masterCount int) {
			if platformType != string(hiveextV1Beta1.NonePlatformType) {
				Skip(fmt.Sprintf("Platform type was not %s", string(hiveextV1Beta1.NonePlatformType)))
			}
			if masterCount != ZTPConfig.SpokeAgentClusterInstall.Object.Spec.ProvisionRequirements.ControlPlaneAgents {
				Skip("Did not match controlplane agent count")
			}
			Expect(spokeClusterPlatform.None).NotTo(BeNil(), "spoke does not contain a none platform key")
		},
			Entry("SNO install", 1, reportxml.ID("56200")),
			Entry("MNO install", 3, reportxml.ID("56202")),
		)

		It("installs on BareMetal platform", reportxml.ID("56203"), func() {
			if platformType != string(hiveextV1Beta1.BareMetalPlatformType) {
				Skip(fmt.Sprintf("Platform type was not %s", string(hiveextV1Beta1.BareMetalPlatformType)))
			}
			Expect(spokeClusterPlatform.BareMetal).NotTo(BeNil(), "spoke does not contain a baremetal platform key")
		})

		It("installs on VSphere platform", reportxml.ID("56201"), func() {
			if platformType != string(hiveextV1Beta1.VSpherePlatformType) {
				Skip(fmt.Sprintf("Platform type was not %s", string(hiveextV1Beta1.VSpherePlatformType)))
			}
			Expect(spokeClusterPlatform.VSphere).NotTo(BeNil(), "spoke does not contain a vsphere platform key")
		})
	},
)
