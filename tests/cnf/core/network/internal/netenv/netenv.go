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
	v2 "github.com/openshift/cluster-node-tuning-operator/pkg/apis/performanceprofile/v2"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/infrastructure"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/mco"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nto"
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
