package tests

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/mcm/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
)

var _ = Describe("KMM-HUB", Ordered, Label(tsparams.LabelSuite), func() {

	Context("KMM-HUB", Label("mcm-webhook"), func() {

		It("should fail if no container image is specified in the module", reportxml.ID("62608"), func() {

			By("Create KernelMapping")
			kernelMapping, err := kmm.NewRegExKernelMappingBuilder("^.+$").BuildKernelMappingConfig()
			Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")

			By("Create ModuleLoaderContainer")
			moduleLoaderContainerCfg, err := kmm.NewModLoaderContainerBuilder("webhook").
				WithKernelMapping(kernelMapping).
				BuildModuleLoaderContainerCfg()
			Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

			By("Build Module")
			moduleSpec, err := kmm.NewModuleBuilder(APIClient, "webhook-no-container-image", "default").
				WithNodeSelector(GeneralConfig.WorkerLabelMap).
				WithModuleLoaderContainer(moduleLoaderContainerCfg).
				BuildModuleSpec()
			Expect(err).ToNot(HaveOccurred(), "error building module spec")

			By("Create ManagedClusterModule")
			_, err = kmm.NewManagedClusterModuleBuilder(APIClient, "webhook-no-container-image",
				kmmparams.KmmHubOperatorNamespace).
				WithModuleSpec(moduleSpec).
				WithSpokeNamespace(kmmparams.KmmOperatorNamespace).
				WithSelector(kmmparams.KmmHubSelector).Create()
			Expect(err).To(HaveOccurred(), "error creating module")
			Expect(err.Error()).To(ContainSubstring("missing spec.moduleLoader.container.kernelMappings"))
			Expect(err.Error()).To(ContainSubstring(".containerImage"))
		})

		It("should fail if no regexp nor literal are set in a kernel mapping", reportxml.ID("62596"), func() {

			By("Create KernelMapping")
			kernelMapping, err := kmm.NewRegExKernelMappingBuilder("willBeRemoved").BuildKernelMappingConfig()
			Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")
			kernelMapping.Regexp = ""

			By("Create ModuleLoaderContainer")
			moduleLoaderContainerCfg, err := kmm.NewModLoaderContainerBuilder("webhook").
				WithKernelMapping(kernelMapping).
				BuildModuleLoaderContainerCfg()
			Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

			By("Build Module")
			moduleSpec, err := kmm.NewModuleBuilder(APIClient, "webhook-regexp-and-literal",
				kmmparams.KmmHubOperatorNamespace).
				WithNodeSelector(GeneralConfig.WorkerLabelMap).
				WithModuleLoaderContainer(moduleLoaderContainerCfg).
				BuildModuleSpec()
			Expect(err).ToNot(HaveOccurred(), "error building module spec")

			By("Create ManagedClusterModule")
			_, err = kmm.NewManagedClusterModuleBuilder(APIClient, "webhook-no-container-image",
				kmmparams.KmmHubOperatorNamespace).
				WithModuleSpec(moduleSpec).
				WithSpokeNamespace(kmmparams.KmmOperatorNamespace).
				WithSelector(kmmparams.KmmHubSelector).Create()
			Expect(err).To(HaveOccurred(), "error creating module")
			Expect(err.Error()).To(ContainSubstring("regexp or literal must be set"))
		})

		It("should fail if both regexp and literal are set in a kernel mapping", reportxml.ID("62597"), func() {

			By("Create KernelMapping")
			kernelMapping, err := kmm.NewRegExKernelMappingBuilder("^.+$").BuildKernelMappingConfig()
			Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")
			kernelMapping.Literal = "5.14.0-284.28.1.el9_2.x86_64"

			By("Create ModuleLoaderContainer")
			moduleLoaderContainerCfg, err := kmm.NewModLoaderContainerBuilder("webhook").
				WithKernelMapping(kernelMapping).
				BuildModuleLoaderContainerCfg()
			Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

			By("Build Module")
			moduleSpec, err := kmm.NewModuleBuilder(APIClient, "webhook-regexp-and-literal",
				kmmparams.KmmHubOperatorNamespace).
				WithNodeSelector(GeneralConfig.WorkerLabelMap).
				WithModuleLoaderContainer(moduleLoaderContainerCfg).
				BuildModuleSpec()
			Expect(err).ToNot(HaveOccurred(), "error building module spec")

			By("Create ManagedClusterModule")
			_, err = kmm.NewManagedClusterModuleBuilder(APIClient, "webhook-no-container-image",
				kmmparams.KmmHubOperatorNamespace).
				WithModuleSpec(moduleSpec).
				WithSpokeNamespace(kmmparams.KmmOperatorNamespace).
				WithSelector(kmmparams.KmmHubSelector).Create()
			Expect(err).To(HaveOccurred(), "error creating module")
			Expect(err.Error()).To(ContainSubstring("regexp and literal are mutually exclusive properties"))
		})

		It("should fail if the regexp isn't valid in the module", reportxml.ID("62609"), func() {

			By("Create KernelMapping")
			kernelMapping, err := kmm.NewRegExKernelMappingBuilder("*-invalid-regexp").BuildKernelMappingConfig()
			Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")

			By("Create ModuleLoaderContainer")
			moduleLoaderContainerCfg, err := kmm.NewModLoaderContainerBuilder("webhook").
				WithKernelMapping(kernelMapping).
				BuildModuleLoaderContainerCfg()
			Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

			By("Building Module")
			moduleSpec, err := kmm.NewModuleBuilder(APIClient, "webhook-invalid-regexp",
				kmmparams.KmmHubOperatorNamespace).
				WithNodeSelector(GeneralConfig.WorkerLabelMap).
				WithModuleLoaderContainer(moduleLoaderContainerCfg).
				BuildModuleSpec()
			Expect(err).ToNot(HaveOccurred(), "error building module spec")

			By("Create ManagedClusterModule")
			_, err = kmm.NewManagedClusterModuleBuilder(APIClient, "webhook-no-container-image",
				kmmparams.KmmHubOperatorNamespace).
				WithModuleSpec(moduleSpec).
				WithSpokeNamespace(kmmparams.KmmOperatorNamespace).
				WithSelector(kmmparams.KmmHubSelector).Create()
			Expect(err).To(HaveOccurred(), "error creating module")
			Expect(err.Error()).To(ContainSubstring("invalid regexp"))
		})
	})

	Context("KMM-HUB", Label("mcm-crd"), func() {

		It("should fail if no spokeNamespace is set in MCM", reportxml.ID("71692"), func() {

			By("Create KernelMapping")
			kernelMapping, err := kmm.NewRegExKernelMappingBuilder("^.+$").BuildKernelMappingConfig()
			Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")

			By("Create ModuleLoaderContainer")
			moduleLoaderContainerCfg, err := kmm.NewModLoaderContainerBuilder("crd").
				WithKernelMapping(kernelMapping).
				BuildModuleLoaderContainerCfg()
			Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

			By("Build Module")
			moduleSpec, err := kmm.NewModuleBuilder(APIClient, "no-spoke-namespace", "default").
				WithNodeSelector(GeneralConfig.WorkerLabelMap).
				WithModuleLoaderContainer(moduleLoaderContainerCfg).
				BuildModuleSpec()
			Expect(err).ToNot(HaveOccurred(), "error building module spec")

			By("Create ManagedClusterModule")
			mcm, err := kmm.NewManagedClusterModuleBuilder(APIClient, "no-spoke-namespace",
				kmmparams.KmmHubOperatorNamespace).
				WithModuleSpec(moduleSpec).
				WithSelector(kmmparams.KmmHubSelector).Create()
			Expect(mcm.Definition.Spec.SpokeNamespace).To(Equal(""))
			Expect(err.Error()).To(ContainSubstring("admission webhook \"vmanagedclustermodule.kb.io\" denied the request"))
		})
	})
})
