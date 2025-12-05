package sriovocpenv

import (
	"fmt"
	"net"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/tsparams"
	"k8s.io/klog/v2"
)

// CreatePodsAndRunTraffic creates test pods and verifies connectivity between them.
func CreatePodsAndRunTraffic(
	clientNodeName,
	serverNodeName,
	sriovResNameClient,
	sriovResNameServer,
	clientMac,
	serverMac string,
	clientIPs,
	serverIPs []string) error {
	klog.V(90).Infof("Creating test pods and checking ICMP connectivity between them")

	clientPod, _, err := createAndWaitTestPods(
		clientNodeName,
		serverNodeName,
		sriovResNameClient,
		sriovResNameServer,
		clientMac,
		serverMac,
		clientIPs,
		serverIPs)
	if err != nil {
		klog.V(90).Infof("Failed to create test pods")

		return err
	}

	return ICMPConnectivityCheck(clientPod, serverIPs)
}

// ICMPConnectivityCheck checks ping against provided IPs from the client pod.
func ICMPConnectivityCheck(clientPod *pod.Builder, destIPAddresses []string, ifName ...string) error {
	klog.V(90).Infof("Checking ping against %v from the client pod %s",
		destIPAddresses, clientPod.Definition.Name)

	for _, destIPAddress := range destIPAddresses {
		ipAddress, _, err := net.ParseCIDR(destIPAddress)
		if err != nil {
			return fmt.Errorf("invalid IP address: %s", destIPAddress)
		}

		TestCmdIcmpCommand := fmt.Sprintf("ping %s -c 5", ipAddress.String())
		if ifName != nil {
			TestCmdIcmpCommand = fmt.Sprintf("ping -I %s %s -c 5", ifName[0], ipAddress.String())
		}

		if ipAddress.To4() == nil {
			TestCmdIcmpCommand = fmt.Sprintf("ping -6 %s -c 5", ipAddress.String())
			if ifName != nil {
				TestCmdIcmpCommand = fmt.Sprintf("ping -6 -I %s %s -c 5", ifName[0], ipAddress.String())
			}
		}

		output, err := clientPod.ExecCommand([]string{"bash", "-c", TestCmdIcmpCommand})
		if err != nil {
			return fmt.Errorf("ICMP connectivity failed: %s\nerror: %w", output.String(), err)
		}
	}

	return nil
}

// CreateAndWaitTestPodWithSecondaryNetwork creates test pod with secondary network
// and waits until it is in the ready state.
func CreateAndWaitTestPodWithSecondaryNetwork(
	podName,
	testNodeName,
	sriovResNameTest,
	testMac string,
	testIPs []string) (*pod.Builder, error) {
	klog.V(90).Infof("Creating a test pod name %s", podName)

	secNetwork := pod.StaticIPAnnotationWithMacAddress(sriovResNameTest, testIPs, testMac)

	testPod, err := pod.NewBuilder(APIClient, podName, tsparams.TestNamespaceName, SriovOcpConfig.OcpSriovTestContainer).
		DefineOnNode(testNodeName).WithPrivilegedFlag().
		WithSecondaryNetwork(secNetwork).CreateAndWaitUntilRunning(tsparams.DefaultTimeout)
	if err != nil {
		klog.V(90).Infof("Failed to create pod %s with secondary network", podName)

		return nil, err
	}

	return testPod, nil
}

// createAndWaitTestPods creates test pods and waits until they are in the ready state.
func createAndWaitTestPods(
	clientNodeName,
	serverNodeName,
	sriovResNameClient,
	sriovResNameServer,
	clientMac,
	serverMac string,
	clientIPs,
	serverIPs []string) (client *pod.Builder, server *pod.Builder, err error) {
	klog.V(90).Infof("Creating client pod with IPs %v, mac %s, SR-IOV resourceName %s"+
		" and server pod with IPs %v, mac %s, SR-IOV resourceName %s.",
		clientIPs, clientMac, sriovResNameClient, serverIPs, serverMac, sriovResNameServer)

	clientPod, err := CreateAndWaitTestPodWithSecondaryNetwork("client", clientNodeName,
		sriovResNameClient, clientMac, clientIPs)
	if err != nil {
		klog.V(90).Infof("Failed to create clientPod")

		return nil, nil, err
	}

	serverPod, err := CreateAndWaitTestPodWithSecondaryNetwork("server", serverNodeName,
		sriovResNameServer, serverMac, serverIPs)
	if err != nil {
		klog.V(90).Infof("Failed to create serverPod")

		return nil, nil, err
	}

	return clientPod, serverPod, nil
}
