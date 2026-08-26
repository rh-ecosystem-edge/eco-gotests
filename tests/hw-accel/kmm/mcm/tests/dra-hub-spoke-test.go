package tests

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/define"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/mcm/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
)

var _ = Describe("KMM-HUB", Ordered, Label(tsparams.LabelSuite), func() {
	Context("DRA Hub-Spoke", Label("mcm-dra", "dra"), func() {
		BeforeEach(func() {
			if kmmparams.DRADriverImage == "" {
				Skip("ECO_HWACCEL_KMM_DRA_DRIVER_IMAGE_REPO is not set")
			}
		})

		It("should accept MCM with valid DRA config", reportxml.ID("89711"),
			Label("89711", "test_id:89711"), func() {
				mcmName := "test-hub-dra"
				image := fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION",
					kmmparams.LocalImageRegistry, kmmparams.KmmHubOperatorNamespace, "dra-kmod")

				By("Build Module spec with DRA")

				moduleSpec, err := define.DRAModuleSpec(APIClient, mcmName, "default", image,
					"dramod", "dra-manager", GeneralConfig.WorkerLabelMap,
					[]string{kmmparams.DRADeviceClassName})
				Expect(err).ToNot(HaveOccurred(), "error building DRA module spec")

				By("Create ManagedClusterModule with DRA")

				mcm, err := kmm.NewManagedClusterModuleBuilder(APIClient, mcmName,
					kmmparams.KmmHubOperatorNamespace).
					WithModuleSpec(moduleSpec).
					WithSpokeNamespace(kmmparams.KmmOperatorNamespace).
					WithSelector(map[string]string{"nonexistent": "spoke"}).Create()
				Expect(err).ToNot(HaveOccurred(), "MCM with valid DRA should be accepted")

				DeferCleanup(func() { _, _ = mcm.Delete() })

				By("Verify MCM has DRA config")

				Expect(mcm.Definition.Spec.ModuleSpec.DRA).ToNot(BeNil(), "DRA spec should be set")
				Expect(mcm.Definition.Spec.ModuleSpec.DRA.DriverName).
					To(Equal(kmmparams.DRADriverName), "DRA driverName should match")
				Expect(mcm.Definition.Spec.ModuleSpec.DRA.DeviceClasses).
					To(HaveLen(1), "should have one DeviceClass")
			})

		It("should reject MCM with both dra and devicePlugin", reportxml.ID("89711"),
			Label("89711", "test_id:89711"), func() {
				mcmName := "test-hub-dra-invalid"
				image := fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION",
					kmmparams.LocalImageRegistry, kmmparams.KmmHubOperatorNamespace, "dra-kmod")

				By("Build Module spec with both DRA and DevicePlugin")

				moduleSpec, err := define.DRAAndDevicePluginModuleSpec(APIClient, mcmName, "default",
					image, "dramod", "dra-manager", "some-device-plugin:latest",
					GeneralConfig.WorkerLabelMap)
				Expect(err).ToNot(HaveOccurred(), "error building module spec")

				By("Create ManagedClusterModule -- should be rejected")

				_, err = kmm.NewManagedClusterModuleBuilder(APIClient, mcmName,
					kmmparams.KmmHubOperatorNamespace).
					WithModuleSpec(moduleSpec).
					WithSpokeNamespace(kmmparams.KmmOperatorNamespace).
					WithSelector(map[string]string{"nonexistent": "spoke"}).Create()
				Expect(err).To(HaveOccurred(), "MCM with both DRA and DevicePlugin should be rejected")
				Expect(err.Error()).To(ContainSubstring("mutually exclusive"),
					"error should mention mutual exclusivity")
			})
	})
})
