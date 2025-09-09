package spoke_test

import (
	"encoding/json"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/assisted"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/models"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/ztpinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/spoke/internal/diskencryption"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/spoke/internal/tsparams"
	"golang.org/x/exp/slices"
)

var (
	tangEncryptionEnabledOn string
	tangServers             map[string]TangServer
)

const (
	tangMasterMachineConfig = "master-tang"
	tangWorkerMachineConfig = "worker-tang"
)

// TangServer represents an entry from the agentclusterinstall tangServers field.
type TangServer struct {
	URL        string `json:"URL"`
	Thumbprint string `json:"Thumbprint"`
}

var _ = Describe(
	"TangDiskEncryption",
	Ordered,
	ContinueOnFailure,
	Label(tsparams.LabelTangDiskEncryptionInstallTestCases), func() {
		When("on MCE 2.0 and above", func() {
			BeforeAll(func() {

				if ZTPConfig.SpokeAgentClusterInstall.Object.Spec.DiskEncryption == nil {
					Skip("Spoke cluster was not installed with disk encryption")
				}

				if *ZTPConfig.SpokeAgentClusterInstall.Object.Spec.DiskEncryption.Mode != models.DiskEncryptionModeTang {
					Skip("Spoke cluster was installed with disk encryption mode other than tang")
				}

				tangEncryptionEnabledOn = *ZTPConfig.SpokeAgentClusterInstall.Object.Spec.DiskEncryption.EnableOn

				var err error
				tangServers, err = createTangServersFromAgentClusterInstall(ZTPConfig.SpokeAgentClusterInstall)
				Expect(err).NotTo(HaveOccurred(), "error getting tang servers from spoke agentclusterinstall")
			})

			It("installs on all nodes", reportxml.ID("51218"), func() {
				if tangEncryptionEnabledOn != models.DiskEncryptionEnableOnAll {
					Skip("Tang disk encryption enabledOn not set to all")
				}

				verifyMasterMachineConfig()
				verifyWorkerMachineConfig()
			})

			It("installs on master nodes", reportxml.ID("47136"), func() {
				if tangEncryptionEnabledOn != models.DiskEncryptionEnableOnMasters {
					Skip("Tang disk encryption enabledOn not set to masters")
				}

				verifyMasterMachineConfig()
			})

			It("installs on worker nodes", reportxml.ID("47137"), func() {
				if tangEncryptionEnabledOn != models.DiskEncryptionEnableOnWorkers {
					Skip("Tang disk encryption enabledOn not set to workers")
				}

				verifyWorkerMachineConfig()
			})

			It("proper positive validation is returned", reportxml.ID("48320"), func() {
				if tangEncryptionEnabledOn == models.DiskEncryptionEnableOnNone {
					Skip("Tang disk encryption enabledOn set to none")
				}

				agentBuilders, err := ZTPConfig.SpokeInfraEnv.GetAllAgents()
				Expect(err).NotTo(HaveOccurred(), "error pulling agents from cluster")

				if len(agentBuilders) == 0 {
					Skip("Agent resources have been removed from hub cluster")
				}

				for _, agent := range agentBuilders {
					if tangEncryptionEnabledOn == models.DiskEncryptionEnableOnAll ||
						strings.Contains(tangEncryptionEnabledOn, string(agent.Object.Status.Role)) {
						hwValidations, ok := agent.Object.Status.ValidationsInfo["hardware"]
						Expect(ok).To(BeTrue(), "error attempting to retrieve agent hardware validationsInfo")
						for _, result := range hwValidations {
							if result.ID == "disk-encryption-requirements-satisfied" {
								Expect(result.Message).To(Equal("Installation disk can be encrypted using tang"),
									"got unexpected hardware validation message")
								Expect(result.Status).To(Equal("success"), "got unexpected hardware validation status")
							}
						}
					}
				}

			})

			It("propagates with multiple tang servers defined", reportxml.ID("48329"), func() {
				if len(tangServers) == 1 {
					Skip("Only a single tang server used for installation")
				}

				var ignitionConfigs []*diskencryption.IgnitionConfig
				if tangEncryptionEnabledOn == models.DiskEncryptionEnableOnAll ||
					tangEncryptionEnabledOn == models.DiskEncryptionEnableOnMasters {
					masterTangIgnition, err := diskencryption.GetIgnitionConfigFromMachineConfig(
						SpokeAPIClient, tangMasterMachineConfig)
					Expect(err).NotTo(HaveOccurred(), errorGettingIgnitionConfigMsg())
					ignitionConfigs = append(ignitionConfigs, masterTangIgnition)
				}

				if tangEncryptionEnabledOn == models.DiskEncryptionEnableOnAll ||
					tangEncryptionEnabledOn == models.DiskEncryptionEnableOnWorkers {
					workerTangIgnition, err := diskencryption.GetIgnitionConfigFromMachineConfig(
						SpokeAPIClient, tangWorkerMachineConfig)
					Expect(err).NotTo(HaveOccurred(), errorGettingIgnitionConfigMsg())
					ignitionConfigs = append(ignitionConfigs, workerTangIgnition)
				}

				for _, ignition := range ignitionConfigs {
					verifyTangServerConsistency(ignition)
				}
			})
		})
	})

func createTangServersFromAgentClusterInstall(
	builder *assisted.AgentClusterInstallBuilder) (map[string]TangServer, error) {
	var tangServers []TangServer

	err := json.Unmarshal([]byte(builder.Object.Spec.DiskEncryption.TangServers), &tangServers)
	if err != nil {
		return nil, err
	}

	var tangServerMap = make(map[string]TangServer)

	for _, server := range tangServers {
		tangServerMap[server.URL] = server
	}

	return tangServerMap, nil
}

func verifyMasterMachineConfig() {
	ignitionConfig, err := diskencryption.GetIgnitionConfigFromMachineConfig(
		SpokeAPIClient, tangMasterMachineConfig)
	Expect(err).NotTo(HaveOccurred(), errorGettingIgnitionConfigMsg(tangMasterMachineConfig))
	verifyLuksTangIgnitionConfig(ignitionConfig)
}

func verifyWorkerMachineConfig() {
	ignitionConfig, err := diskencryption.GetIgnitionConfigFromMachineConfig(
		SpokeAPIClient, tangWorkerMachineConfig)
	Expect(err).NotTo(HaveOccurred(), errorGettingIgnitionConfigMsg(tangWorkerMachineConfig))
	verifyLuksTangIgnitionConfig(ignitionConfig)
}

func verifyLuksTangIgnitionConfig(ignitionConfig *diskencryption.IgnitionConfig) {
	verifyTangServerConsistency(ignitionConfig)
	luksEntry := ignitionConfig.Storage.LUKS[0]

	Expect(luksEntry.Device).To(Equal("/dev/disk/by-partlabel/root"), "found incorrect root device label")
	Expect(luksEntry.Name).To(Equal("root"), "found incorrect luks name")
	Expect(slices.Contains(luksEntry.Options, "--cipher")).To(BeTrue(), "luks device options did not contain --cipher")
	Expect(slices.Contains(luksEntry.Options, "aes-cbc-essiv:sha256")).To(BeTrue(),
		"luks device options did not contain aes-cbc-essiv:sha256")
	Expect(luksEntry.WipeVolume).To(BeTrue(), "luks device has wipevolume set to false")
}

func verifyTangServerConsistency(ignitionConfig *diskencryption.IgnitionConfig) {
	luksEntry := ignitionConfig.Storage.LUKS[0]

	Expect(len(luksEntry.Clevis.Tang)).To(Equal(len(tangServers)),
		"machineconfig tang server entries do not match entries in agentclusterinstall")

	for _, ignitionConfigServer := range luksEntry.Clevis.Tang {
		aciServer, ok := tangServers[ignitionConfigServer.URL]
		Expect(ok).To(BeTrue(), "ignition config contains incorrect tang url: "+ignitionConfigServer.URL)
		Expect(ignitionConfigServer.Thumbprint).To(Equal(aciServer.Thumbprint),
			"machineconfig and agentclusterinstall tang thumbprints do not match")
	}
}

func errorGettingIgnitionConfigMsg(masterMachineConfigName ...string) string {
	errorMsg := "error getting ignition config from "
	if len(masterMachineConfigName) > 0 {
		errorMsg += masterMachineConfigName[0]
	}

	return errorMsg + " machineconfig"
}
