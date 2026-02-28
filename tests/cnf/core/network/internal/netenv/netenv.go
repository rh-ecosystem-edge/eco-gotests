package netenv

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/infrastructure"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// IsSNOCluster checks if the cluster is a Single Node OpenShift (SNO) cluster.
func IsSNOCluster(apiClient *clients.Settings) (bool, error) {
	klog.V(90).Infof("Checking if cluster is SNO (Single Node OpenShift)")

	infraConfig, err := infrastructure.Pull(apiClient)
	if err != nil {
		return false, fmt.Errorf("failed to pull infrastructure configuration: %w", err)
	}

	return infraConfig.Object.Status.ControlPlaneTopology == configv1.SingleReplicaTopologyMode, nil
}

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
	var (
		bfdStatusOut bytes.Buffer
		err          error
	)

	err = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, 10*time.Second, true,
		func(ctx context.Context) (bool, error) {
			bfdStatusOut, err = frrPod.ExecCommand(append(netparam.VtySh, "sh bfd peers brief json"))
			if err != nil {
				klog.V(90).Infof("Failed to execute BFD status command: %v", err)

				return false, err
			}

			if strings.TrimSpace(bfdStatusOut.String()) == "" {
				klog.V(90).Infof("BFD status command returned empty output. retrying...")

				return false, nil
			}

			return true, nil
		})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("BFD status command returned empty output")
		}

		return err
	}

	var peers []netparam.BFDDescription

	err = json.Unmarshal(bfdStatusOut.Bytes(), &peers)
	if err != nil {
		klog.V(90).Infof("Failed to Unmarshal bfdStatus string: %s into bfdStatus struct", bfdStatusOut.String())

		return fmt.Errorf(
			"failed to unmarshal BFD status output with error: %w, bfd status output: %s", err, bfdStatusOut.String())
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

// GetClusterIPFamily detects the cluster's IP family by checking node external OVN addresses.
// Returns netparam.IPV4Family, netparam.IPV6Family, or netparam.DualIPFamily.
func GetClusterIPFamily(apiClient *clients.Settings) (string, error) {
	klog.V(90).Infof("Detecting cluster IP family from OVN external addresses")

	const ovnExternalAddresses = "k8s.ovn.org/node-primary-ifaddr"

	nodesList, err := nodes.List(apiClient)
	if err != nil {
		return "", fmt.Errorf("failed to list nodes: %w", err)
	}

	if len(nodesList) == 0 {
		return "", fmt.Errorf("no nodes found")
	}

	var clusterIPFamily string

	for nodeIndex, node := range nodesList {
		raw := node.Object.Annotations[ovnExternalAddresses]
		if raw == "" {
			return "", fmt.Errorf("node %q missing %q annotation", node.Object.Name, ovnExternalAddresses)
		}

		var extNetwork nodes.ExternalNetworks

		err := json.Unmarshal([]byte(raw), &extNetwork)
		if err != nil {
			return "", fmt.Errorf("failed to parse external network annotation on node %s: %w",
				node.Object.Name, err)
		}

		var ipFamily string

		switch {
		case extNetwork.IPv4 != "" && extNetwork.IPv6 != "":
			ipFamily = netparam.DualIPFamily
		case extNetwork.IPv4 != "":
			ipFamily = netparam.IPV4Family
		case extNetwork.IPv6 != "":
			ipFamily = netparam.IPV6Family
		default:
			return "", fmt.Errorf("no IPv4 or IPv6 external address found on node %s", node.Object.Name)
		}

		if nodeIndex == 0 {
			clusterIPFamily = ipFamily

			continue
		}
	}

	klog.V(90).Infof("Cluster IP family: %s", clusterIPFamily)

	return clusterIPFamily, nil
}

// ClusterSupportsIPv4 returns true if the cluster supports IPv4 (single-stack or dual-stack).
func ClusterSupportsIPv4(ipFamily string) bool {
	return ipFamily == netparam.IPV4Family || ipFamily == netparam.DualIPFamily
}

// ClusterSupportsIPv6 returns true if the cluster supports IPv6 (single-stack or dual-stack).
func ClusterSupportsIPv6(ipFamily string) bool {
	return ipFamily == netparam.IPV6Family || ipFamily == netparam.DualIPFamily
}

// ClusterSupportsDualStack returns true if the cluster is dual-stack.
func ClusterSupportsDualStack(ipFamily string) bool {
	return ipFamily == netparam.DualIPFamily
}
