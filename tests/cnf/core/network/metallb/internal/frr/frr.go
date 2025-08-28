package frr

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/golang/glog"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/tsparams"
)

type (
	bgpDescription struct {
		BGPState         string `json:"bgpState"`
		BGPHoldTime      int    `json:"bgpTimerHoldTimeMsecs"`
		BGPKeepAliveTime int    `json:"bgpTimerKeepAliveIntervalMsecs"`
	}

	bgpStatus struct {
		VrfID         int                `json:"vrfId"`
		VrfName       string             `json:"vrfName"`
		TableVersion  int                `json:"tableVersion"`
		RouterID      string             `json:"routerId"`
		DefaultLocPrf int                `json:"defaultLocPrf"`
		LocalAS       int                `json:"localAS"`
		Routes        map[string][]Route `json:"routes"`
	}

	// Route creates a struct of routes from the output of the "show ip bgp json" command.
	Route struct {
		Valid     bool   `json:"valid"`
		Multipath bool   `json:"multipath,omitempty"`
		PathFrom  string `json:"pathFrom"`
		Prefix    string `json:"prefix"`
		PrefixLen int    `json:"prefixLen"`
		LocalPref uint32 `json:"locPrf"`
		Network   string `json:"network"`
		Metric    int    `json:"metric"`
		Weight    int    `json:"weight"`
		PeerID    string `json:"peerId"`
		Path      string `json:"path"`
		Origin    string `json:"origin"`
		Nexthops  []struct {
			IP       string `json:"ip"`
			Hostname string `json:"hostname"`
			Afi      string `json:"afi"`
			Used     bool   `json:"used"`
		} `json:"nexthops"`
		Bestpath bool `json:"bestpath,omitempty"`
	}

	advertisedRoute struct {
		AddrPrefix    string `json:"addrPrefix"`
		PrefixLen     int    `json:"prefixLen"`
		Network       string `json:"network"`
		NextHop       string `json:"nextHop"`
		Metric        int    `json:"metric"`
		LocPrf        int    `json:"locPrf"`
		Weight        int    `json:"weight"`
		Path          string `json:"path"`
		BGPOriginCode string `json:"bgpOriginCode"`
	}
	// BgpAdvertisedRoutes creates struct from json output.
	BgpAdvertisedRoutes struct {
		BGPTableVersion  int                        `json:"bgpTableVersion"`
		BGPLocalRouterID string                     `json:"bgpLocalRouterId"`
		DefaultLocPrf    int                        `json:"defaultLocPrf"`
		LocalAS          int                        `json:"localAS"`
		AdvertisedRoutes map[string]advertisedRoute `json:"advertisedRoutes"`
	}
	// BgpReceivedRoutes struct includes Routes map.
	BgpReceivedRoutes struct {
		Routes map[string][]RouteInfo `json:"routes"`
	}
	// RouteInfo struct includes route info.
	RouteInfo struct {
		Prefix    string    `json:"prefix"`
		PrefixLen int       `json:"prefixLen"`
		Protocol  string    `json:"protocol"`
		Metric    int       `json:"metric"`
		Uptime    string    `json:"uptime"`
		Nexthops  []Nexthop `json:"nexthops"`
	}
	// Nexthop struct includes nexthop route info.
	Nexthop struct {
		Flags          int    `json:"flags"`
		Fib            bool   `json:"fib"`
		IP             string `json:"ip"`
		Afi            string `json:"afi"`
		InterfaceIndex int    `json:"interfaceIndex"`
		InterfaceName  string `json:"interfaceName"`
		Active         bool   `json:"active"`
		Weight         int    `json:"weight"`
	}
	// BGPConnectionInfo struct includes the connectRetryTimer.
	BGPConnectionInfo struct {
		ConnectRetryTimer int `json:"connectRetryTimer"`
		RemoteAS          int `json:"remoteAS"`
	}

	// GRTimers struct includes the GracefulRestart timers.
	GRTimers struct {
		ConfiguredRestartTimer int `json:"configuredRestartTimer"`
		ReceivedRestartTimer   int `json:"receivedRestartTimer"`
		RestartTimerRemaining  int `json:"restartTimerRemaining"`
	}

	// GRStatus struct includes the GracefulRestart status per BGP neighbor.
	GRStatus struct {
		NeighborAddr string   `json:"neighborAddr"`
		LocalGrMode  string   `json:"localGrMode"`
		RemoteGrMode string   `json:"remoteGrMode"`
		RBit         bool     `json:"rBit"`
		NBit         bool     `json:"nBit"`
		Timers       GRTimers `json:"timers"`
	}

	// BGPNeighborGRStatus is a map of GRStatus per peer.
	BGPNeighborGRStatus map[string]GRStatus

	// InterfaceDetails retrieved from FRR pod interface.
	InterfaceDetails struct {
		IPAddresses []struct {
			Address    string `json:"address"`
			Secondary  bool   `json:"secondary"`
			Unnumbered bool   `json:"unnumbered"`
		} `json:"ipAddresses"`
	}
)

// DefineBaseConfig defines minimal required FRR configuration.
func DefineBaseConfig(daemonsConfig, frrConfig, vtyShConfig string) map[string]string {
	configMapData := make(map[string]string)
	configMapData["daemons"] = daemonsConfig
	configMapData["frr.conf"] = frrConfig
	configMapData["vtysh.conf"] = vtyShConfig

	return configMapData
}

// DefineBGPConfig returns string which represents BGP config file peering to all given IP addresses.
//
//nolint:goconst
func DefineBGPConfig(localBGPASN, remoteBGPASN int, neighborsIPAddresses []string, multiHop, bfd bool) string {
	bgpConfig := tsparams.FRRBaseConfig +
		fmt.Sprintf("router bgp %d\n", localBGPASN) +
		tsparams.FRRDefaultBGPPreConfig

	for _, ipAddress := range neighborsIPAddresses {
		bgpConfig += fmt.Sprintf("  neighbor %s remote-as %d\n  neighbor %s password %s\n",
			ipAddress, remoteBGPASN, ipAddress, tsparams.BGPPassword)

		if bfd {
			bgpConfig += fmt.Sprintf("  neighbor %s bfd\n", ipAddress)
		}

		if multiHop {
			bgpConfig += fmt.Sprintf("  neighbor %s ebgp-multihop 2\n", ipAddress)
		}
	}

	bgpConfig += "!\naddress-family ipv4 unicast\n"
	for _, ipAddress := range neighborsIPAddresses {
		bgpConfig += fmt.Sprintf("  neighbor %s activate\n", ipAddress)
	}

	bgpConfig += "exit-address-family\n!\naddress-family ipv6 unicast\n"
	for _, ipAddress := range neighborsIPAddresses {
		bgpConfig += fmt.Sprintf("  neighbor %s activate\n", ipAddress)
	}

	bgpConfig += "exit-address-family\n!\nline vty\n!\nend\n"

	return bgpConfig
}

// DefineBGPConfigWithIPv4AndIPv6 returns string which represents BGP config file peering to all given IP addresses.
func DefineBGPConfigWithIPv4AndIPv6(localBGPASN, remoteBGPASN int, neighborsIPAddresses []string,
	multiHop, bfd bool) string {
	bgpConfig := tsparams.FRRBaseConfig +
		fmt.Sprintf("router bgp %d\n", localBGPASN) +
		tsparams.FRRDefaultBGPPreConfig

	for _, ipAddress := range neighborsIPAddresses {
		bgpConfig += fmt.Sprintf("  neighbor %s remote-as %d\n  neighbor %s password %s\n",
			ipAddress, remoteBGPASN, ipAddress, tsparams.BGPPassword)

		if bfd {
			bgpConfig += fmt.Sprintf("  neighbor %s bfd\n", ipAddress)
		}

		if multiHop {
			bgpConfig += fmt.Sprintf("  neighbor %s ebgp-multihop 2\n", ipAddress)
		}
	}

	bgpConfig += "!\naddress-family ipv4 unicast\n"

	for _, ipAddress := range neighborsIPAddresses {
		if net.ParseIP(ipAddress).To4() == nil {
			continue
		}

		bgpConfig += fmt.Sprintf("  neighbor %s activate\n", ipAddress)
	}

	bgpConfig += "exit-address-family\n!\naddress-family ipv6 unicast\n"

	for _, ipAddress := range neighborsIPAddresses {
		if net.ParseIP(ipAddress).To4() != nil {
			continue
		}

		bgpConfig += fmt.Sprintf("  neighbor %s activate\n", ipAddress)
	}

	bgpConfig += "exit-address-family\n!\nline vty\n!\nend\n"

	return bgpConfig
}

// DefineBGPConfigWithStaticRouteAndNetwork defines BGP config file with static route and network.
//
//nolint:goconst
func DefineBGPConfigWithStaticRouteAndNetwork(localBGPASN, remoteBGPASN int, hubPodIPs,
	advertisedIPv4Routes, advertisedIPv6Routes, neighborsIPAddresses []string,
	multiHop, bfd bool) string {
	bgpConfig := tsparams.FRRBaseConfig +
		fmt.Sprintf("ip route %s/32 %s\n", neighborsIPAddresses[1], hubPodIPs[0]) +
		fmt.Sprintf("ip route %s/32 %s\n!\n", neighborsIPAddresses[0], hubPodIPs[1]) +
		fmt.Sprintf("router bgp %d\n", localBGPASN) +
		tsparams.FRRDefaultBGPPreConfig

	for _, ipAddress := range neighborsIPAddresses {
		bgpConfig += fmt.Sprintf("  neighbor %s remote-as %d\n  neighbor %s password %s\n",
			ipAddress, remoteBGPASN, ipAddress, tsparams.BGPPassword)

		if bfd {
			bgpConfig += fmt.Sprintf("  neighbor %s bfd\n", ipAddress)
		}

		if multiHop {
			bgpConfig += fmt.Sprintf("  neighbor %s ebgp-multihop 2\n", ipAddress)
		}
	}

	bgpConfig += "!\naddress-family ipv4 unicast\n"
	for _, ipAddress := range neighborsIPAddresses {
		bgpConfig += fmt.Sprintf("  neighbor %s activate\n", ipAddress)
	}

	bgpConfig += fmt.Sprintf("  network %s\n", advertisedIPv4Routes[0])
	bgpConfig += fmt.Sprintf("  network %s\n", advertisedIPv4Routes[1])
	bgpConfig += "exit-address-family\n"

	// Add network commands only once for IPv6
	bgpConfig += "!\naddress-family ipv6 unicast\n"
	for _, ipAddress := range neighborsIPAddresses {
		bgpConfig += fmt.Sprintf("  neighbor %s activate\n", ipAddress)
	}

	bgpConfig += fmt.Sprintf("  network %s\n", advertisedIPv6Routes[0])
	bgpConfig += fmt.Sprintf("  network %s\n", advertisedIPv6Routes[1])
	bgpConfig += "exit-address-family\n"

	bgpConfig += "!\nline vty\n!\nend\n"

	return bgpConfig
}

// DefineBGPConfigWithIPv4Network defines BGP config file with network advertising only ipv4.
func DefineBGPConfigWithIPv4Network(localBGPASN, remoteBGPASN int,
	advertisedIPv4Routes, neighborsIPAddresses []string,
	multiHop, bfd bool) string {
	bgpConfig := tsparams.FRRBaseConfig +
		fmt.Sprintf("router bgp %d\n", localBGPASN) +
		tsparams.FRRDefaultBGPPreConfig

	for _, ipAddress := range neighborsIPAddresses {
		bgpConfig += fmt.Sprintf("  neighbor %s remote-as %d\n  neighbor %s password %s\n",
			ipAddress, remoteBGPASN, ipAddress, tsparams.BGPPassword)

		if bfd {
			bgpConfig += fmt.Sprintf("  neighbor %s bfd\n", ipAddress)
		}

		if multiHop {
			bgpConfig += fmt.Sprintf("  neighbor %s ebgp-multihop 2\n", ipAddress)
		}
	}

	bgpConfig += "!\naddress-family ipv4 unicast\n"
	for _, ipAddress := range neighborsIPAddresses {
		bgpConfig += fmt.Sprintf("  neighbor %s activate\n", ipAddress)
	}

	bgpConfig += fmt.Sprintf("  network %s\n", advertisedIPv4Routes[0])
	bgpConfig += fmt.Sprintf("  network %s\n", advertisedIPv4Routes[1])

	bgpConfig += "exit-address-family\n!\naddress-family ipv6 unicast\n"
	for _, ipAddress := range neighborsIPAddresses {
		bgpConfig += fmt.Sprintf("  neighbor %s activate\n", ipAddress)
	}

	bgpConfig += "exit-address-family\n!\nline vty\n!\nend\n"

	return bgpConfig
}

// DefineBGPConfigWithIPv6Network defines BGP config file with network advertising only ipv6.
func DefineBGPConfigWithIPv6Network(localBGPASN, remoteBGPASN int,
	advertisedIPv6Routes, neighborsIPAddresses []string,
	multiHop, bfd bool) string {
	bgpConfig := tsparams.FRRBaseConfig +
		fmt.Sprintf("router bgp %d\n", localBGPASN) +
		tsparams.FRRDefaultBGPPreConfig

	for _, ipAddress := range neighborsIPAddresses {
		bgpConfig += fmt.Sprintf("  neighbor %s remote-as %d\n  neighbor %s password %s\n",
			ipAddress, remoteBGPASN, ipAddress, tsparams.BGPPassword)

		if bfd {
			bgpConfig += fmt.Sprintf("  neighbor %s bfd\n", ipAddress)
		}

		if multiHop {
			bgpConfig += fmt.Sprintf("  neighbor %s ebgp-multihop 2\n", ipAddress)
		}
	}

	bgpConfig += "!\naddress-family ipv6 unicast\n"
	for _, ipAddress := range neighborsIPAddresses {
		bgpConfig += fmt.Sprintf("  neighbor %s activate\n", ipAddress)
	}

	bgpConfig += fmt.Sprintf("  network %s\n", advertisedIPv6Routes[0])
	bgpConfig += fmt.Sprintf("  network %s\n", advertisedIPv6Routes[1])

	bgpConfig += "exit-address-family\n!\nline vty\n!\nend\n"

	return bgpConfig
}

// BGPNeighborshipHasState verifies that BGP session on a pod has given state.
func BGPNeighborshipHasState(frrPod *pod.Builder, neighborIPAddress string, state string) (bool, error) {
	var result map[string]bgpDescription

	bgpStateOut, err := frrPod.ExecCommand(append(netparam.VtySh, "sh bgp neighbors json"))
	if err != nil {
		return false, err
	}

	err = json.Unmarshal(bgpStateOut.Bytes(), &result)
	if err != nil {
		return false, err
	}

	return result[neighborIPAddress].BGPState == state, nil
}

// IsProtocolConfigured verifies that given protocol is set in frr config.
func IsProtocolConfigured(frrPod *pod.Builder, protocol string) (bool, error) {
	frrConf, err := runningConfig(frrPod)
	if err != nil {
		return false, err
	}

	frrConfList := strings.Split(frrConf, "!")
	for _, configLine := range frrConfList {
		if strings.HasPrefix(strings.TrimSpace(configLine), protocol) {
			return true, nil
		}
	}

	return false, nil
}

// GetMetricsByPrefix pulls all metrics from frr pods and sort them in the list by given prefix.
func GetMetricsByPrefix(frrPod *pod.Builder, metricPrefix string) ([]string, error) {
	stdout, err := frrPod.ExecCommand([]string{"curl", "localhost:7573/metrics"}, "frr")

	if err != nil {
		return nil, err
	}

	if len(strings.Split(stdout.String(), "\n")) == 0 {
		return nil, fmt.Errorf("failed to collect metrics due to empty response")
	}

	var collectedMetrics []string

	for _, line := range strings.Split(stdout.String(), "\n") {
		if strings.HasPrefix(line, metricPrefix) {
			metricsKey := line[0:strings.Index(line, "{")]
			collectedMetrics = append(collectedMetrics, metricsKey)
		}
	}

	if len(collectedMetrics) < 1 {
		return nil, fmt.Errorf("failed to collect metrics")
	}

	return collectedMetrics, nil
}

// SetStaticRoute could set or delete static route on all Speaker pods.
func SetStaticRoute(frrPod *pod.Builder, action, destIP string, nextHopMap map[string]string) (string, error) {
	buffer, err := frrPod.ExecCommand(
		[]string{"ip", "route", action, destIP, "via", nextHopMap[frrPod.Definition.Spec.NodeName]}, "frr")
	if err != nil {
		if strings.Contains(buffer.String(), "File exists") {
			glog.V(90).Infof("Warning: Route to %s already exist", destIP)

			return buffer.String(), nil
		}

		if strings.Contains(buffer.String(), "No such process") {
			glog.V(90).Infof("Warning: Route to %s already absent", destIP)

			return buffer.String(), nil
		}

		return buffer.String(), err
	}

	return buffer.String(), nil
}

// GetBGPStatus returns bgp status output from frr pod.
func GetBGPStatus(frrPod *pod.Builder, protocolVersion string, containerName ...string) (*bgpStatus, error) {
	glog.V(90).Infof("Getting bgp status from pod: %s", frrPod.Definition.Name)

	return getBgpStatus(frrPod, fmt.Sprintf("show bgp %s json", protocolVersion), containerName...)
}

// GetBGPCommunityStatus returns bgp community status from frr pod.
func GetBGPCommunityStatus(frrPod *pod.Builder, communityString, ipProtocolVersion string) (*bgpStatus, error) {
	glog.V(90).Infof("Getting bgp community status from container on pod: %s", frrPod.Definition.Name)

	return getBgpStatus(frrPod, fmt.Sprintf("show bgp %s community %s json", ipProtocolVersion, communityString))
}

// FetchBGPConnectTimeValue fetches and returns the ConnectRetryTimer value for the specified BGP peer.
func FetchBGPConnectTimeValue(frrk8sPods []*pod.Builder, bgpPeerIP string) (int, error) {
	for _, frrk8sPod := range frrk8sPods {
		// Run the "show bgp neighbor <bgpPeerIP> json" command on each pod
		output, err := frrk8sPod.ExecCommand(append(netparam.VtySh,
			fmt.Sprintf("show bgp neighbor %s json", bgpPeerIP)), "frr")
		if err != nil {
			return 0, fmt.Errorf("error collecting BGP neighbor info from pod %s: %w",
				frrk8sPod.Definition.Name, err)
		}

		// Parsing JSON
		var bgpData map[string]BGPConnectionInfo
		err = json.Unmarshal(output.Bytes(), &bgpData)

		if err != nil {
			return 0, fmt.Errorf("error parsing BGP neighbor JSON for pod %s: %w", frrk8sPod.Definition.Name, err)
		}

		// Extracting ConnectRetryTimer from the parsed JSON
		for _, bgpInfo := range bgpData {
			return bgpInfo.ConnectRetryTimer, nil
		}
	}

	return 0, fmt.Errorf("no BGP neighbor data found for peer %s", bgpPeerIP)
}

// ValidateBGPRemoteAS validates the remoteAS value for the specified BGP peer across all FRR pods.
func ValidateBGPRemoteAS(frrk8sPods []*pod.Builder, bgpPeerIP string, expectedRemoteAS int) error {
	glog.V(90).Infof("Validating the frr nodes receive the correct remote bgp peer AS : %d", expectedRemoteAS)

	for _, frrk8sPod := range frrk8sPods {
		// Run the "show bgp neighbor <bgpPeerIP> json" command on each pod
		output, err := frrk8sPod.ExecCommand(append(netparam.VtySh,
			fmt.Sprintf("show bgp neighbor %s json", bgpPeerIP)), "frr")
		if err != nil {
			return fmt.Errorf("error collecting BGP neighbor info from pod %s: %w",
				frrk8sPod.Definition.Name, err)
		}

		// Parsing JSON
		var bgpData map[string]BGPConnectionInfo
		err = json.Unmarshal(output.Bytes(), &bgpData)

		if err != nil {
			return fmt.Errorf("error parsing BGP neighbor JSON for pod %s: %w", frrk8sPod.Definition.Name, err)
		}

		// Validate RemoteAS
		for _, bgpInfo := range bgpData {
			if bgpInfo.RemoteAS == expectedRemoteAS {
				return nil // Match found
			}
		}
	}

	// If no matches are found across all pods
	return fmt.Errorf("no BGP neighbor with RemoteAS %d found for peer %s", expectedRemoteAS, bgpPeerIP)
}

func getBgpStatus(frrPod *pod.Builder, cmd string, containerName ...string) (*bgpStatus, error) {
	cName := "frr"

	if len(containerName) > 0 {
		cName = containerName[0]
	}

	glog.V(90).Infof("Getting bgp status from container: %s of pod: %s", cName, frrPod.Definition.Name)

	bgpStateOut, err := frrPod.ExecCommand(append(netparam.VtySh, cmd))

	if err != nil {
		return nil, err
	}

	bgpStatus := bgpStatus{}

	err = json.Unmarshal(bgpStateOut.Bytes(), &bgpStatus)
	if err != nil {
		glog.V(90).Infof("Failed to Unmarshal bgpStatus string: %s in to bgpStatus struct", bgpStateOut.String())

		return nil, err
	}

	return &bgpStatus, nil
}

// GetGracefulRestartStatus fetches and returns the GracefulRestart status value for the
// specified BGP peer in the default VRF.
func GetGracefulRestartStatus(frrPod *pod.Builder, neighborIP string) (GRStatus, error) {
	glog.V(90).Infof("Getting GracefulRestart status from container: %s of pod: %s", "frr", frrPod.Definition.Name)

	grStateOut, err := frrPod.ExecCommand(append(netparam.VtySh, "sh bgp neighbors graceful-restart json"))

	if err != nil {
		glog.V(90).Infof("Failed to execute Graceful Restart command")

		return GRStatus{}, err
	}

	bgpNeighborGRStatus := BGPNeighborGRStatus{}

	err = json.Unmarshal(grStateOut.Bytes(), &bgpNeighborGRStatus)
	if err != nil {
		glog.V(90).Infof("Failed to Unmarshal grStateOut string: %s in to GRStatus struct", grStateOut.String())

		return GRStatus{}, err
	}

	return bgpNeighborGRStatus[neighborIP], nil
}

func runningConfig(frrPod *pod.Builder) (string, error) {
	bgpStateOut, err := frrPod.ExecCommand(append(netparam.VtySh, "sh run"), tsparams.FRRContainerName)
	if err != nil {
		return "", fmt.Errorf("error collecting frr running config from pod %s due to %w",
			frrPod.Definition.Name, err)
	}

	return bgpStateOut.String(), nil
}

// GetBGPAdvertisedRoutes retrieves the routes advertised from the external frr pod to the frr nodes.
func GetBGPAdvertisedRoutes(frrPod *pod.Builder, nodeIPs []string) (map[string]string, error) {
	allRoutes := make(map[string]string)

	// Loop through each nodeIP and execute the command
	for _, nodeIP := range nodeIPs {
		// Execute the BGP command for each nodeIP
		routes, err := frrPod.ExecCommand(append(netparam.VtySh,
			fmt.Sprintf("sh ip bgp neighbors %s advertised-routes json", nodeIP)))
		if err != nil {
			return nil, fmt.Errorf("error collecting BGP advertised routes from pod %s for nodeIP %s: %w",
				frrPod.Definition.Name, nodeIP, err)
		}

		// Parse the BGP advertised routes for each nodeIP
		bgpAdvertised, err := parseBGPAdvertisedRoutes(routes.String())
		if err != nil {
			return nil, fmt.Errorf("error parsing BGP advertised routes for nodeIP %s: %w", nodeIP, err)
		}

		// Store the result in the map for the current nodeIP
		allRoutes[nodeIP] = bgpAdvertised
	}

	// Return the map of routes
	return allRoutes, nil
}

// VerifyBGPReceivedRoutesOnFrrNodes verifies routes were received via BGP on Frr nodes.
func VerifyBGPReceivedRoutesOnFrrNodes(frrk8sPods []*pod.Builder) (string, error) {
	var result strings.Builder

	for _, frrk8sPod := range frrk8sPods {
		// Run the "sh ip route bgp json" command on each pod
		output, err := frrk8sPod.ExecCommand(append(netparam.VtySh, "sh ip route bgp json"), "frr")
		if err != nil {
			return "", fmt.Errorf("error collecting BGP received routes from pod %s: %w",
				frrk8sPod.Definition.Name, err)
		}

		// Parse the JSON output to get the BGP routes
		bgpRoutes, err := parseBGPReceivedRoutes(output.String())

		if err != nil {
			return "", fmt.Errorf("error parsing BGP JSON from pod %s: %w", frrk8sPod.Definition.Name, err)
		}

		// Write the pod name to the result
		result.WriteString(fmt.Sprintf("Pod: %s\n", frrk8sPod.Definition.Name))

		// Extract and write the prefixes (keys of the Routes map) and corresponding route info
		for prefix, routeInfos := range bgpRoutes.Routes {
			result.WriteString(fmt.Sprintf("  Prefix: %s\n", prefix))

			for _, routeInfo := range routeInfos {
				result.WriteString(fmt.Sprintf("    Route Info: Prefix: %s", routeInfo.Prefix))
			}
		}

		result.WriteString("\n") // Add an empty line between pods
	}

	return result.String(), nil
}

func parseBGPReceivedRoutes(jsonData string) (*BgpReceivedRoutes, error) {
	var bgpRoutes map[string][]RouteInfo // This matches the structure of your JSON

	// Parse the JSON data into the struct
	err := json.Unmarshal([]byte(jsonData), &bgpRoutes)
	if err != nil {
		return nil, fmt.Errorf("error parsing BGP received routes: %w", err)
	}

	// Create a new BgpReceivedRoutes struct and populate it with the parsed data
	parsedRoutes := &BgpReceivedRoutes{
		Routes: bgpRoutes, // The map directly holds prefixes as keys
	}

	// Print the parsed routes for debugging
	fmt.Printf("Parsed Routes: %+v\n", bgpRoutes)

	return parsedRoutes, nil
}

func parseBGPAdvertisedRoutes(jsonData string) (string, error) {
	var bgpRoutes BgpAdvertisedRoutes

	// Parse the JSON data into the struct
	err := json.Unmarshal([]byte(jsonData), &bgpRoutes)
	if err != nil {
		return "", fmt.Errorf("error parsing BGP advertised routes: %w", err)
	}

	// Format only the network values as a string
	var result strings.Builder
	for _, route := range bgpRoutes.AdvertisedRoutes {
		result.WriteString(fmt.Sprintf("%s\n", route.Network))
	}

	return result.String(), nil
}

// ResetBGPConnection restarts the TCP connection.
func ResetBGPConnection(frrPod *pod.Builder) error {
	glog.V(90).Infof("Resetting BGP session to all neighbors: %s", frrPod.Definition.Name)

	_, err := frrPod.ExecCommand(append(netparam.VtySh, "clear ip bgp *"))

	return err
}

// ValidateLocalPref verifies local pref from FRR is equal to configured Local Pref.
func ValidateLocalPref(frrPod *pod.Builder, localPref uint32, ipFamily string) error {
	bgpStatus, err := getBgpStatus(frrPod, fmt.Sprintf("show ip bgp %s json", ipFamily))

	if err != nil {
		return fmt.Errorf("failed to get BGP status %w", err)
	}

	for _, route := range bgpStatus.Routes {
		if route[0].LocalPref != localPref {
			return fmt.Errorf("expected localpref %d but received localPref: %d", localPref, route[0].LocalPref)
		}
	}

	return nil
}

// VerifyBGPNeighborTimer verifies that a given BGP neighbor has mentioned holdTimer and keepAliveTimer.
func VerifyBGPNeighborTimer(
	frrPod *pod.Builder,
	neighborIPAddress string,
	holdTimer, keepAliveTimer int,
) (bool, error) {
	var result map[string]bgpDescription

	glog.Infof("Verifying BGP Neighbor Timers for neighbor %s", neighborIPAddress)

	bgpStateOut, err := frrPod.ExecCommand(append(netparam.VtySh, "sh ip bgp neighbor json"))
	if err != nil {
		return false, err
	}

	err = json.Unmarshal(bgpStateOut.Bytes(), &result)
	if err != nil {
		return false, err
	}

	return result[neighborIPAddress].BGPHoldTime == holdTimer &&
		result[neighborIPAddress].BGPKeepAliveTime == keepAliveTimer, nil
}

// CheckFRRConfigLine checks for a configuration line.
func CheckFRRConfigLine(frrPod *pod.Builder, config string) (bool, error) {
	frrConf, err := runningConfig(frrPod)
	if err != nil {
		return false, err
	}

	frrConfList := strings.Split(frrConf, "!")
	for _, configLine := range frrConfList {
		if strings.Contains(configLine, config) {
			return true, nil
		}
	}

	return false, nil
}

// DefineBGPConfigWithUnnumbered defines BGP config file with static route and network.
func DefineBGPConfigWithUnnumbered(localBGPASN, remoteBGPASN int, interfaceName string,
	advertisedIPv4Routes, advertisedIPv6Routes []string, multiHop, bfd bool) string {
	bgpConfig := tsparams.FRRBaseConfig +
		fmt.Sprintf("interface %s", interfaceName) +
		"\n ipv6 nd ra-interval 10" +
		"\n no ipv6 nd suppress-ra" +
		"\n exit\n!\n" +
		fmt.Sprintf("router bgp %d\n", localBGPASN) +
		tsparams.FRRDefaultBGPPreConfig
	bgpConfig += "!\n  neighbor unnumbered peer-group\n"
	bgpConfig += fmt.Sprintf("  neighbor %s interface peer-group unnumbered\n", interfaceName)

	if bfd {
		bgpConfig += "  neighbor unnumbered bfd\n"
	}

	if multiHop {
		bgpConfig += "  neighbor unnumbered ebgp-multihop 2\n"
	}

	bgpConfig += fmt.Sprintf("  neighbor unnumbered remote-as %d\n", remoteBGPASN)
	bgpConfig += fmt.Sprintf("  neighbor unnumbered password %s\n", tsparams.BGPPassword)
	bgpConfig += "  neighbor unnumbered timers 30 90\n  neighbor unnumbered bfd\n"

	bgpConfig += "!\naddress-family ipv4 unicast\n"
	bgpConfig += "  neighbor unnumbered activate\n"

	for _, route := range advertisedIPv4Routes {
		bgpConfig += fmt.Sprintf("  network %s\n", route)
	}

	bgpConfig += "exit-address-family\n"

	// Add network commands only once for IPv6
	bgpConfig += "!\naddress-family ipv6 unicast\n"
	bgpConfig += "  neighbor unnumbered activate\n"

	for _, route := range advertisedIPv6Routes {
		bgpConfig += fmt.Sprintf("  network %s\n", route)
	}

	bgpConfig += "exit-address-family\n"
	bgpConfig += "!\nroute-map RMAP permit 10"
	bgpConfig += "\nset ipv6 next-hop prefer-global\n"
	bgpConfig += "!\nipv6 nht resolve-via-default\n"
	bgpConfig += "!\nline vty\n!\nend\n"

	return bgpConfig
}

// GetInterfaceStatus returns BGP interface details from an FRR pod.
func GetInterfaceStatus(frrPod *pod.Builder, interfaceName string, containerName []string) (*InterfaceDetails, error) {
	cName := "frr"
	if len(containerName) > 0 {
		cName = containerName[0]
	}

	glog.V(90).Infof("Getting interface status from container: %s of pod: %s", cName, frrPod.Definition.Name)

	cmd := fmt.Sprintf("show interface %s json", interfaceName)
	interfaceStateOut, err := frrPod.ExecCommand(append(netparam.VtySh, cmd), tsparams.FRRContainerName)

	if err != nil {
		glog.V(90).Infof(fmt.Sprintf("Failed to execute command show interface %s json", interfaceName))

		return nil, err
	}

	// JSON is a map[string]InterfaceDetails
	ifaceMap := make(map[string]InterfaceDetails)
	err = json.Unmarshal(interfaceStateOut.Bytes(), &ifaceMap)

	if err != nil {
		glog.V(90).Infof("Failed to unmarshal JSON: %s", interfaceStateOut.String())

		return nil, err
	}

	iface, ok := ifaceMap[interfaceName]
	if !ok {
		glog.V(90).Infof("Failed to find interface: %s", interfaceName)

		return nil, err
	}

	return &iface, nil
}
