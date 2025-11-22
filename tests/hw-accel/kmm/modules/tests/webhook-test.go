package tests

import (
	"context"
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/kmm/v1beta1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/modules/internal/tsparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
)

var _ = Describe("KMM", Ordered, Label(kmmparams.LabelSuite, kmmparams.LabelSanity), func() {

	Context("Module", Label("webhook"), func() {

		var nSpace = kmmparams.WebhookModuleTestNamespace

		BeforeAll(func() {

			By("Create Namespace")
			_, err := namespace.NewBuilder(APIClient, nSpace).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating test namespace")
		})

		AfterAll(func() {

			By("Delete Namespace")
			err := namespace.NewBuilder(APIClient, nSpace).Delete()
			Expect(err).ToNot(HaveOccurred(), "error deleting test namespace")
		})

		Context("KernelMapping", Label("webhook"), func() {

			It("should fail if no container image is specified in the module", reportxml.ID("62601"), func() {

				By("Create KernelMapping")
				kernelMapping, err := kmm.NewRegExKernelMappingBuilder("^.+$").BuildKernelMappingConfig()
				Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")

				By("Create ModuleLoaderContainer")
				moduleLoaderContainerCfg, err := kmm.NewModLoaderContainerBuilder("webhook").
					WithKernelMapping(kernelMapping).
					BuildModuleLoaderContainerCfg()
				Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

				By("Create Module")
				_, err = kmm.NewModuleBuilder(APIClient, "webhook-no-container-image", nSpace).
					WithNodeSelector(GeneralConfig.WorkerLabelMap).
					WithModuleLoaderContainer(moduleLoaderContainerCfg).
					Create()
				Expect(err).To(HaveOccurred(), "error creating module")
				Expect(err.Error()).To(ContainSubstring("missing spec.moduleLoader.container.kernelMappings"))
				Expect(err.Error()).To(ContainSubstring(".containerImage"))
			})

			It("should fail if the regexp isn't valid in the module", reportxml.ID("62602"), func() {

				By("Create KernelMapping")
				kernelMapping, err := kmm.NewRegExKernelMappingBuilder("*-invalid-regexp").BuildKernelMappingConfig()
				Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")

				By("Create ModuleLoaderContainer")
				moduleLoaderContainerCfg, err := kmm.NewModLoaderContainerBuilder("webhook").
					WithKernelMapping(kernelMapping).
					BuildModuleLoaderContainerCfg()
				Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

				By("Create Module")
				_, err = kmm.NewModuleBuilder(APIClient, "webhook-invalid-regexp", nSpace).
					WithNodeSelector(GeneralConfig.WorkerLabelMap).
					WithModuleLoaderContainer(moduleLoaderContainerCfg).
					Create()
				Expect(err).To(HaveOccurred(), "error creating module")
				Expect(err.Error()).To(ContainSubstring("invalid regexp"))
			})

			It("should fail if no regexp nor literal are set in a kernel mapping", reportxml.ID("62603"), func() {

				By("Create KernelMapping")
				kernelMapping, err := kmm.NewRegExKernelMappingBuilder("willBeRemoved").BuildKernelMappingConfig()
				Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")
				kernelMapping.Regexp = ""

				By("Create ModuleLoaderContainer")
				moduleLoaderContainerCfg, err := kmm.NewModLoaderContainerBuilder("webhook").
					WithKernelMapping(kernelMapping).
					BuildModuleLoaderContainerCfg()
				Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

				By("Create Module")
				_, err = kmm.NewModuleBuilder(APIClient, "webhook-regexp-and-literal", nSpace).
					WithNodeSelector(GeneralConfig.WorkerLabelMap).
					WithModuleLoaderContainer(moduleLoaderContainerCfg).
					Create()
				Expect(err).To(HaveOccurred(), "error creating module")
				Expect(err.Error()).To(ContainSubstring("regexp or literal must be set"))
			})

			It("should fail if both regexp and literal are set in a kernel mapping", reportxml.ID("62604"), func() {

				By("Create KernelMapping")
				kernelMapping, err := kmm.NewRegExKernelMappingBuilder("^.+$").BuildKernelMappingConfig()
				Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")
				kernelMapping.Literal = "5.14.0-284.28.1.el9_2.x86_64"

				By("Create ModuleLoaderContainer")
				moduleLoaderContainerCfg, err := kmm.NewModLoaderContainerBuilder("webhook").
					WithKernelMapping(kernelMapping).
					BuildModuleLoaderContainerCfg()
				Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

				By("Create Module")
				_, err = kmm.NewModuleBuilder(APIClient, "webhook-regexp-and-literal", nSpace).
					WithNodeSelector(GeneralConfig.WorkerLabelMap).
					WithModuleLoaderContainer(moduleLoaderContainerCfg).
					Create()
				Expect(err).To(HaveOccurred(), "error creating module")
				Expect(err.Error()).To(ContainSubstring("regexp and literal are mutually exclusive properties"))
			})
		})

		Context("Modprobe", Label("webhook"), func() {

			It("should fail if there are duplications in modulesLoadingOrder", reportxml.ID("62742"), func() {

				By("Create KernelMapping")
				image := fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION",
					tsparams.LocalImageRegistry, kmmparams.WebhookModuleTestNamespace, "my-kmod")
				kernelMapping, err := kmm.NewRegExKernelMappingBuilder("^.+$").
					WithContainerImage(image).
					BuildKernelMappingConfig()
				Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")

				By("Create moduleLoader container")
				moduleLoader, err := kmm.NewModLoaderContainerBuilder("kmod-a").
					WithModprobeSpec("", "", nil, nil, nil, []string{"kmod-a", "kmod-b", "kmod-a"}).
					WithKernelMapping(kernelMapping).
					BuildModuleLoaderContainerCfg()
				Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

				By("Create Module")
				_, err = kmm.NewModuleBuilder(APIClient, "webhook-module-loading-order-dups", nSpace).
					WithNodeSelector(GeneralConfig.WorkerLabelMap).
					WithModuleLoaderContainer(moduleLoader).
					Create()
				Expect(err).To(HaveOccurred(), "error creating module")
				Expect(err.Error()).To(ContainSubstring("duplicate value in the loading order list"))
			})

			It("should fail if the 'main' kmod isn't the first one in modulesLoadingOrder", reportxml.ID("64227"), func() {

				By("Create KernelMapping")
				image := fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION",
					tsparams.LocalImageRegistry, kmmparams.WebhookModuleTestNamespace, "my-kmod")
				kernelMapping, err := kmm.NewRegExKernelMappingBuilder("^.+$").
					WithContainerImage(image).
					BuildKernelMappingConfig()
				Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")

				By("Create moduleLoader container")
				moduleLoader, err := kmm.NewModLoaderContainerBuilder("kmod-a").
					WithModprobeSpec("", "", nil, nil, nil, []string{"kmod-b", "kmod-a", "kmod-c"}).
					WithKernelMapping(kernelMapping).
					BuildModuleLoaderContainerCfg()
				Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

				By("Create Module")
				_, err = kmm.NewModuleBuilder(APIClient, "main-module-not-first-in-list", nSpace).
					WithNodeSelector(GeneralConfig.WorkerLabelMap).
					WithModuleLoaderContainer(moduleLoader).
					Create()
				Expect(err).To(HaveOccurred(), "error creating module")
				Expect(err.Error()).To(ContainSubstring("if a loading order is defined, the first element must be moduleName"))
			})

			It("should fail creating module with both moduleName and rawargs", reportxml.ID("62600"), func() {

				By("Create KernelMapping")
				image := fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION",
					tsparams.LocalImageRegistry, kmmparams.WebhookModuleTestNamespace, "my-kmod")
				kernelMapping, err := kmm.NewRegExKernelMappingBuilder("^.+$").
					WithContainerImage(image).
					BuildKernelMappingConfig()
				Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")

				By("Create moduleLoader container")
				moduleLoader, err := kmm.NewModLoaderContainerBuilder("kmod-a").
					WithModprobeSpec("", "", nil, nil, []string{"defined"}, nil).
					WithKernelMapping(kernelMapping).
					BuildModuleLoaderContainerCfg()
				Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

				By("Create Module")
				_, err = kmm.NewModuleBuilder(APIClient, "module", nSpace).
					WithNodeSelector(GeneralConfig.WorkerLabelMap).
					WithModuleLoaderContainer(moduleLoader).
					Create()
				Expect(err).To(HaveOccurred(), "error creating module")
				klog.V(kmmparams.KmmLogLevel).Infof("err is: %s", err)
				Expect(err.Error()).To(ContainSubstring("rawArgs cannot be set when moduleName is set"))
			})

			It("should require rawargs when moduleName is not net", reportxml.ID("62599"), func() {

				By("Preparing module")
				module := &v1beta1.Module{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nomodule-raw",
						Namespace: nSpace,
					},
				}
				module.Spec.Selector = GeneralConfig.WorkerLabelMap
				kerMap := v1beta1.KernelMapping{Regexp: "^.+$", ContainerImage: "something:latest"}
				var KerMapList []v1beta1.KernelMapping
				mappings := append(KerMapList, kerMap)
				if module.Spec.ModuleLoader == nil {
					module.Spec.ModuleLoader = &v1beta1.ModuleLoaderSpec{}
				}
				module.Spec.ModuleLoader.Container.KernelMappings = mappings

				By("Create Module")
				err := APIClient.Create(context.TODO(), module)
				Expect(err).To(HaveOccurred(), "error creating module")
				klog.V(kmmparams.KmmLogLevel).Infof("err is: %s", err)
				Expect(err.Error()).To(ContainSubstring("load and unload rawArgs must be set when moduleName is unset"))
			})

			It("should require image tag or digest for container image", reportxml.ID("75990"), func() {
				By("Preparing module")
				module := &v1beta1.Module{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nomodule-raw",
						Namespace: nSpace,
					},
				}
				module.Spec.Selector = GeneralConfig.WorkerLabelMap
				kerMap := v1beta1.KernelMapping{Regexp: "^.+$", ContainerImage: "something"}
				var KerMapList []v1beta1.KernelMapping
				mappings := append(KerMapList, kerMap)
				if module.Spec.ModuleLoader == nil {
					module.Spec.ModuleLoader = &v1beta1.ModuleLoaderSpec{}
				}
				module.Spec.ModuleLoader.Container.KernelMappings = mappings

				By("Create Module")
				err := APIClient.Create(context.TODO(), module)
				Expect(err).To(HaveOccurred(), "error creating module")
				klog.V(kmmparams.KmmLogLevel).Infof("err is: %s", err)
				Expect(err.Error()).To(ContainSubstring("container image must explicitely set a tag or digest")) //nolint:misspell
			})
		})

		Context("Tolerations", Label("wehbook"), func() {

			It("should allow only specific effects", reportxml.ID("81516"), func() {

				By("Create KernelMapping")
				image := fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION",
					tsparams.LocalImageRegistry, kmmparams.WebhookModuleTestNamespace, "my-kmod")
				kernelMapping, err := kmm.NewRegExKernelMappingBuilder("^.+$").
					WithContainerImage(image).
					BuildKernelMappingConfig()
				Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")

				By("Create moduleLoader container")
				moduleLoader, err := kmm.NewModLoaderContainerBuilder("kmod-a").
					WithKernelMapping(kernelMapping).
					BuildModuleLoaderContainerCfg()
				Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

				By("Create Module")
				_, err = kmm.NewModuleBuilder(APIClient, "webhook-toleration-effect", nSpace).
					WithNodeSelector(GeneralConfig.WorkerLabelMap).
					WithModuleLoaderContainer(moduleLoader).
					WithToleration("dummy-key",
						"Exists",
						"",
						"BadEffect", nil).
					Create()
				Expect(err).To(HaveOccurred(), "error creating module")
				Expect(err.Error()).To(ContainSubstring("Toleration[0] invalid effect 'BadEffect'"))
			})

			It("should allow only specific operator", reportxml.ID("81515"), func() {

				By("Create KernelMapping")
				image := fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION",
					tsparams.LocalImageRegistry, kmmparams.WebhookModuleTestNamespace, "my-kmod")
				kernelMapping, err := kmm.NewRegExKernelMappingBuilder("^.+$").
					WithContainerImage(image).
					BuildKernelMappingConfig()
				Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")

				By("Create moduleLoader container")
				moduleLoader, err := kmm.NewModLoaderContainerBuilder("kmod-a").
					WithKernelMapping(kernelMapping).
					BuildModuleLoaderContainerCfg()
				Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

				By("Create Module")
				_, err = kmm.NewModuleBuilder(APIClient, "webhook-toleration-bad-op", nSpace).
					WithNodeSelector(GeneralConfig.WorkerLabelMap).
					WithModuleLoaderContainer(moduleLoader).
					WithToleration("dummy-key",
						"Existss",
						"",
						"NoSchedule", nil).
					Create()
				Expect(err).To(HaveOccurred(), "error creating module")
				Expect(err.Error()).To(
					ContainSubstring("Toleration[0] invalid operator 'Existss', allowed values are ['Equal', 'Exists']"))
			})

			It("should check value is empty for operator Exists", reportxml.ID("81514"), func() {

				By("Create KernelMapping")
				image := fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION",
					tsparams.LocalImageRegistry, kmmparams.WebhookModuleTestNamespace, "my-kmod")
				kernelMapping, err := kmm.NewRegExKernelMappingBuilder("^.+$").
					WithContainerImage(image).
					BuildKernelMappingConfig()
				Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")

				By("Create moduleLoader container")
				moduleLoader, err := kmm.NewModLoaderContainerBuilder("kmod-a").
					WithKernelMapping(kernelMapping).
					BuildModuleLoaderContainerCfg()
				Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

				By("Create Module")
				_, err = kmm.NewModuleBuilder(APIClient, "webhook-value-not-empty", nSpace).
					WithNodeSelector(GeneralConfig.WorkerLabelMap).
					WithModuleLoaderContainer(moduleLoader).
					WithToleration("dummy-key",
						"Exists",
						"not-empty",
						"NoSchedule", nil).
					Create()
				Expect(err).To(HaveOccurred(), "error creating module")
				Expect(err.Error()).To(ContainSubstring("Toleration[0] value must be empty when operator is 'Exists'"))
			})

		})
	})
})
