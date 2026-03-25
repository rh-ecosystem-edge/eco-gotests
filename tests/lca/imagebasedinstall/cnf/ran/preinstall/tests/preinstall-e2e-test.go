package preinstall_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"

	siteconfigv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/siteconfig/v1alpha1"
	raninittools "github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedinstall/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedinstall/cnf/ran/preinstall/internal/helpers"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedinstall/cnf/ran/preinstall/internal/tsparams"
)

var _ = Describe(
	"IBI preinstall",
	Ordered,
	Label(tsparams.LabelEndToEndPreinstall),
	func() {
		var (
			workDir              string
			openshiftInstallPath string
			clusterInstance      *siteconfigv1alpha1.ClusterInstance
		)

		BeforeAll(func() {
			if raninittools.RanConfig == nil {
				Skip("Ran configuration failed to load")
			}

			if raninittools.HubAPIClient == nil {
				Skip("Hub API client is nil (set ECO_LCA_IBI_CNF_RAN_HUB_KUBECONFIG)")
			}

			if err := raninittools.RanConfig.ValidateMandatory(); err != nil {
				Skip(fmt.Sprintf("IBI preinstall mandatory configuration incomplete: %v", err))
			}

			var err error

			workDir, err = os.MkdirTemp("", "ibi-preinstall-*")
			Expect(err).NotTo(HaveOccurred(), "create work dir")

			binDir := filepath.Join(workDir, "ocp-bin")
			Expect(os.MkdirAll(binDir, 0o750)).To(Succeed())

			pullSecretJSON, err := helpers.GetPullSecretFromHub(raninittools.HubAPIClient)
			Expect(err).NotTo(HaveOccurred(), "hub pull secret for oc adm release extract")

			registryConfigPath := filepath.Join(binDir, "release-extract-registry-config.json")
			Expect(os.WriteFile(registryConfigPath, []byte(pullSecretJSON), 0o600)).To(Succeed())

			bootstrapOC, err := raninittools.RanConfig.ResolveBootstrapOCPath()
			Expect(err).NotTo(HaveOccurred(), "resolve bootstrap oc for release extract")

			err = helpers.ExtractOpenshiftInstall(
				raninittools.RanConfig.ReleaseImage,
				binDir,
				raninittools.RanConfig.HubKubeConfig,
				registryConfigPath,
				bootstrapOC,
			)
			Expect(err).NotTo(HaveOccurred(), "extract openshift-install from release image")

			openshiftInstallPath = filepath.Join(binDir, "openshift-install")
			Expect(os.Chmod(openshiftInstallPath, 0o755)).To(Succeed())

			siteCloneDir := filepath.Join(workDir, "ztp-site-config")
			err = helpers.CloneZTPSiteConfigRepo(
				raninittools.RanConfig.SiteConfigRepo,
				raninittools.RanConfig.SiteConfigBranch,
				siteCloneDir,
				raninittools.RanConfig.SiteConfigGitSkipTLS,
			)
			Expect(err).NotTo(HaveOccurred(), "clone siteconfig repository")

			kustomizeDir := filepath.Join(siteCloneDir, raninittools.RanConfig.SiteConfigKustomizePath)
			kustomizeOut, err := helpers.RunKustomize(kustomizeDir)
			Expect(err).NotTo(HaveOccurred(), "kustomize build siteconfig")

			clusterInstance, err = helpers.ParseClusterInstance(kustomizeOut)
			Expect(err).NotTo(HaveOccurred(), "parse ClusterInstance from kustomize output")
		})

		AfterAll(func() {
			if workDir != "" {
				_ = os.RemoveAll(workDir)
			}
		})

		It("performs disconnected IBI cluster node preinstall end to end", reportxml.ID("1111111111"), func() {
			ctx := context.Background()

			DeferCleanup(func() {
				_ = helpers.DeletePreinstallBMHResources(
					raninittools.HubAPIClient,
					tsparams.PreinstallBMHName,
					tsparams.PreinstallBMHNamespace,
					tsparams.PreinstallBMCSecretName,
				)
			})

			By("Removing any leftover BareMetalHost / BMC secret from a prior run")

			err := helpers.DeletePreinstallBMHResources(
				raninittools.HubAPIClient,
				tsparams.PreinstallBMHName,
				tsparams.PreinstallBMHNamespace,
				tsparams.PreinstallBMCSecretName,
			)
			Expect(err).NotTo(HaveOccurred())

			By("Reading ClusterInstance node and cluster fields")

			hostName, err := helpers.NodeHostNameFromClusterInstance(clusterInstance)
			Expect(err).NotTo(HaveOccurred(), "hostName")

			bmcAddr, err := helpers.BMCAddressFromClusterInstance(clusterInstance)
			Expect(err).NotTo(HaveOccurred(), "bmcAddress")

			bootMAC, err := helpers.BootMACAddressFromClusterInstance(clusterInstance)
			Expect(err).NotTo(HaveOccurred(), "bootMAC")

			netCfg, err := helpers.NetworkConfigForInstallation(clusterInstance)
			Expect(err).NotTo(HaveOccurred(), "node network config")

			installDisk, err := helpers.InstallationDiskFromClusterInstance(clusterInstance)
			Expect(err).NotTo(HaveOccurred(), "installation disk")

			arch, err := helpers.IBICPUArchitectureFromClusterInstance(clusterInstance)
			Expect(err).NotTo(HaveOccurred(), "cpu architecture")

			ignition, err := helpers.IgnitionConfigOverrideFromClusterInstance(clusterInstance)
			Expect(err).NotTo(HaveOccurred(), "ignition override")

			bmcUser := raninittools.RanConfig.BMCUsername
			bmcPass := raninittools.RanConfig.BMCPassword

			By("Gathering hub pull secret, SSH key, CA bundle, and image digest mirrors")

			pullSecret, err := helpers.GetPullSecretFromHub(raninittools.HubAPIClient)
			Expect(err).NotTo(HaveOccurred(), "pull secret")

			sshKey, err := helpers.GetSSHKeyFromHub(raninittools.HubAPIClient)
			Expect(err).NotTo(HaveOccurred(), "ssh key")

			caBundle, err := helpers.GetCACertFromHub(raninittools.HubAPIClient)
			Expect(err).NotTo(HaveOccurred(), "CA bundle")

			idSources, err := helpers.BuildImageDigestSourcesFromHub(ctx, raninittools.HubAPIClient)
			Expect(err).NotTo(HaveOccurred(), "image digest sources")

			seedVersion := helpers.SeedVersionFromSeedImage(raninittools.RanConfig.SeedImage)

			ibiData := helpers.IBIConfigData{
				SeedImage:             raninittools.RanConfig.SeedImage,
				SeedVersion:           seedVersion,
				AdditionalTrustBundle: caBundle,
				ImageDigestSources:    idSources,
				PullSecret:            pullSecret,
				InstallationDisk:      installDisk,
				SSHKey:                sshKey,
				NetworkConfig:         netCfg,
				ExtraPartitionLabel:   raninittools.RanConfig.ExtraPartitionLabel,
			}

			if arch != "" {
				ibiData.Architecture = arch
			}

			if ignition != "" {
				ibiData.IgnitionConfigOverride = ignition
			}

			err = helpers.GenerateIBIConfig(ibiData, workDir)
			Expect(err).NotTo(HaveOccurred(), "render image-based-installation-config.yaml")

			By("Running openshift-install image-based create image")

			isoPath, err := helpers.CreateIBIISO(openshiftInstallPath, workDir)
			Expect(err).NotTo(HaveOccurred())

			remoteISO := raninittools.RanConfig.RemoteISOPath
			By(fmt.Sprintf("Copying ISO to provisioning host %s:%s", raninittools.RanConfig.ProvisioningHost, remoteISO))

			err = helpers.SCPToProvisioningHost(
				isoPath,
				remoteISO,
				raninittools.RanConfig.ProvisioningHost,
				raninittools.RanConfig.ProvisioningUser,
				raninittools.RanConfig.EffectiveProvisioningSSHKey(),
			)
			Expect(err).NotTo(HaveOccurred())

			isoURL := raninittools.RanConfig.ISOArtifactURL("rhcos-ibi.iso")

			By("Creating BMC secret and BareMetalHost on the hub")

			_, err = helpers.CreateBMCSecret(
				raninittools.HubAPIClient,
				tsparams.PreinstallBMCSecretName,
				tsparams.PreinstallBMHNamespace,
				bmcUser,
				bmcPass,
			)
			Expect(err).NotTo(HaveOccurred())

			_, err = helpers.CreateBareMetalHost(
				raninittools.HubAPIClient,
				tsparams.PreinstallBMHName,
				tsparams.PreinstallBMHNamespace,
				bmcAddr,
				bootMAC,
				tsparams.PreinstallBMCSecretName,
				isoURL,
			)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for install-rhcos-and-restore-seed on " + hostName)

			waitTotal := raninittools.RanConfig.EffectivePreinstallWait()
			err = helpers.WaitForPreinstallCompletion(
				hostName,
				raninittools.RanConfig.PreinstallNodeSSHUser,
				raninittools.RanConfig.EffectiveProvisioningSSHKey(),
				waitTotal,
				time.Minute,
			)
			Expect(err).NotTo(HaveOccurred())

			By("Removing BareMetalHost and BMC secret after successful preinstall")

			err = helpers.DeletePreinstallBMHResources(
				raninittools.HubAPIClient,
				tsparams.PreinstallBMHName,
				tsparams.PreinstallBMHNamespace,
				tsparams.PreinstallBMCSecretName,
			)
			Expect(err).NotTo(HaveOccurred())
		})
	},
)
