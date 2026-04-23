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

const (
	// releaseExtractTimeout bounds oc adm release extract in BeforeAll (helper subprocess cap is 2m).
	releaseExtractTimeout = 5 * time.Minute
	// isoOperationTimeout bounds ISO generation and SCP of the resulting artifact.
	isoOperationTimeout = 30 * time.Minute
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
			var err error

			workDir, err = os.MkdirTemp("", "ibi-preinstall-*")
			Expect(err).NotTo(HaveOccurred(), "create work dir")

			binDir := filepath.Join(workDir, "ocp-bin")
			Expect(os.MkdirAll(binDir, 0o750)).To(Succeed())

			pullSecretJSON, err := helpers.GetPullSecretFromHub(raninittools.HubAPIClient)
			Expect(err).NotTo(HaveOccurred(), "hub pull secret for oc adm release extract")

			registryConfigPath := filepath.Join(binDir, "release-extract-registry-config.json")
			Expect(os.WriteFile(registryConfigPath, []byte(pullSecretJSON), 0o600)).To(Succeed())

			bootstrapOC, err := raninittools.RANConfig.ResolveBootstrapOCPath()
			Expect(err).NotTo(HaveOccurred(), "resolve bootstrap oc for release extract")

			extractCtx, cancelExtract := context.WithTimeout(context.TODO(), releaseExtractTimeout)
			defer cancelExtract()

			err = helpers.ExtractOpenshiftInstall(
				extractCtx,
				raninittools.RANConfig.ReleaseImage,
				binDir,
				raninittools.RANConfig.HubKubeConfig,
				registryConfigPath,
				bootstrapOC,
			)
			Expect(err).NotTo(HaveOccurred(), "extract openshift-install from release image")

			openshiftInstallPath = filepath.Join(binDir, "openshift-install")
			Expect(os.Chmod(openshiftInstallPath, 0o755)).To(Succeed())

			siteCloneDir := filepath.Join(workDir, "ztp-site-config")
			err = helpers.CloneZTPSiteConfigRepo(
				raninittools.RANConfig.SiteConfigRepo,
				raninittools.RANConfig.SiteConfigBranch,
				siteCloneDir,
				raninittools.RANConfig.SiteConfigGitSkipTLS,
			)
			Expect(err).NotTo(HaveOccurred(), "clone siteconfig repository")

			kustomizeDir := filepath.Join(siteCloneDir, raninittools.RANConfig.SiteConfigKustomizePath)
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

		It("performs disconnected IBI cluster node preinstall end to end", reportxml.ID("no-testcase"), func() {
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

			nodeInput, err := helpers.ClusterInstanceInstallInputFrom(clusterInstance)
			Expect(err).NotTo(HaveOccurred(), "cluster instance install input")

			bmcUser := raninittools.RANConfig.BMCUsername
			bmcPass := raninittools.RANConfig.BMCPassword

			By("Gathering hub pull secret, SSH key, CA bundle, and image digest mirrors")

			pullSecret, err := helpers.GetPullSecretFromHub(raninittools.HubAPIClient)
			Expect(err).NotTo(HaveOccurred(), "pull secret")

			sshKey, err := helpers.GetSSHKeyFromHub(raninittools.HubAPIClient)
			Expect(err).NotTo(HaveOccurred(), "ssh key")

			caBundle, err := helpers.GetCACertFromHub(raninittools.HubAPIClient)
			Expect(err).NotTo(HaveOccurred(), "CA bundle")

			idSources, err := helpers.BuildImageDigestSourcesFromHub(context.TODO(), raninittools.HubAPIClient)
			Expect(err).NotTo(HaveOccurred(), "image digest sources")

			seedVersion := raninittools.RANConfig.SeedVersion
			if seedVersion == "" {
				seedVersion = helpers.SeedVersionFromSeedImage(raninittools.RANConfig.SeedImage)
			}

			ibiData := helpers.IBIConfigData{
				SeedImage:             raninittools.RANConfig.SeedImage,
				SeedVersion:           seedVersion,
				AdditionalTrustBundle: caBundle,
				ImageDigestSources:    idSources,
				PullSecret:            pullSecret,
				InstallationDisk:      nodeInput.InstallationDisk,
				SSHKey:                sshKey,
				NetworkConfig:         nodeInput.NetworkConfig,
				ExtraPartitionLabel:   raninittools.RANConfig.ExtraPartitionLabel,
			}

			if nodeInput.CPUArchitecture != "" {
				ibiData.Architecture = nodeInput.CPUArchitecture
			}

			if nodeInput.IgnitionConfigOverride != "" {
				ibiData.IgnitionConfigOverride = nodeInput.IgnitionConfigOverride
			}

			err = helpers.GenerateIBIConfig(ibiData, workDir)
			Expect(err).NotTo(HaveOccurred(), "render image-based-installation-config.yaml")

			By("Running openshift-install image-based create image")

			isoCtx, cancelISO := context.WithTimeout(context.TODO(), isoOperationTimeout)
			defer cancelISO()

			isoPath, err := helpers.CreateIBIISO(isoCtx, openshiftInstallPath, workDir)
			Expect(err).NotTo(HaveOccurred())

			remoteISO := raninittools.RANConfig.RemoteISOPath
			By(fmt.Sprintf("Copying ISO to provisioning host %s:%s", raninittools.RANConfig.ProvisioningHost, remoteISO))

			scpCtx, cancelSCP := context.WithTimeout(context.TODO(), isoOperationTimeout)
			defer cancelSCP()

			err = helpers.SCPToProvisioningHost(
				scpCtx,
				isoPath,
				remoteISO,
				raninittools.RANConfig.ProvisioningHost,
				raninittools.RANConfig.ProvisioningUser,
				raninittools.RANConfig.EffectiveProvisioningSSHKey(),
			)
			Expect(err).NotTo(HaveOccurred())

			remoteISOFile := filepath.Base(remoteISO)
			isoURL := raninittools.RANConfig.ISOArtifactURL(remoteISOFile)

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
				nodeInput.BMCAddress,
				nodeInput.BootMACAddress,
				tsparams.PreinstallBMCSecretName,
				isoURL,
			)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for install-rhcos-and-restore-seed on " + nodeInput.HostName)

			waitTotal := raninittools.RANConfig.EffectivePreinstallWait()
			waitCtx, cancelWait := context.WithTimeout(context.TODO(), waitTotal)
			defer cancelWait()

			err = helpers.WaitForPreinstallCompletion(
				waitCtx,
				nodeInput.HostName,
				raninittools.RANConfig.PreinstallNodeSSHUser,
				raninittools.RANConfig.EffectiveProvisioningSSHKey(),
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
