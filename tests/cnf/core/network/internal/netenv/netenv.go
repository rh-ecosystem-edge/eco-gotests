package netenv

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	v2 "github.com/openshift/cluster-node-tuning-operator/pkg/apis/performanceprofile/v2"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/mco"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nto"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
)

// DoesClusterHasEnoughNodes verifies if given cluster has enough nodes to run tests.
func DoesClusterHasEnoughNodes(
	apiClient *clients.Settings,
	netConfig *netconfig.NetworkConfig,
	requiredCPNodeNumber int,
	requiredWorkerNodeNumber int) error {
	klog.V(90).Infof("Verifying if cluster has enough workers to run tests")

	workerNodeList, err := nodes.List(
		apiClient,
		metav1.ListOptions{LabelSelector: labels.Set(netConfig.WorkerLabelMap).String()},
	)
	if err != nil {
		return err
	}

	if len(workerNodeList) < requiredWorkerNodeNumber {
		return fmt.Errorf("cluster has less than %d worker nodes", requiredWorkerNodeNumber)
	}

	controlPlaneNodeList, err := nodes.List(
		apiClient,
		metav1.ListOptions{LabelSelector: labels.Set(netConfig.ControlPlaneLabelMap).String()},
	)
	if err != nil {
		return err
	}

	klog.V(90).Infof("Verifying if cluster has enough control-plane nodes to run tests")

	if len(controlPlaneNodeList) < requiredCPNodeNumber {
		return fmt.Errorf("cluster has less than %d control-plane nodes", requiredCPNodeNumber)
	}

	return nil
}

// BFDHasStatus verifies that BFD session on a pod has given status.
func BFDHasStatus(frrPod *pod.Builder, bfdPeer string, status string) error {
	bfdStatusOut, err := frrPod.ExecCommand(append(netparam.VtySh, "sh bfd peers brief json"))
	if err != nil {
		return err
	}

	var peers []netparam.BFDDescription

	err = json.Unmarshal(bfdStatusOut.Bytes(), &peers)
	if err != nil {
		return err
	}

	for _, peer := range peers {
		if peer.BFDPeer == bfdPeer {
			if peer.BFDStatus != status {
				return fmt.Errorf("BFD peer %s on pod %s has status %s (expected %s)",
					bfdPeer, frrPod.Object.Name, peer.BFDStatus, status)
			}

			return nil
		}
	}

	return fmt.Errorf("BFD peer %s not found on pod %s", bfdPeer, frrPod.Object.Name)
}

// MapFirstKeyValue returns the first key-value pair found in the input map.
// If the input map is empty, it returns empty strings.
func MapFirstKeyValue(inputMap map[string]string) (string, string) {
	for key, value := range inputMap {
		return key, value
	}

	return "", ""
}

// DeployPerformanceProfile installs performanceProfile on cluster.
func DeployPerformanceProfile(
	apiClient *clients.Settings,
	netConfig *netconfig.NetworkConfig,
	profileName string,
	isolatedCPU string,
	reservedCPU string,
	hugePages1GCount int32) error {
	klog.V(90).Infof("Ensuring cluster has correct PerformanceProfile deployed")

	mcp, err := mco.Pull(apiClient, netConfig.CnfMcpLabel)
	if err != nil {
		return fmt.Errorf("fail to pull MCP due to : %w", err)
	}

	performanceProfiles, err := nto.ListProfiles(apiClient)
	if err != nil {
		return fmt.Errorf("fail to list PerformanceProfile objects on cluster due to: %w", err)
	}

	if len(performanceProfiles) > 0 {
		for _, perfProfile := range performanceProfiles {
			if perfProfile.Object.Name == profileName {
				klog.V(90).Infof("PerformanceProfile %s exists", profileName)

				return nil
			}
		}

		klog.V(90).Infof("PerformanceProfile doesn't exist on cluster. Removing all pre-existing profiles")

		err := nto.CleanAllPerformanceProfiles(apiClient)
		if err != nil {
			return fmt.Errorf("fail to clean pre-existing performance profiles due to %w", err)
		}

		err = mcp.WaitToBeStableFor(time.Minute, netparam.MCOWaitTimeout)
		if err != nil {
			return err
		}
	}

	klog.V(90).Infof("Required PerformanceProfile doesn't exist. Installing new profile PerformanceProfile")

	_, err = nto.NewBuilder(apiClient, profileName, isolatedCPU, reservedCPU, netConfig.WorkerLabelMap).
		WithHugePages("1G", []v2.HugePage{{Size: "1G", Count: hugePages1GCount}}).Create()
	if err != nil {
		return fmt.Errorf("fail to deploy PerformanceProfile due to: %w", err)
	}

	return mcp.WaitToBeStableFor(time.Minute, netparam.MCOWaitTimeout)
}

// RemoveSriovConfigurationAndWaitForSriovAndMCPStable removes all SR-IOV networks
// and policies in SR-IOV operator namespace.
func RemoveSriovConfigurationAndWaitForSriovAndMCPStable() error {
	klog.V(90).Infof("Removing all SR-IOV networks and policies")

	err := RemoveAllSriovNetworks()
	if err != nil {
		klog.V(90).Infof("Failed to remove all SR-IOV networks")

		return err
	}

	err = RemoveAllPoliciesAndWaitForSriovAndMCPStable()
	if err != nil {
		klog.V(90).Infof("Failed to remove all SR-IOV policies")

		return err
	}

	return nil
}

// RemoveAllSriovNetworks removes all SR-IOV networks.
func RemoveAllSriovNetworks() error {
	klog.V(90).Infof("Removing all SR-IOV networks")

	sriovNs, err := namespace.Pull(netinittools.APIClient, netinittools.NetConfig.SriovOperatorNamespace)
	if err != nil {
		klog.V(90).Infof("Failed to pull SR-IOV operator namespace")

		return err
	}

	err = sriovNs.CleanObjects(
		netparam.DefaultTimeout,
		sriov.GetSriovNetworksGVR())
	if err != nil {
		klog.V(90).Infof("Failed to remove SR-IOV networks from SR-IOV operator namespace")

		return err
	}

	return nil
}

// RemoveAllPoliciesAndWaitForSriovAndMCPStable removes all  SriovNetworkNodePolicies and waits until
// SR-IOV and MCP become stable.
func RemoveAllPoliciesAndWaitForSriovAndMCPStable() error {
	klog.V(90).Infof("Deleting all SriovNetworkNodePolicies and waiting for SR-IOV and MCP become stable.")

	err := sriov.CleanAllNetworkNodePolicies(netinittools.APIClient, netinittools.NetConfig.SriovOperatorNamespace)
	if err != nil {
		return err
	}

	return WaitForSriovAndMCPStable(
		netinittools.APIClient, netparam.MCOWaitTimeout, time.Minute,
		netinittools.NetConfig.CnfMcpLabel, netinittools.NetConfig.SriovOperatorNamespace)
}

// BuildRoutesMapWithSpecificRoutes creates a route map with specific routes.
func BuildRoutesMapWithSpecificRoutes(podList []*pod.Builder, workerNodeList []*nodes.Builder,
	nextHopList []string) (map[string]string, error) {
	if len(podList) == 0 {
		klog.V(90).Infof("Pod list is empty")

		return nil, fmt.Errorf("pod list is empty")
	}

	if len(nextHopList) == 0 {
		klog.V(90).Infof("Nexthop IP addresses list is empty")

		return nil, fmt.Errorf("nexthop IP addresses list is empty")
	}

	if len(nextHopList) < len(podList) {
		klog.V(90).Infof("Number of speaker IP addresses[%d] is less than the number of pods[%d]",
			len(nextHopList), len(podList))

		return nil, fmt.Errorf("insufficient speaker IP addresses: got %d, need at least %d",
			len(nextHopList), len(podList))
	}

	routesMap := make(map[string]string)

	for _, frrPod := range podList {
		if frrPod.Definition.Spec.NodeName == workerNodeList[0].Definition.Name {
			routesMap[frrPod.Definition.Spec.NodeName] = nextHopList[1]
		} else {
			routesMap[frrPod.Definition.Spec.NodeName] = nextHopList[0]
		}
	}

	return routesMap, nil
}

// SetStaticRoute could set or delete static route on all Speaker pods.
func SetStaticRoute(frrPod *pod.Builder, action, destIP, containerName string,
	nextHopMap map[string]string) (string, error) {
	buffer, err := frrPod.ExecCommand(
		[]string{"ip", "route", action, destIP, "via", nextHopMap[frrPod.Definition.Spec.NodeName]}, containerName)
	if err != nil {
		if strings.Contains(buffer.String(), "File exists") {
			klog.V(90).Infof("Warning: Route to %s already exist", destIP)

			return buffer.String(), nil
		}

		if strings.Contains(buffer.String(), "No such process") {
			klog.V(90).Infof("Warning: Route to %s already absent", destIP)

			return buffer.String(), nil
		}

		return buffer.String(), err
	}

	return buffer.String(), nil
}

// WaitForMcpStable waits for the stability of the MCP with the given name.
func WaitForMcpStable(apiClient *clients.Settings, waitingTime, stableDuration time.Duration, mcpName string) error {
	mcp, err := mco.Pull(apiClient, mcpName)
	if err != nil {
		return fmt.Errorf("fail to pull mcp %s from cluster due to: %s", mcpName, err.Error())
	}

	err = mcp.WaitToBeStableFor(stableDuration, waitingTime)
	if err != nil {
		return fmt.Errorf("cluster is not stable: %s", err.Error())
	}

	return nil
}
