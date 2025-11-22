package tests

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/cgu"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/rancluster"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/talm/internal/helper"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/talm/internal/mount"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/talm/internal/setup"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/talm/internal/tsparams"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
)

var _ = Describe("TALM backup tests", Label(tsparams.LabelBackupTestCases), func() {
	var (
		loopbackDevicePath string
		err                error
	)

	BeforeEach(func() {
		By("checking that the talm version is at least 4.11")
		versionInRange, err := version.IsVersionStringInRange(RANConfig.HubOperatorVersions[ranparam.TALM], "4.11", "")
		Expect(err).ToNot(HaveOccurred(), "Failed to compared talm version string")

		if !versionInRange {
			Skip("backup tests require talm 4.11 or higher")
		}

		By("checking that the talm version is at most 4.15")
		versionInRange, err = version.IsVersionStringInRange(RANConfig.HubOperatorVersions[ranparam.TALM], "", "4.15")
		Expect(err).ToNot(HaveOccurred(), "Failed to compare talm version string")

		if !versionInRange {
			Skip("backup tests are deprecated for talm 4.16 and higher")
		}
	})

	When("there is a single spoke", func() {
		BeforeEach(func() {
			By("checking that the hub and spoke 1 are present")
			Expect(rancluster.AreClustersPresent([]*clients.Settings{HubAPIClient, Spoke1APIClient})).
				To(BeTrue(), "Failed due to missing API client")
		})

		AfterEach(func() {
			By("cleaning up resources on hub")
			errorList := setup.CleanupTestResourcesOnHub(HubAPIClient, tsparams.TestNamespace, "")
			Expect(errorList).To(BeEmpty(), "Failed to clean up test resources on hub")

			By("cleaning up resources on spoke 1")
			errorList = setup.CleanupTestResourcesOnSpokes([]*clients.Settings{Spoke1APIClient}, "")
			Expect(errorList).To(BeEmpty(), "Failed to clean up test resources on spoke 1")
		})

		Context("with full disk for spoke1", func() {
			BeforeEach(func() {
				By("setting up filesystem to simulate low space")
				loopbackDevicePath, err = mount.PrepareEnvWithSmallMountPoint(Spoke1APIClient)
				Expect(err).ToNot(HaveOccurred(), "Failed to prepare mount point")
			})

			AfterEach(func() {
				By("starting disk-full env clean up")
				err = mount.DiskFullEnvCleanup(Spoke1APIClient, loopbackDevicePath)
				Expect(err).ToNot(HaveOccurred(), "Failed to clean up mount point")
			})

			// 50835 - Insufficient Backup Partition Size
			It("should have a failed cgu for single spoke", reportxml.ID("50835"), func() {
				By("applying all the required CRs for backup")
				cguBuilder := cgu.NewCguBuilder(HubAPIClient, tsparams.CguName, tsparams.TestNamespace, 1).
					WithCluster(RANConfig.Spoke1Name).
					WithManagedPolicy(tsparams.PolicyName)
				cguBuilder.Definition.Spec.Backup = true

				_, err = helper.SetupCguWithNamespace(cguBuilder, "")
				Expect(err).ToNot(HaveOccurred(), "Failed to setup cgu")

				By("waiting for cgu to fail for spoke1")
				assertBackupStatus(RANConfig.Spoke1Name, "UnrecoverableError")
			})
		})

		Context("with CGU disabled", func() {
			BeforeEach(func() {
				By("checking that the talm version is at least 4.12")
				versionInRange, err := version.IsVersionStringInRange(RANConfig.HubOperatorVersions[ranparam.TALM], "4.12", "")
				Expect(err).ToNot(HaveOccurred(), "Failed to compare talm version string")

				if !versionInRange {
					Skip("CGU disabled requires talm 4.12 or higher")
				}
			})

			// 54294 - Cluster Backup and Precaching in a Disabled CGU
			It("verifies backup begins and succeeds after CGU is enabled", reportxml.ID("54294"), func() {
				By("creating a disabled cgu with backup enabled")
				cguBuilder := cgu.NewCguBuilder(HubAPIClient, tsparams.CguName, tsparams.TestNamespace, 1).
					WithCluster(RANConfig.Spoke1Name).
					WithManagedPolicy(tsparams.PolicyName)
				cguBuilder.Definition.Spec.Backup = true
				cguBuilder.Definition.Spec.Enable = ptr.To(false)
				cguBuilder.Definition.Spec.RemediationStrategy.Timeout = 30

				cguBuilder, err = helper.SetupCguWithNamespace(cguBuilder, "")
				Expect(err).ToNot(HaveOccurred(), "Failed to setup cgu")

				By("checking backup does not begin when CGU is disabled")
				// don't want to overwrite cguBuilder since it'll be nil after the error
				_, err = cguBuilder.WaitUntilBackupStarts(2 * time.Minute)
				Expect(err).To(HaveOccurred(), "Backup started when CGU is disabled")

				By("enabling CGU")
				cguBuilder.Definition.Spec.Enable = ptr.To(true)
				cguBuilder, err = cguBuilder.Update(true)
				Expect(err).ToNot(HaveOccurred(), "Failed to enable CGU")

				By("waiting for backup to begin")
				_, err = cguBuilder.WaitUntilBackupStarts(1 * time.Minute)
				Expect(err).ToNot(HaveOccurred(), "Failed to start backup")

				By("waiting for cgu to indicate backup succeeded for spoke")
				assertBackupStatus(RANConfig.Spoke1Name, "Succeeded")
			})

		})
	})

	When("there are two spokes", func() {
		BeforeEach(func() {
			By("checking that hub and two spokes are present")
			Expect(rancluster.AreClustersPresent([]*clients.Settings{HubAPIClient, Spoke1APIClient, Spoke2APIClient})).
				To(BeTrue(), "Failed due to missing API client")

			By("setting up filesystem to simulate low space")
			loopbackDevicePath, err = mount.PrepareEnvWithSmallMountPoint(Spoke1APIClient)
			Expect(err).ToNot(HaveOccurred(), "Failed to prepare mount point")
		})

		AfterEach(func() {
			By("cleaning up resources on hub")
			errorList := setup.CleanupTestResourcesOnHub(HubAPIClient, tsparams.TestNamespace, "")
			Expect(errorList).To(BeEmpty(), "Failed to clean up test resources on hub")

			By("starting disk-full env clean up")
			err = mount.DiskFullEnvCleanup(Spoke1APIClient, loopbackDevicePath)
			Expect(err).ToNot(HaveOccurred(), "Failed to clean up mount point")

			By("cleaning up resources on spokes")
			errorList = setup.CleanupTestResourcesOnSpokes(
				[]*clients.Settings{Spoke1APIClient, Spoke2APIClient}, "")
			Expect(errorList).To(BeEmpty(), "Failed to clean up test resources on spokes")
		})

		// 74752 Unblock Backup in Batch OCP Upgrade
		It("should not affect backup on second spoke in same batch", reportxml.ID("74752"), func() {
			By("applying all the required CRs for backup")
			// max concurrency of 2 so both spokes are in the same batch
			cguBuilder := cgu.NewCguBuilder(HubAPIClient, tsparams.CguName, tsparams.TestNamespace, 2).
				WithCluster(RANConfig.Spoke1Name).
				WithCluster(RANConfig.Spoke2Name).
				WithManagedPolicy(tsparams.PolicyName)
			cguBuilder.Definition.Spec.Backup = true

			_, err = helper.SetupCguWithNamespace(cguBuilder, "")
			Expect(err).ToNot(HaveOccurred(), "Failed to setup cgu")

			By("waiting for cgu to indicate it failed for spoke1")
			assertBackupStatus(RANConfig.Spoke1Name, "UnrecoverableError")

			By("waiting for cgu to indicate it succeeded for spoke2")
			assertBackupStatus(RANConfig.Spoke2Name, "Succeeded")
		})

	})
})

// assertBackupStatus asserts that the cgu backup status becomes expected within 10 minutes.
func assertBackupStatus(spokeName, expected string) {
	Eventually(func() string {
		cguBuilder, err := cgu.Pull(HubAPIClient, tsparams.CguName, tsparams.TestNamespace)
		Expect(err).ToNot(HaveOccurred(),
			"Failed to pull cgu %s in namespace %s", tsparams.CguName, tsparams.TestNamespace)

		if cguBuilder.Object.Status.Backup == nil {
			klog.V(tsparams.LogLevel).Info("backup struct not ready yet")

			return ""
		}

		_, ok := cguBuilder.Object.Status.Backup.Status[spokeName]
		if !ok {
			klog.V(tsparams.LogLevel).Info("cluster name as key did not appear yet")

			return ""
		}

		klog.V(tsparams.LogLevel).Infof("[%s] %s backup status: %s\n", cguBuilder.Object.Name, spokeName,
			cguBuilder.Object.Status.Backup.Status[spokeName])

		return cguBuilder.Object.Status.Backup.Status[spokeName]
	}, 10*time.Minute, 10*time.Second).Should(Equal(expected))
}
