package tests

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	moduleV1Beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/kmm/v1beta1"
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

	Context("Modprobe", Label("mcm-webhook"), func() {
		It("should fail creating MCM with both moduleName and rawargs", reportxml.ID("62607"), func() {
			By("Create KernelMapping")

			image := fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION",
				kmmparams.LocalImageRegistry, kmmparams.KmmHubOperatorNamespace, "my-kmod")
			kernelMapping, err := kmm.NewRegExKernelMappingBuilder("^.+$").
				WithContainerImage(image).
				BuildKernelMappingConfig()
			Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")

			By("Create ModuleLoaderContainer")

			moduleLoader, err := kmm.NewModLoaderContainerBuilder("kmod-a").
				WithModprobeSpec("", "", nil, nil, []string{"defined"}, nil).
				WithKernelMapping(kernelMapping).
				BuildModuleLoaderContainerCfg()
			Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

			By("Build Module")

			moduleSpec, err := kmm.NewModuleBuilder(APIClient, "mcm-modulename-rawargs", "default").
				WithNodeSelector(GeneralConfig.WorkerLabelMap).
				WithModuleLoaderContainer(moduleLoader).
				BuildModuleSpec()
			Expect(err).ToNot(HaveOccurred(), "error building module spec")

			By("Create ManagedClusterModule")

			_, err = kmm.NewManagedClusterModuleBuilder(APIClient, "mcm-modulename-rawargs",
				kmmparams.KmmHubOperatorNamespace).
				WithModuleSpec(moduleSpec).
				WithSpokeNamespace(kmmparams.KmmOperatorNamespace).
				WithSelector(kmmparams.KmmHubSelector).Create()
			Expect(err).To(HaveOccurred(), "MCM should have been rejected by webhook")
			Expect(err.Error()).To(ContainSubstring("rawArgs cannot be set when moduleName is set"))
		})

		It("should require rawargs when moduleName is not set in MCM", reportxml.ID("62606"), func() {
			By("Preparing module spec without moduleName and without rawArgs")

			moduleSpec := moduleV1Beta1.ModuleSpec{}
			moduleSpec.Selector = GeneralConfig.WorkerLabelMap
			kerMap := moduleV1Beta1.KernelMapping{Regexp: "^.+$", ContainerImage: "something:latest"}
			moduleSpec.ModuleLoader = &moduleV1Beta1.ModuleLoaderSpec{}
			moduleSpec.ModuleLoader.Container.KernelMappings = []moduleV1Beta1.KernelMapping{kerMap}

			By("Create ManagedClusterModule")

			_, err := kmm.NewManagedClusterModuleBuilder(APIClient, "mcm-no-rawargs",
				kmmparams.KmmHubOperatorNamespace).
				WithModuleSpec(moduleSpec).
				WithSpokeNamespace(kmmparams.KmmOperatorNamespace).
				WithSelector(kmmparams.KmmHubSelector).Create()
			Expect(err).To(HaveOccurred(), "MCM should have been rejected by webhook")
			Expect(err.Error()).To(ContainSubstring("load and unload rawArgs must be set when moduleName is unset"))
		})

		It("should fail to update an existing MCM with something that is wrong", reportxml.ID("62605"), func() {
			By("Create valid KernelMapping")

			image := fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION",
				kmmparams.LocalImageRegistry, kmmparams.KmmHubOperatorNamespace, "my-kmod")
			kernelMapping, err := kmm.NewRegExKernelMappingBuilder("^.+$").
				WithContainerImage(image).
				BuildKernelMappingConfig()
			Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")

			By("Create valid ModuleLoaderContainer")

			moduleLoader, err := kmm.NewModLoaderContainerBuilder("kmod-valid").
				WithKernelMapping(kernelMapping).
				BuildModuleLoaderContainerCfg()
			Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

			By("Build valid Module spec")

			moduleSpec, err := kmm.NewModuleBuilder(APIClient, "mcm-update-invalid", "default").
				WithNodeSelector(GeneralConfig.WorkerLabelMap).
				WithModuleLoaderContainer(moduleLoader).
				BuildModuleSpec()
			Expect(err).ToNot(HaveOccurred(), "error building module spec")

			By("Create valid ManagedClusterModule")

			mcm, err := kmm.NewManagedClusterModuleBuilder(APIClient, "mcm-update-invalid",
				kmmparams.KmmHubOperatorNamespace).
				WithModuleSpec(moduleSpec).
				WithSpokeNamespace(kmmparams.KmmOperatorNamespace).
				WithSelector(kmmparams.KmmHubSelector).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating valid MCM")

			DeferCleanup(func() { _, _ = mcm.Delete() })

			By("Create invalid KernelMapping (missing containerImage)")

			invalidKernelMapping, err := kmm.NewRegExKernelMappingBuilder("^.+$").BuildKernelMappingConfig()
			Expect(err).ToNot(HaveOccurred(), "error creating invalid kernel mapping")

			By("Create invalid ModuleLoaderContainer")

			invalidModuleLoader, err := kmm.NewModLoaderContainerBuilder("webhook").
				WithKernelMapping(invalidKernelMapping).
				BuildModuleLoaderContainerCfg()
			Expect(err).ToNot(HaveOccurred(), "error creating invalid moduleloadercontainer")

			By("Update MCM with invalid spec")

			mcm.Definition.Spec.ModuleSpec.ModuleLoader.Container = *invalidModuleLoader
			_, err = mcm.Update()
			Expect(err).To(HaveOccurred(), "MCM update should have been rejected by webhook")
			Expect(err.Error()).To(ContainSubstring("missing spec.moduleLoader.container.kernelMappings"))
			Expect(err.Error()).To(ContainSubstring(".containerImage"))
		})
	})
})
