package negative_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	lcav1 "github.com/openshift-kni/lifecycle-agent/api/imagebasedupgrade/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/lca"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oadp"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/velero"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/mgmt/internal/mgmtinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/mgmt/internal/mgmtparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/mgmt/negative/internal/tsparams"
)

var _ = Describe(
	"Starting imagebasedupgrade with missing dataprotectionlocation",
	Ordered,
	Label(tsparams.LabelMissingBackupLocation), func() {
		var (
			ibu *lca.ImageBasedUpgradeBuilder
			err error

			originalDPA   *oadp.DPABuilder
			oadpConfigmap *configmap.Builder
		)

		BeforeAll(func() {
			By("Pull the imagebasedupgrade from the cluster")
			ibu, err = lca.PullImageBasedUpgrade(APIClient)
			Expect(err).NotTo(HaveOccurred(), "error pulling ibu resource from cluster")

			By("Ensure that imagebasedupgrade values are empty")
			ibu.Definition.Spec.ExtraManifests = []lcav1.ConfigMapRef{}
			ibu.Definition.Spec.OADPContent = []lcav1.ConfigMapRef{}
			_, err = ibu.Update()
			Expect(err).NotTo(HaveOccurred(), "error updating ibu resource with empty values")

			By("Get configured dataprotection application")
			dpaBuilders, err := oadp.ListDataProtectionApplication(APIClient, mgmtparams.LCAOADPNamespace)
			Expect(err).NotTo(HaveOccurred(), "error listing dataprotectionapplications")
			Expect(len(dpaBuilders)).To(Equal(1), "error: receieved multiple dataprotectionapplication resources")

			originalDPA = dpaBuilders[0]

			err = originalDPA.Delete()
			Expect(err).NotTo(HaveOccurred(), "error deleting original dataprotectionapplication")

			By("Get klusterlet backup string")
			klusterletBackup, err := mgmtparams.KlusterletBackup.String()
			Expect(err).NotTo(HaveOccurred(), "error creating configmap data for klusterlet backup")

			By("Get klusterlet restore string")
			klusterletRestore, err := mgmtparams.KlusterletRestore.String()
			Expect(err).NotTo(HaveOccurred(), "error creating configmap data for klusterlet restore")

			oadpConfigmap, err = configmap.NewBuilder(
				APIClient, "oadp-configmap", mgmtparams.LCAOADPNamespace).WithData(map[string]string{
				"klusterlet_backup.yaml":  klusterletBackup,
				"klusterlet_restore.yaml": klusterletRestore,
			}).Create()
			Expect(err).NotTo(HaveOccurred(), "error creating oadp configmap")
		})

		AfterAll(func() {

			if originalDPA != nil && !originalDPA.Exists() {
				By("Restoring data protection application")
				originalDPA.Definition.ResourceVersion = ""
				_, err := originalDPA.Create()
				Expect(err).NotTo(HaveOccurred(), "error restoring original dataprotection application")
			}

			var backupStorageLocations []*velero.BackupStorageLocationBuilder

			Eventually(func() (bool, error) {
				backupStorageLocations, err = velero.ListBackupStorageLocationBuilder(APIClient, mgmtparams.LCAOADPNamespace)
				if err != nil {
					return false, err
				}

				if len(backupStorageLocations) > 0 {
					return backupStorageLocations[0].Object.Status.Phase == "Available", nil
				}

				return false, nil
			}).WithTimeout(time.Second*60).WithPolling(time.Second*2).Should(
				BeTrue(), "error waiting for backupstoragelocation to be created")

		})

		It("fails oadp operator availability check", reportxml.ID("71478"), func() {
			ibu, err = ibu.WithSeedImage(MGMTConfig.SeedImage).
				WithSeedImageVersion(MGMTConfig.SeedClusterInfo.SeedClusterOCPVersion).WithOadpContent(
				oadpConfigmap.Definition.Name,
				oadpConfigmap.Definition.Namespace).Update()
			Expect(err).NotTo(HaveOccurred(), "error updating ibu with image and version")

			By("Setting the IBU stage to Prep")
			_, err = ibu.WithStage("Prep").Update()
			Expect(err).NotTo(HaveOccurred(), "error setting ibu to prep stage")

			ibu.Object, err = ibu.Get()
			Expect(err).To(BeNil(), "error: getting updated ibu")

			Eventually(func() (string, error) {
				ibu.Object, err = ibu.Get()
				if err != nil {
					return "", err
				}

				for _, condition := range ibu.Object.Status.Conditions {
					if condition.Type == "PrepInProgress" {
						return condition.Message, nil
					}
				}

				return "", nil
			}).WithTimeout(time.Second * 30).WithPolling(time.Second * 2).Should(
				Equal(fmt.Sprintf("failed to validate IBU spec: failed to check oadp operator availability: "+
					"No DataProtectionApplication CR found in the %s",
					mgmtparams.LCAOADPNamespace)))
		})
	})
