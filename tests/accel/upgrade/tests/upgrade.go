package upgrade

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clusteroperator"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clusterversion"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/route"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/accel/internal/accelinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/accel/upgrade/internal/createres"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/accel/upgrade/internal/deleteres"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/accel/upgrade/internal/upgradeparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/url"
)

var (
	waitToUpgradeStart     = 5 * time.Minute
	waitToUpgradeCompleted = 130 * time.Minute
	desiredUpgradeChannel  = "stable-4." + AccelConfig.HubMinorVersion
)

var _ = Describe("OCP_UPGRADE", Ordered, Label("minor"), func() {
	Context("OCP", func() {
		It("should upgrade successfully", reportxml.ID("72245"), func() {
			By("Get the clusterversion struct")
			version, err := clusterversion.Pull(HubAPIClient)
			Expect(err).ToNot(HaveOccurred(), "error retrieving clusterversion")
			klog.V(90).Infof("got the clusterversion struct %+v", version)

			By("Deploy a workload in the cluster, expose a service and create a route")
			workloadRoute := startTestWorkloadAndGetRoute()

			By("Patch the clusterversion with the desired upgrade channel")
			klog.V(90).Infof("this is the desired upgrade channel: %+v", desiredUpgradeChannel)
			if desiredUpgradeChannel == "stable-4." {
				desiredUpgradeChannel = version.Object.Spec.Channel
				klog.V(90).Infof("clusterversion channel %s", desiredUpgradeChannel)
			}
			version, err = version.WithDesiredUpdateChannel(desiredUpgradeChannel).Update()
			Expect(err).ToNot(HaveOccurred(), "error patching the desired upgrade channel")
			klog.V(90).Infof("patched the clusterversion channel %s", desiredUpgradeChannel)

			By("Get the desired update image")
			desiredImage := AccelConfig.UpgradeTargetVersion
			if desiredImage == "" {
				desiredImage, err = version.GetNextUpdateVersionImage(upgradeparams.Z, false)
				Expect(err).ToNot(HaveOccurred(), "error getting the next update image")
			}
			klog.V(90).Infof("got the desired update image in %s stream %s", upgradeparams.Z, desiredImage)

			By("Patch the clusterversion with the desired upgrade image")
			version, err = version.WithDesiredUpdateImage(desiredImage, true).Update()
			Expect(err).ToNot(HaveOccurred(), "error patching the desired image")
			Expect(version.Object.Spec.DesiredUpdate.Image).To(Equal(desiredImage))
			klog.V(90).Infof("patched the clusterversion with desired image %s", desiredImage)

			By("Wait until upgrade starts")
			err = version.WaitUntilUpdateIsStarted(waitToUpgradeStart)
			Expect(err).ToNot(HaveOccurred(), "the upgrade didn't start after %s", waitToUpgradeStart)
			klog.V(90).Infof("upgrade has started")

			By("Wait until upgrade completes")
			err = version.WaitUntilUpdateIsCompleted(waitToUpgradeCompleted)
			Expect(err).ToNot(HaveOccurred(), "the upgrade didn't complete after %s", waitToUpgradeCompleted)
			klog.V(90).Infof("upgrade has completed")

			By("Check that the clusterversion is updated to the desired version")
			Expect(version.Object.Status.Desired.Image).To(Equal(desiredImage))
			klog.V(90).Infof("upgrade to image %s has completed successfully", desiredImage)

			By("Check that all the operators version is the desired version")
			clusteroperatorList, err := clusteroperator.List(HubAPIClient)
			Expect(err).ToNot(HaveOccurred(), "failed to get the clusteroperators list %v", err)
			hasVersion, err := clusteroperator.VerifyClusterOperatorsVersion(version.Object.Status.Desired.Version,
				clusteroperatorList)
			Expect(err).NotTo(HaveOccurred(), "error while checking operators version")
			Expect(hasVersion).To(BeTrue())

			By("Check that no cluster operator is progressing")
			cosStoppedProgressing, err := clusteroperator.
				WaitForAllClusteroperatorsStopProgressing(HubAPIClient, time.Minute*5)
			Expect(err).ToNot(HaveOccurred(), "error while waiting for cluster operators to stop progressing")
			Expect(cosStoppedProgressing).To(BeTrue(), "error: some cluster operators are still progressing")

			By("Check that all cluster operators are available")
			cosAvailable, err := clusteroperator.
				WaitForAllClusteroperatorsAvailable(HubAPIClient, time.Minute*5)
			Expect(err).NotTo(HaveOccurred(), "error while waiting for cluster operators to become available")
			Expect(cosAvailable).To(BeTrue(), "error: some cluster operators are not available")

			By("Check that all pods are running in workload namespace")
			workloadPods, err := pod.List(HubAPIClient, upgradeparams.TestNamespaceName)
			Expect(err).NotTo(HaveOccurred(), "error listing pods in workload namespace %s", upgradeparams.TestNamespaceName)
			Expect(len(workloadPods) > 0).To(BeTrue(),
				"error: found no running pods in workload namespace %s", upgradeparams.TestNamespaceName)

			for _, workloadPod := range workloadPods {
				err := workloadPod.WaitUntilReady(time.Minute * 2)
				Expect(err).To(BeNil(), "error waiting for workload pod to become ready")
			}

			verifyWorkloadReachable(workloadRoute)
		})

		AfterAll(func() {
			By("Delete workload test namespace")
			klog.V(90).Infof("Deleting test deployments")
			deleteWorkloadNamespace()
		})
	})
})

func startTestWorkloadAndGetRoute() *route.Builder {
	By("Check if workload app namespace exists")

	if _, err := namespace.Pull(HubAPIClient, upgradeparams.TestNamespaceName); err == nil {
		deleteWorkloadNamespace()
	}

	By("Create workload app namespace")

	_, err := namespace.NewBuilder(HubAPIClient, upgradeparams.TestNamespaceName).Create()
	Expect(err).NotTo(HaveOccurred(), "error creating namespace for workload app")

	By("Create workload app deployment")

	_, err = createres.Workload(HubAPIClient, AccelConfig.IBUWorkloadImage)
	Expect(err).ToNot(HaveOccurred(), "error creating workload application")

	By("Create workload app service")

	_, err = createres.Service(HubAPIClient, upgradeparams.ServicePort)
	Expect(err).ToNot(HaveOccurred(), "error creating workload service %v", err)

	By("Create workload app route")

	workloadRoute, err := createres.WorkloadRoute(HubAPIClient)
	Expect(err).ToNot(HaveOccurred(), "error creating workload route %v", err)

	verifyWorkloadReachable(workloadRoute)

	return workloadRoute
}

func deleteWorkloadNamespace() {
	By("Delete workload")

	err := deleteres.Namespace(HubAPIClient)
	Expect(err).NotTo(HaveOccurred(), "error deleting workload namespace %v", err)
}

func verifyWorkloadReachable(workloadRoute *route.Builder) {
	By("Verify workload is reachable")

	Eventually(func() bool {
		_, rc, err := url.Fetch(fmt.Sprintf("http://%s", workloadRoute.Object.Spec.Host), "get", true)
		klog.V(90).Infof("trying to reach the workload with error %v", err)

		return rc == 200
	}, time.Second*10, time.Second*2).Should(BeTrue(), "error reaching the workload")
}
