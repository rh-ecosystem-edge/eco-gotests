package meets

import (
	"fmt"
	"net"
	"time"

	"github.com/hashicorp/go-version"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/hive"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/ztpinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	corev1 "k8s.io/api/core/v1"
)

// AllRequirements accepts multiple requirement functions to ensure the environment meets all requirements.
func AllRequirements(f ...func() (bool, string)) (bool, string) {
	for _, req := range f {
		met, msg := req()
		if !met {
			return met, msg
		}
	}

	return true, ""
}

// HubInfrastructureOperandRunningRequirement ensures that both
// the assisted-service and assisted-image-service pods are running on the hub cluster.
func HubInfrastructureOperandRunningRequirement() (bool, string) {
	servicePodBuilder := ZTPConfig.HubAssistedServicePod()

	running, msg := checkPodRunning(servicePodBuilder)
	if !running {
		return running, msg
	}

	imageBuilder := ZTPConfig.HubAssistedImageServicePod()

	return checkPodRunning(imageBuilder)
}

// SpokeAPIClientReadyRequirement checks that the spoke APIClient has been properly initialized.
func SpokeAPIClientReadyRequirement() (bool, string) {
	if SpokeAPIClient == nil {
		return false, "spoke APIClient has not been initialized"
	}

	return true, ""
}

// SpokeClusterImageSetVersionRequirement checks that the provided clusterimageset meets the version provided.
func SpokeClusterImageSetVersionRequirement(requiredVersion string) (bool, string) {
	if ZTPConfig.SpokeClusterImageSet == "" {
		return false, "Spoke clusterimageset version was not provided through environment"
	}

	_, err := hive.PullClusterImageSet(HubAPIClient, ZTPConfig.SpokeClusterImageSet)
	if err != nil {
		return false, fmt.Sprintf("ClusterImageSet could not be found: %v", err)
	}

	imgSetVersion, _ := version.NewVersion(ZTPConfig.SpokeClusterImageSet)
	currentVersion, _ := version.NewVersion(requiredVersion)

	if imgSetVersion.LessThan(currentVersion) {
		return false, fmt.Sprintf("Discovered clusterimageset version does not meet requirement: %v",
			imgSetVersion.String())
	}

	return true, ""
}

// HubOCPVersionRequirement checks that hub ocp version meets the version provided.
func HubOCPVersionRequirement(requiredVersion string) (bool, string) {
	return ocpVersionRequirement(HubAPIClient, requiredVersion)
}

// SpokeOCPVersionRequirement checks that spoke ocp version meets the version provided.
func SpokeOCPVersionRequirement(requiredVersion string) (bool, string) {
	return ocpVersionRequirement(SpokeAPIClient, requiredVersion)
}

// HubProxyConfiguredRequirement checks that the cluster proxy is configured on the hub.
func HubProxyConfiguredRequirement() (bool, string) {
	return proxyConfiguredRequirement(HubAPIClient)
}

// SpokeProxyConfiguredRequirement checks that the cluster proxy is configured on the spoke.
func SpokeProxyConfiguredRequirement() (bool, string) {
	return proxyConfiguredRequirement(SpokeAPIClient)
}

// HubDisconnectedRequirement checks that the hub is disconnected.
func HubDisconnectedRequirement() (bool, string) {
	return disconnectedRequirement(HubAPIClient)
}

// SpokeDisconnectedRequirement checks that the spoke is disconnected.
func SpokeDisconnectedRequirement() (bool, string) {
	return disconnectedRequirement(SpokeAPIClient)
}

// HubConnectedRequirement checks that the hub is connected.
func HubConnectedRequirement() (bool, string) {
	return connectedRequirement(HubAPIClient)
}

// SpokeConnectedRequirement checks that the spoke is connected.
func SpokeConnectedRequirement() (bool, string) {
	return connectedRequirement(SpokeAPIClient)
}

// HubSingleStackIPv4Requirement checks that the hub has IPv4 single-stack networking.
func HubSingleStackIPv4Requirement() (bool, string) {
	return singleStackIPv4Requirement(HubAPIClient)
}

// SpokeSingleStackIPv4Requirement checks that the spoke has IPv4 single-stack networking.
func SpokeSingleStackIPv4Requirement() (bool, string) {
	return singleStackIPv4Requirement(SpokeAPIClient)
}

// HubSingleStackIPv6Requirement checks that the hub has IPv6 single-stack networking.
func HubSingleStackIPv6Requirement() (bool, string) {
	return singleStackIPv6Requirement(HubAPIClient)
}

// SpokeSingleStackIPv6Requirement checks that the spoke has IPv6 single-stack networking.
func SpokeSingleStackIPv6Requirement() (bool, string) {
	return singleStackIPv6Requirement(SpokeAPIClient)
}

// HubDualStackRequirement checks that the hub has dual-stack networking.
func HubDualStackRequirement() (bool, string) {
	return dualStackRequirement(HubAPIClient)
}

// SpokeDualStackRequirement checks that the spoke has dual-stack networking.
func SpokeDualStackRequirement() (bool, string) {
	return dualStackRequirement(SpokeAPIClient)
}

// checkPodRunning waits for the specified pod to be running.
func checkPodRunning(podBuilder *pod.Builder) (bool, string) {
	err := podBuilder.WaitUntilInStatus(corev1.PodRunning, time.Second*10)
	if err != nil {
		return false, fmt.Sprintf("%s pod found but was not running", podBuilder.Definition.Name)
	}

	return true, ""
}

// ocpVersionRequirement checks that the OCP version of the provided client meets requiredVersion.
func ocpVersionRequirement(clusterobj cluster.APIClientGetter, requiredVersion string) (bool, string) {
	clusterVersion, err := cluster.GetOCPClusterVersion(clusterobj)
	if err != nil {
		return false, fmt.Sprintf("Failed to get clusterversion from %s cluster: %v", getClusterType(clusterobj), err)
	}

	ocpVersion, _ := version.NewVersion(clusterVersion.Definition.Status.Desired.Version)
	currentVersion, _ := version.NewVersion(requiredVersion)

	if ocpVersion.LessThan(currentVersion) {
		return false, fmt.Sprintf("Discovered openshift version does not meet requirement: %v",
			ocpVersion.String())
	}

	return true, ""
}

// proxyConfiguredRequirement checks that the OCP proxy of the provided client is configured.
func proxyConfiguredRequirement(clusterobj cluster.APIClientGetter) (bool, string) {
	ocpProxy, err := cluster.GetOCPProxy(clusterobj)
	if err != nil {
		return false, fmt.Sprintf("Failed to get cluster proxy from %s cluster: %v", getClusterType(clusterobj), err)
	}

	if ocpProxy.Object.Status.HTTPProxy == "" &&
		ocpProxy.Object.Status.HTTPSProxy == "" &&
		ocpProxy.Object.Status.NoProxy == "" {
		return false, fmt.Sprintf("Discovered proxy not configured: %v", ocpProxy.Object.Status)
	}

	return true, ""
}

// disconnectedRequirement checks that the OCP cluster of the provided client is disconnected.
func disconnectedRequirement(clusterobj cluster.APIClientGetter) (bool, string) {
	clusterVersion, err := cluster.GetOCPClusterVersion(clusterobj)
	if err != nil {
		return false, fmt.Sprintf("Failed to get clusterversion from %s cluster: %v", getClusterType(clusterobj), err)
	}

	for _, condition := range clusterVersion.Object.Status.Conditions {
		if condition.Type == configv1.RetrievedUpdates {
			if condition.Reason == "RemoteFailed" {
				return true, ""
			}

			return false, "Provided cluster is connected"
		}
	}

	return false, fmt.Sprintf("Failed to determine if cluster is disconnected, "+
		"could not find '%s' condition", configv1.RetrievedUpdates)
}

// connectedRequirement checks that the OCP cluster of the provided client is connected.
func connectedRequirement(clusterobj cluster.APIClientGetter) (bool, string) {
	clusterVersion, err := cluster.GetOCPClusterVersion(clusterobj)
	if err != nil {
		return false, fmt.Sprintf("Failed to get clusterversion from %s cluster: %v", getClusterType(clusterobj), err)
	}

	for _, condition := range clusterVersion.Object.Status.Conditions {
		if condition.Type == configv1.RetrievedUpdates {
			if condition.Reason == "RemoteFailed" {
				return false, "Provided cluster is disconnected"
			}

			return true, ""
		}
	}

	return false, fmt.Sprintf("Failed to determine if cluster is connected, "+
		"could not find '%s' condition", configv1.RetrievedUpdates)
}

// singleStackIPv4Requirement checks that the OCP network of the provided client is single-stack ipv4.
func singleStackIPv4Requirement(clusterobj cluster.APIClientGetter) (bool, string) {
	ocpNetwork, err := cluster.GetOCPNetworkConfig(clusterobj)
	if err != nil {
		return false, fmt.Sprintf("Failed to get cluster network from %s cluster: %v", getClusterType(clusterobj), err)
	}

	for _, clusterNet := range ocpNetwork.Object.Status.ClusterNetwork {
		ip, _, _ := net.ParseCIDR(clusterNet.CIDR)
		v4Check := ip.To4()

		if v4Check == nil {
			return false, "ClusterNetwork was not IPv4"
		}
	}

	return true, ""
}

// singleStackIPv6Requirement checks that the OCP network of the provided client is single-stack ipv6.
func singleStackIPv6Requirement(clusterobj cluster.APIClientGetter) (bool, string) {
	ocpNetwork, err := cluster.GetOCPNetworkConfig(clusterobj)
	if err != nil {
		return false, fmt.Sprintf("Failed to get cluster network from %s cluster: %v", getClusterType(clusterobj), err)
	}

	for _, clusterNet := range ocpNetwork.Object.Status.ClusterNetwork {
		ip, _, _ := net.ParseCIDR(clusterNet.CIDR)
		v4Check := ip.To4()

		if v4Check != nil {
			return false, "ClusterNetwork was not IPv6"
		}
	}

	return true, ""
}

// dualStackRequirement checks that the OCP network of the provided client is dual-stack.
func dualStackRequirement(clusterobj cluster.APIClientGetter) (bool, string) {
	ipv4 := false
	ipv6 := false

	hubNetwork, err := cluster.GetOCPNetworkConfig(clusterobj)
	if err != nil {
		return false, fmt.Sprintf("Failed to get cluster network from %s cluster: %v", getClusterType(clusterobj), err)
	}

	for _, clusterNet := range hubNetwork.Object.Status.ClusterNetwork {
		ip, _, _ := net.ParseCIDR(clusterNet.CIDR)
		v4Check := ip.To4()

		if v4Check != nil {
			ipv4 = true
		} else {
			ipv6 = true
		}
	}

	if !ipv4 || !ipv6 {
		return false, "Only found cluster networks in one address family"
	}

	return true, ""
}

// getClusterType returns cluster type based on provided apiClient.
func getClusterType(clusterobj cluster.APIClientGetter) string {
	if clusterobj == HubAPIClient {
		return "hub"
	}

	return "spoke"
}
