package spoke_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/models"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/ztpinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/spoke/internal/diskencryption"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/spoke/internal/tsparams"
	"golang.org/x/exp/slices"
)

var (
	tpmEncryptionEnabledOn string
)

const (
	tpmMasterMachineConfig = "master-tpm"
	tpmWorkerMachineConfig = "worker-tpm"
)

var _ = Describe(
	"TPMDiskEncryption",
	Ordered,
	ContinueOnFailure,
	Label(tsparams.LabelTPMDiskEncryptionInstallTestCases), func() {
		When("on MCE 2.0 and above", func() {
			BeforeAll(func() {

				if ZTPConfig.SpokeAgentClusterInstall.Object.Spec.DiskEncryption == nil {
					Skip("Spoke cluster was not installed with disk encryption")
				}

				if *ZTPConfig.SpokeAgentClusterInstall.Object.Spec.DiskEncryption.Mode != models.DiskEncryptionModeTpmv2 {
					Skip("Spoke cluster was installed with disk encryption mode other than tpm")
				}

				tpmEncryptionEnabledOn = *ZTPConfig.SpokeAgentClusterInstall.Object.Spec.DiskEncryption.EnableOn
			})

			It("installs on all nodes", reportxml.ID("47135"), func() {
				if tpmEncryptionEnabledOn != models.DiskEncryptionEnableOnAll {
					Skip("tpm disk encryption enabledOn not set to all")
				}

				verifyTpmMasterMachineConfig()
				verifyTpmWorkerMachineConfig()
			})

			It("installs on master nodes", reportxml.ID("47133"), func() {
				if tpmEncryptionEnabledOn != models.DiskEncryptionEnableOnMasters {
					Skip("tpm disk encryption enabledOn not set to masters")
				}

				verifyTpmMasterMachineConfig()
			})

			It("installs on worker nodes", reportxml.ID("47134"), func() {
				if tpmEncryptionEnabledOn != models.DiskEncryptionEnableOnWorkers {
					Skip("tpm disk encryption enabledOn not set to workers")
				}

				verifyTpmWorkerMachineConfig()
			})

			It("proper positive validation is returned", reportxml.ID("48319"), func() {
				if tpmEncryptionEnabledOn == models.DiskEncryptionEnableOnNone {
					Skip("tpm disk encryption enabledOn set to none")
				}

				agentBuilders, err := ZTPConfig.SpokeInfraEnv.GetAllAgents()
				Expect(err).NotTo(HaveOccurred(), "error pulling agents from cluster")

				if len(agentBuilders) == 0 {
					Skip("Agent resources have been removed from hub cluster")
				}

				for _, agent := range agentBuilders {
					if tpmEncryptionEnabledOn == models.DiskEncryptionEnableOnAll ||
						strings.Contains(tpmEncryptionEnabledOn, string(agent.Object.Status.Role)) {
						hwValidations, ok := agent.Object.Status.ValidationsInfo["hardware"]
						Expect(ok).To(BeTrue(), "error attempting to retrieve agent hardware validationsInfo")
						for _, result := range hwValidations {
							if result.ID == "disk-encryption-requirements-satisfied" {
								Expect(result.Message).To(Equal("Installation disk can be encrypted using tpmv2"),
									"got unexpected hardware validation message")
								Expect(result.Status).To(Equal("success"), "got unexpected hardware validation status")
							}
						}
					}
				}

			})

		})
	})

func verifyTpmMasterMachineConfig() {
	ignitionConfig, err := diskencryption.GetIgnitionConfigFromMachineConfig(SpokeAPIClient, tpmMasterMachineConfig)
	Expect(err).NotTo(HaveOccurred(), errorGettingIgnitionConfigMsg(tpmMasterMachineConfig))

	verifyLuksTpmIgnitionConfig(ignitionConfig)
}

func verifyTpmWorkerMachineConfig() {
	ignitionConfig, err := diskencryption.GetIgnitionConfigFromMachineConfig(SpokeAPIClient, tpmWorkerMachineConfig)
	Expect(err).NotTo(HaveOccurred(), errorGettingIgnitionConfigMsg(tpmWorkerMachineConfig))

	verifyLuksTpmIgnitionConfig(ignitionConfig)
}

func verifyLuksTpmIgnitionConfig(ignitionConfig *diskencryption.IgnitionConfig) {
	luksEntry := ignitionConfig.Storage.LUKS[0]

	Expect(luksEntry.Clevis.Tpm).To(BeTrue(), "tpm not set to true in machineconfig")
	Expect(luksEntry.Device).To(Equal("/dev/disk/by-partlabel/root"), "found incorrect root device label")
	Expect(luksEntry.Name).To(Equal("root"), "found incorrect luks name")
	Expect(slices.Contains(luksEntry.Options, "--cipher")).To(BeTrue(), "luks device options did not contain --cipher")
	Expect(slices.Contains(luksEntry.Options, "aes-cbc-essiv:sha256")).To(BeTrue(),
		"luks device options did not contain aes-cbc-essiv:sha256")
	Expect(luksEntry.WipeVolume).To(BeTrue(), "luks device has wipevolume set to false")
}
