package negative_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/lca"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/mgmt/internal/mgmtinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/mgmt/negative/internal/tsparams"
)

var _ = Describe(
	"Patching ibu with a not supported next stage",
	Ordered,
	Label(tsparams.LabelStageTransition), func() {
		var (
			ibu *lca.ImageBasedUpgradeBuilder
			err error
		)

		BeforeAll(func() {
			By("Pull the imagebasedupgrade from the cluster")
			ibu, err = lca.PullImageBasedUpgrade(APIClient)
			Expect(err).NotTo(HaveOccurred(), "error pulling ibu resource from cluster")

			By("Ensure that imagebasedupgrade stage is set to Idle")
			Expect(string(ibu.Object.Spec.Stage)).To(Equal("Idle"), "error: ibu resource contains unexpected state")

			ibu, err = ibu.WithSeedImage(MGMTConfig.SeedImage).
				WithSeedImageVersion(MGMTConfig.SeedClusterInfo.SeedClusterOCPVersion).Update()
			Expect(err).NotTo(HaveOccurred(), "error updating ibu with image and version")
		})

		It("fails because from Idle it's not possible to move to Rollback stage", reportxml.ID("71738"), func() {

			By("Setting the IBU stage to Rollback")

			_, err := ibu.WithStage("Rollback").Update()
			Expect(err.Error()).To(ContainSubstring("the stage transition is not permitted"),
				"error: ibu seedimage updated with wrong next stage")

		})

		It("fails because from Idle it's not possible to move to Upgrade stage", reportxml.ID("71739"), func() {

			By("Setting the IBU stage to Upgrade")

			_, err := ibu.WithStage("Upgrade").Update()
			Expect(err.Error()).To(ContainSubstring("the stage transition is not permitted"),
				"error: ibu seedimage updated with wrong next stage")

		})

		It("fails because from Prep it's not possible to move to Rollback stage", reportxml.ID("71740"), func() {

			By("Setting the IBU stage to Prep")

			_, err := ibu.WithStage("Prep").Update()
			Expect(err).NotTo(HaveOccurred(), "error setting ibu to Prep stage")

			By("Pull the imagebasedupgrade from the cluster")

			ibu, err = lca.PullImageBasedUpgrade(APIClient)
			Expect(err).NotTo(HaveOccurred(), "error pulling imagebasedupgrade resource")

			By("Setting the IBU stage to Rollback")

			_, err = ibu.WithStage("Rollback").Update()
			Expect(err.Error()).To(ContainSubstring("the stage transition is not permitted"),
				"error: ibu seedimage updated with wrong next stage")

		})
	})
