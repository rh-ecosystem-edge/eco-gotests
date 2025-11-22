package operator_test

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/cavaliergopher/cpio"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/hive"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/meets"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/setup"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/ztpinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/operator/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/url"
)

const (
	rootfsSpokeName   = "rootfs-test"
	rootfsDownloadDir = "/tmp/assistedrootfs"
)

var (
	rootfsSpokeResources     *setup.SpokeClusterResources
	rootfsISOId, rootfsImgID string
)

var _ = Describe(
	"RootFS build artifact can be constructed from discovery ISO",
	Ordered,
	ContinueOnFailure,
	Label(tsparams.LabelBuildArtifcatRootFSTestCases), func() {
		When("on MCE 2.1 and above", func() {
			BeforeAll(func() {
				By("Check that clusterimageset exists")
				_, err = hive.PullClusterImageSet(HubAPIClient, ZTPConfig.HubOCPXYVersion)
				if err != nil {
					Skip(ZTPConfig.HubOCPXYVersion + " clusterimageset not present")
				}

				By("Check that rootfs was not passed via osImages")
				for _, osImage := range ZTPConfig.HubAgentServiceConfig.Object.Spec.OSImages {
					if osImage.OpenshiftVersion == ZTPConfig.HubOCPXYVersion && osImage.RootFSUrl != "" {
						Skip("RootFSUrl was provided through osImages")
					}
				}

				tsparams.ReporterNamespacesToDump[rootfsSpokeName] = "rootfs-test namespace"

				By("Creating " + rootfsSpokeName + " spoke cluster resources")
				rootfsSpokeResources, err = setup.NewSpokeCluster(HubAPIClient).WithName(rootfsSpokeName).WithDefaultNamespace().
					WithDefaultPullSecret().WithDefaultClusterDeployment().
					WithDefaultIPv4AgentClusterInstall().WithDefaultInfraEnv().Create()
				Expect(err).ToNot(HaveOccurred(), "error creating %s spoke resources", rootfsSpokeName)

				Eventually(func() (string, error) {
					rootfsSpokeResources.InfraEnv.Object, err = rootfsSpokeResources.InfraEnv.Get()
					if err != nil {
						return "", err
					}

					return rootfsSpokeResources.InfraEnv.Object.Status.ISODownloadURL, nil
				}).WithTimeout(time.Minute*3).ProbeEvery(time.Second*3).
					Should(Not(BeEmpty()), "error waiting for download url to be created")

				if _, err = os.Stat(rootfsDownloadDir); err != nil {
					err = os.RemoveAll(rootfsDownloadDir)
					Expect(err).ToNot(HaveOccurred(), "error removing existing downloads directory")
				}

				err = os.Mkdir(rootfsDownloadDir, 0755)
				Expect(err).ToNot(HaveOccurred(), "error creating downloads directory")

				err = url.DownloadToDir(rootfsSpokeResources.InfraEnv.Object.Status.ISODownloadURL, rootfsDownloadDir, true)
				Expect(err).ToNot(HaveOccurred(), "error downloading ISO")

				err = url.DownloadToDir(rootfsSpokeResources.InfraEnv.Object.Status.BootArtifacts.RootfsURL,
					rootfsDownloadDir, true)
				Expect(err).ToNot(HaveOccurred(), "error downloading rootfs")

				dirEntry, err := os.ReadDir(rootfsDownloadDir)
				Expect(err).ToNot(HaveOccurred(), "error reading in downloads dir")

				var isoFile string
				var rootfsFile string

				for _, entry := range dirEntry {
					if strings.Contains(entry.Name(), ".iso") {
						isoFile = entry.Name()
					}

					if strings.Contains(entry.Name(), ".img") {
						rootfsFile = entry.Name()
					}
				}

				Expect(isoFile).NotTo(BeEmpty(), "error finding discovery iso")
				Expect(rootfsFile).NotTo(BeEmpty(), "error finding rootfs img")

				rootfsISOId, err = getISOVolumeID(fmt.Sprintf("%s/%s", rootfsDownloadDir, isoFile))
				Expect(err).NotTo(HaveOccurred(), "error reading volume id from iso")

				rootfsImgID, err = getCoreOSRootfsValue(fmt.Sprintf("%s/%s", rootfsDownloadDir, rootfsFile))
				Expect(err).NotTo(HaveOccurred(), "error reading coreos-live-rootfs from rootfs img")
			})

			DescribeTable("Rootfs is correctly extracted", func(requirement func() (bool, string)) {
				if reqMet, msg := requirement(); !reqMet {
					Skip(msg)
				}

				Expect(rootfsISOId).To(Equal(rootfsImgID), "error: iso and rootfs ids do not match")
			},
				Entry("in connected environments", meets.HubConnectedRequirement, reportxml.ID("53721")),
				Entry("in disconnected environments", meets.HubDisconnectedRequirement, reportxml.ID("53722")),
			)

			AfterAll(func() {
				err = rootfsSpokeResources.Delete()
				Expect(err).ToNot(HaveOccurred(), "error deleting %s spoke resources", rootfsSpokeName)

				err = os.RemoveAll(rootfsDownloadDir)
				Expect(err).ToNot(HaveOccurred(), "error removing download dir")
			})
		})
	})

func getISOVolumeID(path string) (string, error) {
	iso, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer iso.Close()

	// The first 32768 bytes are unused by the ISO 9660 standard, typically for bootable media
	// This is where the data area begins and the 32 byte string representing the volume identifier
	// is offset 40 bytes into the primary volume descriptor
	// Ref: https://github.com/openshift/assisted-image-service/blob/main/pkg/isoeditor/isoutil.go#L208
	volID := make([]byte, 32)

	_, err = iso.ReadAt(volID, 32808)
	if err != nil {
		return "", err
	}

	strVolID := string(volID)
	strVolID = strings.TrimLeft(strVolID, "rhcos-")
	strVolID = strings.ReplaceAll(strVolID, "\x00", "")

	return strVolID, nil
}

func getCoreOSRootfsValue(path string) (string, error) {
	img, err := os.Open(path)
	if err != nil {
		return "", err
	}

	defer img.Close()

	reader := cpio.NewReader(img)

	for hdr, err := reader.Next(); !errors.Is(err, io.EOF); hdr, err = reader.Next() {
		if err != nil {
			return "", err
		}

		if hdr.Name == "etc/coreos-live-rootfs" {
			content := make([]byte, hdr.Size)

			_, err = reader.Read(content)
			if err != nil {
				return "", err
			}

			return strings.TrimSpace(string(content)), nil
		}
	}

	return "", fmt.Errorf("error finding etc/coreos-live-rootfs from %s", path)
}
