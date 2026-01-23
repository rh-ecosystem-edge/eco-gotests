package seedgeneration_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/lca"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/internal/lcaparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/internal/seedimage"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/lca/seedgeneration/internal/seedgenerationinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/seedgeneration/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

var _ = Describe(
	"Seed image generation",
	func() {

		var (
			imageName string
		)

		BeforeEach(func() {
			// Use ECO_LCA_IBGU_SEED_IMAGE directly as the seed image name
			Expect(SeedGenerationConfig.IbguSeedImage).NotTo(BeEmpty(), "ECO_LCA_IBGU_SEED_IMAGE is not set.")

			imageName = SeedGenerationConfig.IbguSeedImage

			klog.V(lcaparams.LCALogLevel).Infof("Source seed image: %s", imageName)

			// Verify we have the required API client
			Expect(TargetSNOAPIClient).NotTo(BeNil(), "TargetSNOAPIClient is not initialized")

			By("checking for existing SeedGenerator and deleting if present", func() {
				seedGenerator, err := lca.PullSeedGenerator(TargetSNOAPIClient, "seedimage")
				if err == nil && seedGenerator != nil {
					klog.V(lcaparams.LCALogLevel).Info("Existing SeedGenerator found, deleting it")
					_, err = seedGenerator.Delete()
					Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to delete existing SeedGenerator: %v", err))
				} else {
					klog.V(lcaparams.LCALogLevel).Info("No existing SeedGenerator found, proceeding with creation")
				}
			})

			By("creating seedgen secret in openshift-lifecycle-agent namespace", func() {
				// Get pull secret from the cluster's openshift-config namespace
				pullSecret, err := cluster.GetOCPPullSecret(TargetSNOAPIClient)
				Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to get pull-secret from cluster: %v", err))

				// Extract the .dockerconfigjson data
				dockerConfigJSON, ok := pullSecret.Object.Data[".dockerconfigjson"]
				Expect(ok).To(BeTrue(), "pull-secret does not contain .dockerconfigjson")

				// Create or update the seedgen secret
				secretBuilder := secret.NewBuilder(
					TargetSNOAPIClient,
					"seedgen",
					tsparams.LCANamespace,
					corev1.SecretTypeOpaque,
				)

				data := make(map[string][]byte)
				data["seedAuth"] = dockerConfigJSON
				secretBuilder.WithData(data)

				_, err = secretBuilder.Create()
				Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to create seedgen secret: %v", err))
			})
		})

		It("generates a seed image", reportxml.ID("87114"), func() {
			By("creating SeedGenerator CR and waiting for seed image generation", func() {
				generatedImage, err := seedimage.GenerateSeedImage(
					TargetSNOAPIClient,
					imageName,
					"",             // recertImage - use default
					30*time.Minute, // timeout
				)
				Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to generate seed image: %v", err))
				Expect(generatedImage).To(Equal(imageName), "Generated image location should match source image location")
			})
		})
	})
