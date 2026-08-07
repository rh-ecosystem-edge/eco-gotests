package tests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmh"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ibipreinstall/internal/helpers"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ibipreinstall/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
)

const (
	isoOperationTimeout       = 30 * time.Minute
	deleteResourceWaitTimeout = 5 * time.Minute
)

// SpokeHostName is the resolved spoke hostname, set in BeforeAll for use by the
// suite-level diagnostics collector.
var SpokeHostName string

// WorkDir is the temporary directory created for openshift-install artifacts,
// set in BeforeAll for use by the suite-level diagnostics collector.
var WorkDir string

var _ = Describe(
	"IBI preinstall",
	Ordered,
	Label(tsparams.LabelEndToEndPreinstall),
	func() {
		var (
			nodeInput     *helpers.ClusterInstanceInstallInput
			bmhName       string
			bmcSecretName string
		)

		BeforeAll(func() {
			var err error

			WorkDir, err = os.MkdirTemp("", "ibi-preinstall-*")
			Expect(err).NotTo(HaveOccurred(), "create work dir")

			ibiCfg := RANConfig.IBIPreinstallConfig

			By("Phase 1: Resolving all data sources before expensive operations")

			By("Resolving seed version from env override or image tag")

			seedVersion, err := helpers.ResolveSeedVersion(
				ibiCfg.SeedVersion,
				ibiCfg.SeedImage,
			)
			Expect(err).NotTo(HaveOccurred(), "resolve seed version")

			By("Fetching hub resources (pull-secret, SSH key, CA bundle)")

			pullSecret, err := helpers.GetPullSecretFromHub(HubAPIClient)
			Expect(err).NotTo(HaveOccurred(), "hub pull secret")

			sshKey, err := helpers.GetSSHKeyFromHub(HubAPIClient)
			Expect(err).NotTo(HaveOccurred(), "hub ssh key")

			caBundle, err := helpers.GetCACertFromHub(HubAPIClient)
			Expect(err).NotTo(HaveOccurred(), "hub CA bundle")

			By("Discovering MCE/ACM mirror locations from hub IDMS")

			mceACMMirrors, err := helpers.DiscoverMCEACMMirrorsFromHub(HubAPIClient)
			Expect(err).NotTo(HaveOccurred(), "discover MCE/ACM mirrors from hub")

			By("Fetching ClusterInstance for BMH provisioning fields")

			ciData, err := helpers.FetchYAMLFromURL(ibiCfg.ClusterInstanceURL, RANConfig.SkipTLSVerify)
			Expect(err).NotTo(HaveOccurred(), "fetch ClusterInstance YAML")

			clusterInstance, err := helpers.ParseClusterInstance(ciData)
			Expect(err).NotTo(HaveOccurred(), "parse ClusterInstance")

			nodeInput, err = helpers.ClusterInstanceInstallInputFrom(clusterInstance)
			Expect(err).NotTo(HaveOccurred(), "extract node input from ClusterInstance")

			bmhName = nodeInput.PreinstallBMHName()
			bmcSecretName = nodeInput.PreinstallBMCSecretName()
			SpokeHostName = nodeInput.HostName

			if nodeInput.NetworkConfig != "" {
				By("Verifying nmstatectl is available (required by openshift-install for networkConfig)")
				Expect(helpers.VerifyNmstatectlAvailable()).To(Succeed(),
					"nmstatectl must be installed when networkConfig is specified — run: dnf install -y nmstate")
			}

			By("Resolving IBI config template URL")

			templateURL, err := helpers.ResolveIBIConfigTemplateURL(
				ibiCfg.IBIConfigTemplateURL, ibiCfg.ClusterInstanceURL)
			Expect(err).NotTo(HaveOccurred(), "resolve IBI config template URL")

			preinstallRegistry := ibiCfg.PreinstallRegistry
			if preinstallRegistry == "" {
				preinstallRegistry, err = helpers.RegistryHostFromMirror(mceACMMirrors.MCE)
				Expect(err).NotTo(HaveOccurred(), "derive preinstall registry from MCE mirror")

				klog.V(tsparams.LogLevel).Infof(
					"Derived preinstall registry from MCE mirror: %s", preinstallRegistry)
			}

			By("Resolving IBI config template and writing to work dir")

			err = helpers.ResolveIBIConfigTemplate(
				templateURL,
				&helpers.IBIConfigTemplateData{
					SeedImage:          ibiCfg.SeedImage,
					SeedVersion:        seedVersion,
					PullSecret:         pullSecret,
					SSHKey:             sshKey,
					CACert:             caBundle,
					NetworkConfig:      nodeInput.NetworkConfig,
					PreinstallRegistry: preinstallRegistry,
					MCEMirror:          mceACMMirrors.MCE,
					ACMMirror:          mceACMMirrors.ACM,
				},
				WorkDir,
				RANConfig.SkipTLSVerify,
			)
			Expect(err).NotTo(HaveOccurred(), "resolve IBI config template")

			By("Phase 1 complete - all data sources resolved, config written")
		})

		AfterAll(func() {
			if WorkDir != "" {
				_ = os.RemoveAll(WorkDir)
			}

			if httpDir := RANConfig.IBIPreinstallConfig.PreinstallHTTPDir; httpDir != "" {
				_ = os.Remove(filepath.Join(httpDir, tsparams.ISOFilename))
			}
		})

		AfterEach(func() {
			if bmhName == "" {
				return
			}

			bmhBuilder, err := bmh.Pull(HubAPIClient, bmhName, tsparams.PreinstallBMHNamespace)
			if err == nil {
				if _, delErr := bmhBuilder.DeleteAndWaitUntilDeleted(deleteResourceWaitTimeout); delErr != nil {
					klog.V(tsparams.LogLevel).Infof("Cleanup: failed to delete BMH %s: %v", bmhName, delErr)
				}
			}

			if delErr := secret.NewBuilder(
				HubAPIClient, bmcSecretName, tsparams.PreinstallBMHNamespace, corev1.SecretTypeOpaque,
			).Delete(); delErr != nil {
				klog.V(tsparams.LogLevel).Infof("Cleanup: failed to delete BMC secret %s: %v", bmcSecretName, delErr)
			}
		})

		It("performs disconnected IBI cluster node preinstall end to end", reportxml.ID("89666"), func() {
			ibiCfg := RANConfig.IBIPreinstallConfig

			By("Running openshift-install image-based create image")

			isoCtx, cancelISO := context.WithTimeout(context.TODO(), isoOperationTimeout)
			defer cancelISO()

			isoPath, err := helpers.CreateIBIISO(isoCtx, ibiCfg.OpenshiftInstallPath, WorkDir, tsparams.ISOFilename)
			Expect(err).NotTo(HaveOccurred(), "openshift-install image-based create image failed")

			By("Copying ISO to local HTTP serving directory")

			err = helpers.CopyISOToHTTPDir(isoPath, ibiCfg.PreinstallHTTPDir)
			Expect(err).NotTo(HaveOccurred(), "failed to copy ISO to HTTP serving directory")

			isoURL := ibiCfg.ISOArtifactURL(tsparams.ISOFilename)

			By(fmt.Sprintf("Verifying ISO is accessible at %s", isoURL))
			Expect(helpers.VerifyHTTPAccessible(isoURL)).To(Succeed(),
				"ISO must be reachable via HTTP before creating BMH")

			By("Creating BMC secret and BareMetalHost on the hub")

			_, err = helpers.CreateBMCSecret(
				HubAPIClient,
				bmcSecretName,
				tsparams.PreinstallBMHNamespace,
				RANConfig.BMCUsername,
				RANConfig.BMCPassword,
			)
			Expect(err).NotTo(HaveOccurred(), "failed to create BMC secret")

			_, err = helpers.CreateBareMetalHost(
				HubAPIClient,
				bmhName,
				tsparams.PreinstallBMHNamespace,
				nodeInput.BMCAddress,
				nodeInput.BootMACAddress,
				bmcSecretName,
				isoURL,
			)
			Expect(err).NotTo(HaveOccurred(), "failed to create BareMetalHost")

			By("Waiting for BMH to reach provisioned state")

			err = helpers.WaitForBMHProvisioned(
				context.TODO(),
				HubAPIClient,
				bmhName, tsparams.PreinstallBMHNamespace,
				20*time.Minute,
				30*time.Second,
			)
			Expect(err).NotTo(HaveOccurred(), "BMH must reach provisioned state")

			By("Waiting for " + tsparams.PreinstallServiceUnit + " on " + nodeInput.HostName)

			err = helpers.WaitForPreinstallCompletion(
				context.TODO(),
				nodeInput.HostName,
				tsparams.TargetNodeSSHUser,
				ibiCfg.PreinstallSSHKey,
				tsparams.PreinstallWaitTimeout,
				time.Minute,
			)
			Expect(err).NotTo(HaveOccurred(),
				tsparams.PreinstallServiceUnit+" must complete successfully on "+nodeInput.HostName)
		})
	},
)
