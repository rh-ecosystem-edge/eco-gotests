package tests

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/metallb"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/service"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/metallbenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/tsparams"
)

var _ = Describe("BGP", Ordered, Label("pool-selector"), ContinueOnFailure, func() {
	BeforeAll(func() {
		validateEnvVarAndGetNodeList()

		By("Creating a new instance of MetalLB Speakers on workers")
		err := metallbenv.CreateNewMetalLbDaemonSetAndWaitUntilItsRunning(tsparams.DefaultTimeout, workerLabelMap)
		Expect(err).ToNot(HaveOccurred(), "Failed to recreate metalLb daemonset")

		By("Activating SCTP module on master nodes")
		activateSCTPModuleOnMasterNodes()
	})

	AfterAll(func() {
		if len(cnfWorkerNodeList) > 2 {
			By("Remove custom metallb test label from nodes")
			removeNodeLabel(workerNodeList, metalLbTestsLabel)
		}

		resetOperatorAndTestNS()
	})

	BeforeEach(func() {
		setupTestEnv(ipv4, 32, false)
	})

	AfterEach(func() {
		By("Cleaning MetalLb operator namespace")
		metalLbNs, err := namespace.Pull(APIClient, NetConfig.MlbOperatorNamespace)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull metalLb operator namespace")
		err = metalLbNs.CleanObjects(
			tsparams.DefaultTimeout,
			metallb.GetBGPPeerGVR(),
			metallb.GetBFDProfileGVR(),
			metallb.GetBGPAdvertisementGVR(),
			metallb.GetIPAddressPoolGVR())
		Expect(err).ToNot(HaveOccurred(), "Failed to remove object's from operator namespace")

		By("Cleaning test namespace")
		err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
			tsparams.DefaultTimeout,
			pod.GetGVR(),
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to clean test namespace")
	})

	DescribeTable("MetalLB Load balance external IP accessible to internal cluster IPs",
		func(diffNode bool) {
			By("Fetching LB IP assigned to service")
			lbSvc, err := service.Pull(APIClient, tsparams.MetallbServiceName, tsparams.TestNamespaceName)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull service %s", tsparams.MetallbServiceName)
			Expect(lbSvc.Object.Status.LoadBalancer.Ingress).NotTo(BeEmpty(),
				"Load Balancer IP is not assigned to the tcp service")

			By("Fetching Nginx server pod IP address")
			nginxPod, err := pod.Pull(APIClient, "nginxpod1", tsparams.TestNamespaceName)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull nginx server pod")
			Expect(nginxPod.Object.Status.PodIP).NotTo(BeEmpty(), "Pod IP is empty")

			By("Creating client pod")
			var clientPod *pod.Builder
			if !diffNode {
				clientPod, err = pod.NewBuilder(APIClient, "clientpod", tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
					DefineOnNode(workerNodeList[0].Object.Name).
					CreateAndWaitUntilRunning(tsparams.DefaultTimeout)
				Expect(err).ToNot(HaveOccurred(), "Failed to create a client pod")
			} else {
				clientPod, err = pod.NewBuilder(APIClient, "clientpod", tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
					DefineOnNode(workerNodeList[1].Object.Name).
					CreateAndWaitUntilRunning(tsparams.DefaultTimeout)
				Expect(err).ToNot(HaveOccurred(), "Failed to create a client pod")
			}

			By("Checking that client pod can run curl to Nginx server pod with LB IP address")
			By("Checking Nginx server Pod receives curl on the server pod IP address")
			// update client pod with latest status information with IP address.
			clientPod.Exists()
			output, err := clientPod.ExecCommand([]string{"/bin/bash", "-c",
				fmt.Sprintf("curl %s/serverip", lbSvc.Object.Status.LoadBalancer.Ingress[0].IP)})
			Expect(err).ToNot(HaveOccurred(), "Failed to curl to Nginx pod")
			Expect(output.String()).To(ContainSubstring(nginxPod.Object.Status.PodIP))

			By("Checking client IP seen by nginx server is same as client pod IP address")
			output, err = clientPod.ExecCommand([]string{"/bin/bash", "-c",
				fmt.Sprintf("curl %s/clientip", lbSvc.Object.Status.LoadBalancer.Ingress[0].IP)})
			Expect(err).ToNot(HaveOccurred(), "Failed to curl to Nginx pod")
			Expect(output.String()).To(ContainSubstring(clientPod.Object.Status.PodIP))
		},
		Entry("same node", reportxml.ID("53792"), false),
		Entry("different node", reportxml.ID("53766"), true))
})
