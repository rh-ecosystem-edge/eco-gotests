package negative_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	lcav1 "github.com/openshift-kni/lifecycle-agent/api/imagebasedupgrade/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/lca"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/mgmt/internal/mgmtinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/mgmt/negative/internal/tsparams"
)

var _ = Describe(
	"Patching ibu seedimage ref while prepping images",
	Ordered,
	Label(tsparams.LabelImmutableSeedImage), func() {
		var (
			ibu *lca.ImageBasedUpgradeBuilder
			err error

			negativeSeedImage        = "quay.io/ibutest/negative:latest"
			negativeSeedImageVersion = "4.16.0-negative"
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
		})

		It("fails because seedImageRef is immutable while progressing", reportxml.ID("71383"), func() {
			ibu, err = ibu.WithSeedImage(MGMTConfig.SeedImage).
				WithSeedImageVersion(MGMTConfig.SeedClusterInfo.SeedClusterOCPVersion).Update()
			Expect(err).NotTo(HaveOccurred(), "error updating ibu with image and version")

			By("Setting the IBU stage to Prep")

			_, err := ibu.WithStage("Prep").Update()
			Expect(err).NotTo(HaveOccurred(), "error setting ibu to prep stage")

			ibu.Definition, err = ibu.Get()
			Expect(err).To(BeNil(), "error: getting updated ibu")

			By("Updating IBU with different seedimage")
			_, err = ibu.WithSeedImage(negativeSeedImage).Update()
			Expect(err.Error()).To(ContainSubstring("can not change spec.seedImageRef while ibu is in progress"),
				"error: ibu seedimage updated while in prep phase")

			By("Updating IBU with different seedimage version")
			_, err = ibu.WithSeedImageVersion(negativeSeedImageVersion).Update()
			Expect(err.Error()).To(ContainSubstring("can not change spec.seedImageRef while ibu is in progress"),
				"error: ibu seedimage version updated while in prep phase")
		})
	})
