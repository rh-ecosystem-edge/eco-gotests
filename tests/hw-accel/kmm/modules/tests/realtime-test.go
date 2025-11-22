package tests

import (
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/go-version"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	"k8s.io/klog/v2"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/mco"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nto"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/serviceaccount"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/check"
	define "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/define"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/get"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/modules/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
)

var _ = Describe("KMM", Ordered, Label(kmmparams.LabelSuite, kmmparams.LabelLongRun), func() {

	Context("Module", Label("use-rt-kernel"), func() {
		var mcpName string

		moduleName := kmmparams.RealtimeKernelNamespace
		kmodName := "module-rt"
		serviceAccountName := "rtkernel-manager"
		image := fmt.Sprintf("%s/%s/%s:$KERNEL_FULL_VERSION",
			tsparams.LocalImageRegistry, kmmparams.RealtimeKernelNamespace, kmodName)
		buildArgValue := fmt.Sprintf("%s.o", kmodName)
		performanceProfileName := "rt-profile"
		rtCPUIsolated := "1,3,5,7"
		rtCPUReserved := "0,2,4,6"

		BeforeAll(func() {
			By("Detect if we can run test on architecture")
			arch, err := get.ClusterArchitecture(APIClient, GeneralConfig.WorkerLabelMap)

			if err != nil {
				Skip("could not detect cluster architecture")
			}

			if arch == "arm64" || arch == "ppc64le" {
				Skip("ARM  and ppc64le platforms do not support the realtime kernel.")
			}

			By("Collect MachineConfigPoolName")
			mcpName = get.MachineConfigPoolName(APIClient)
		})

		AfterAll(func() {
			By("Delete preflight validation")
			pre, _ := kmm.PullPreflightValidationOCP(APIClient, kmmparams.PreflightName, kmmparams.RealtimeKernelNamespace)
			if pre != nil {
				_, _ = pre.Delete()
			}

			By("Delete Module")
			_, _ = kmm.NewModuleBuilder(APIClient, moduleName, kmmparams.RealtimeKernelNamespace).Delete()

			By("Await module to be deleted")
			err := await.ModuleObjectDeleted(APIClient, moduleName, kmmparams.RealtimeKernelNamespace, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while waiting module to be deleted")

			svcAccount := serviceaccount.NewBuilder(APIClient, serviceAccountName, kmmparams.RealtimeKernelNamespace)
			svcAccount.Exists()

			By("Delete ClusterRoleBinding")
			crb := define.ModuleCRB(*svcAccount, kmodName)
			_ = crb.Delete()

			By("Delete Namespace")
			_ = namespace.NewBuilder(APIClient, kmmparams.RealtimeKernelNamespace).Delete()

			By("Delete performance profile that sets Realtime Kernel on workers")
			realtimeProfile := nto.NewBuilder(APIClient, performanceProfileName,
				rtCPUIsolated, rtCPUReserved, GeneralConfig.WorkerLabelMap)
			_, _ = realtimeProfile.Delete()

			By("Waiting machine config pool to update")
			mcp, err := mco.Pull(APIClient, mcpName)
			Expect(err).ToNot(HaveOccurred(), "error while pulling machineconfigpool")

			err = mcp.WaitToBeStableFor(time.Minute, 2*time.Minute)
			Expect(err).To(HaveOccurred(), "the performance profile delete did not triggered a mcp update")

			err = mcp.WaitForUpdate(30 * time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while waiting machineconfigpool to get updated")

		})

		It("should properly build a module on Realtime Kernel", reportxml.ID("53656"), func() {

			By("Create Namespace")
			testNamespace, err := namespace.NewBuilder(APIClient, kmmparams.RealtimeKernelNamespace).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating test namespace")

			configmapContents := define.MultiStageConfigMapContent(kmodName)

			By("Create ConfigMap")
			dockerfileConfigMap, err := configmap.
				NewBuilder(APIClient, kmodName, testNamespace.Object.Name).
				WithData(configmapContents).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating configmap")

			By("Create ServiceAccount")
			svcAccount, err := serviceaccount.
				NewBuilder(APIClient, serviceAccountName, kmmparams.RealtimeKernelNamespace).Create()
			Expect(err).ToNot(HaveOccurred(), "error creating serviceaccount")

			By("Create ClusterRoleBinding")
			crb := define.ModuleCRB(*svcAccount, kmodName)
			_, err = crb.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating clusterrolebinding")

			By("Creating performance profile that sets Realtime Kernel on workers")
			realtimeProfile := nto.NewBuilder(APIClient, performanceProfileName,
				rtCPUIsolated, rtCPUReserved, GeneralConfig.WorkerLabelMap).WithRTKernel()
			_, err = realtimeProfile.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating realtime performance profile")

			By("Get cluster's version")
			clusterVersion, err := cluster.GetOCPClusterVersion(APIClient)
			Expect(err).ToNot(HaveOccurred(), "error detecting clusterversion")

			ocpVersion, _ := version.NewVersion(clusterVersion.Definition.Status.Desired.Version)
			klog.V(kmmparams.KmmLogLevel).Infof("Cluster Version: %s", ocpVersion)
			minVersion, _ := version.NewVersion("4.14.0-0.nightly-2023-01-01-184526")
			maxVersion, _ := version.NewVersion("4.16.0-0.nightly")

			if ocpVersion.GreaterThanOrEqual(minVersion) && ocpVersion.LessThanOrEqual(maxVersion) {
				By("Waiting revert to cgroups v1 on 4.14 and 4.15")
				mcp, err := mco.Pull(APIClient, "master")
				Expect(err).ToNot(HaveOccurred(), "error while pulling master machineconfigpool")

				err = mcp.WaitToBeStableFor(time.Minute, 2*time.Minute)
				Expect(err).To(HaveOccurred(), "the performance profile did not triggered a mcp update")

				err = mcp.WaitForUpdate(30 * time.Minute)
				Expect(err).ToNot(HaveOccurred(), "error while waiting machineconfigpool to get updated")
			}

			By("Waiting machine config pool to update")
			mcp, err := mco.Pull(APIClient, mcpName)
			Expect(err).ToNot(HaveOccurred(), "error while pulling machineconfigpool")

			err = mcp.WaitToBeStableFor(time.Minute, 2*time.Minute)
			Expect(err).To(HaveOccurred(), "the performance profile did not triggered a mcp update")

			err = mcp.WaitForUpdate(30 * time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while waiting machineconfigpool to get updated")

			By("Create KernelMapping")
			kernelMapping := kmm.NewRegExKernelMappingBuilder("^.+$")

			kernelMapping.WithContainerImage(image).
				WithBuildArg(kmmparams.BuildArgName, buildArgValue).
				WithBuildDockerCfgFile(dockerfileConfigMap.Object.Name)
			kerMapOne, err := kernelMapping.BuildKernelMappingConfig()
			Expect(err).ToNot(HaveOccurred(), "error creating kernel mapping")

			By("Create ModuleLoaderContainer")
			moduleLoaderContainer := kmm.NewModLoaderContainerBuilder(kmodName)
			moduleLoaderContainer.WithKernelMapping(kerMapOne)
			moduleLoaderContainer.WithImagePullPolicy("Always")
			moduleLoaderContainerCfg, err := moduleLoaderContainer.BuildModuleLoaderContainerCfg()
			Expect(err).ToNot(HaveOccurred(), "error creating moduleloadercontainer")

			By("Create Module")
			module := kmm.NewModuleBuilder(APIClient, moduleName, kmmparams.RealtimeKernelNamespace).
				WithNodeSelector(GeneralConfig.WorkerLabelMap)
			module = module.WithModuleLoaderContainer(moduleLoaderContainerCfg).
				WithLoadServiceAccount(svcAccount.Object.Name)
			_, err = module.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating module")

			By("Await build pod to complete build")
			err = await.BuildPodCompleted(APIClient, kmmparams.RealtimeKernelNamespace, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while building module")

			By("Await driver container deployment")
			err = await.ModuleDeployment(APIClient, moduleName, kmmparams.RealtimeKernelNamespace, 5*time.Minute,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while waiting on driver deployment")

			By("Check module is loaded on node")
			err = check.ModuleLoaded(APIClient, kmodName, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

			By("Check label is set on all nodes")
			_, err = check.NodeLabel(APIClient, moduleName, kmmparams.RealtimeKernelNamespace,
				GeneralConfig.WorkerLabelMap)
			Expect(err).ToNot(HaveOccurred(), "error while checking the module is loaded")

		})

		It("should be able to run preflightvalidation for realtime kernel", reportxml.ID("84177"), func() {
			By("Detecting cluster architecture")
			arch, err := get.ClusterArchitecture(APIClient, GeneralConfig.WorkerLabelMap)
			if err != nil {
				Skip("could not detect cluster architecture")
			}

			By("Get kernel version for preflight")
			kernelVersion := get.PreflightKernel(arch, true)

			By("Get the DTK Image for preflight test")
			dtkImage := get.PreflightImage(arch)

			By("Create preflightvalidationocp for realtime kernel")
			_, err = kmm.NewPreflightValidationOCPBuilder(APIClient, kmmparams.PreflightName,
				kmmparams.RealtimeKernelNamespace).
				WithKernelVersion(kernelVersion).
				WithDtkImage(dtkImage).
				WithPushBuiltImage(false).
				Create()
			Expect(err).ToNot(HaveOccurred(), "error while creating realtime preflight")

			By("Await preflightvalidationocp checks for realtime kernel")
			err = await.PreflightStageDone(APIClient, kmmparams.PreflightName, moduleName,
				kmmparams.RealtimeKernelNamespace, 3*time.Minute)
			Expect(err).NotTo(HaveOccurred(), "preflightvalidationocp did not complete for realtime kernel")

			By("Get status of the realtime preflightvalidationocp checks")
			status, _ := get.PreflightReason(APIClient, kmmparams.PreflightName, moduleName,
				kmmparams.RealtimeKernelNamespace)
			Expect(strings.Contains(status, "verified image exists")).
				To(BeTrue(), "expected realtime preflight success message not found")

			By("Validate imagestream if using internal registry")
			err = check.ImageStreamExistsForModule(APIClient, kmmparams.RealtimeKernelNamespace,
				moduleName, kmodName, kernelVersion)
			Expect(err).To(HaveOccurred(), " imagstream exists while it is expected not to be there ")
		})
	})
})
