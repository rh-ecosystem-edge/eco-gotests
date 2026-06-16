package rdscorecommon

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"time"

	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmc"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/service"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/statefulset"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// AddrInfo represents the address information for a network interface.
type AddrInfo struct {
	Family            string `json:"family,omitempty"`
	Local             string `json:"local,omitempty"`
	Prefixlen         int    `json:"prefixlen,omitempty"`
	Broadcast         string `json:"broadcast,omitempty"`
	Scope             string `json:"scope,omitempty"`
	Label             string `json:"label,omitempty"`
	ValidLifeTime     uint32 `json:"valid_life_time,omitempty"`
	PreferredLifeTime uint32 `json:"preferred_life_time,omitempty"`
}

// NetworkInterface represents the overall structure of a single network interface.
type NetworkInterface struct {
	Ifindex     int        `json:"ifindex,omitempty"`
	LinkIndex   int        `json:"link_index,omitempty"`
	Ifname      string     `json:"ifname,omitempty"`
	Flags       []string   `json:"flags,omitempty"`
	MTU         int        `json:"mtu,omitempty"`
	Qdisc       string     `json:"qdisc,omitempty"`
	Operstate   string     `json:"operstate,omitempty"`
	Group       string     `json:"group,omitempty"`
	Txqlen      int        `json:"txqlen,omitempty"`
	LinkType    string     `json:"link_type,omitempty"`
	Address     string     `json:"address,omitempty"`
	Broadcast   string     `json:"broadcast,omitempty"`
	LinkNetnsid int        `json:"link_netnsid,omitempty"`
	AddrInfo    []AddrInfo `json:"addr_info,omitempty"`
}

const (
	// WhereaboutsReconcilerSchedule is the schedule for the whereabouts reconciler.
	WhereaboutsReconcilerSchedule = "*/3 * * * *"
	// WhereaboutsReconcilerKey is the key for the whereabouts reconciler.
	WhereaboutsReconcilerKey = "reconciler_cron_expression"
	// WhereaboutsReconcilerNamespace is the namespace for the whereabouts reconciler.
	WhereaboutsReconcilerNamespace = "openshift-multus"
	// WhereaboutsReconcilerCMName is the name of the whereabouts reconciler configmap.
	WhereaboutsReconcilerCMName = "whereabouts-config"

	myHeadlessSvcOne            = "rds-st-one-headless-1"
	myStatefulsetOne            = "rds-st-one"
	myStatefulsetOneLabel       = "app=rds-st-one"
	myStatefulsetOneReplicas    = 2
	myStatefulsetOneSA          = "rds-st-one-sa"
	myStatefulsetOneRBACRole    = "system:openshift:scc:nonroot-v2"
	myStatefulsetOneTopologyKey = "kubernetes.io/hostname"
	// interfaceName is the name of the network interface inside the pod.
	interfaceName = "net1"
	// ipv6Family is the IPv6 address family identifier.
	ipv6Family = "inet6"

	myHeadlessSvcTwo            = "rds-st-two-headless-2"
	myStatefulsetTwo            = "rds-st-two"
	myStatefulsetTwoLabel       = "app=rds-st-two"
	myStatefulsetTwoReplicas    = 2
	myStatefulsetTwoSA          = "rds-st-two-sa"
	myStatefulsetTwoRBACRole    = "system:openshift:scc:nonroot-v2"
	myStatefulsetTwoTopologyKey = "kubernetes.io/hostname"
)

// StatefulsetConfig holds configuration for creating whereabouts statefulsets.
type StatefulsetConfig struct {
	Name            string
	Label           string
	ServiceName     string
	Port            string
	Image           string
	Command         []string
	NAD             string
	ServiceAccount  string
	RBACRole        string
	TopologyKey     string
	Replicas        int32
	UseAntiAffinity bool
	Description     string
}

// Pre-defined configurations.
var (
	SameNodeConfig = StatefulsetConfig{
		Name:            myStatefulsetOne,
		Label:           myStatefulsetOneLabel,
		ServiceName:     myHeadlessSvcOne,
		Port:            "",  // Will be set from RDSCoreConfig
		Image:           "",  // Will be set from RDSCoreConfig
		Command:         nil, // Will be set from RDSCoreConfig
		NAD:             "",  // Will be set from RDSCoreConfig
		ServiceAccount:  myStatefulsetOneSA,
		RBACRole:        myStatefulsetOneRBACRole,
		TopologyKey:     myStatefulsetOneTopologyKey,
		Replicas:        myStatefulsetOneReplicas,
		UseAntiAffinity: false,
		Description:     "pods running on the same node",
	}

	DifferentNodeConfig = StatefulsetConfig{
		Name:            myStatefulsetTwo,
		Label:           myStatefulsetTwoLabel,
		ServiceName:     myHeadlessSvcTwo,
		Port:            "",  // Will be set from RDSCoreConfig
		Image:           "",  // Will be set from RDSCoreConfig
		Command:         nil, // Will be set from RDSCoreConfig
		NAD:             "",  // Will be set from RDSCoreConfig
		ServiceAccount:  myStatefulsetTwoSA,
		RBACRole:        myStatefulsetTwoRBACRole,
		TopologyKey:     myStatefulsetTwoTopologyKey,
		Replicas:        myStatefulsetTwoReplicas,
		UseAntiAffinity: true,
		Description:     "pods running on different nodes",
	}
)

// cleanupStatefulset removes a statefulset and waits for its pods to be deleted.
func cleanupStatefulset(stName, namespace, stLabel string) {
	By(fmt.Sprintf("Checking that statefulset %q doesn't exist in %q namespace",
		stName, namespace))

	var ctx SpecContext

	stOne, err := statefulset.Pull(APIClient, stName, namespace)
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to get statefulset %q in %q namespace: %s",
			stName, namespace, err)
	} else {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deleting statefulset %q in %q namespace",
			stName, namespace)

		delError := stOne.Delete()

		Expect(delError).ToNot(HaveOccurred(), "Failed to delete statefulset %q in %q namespace",
			stName, namespace)

		// wait for pods to be deleted
		By(fmt.Sprintf("Waiting for pods from %q statefulset in %q namespace to be deleted",
			stName, namespace))

		Eventually(func() bool {
			pods, err := pod.List(APIClient, namespace, metav1.ListOptions{
				LabelSelector: stLabel,
			})
			if err != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to list pods from %q statefulset in %q namespace: %s",
					stName, namespace, err)

				return false
			}

			return len(pods) == 0
		}).WithContext(ctx).WithPolling(15*time.Second).WithTimeout(5*time.Minute).Should(BeTrue(),
			"Pods from %q statefulset in %q namespace are not deleted", stName, namespace)
	}
}

// createStatefulsetAndWaitReplicasReady creates a statefulset and waits for all replicas to become ready.
func createStatefulsetAndWaitReplicasReady(stName, namespace string, stBuilder *statefulset.Builder) {
	By(fmt.Sprintf("Creating statefulset %q in %q namespace", stName, namespace))

	var ctx SpecContext

	Eventually(func() bool {
		_, err := stBuilder.Create()

		return err == nil
	}).WithContext(ctx).WithPolling(15*time.Second).WithTimeout(5*time.Minute).Should(BeTrue(),
		"Failed to create statefulset %q in %q namespace", stName, namespace)

	By(fmt.Sprintf("Waiting for statefulset %q in %q namespace to be ready",
		stName, namespace))

	Eventually(func() bool {
		exists := stBuilder.Exists()

		if exists {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Statefulset ReadyReplicas: %d",
				stBuilder.Object.Status.ReadyReplicas)

			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Statefulset Spec.Replicas: %d",
				*stBuilder.Definition.Spec.Replicas)

			if stBuilder.Definition.Spec.Replicas != nil {
				return stBuilder.Object.Status.ReadyReplicas == *stBuilder.Definition.Spec.Replicas
			}

			return stBuilder.Object.Status.ReadyReplicas != 0
		}

		return false
	}).WithContext(ctx).WithPolling(15*time.Second).WithTimeout(5*time.Minute).Should(BeTrue(),
		"Statefulset %q in %q namespace is not ready", stName, namespace)
}

// determineIPFamilyPolicy fetches the NAD and inspects the IPAM range fields to determine
// whether to use RequireDualStack, or SingleStack with IPv4 or IPv6.
func determineIPFamilyPolicy(nadName, namespace string) ([]corev1.IPFamily, corev1.IPFamilyPolicy) {
	nadBuilder, err := nad.Pull(APIClient, nadName, namespace)

	Expect(err).ToNot(HaveOccurred(),
		"Failed to get NAD %q in %q namespace", nadName, namespace)
	Expect(nadBuilder.Definition.Spec.Config).ToNot(BeEmpty(),
		"NAD %q has empty spec.config field", nadName)

	config := nadBuilder.Definition.Spec.Config

	ranges := extractIPAMRanges(nadName, config)
	Expect(ranges).ToNot(BeEmpty(),
		"NAD %q has no IPAM range entries in spec.config", nadName)

	hasIPv4, hasIPv6 := detectIPFamiliesFromRanges(ranges)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("NAD %q IP family detection: hasIPv4=%v, hasIPv6=%v (ranges: %v)",
		nadName, hasIPv4, hasIPv6, ranges)

	switch {
	case hasIPv4 && hasIPv6:
		return []corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol}, corev1.IPFamilyPolicyRequireDualStack
	case hasIPv6:
		return []corev1.IPFamily{corev1.IPv6Protocol}, corev1.IPFamilyPolicySingleStack
	case hasIPv4:
		return []corev1.IPFamily{corev1.IPv4Protocol}, corev1.IPFamilyPolicySingleStack
	default:
		Fail(fmt.Sprintf("NAD %q IPAM ranges contain no detectable IPv4 or IPv6 CIDR: %v", nadName, ranges))

		return nil, "" // Unreachable in Ginkgo: Fail panics.
	}
}

// extractIPAMRanges returns IPAM range strings from NAD spec.config
// (from top-level ipam and plugins[*].ipam: range, subnet, ipRanges, range_start/range_end if no range).
func extractIPAMRanges(nadName, config string) []string {
	var parsed map[string]interface{}

	err := json.Unmarshal([]byte(config), &parsed)
	Expect(err).ToNot(HaveOccurred(),
		"Failed to parse NAD %q spec.config JSON", nadName)

	var ranges []string

	ranges = append(ranges, extractRangesFromIPAM(parsed)...)

	if plugins, ok := parsed["plugins"].([]interface{}); ok {
		for _, p := range plugins {
			if plugin, ok := p.(map[string]interface{}); ok {
				ranges = append(ranges, extractRangesFromIPAM(plugin)...)
			}
		}
	}

	return ranges
}

// extractRangesFromIPAM extracts range strings from an object's ipam field.
//
//nolint:funlen
func extractRangesFromIPAM(obj map[string]interface{}) []string {
	ipamRaw, hasIPAM := obj["ipam"]
	if !hasIPAM {
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"extractRangesFromIPAM: ipam field missing, skipping (object keys: %v)", keys)

		return nil
	}

	ipam, ok := ipamRaw.(map[string]interface{})
	if !ok {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"extractRangesFromIPAM: ipam has unexpected type %T (want map[string]interface{}), value=%v",
			ipamRaw, ipamRaw)

		return nil
	}

	var ranges []string

	// Convert optional range_start/range_end into a single range string.
	appendRangeFromStartEnd := func(rangeObj map[string]interface{}) (string, bool) {
		rangeStart, hasStart := rangeObj["range_start"].(string)
		if !hasStart {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"appendRangeFromStartEnd: range_start field missing or not a string, skipping")

			return "", false
		}

		rangeStart = strings.TrimSpace(rangeStart)
		if rangeStart == "" {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"appendRangeFromStartEnd: range_start is empty after trimming, skipping")

			return "", false
		}

		rangeEnd, hasEnd := rangeObj["range_end"].(string)
		rangeEnd = strings.TrimSpace(rangeEnd)

		if hasEnd && rangeEnd != "" {
			result := fmt.Sprintf("%s-%s", rangeStart, rangeEnd)
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"appendRangeFromStartEnd: built range from start/end: %q", result)

			return result, true
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"appendRangeFromStartEnd: no valid range_end found, returning start only: %q", rangeStart)

		return rangeStart, true
	}

	if rangeStr, ok := ipam["range"].(string); ok {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"extractRangesFromIPAM: found top-level range field: %q", rangeStr)
		ranges = append(ranges, rangeStr)
	} else if rangeFromStartEnd, found := appendRangeFromStartEnd(ipam); found {
		ranges = append(ranges, rangeFromStartEnd)
	}

	if subnet, ok := ipam["subnet"].(string); ok {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"extractRangesFromIPAM: found subnet field: %q", subnet)
		ranges = append(ranges, subnet)
	}

	if ipRanges, ok := ipam["ipRanges"].([]interface{}); ok {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"extractRangesFromIPAM: found ipRanges array with %d entries", len(ipRanges))

		for _, entry := range ipRanges {
			if rangeMap, ok := entry.(map[string]interface{}); ok {
				if rangeStr, ok := rangeMap["range"].(string); ok {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
						"extractRangesFromIPAM: ipRanges entry range field: %q", rangeStr)
					ranges = append(ranges, rangeStr)
				} else if rangeFromStartEnd, found := appendRangeFromStartEnd(rangeMap); found {
					ranges = append(ranges, rangeFromStartEnd)
				}
			}
		}
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"extractRangesFromIPAM: returning %d range(s): %v", len(ranges), ranges)

	return ranges
}

// detectIPFamiliesFromRanges inspects a list of CIDR range strings and returns
// whether IPv4 and/or IPv6 ranges are present.
func detectIPFamiliesFromRanges(ranges []string) (hasIPv4, hasIPv6 bool) {
	markFamily := func(ipAddr net.IP) {
		if len(ipAddr) == 0 {
			return
		}

		if ipAddr.To4() != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"detectIPFamiliesFromRanges: detected IPv4 address: %s", ipAddr)

			hasIPv4 = true
		} else {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"detectIPFamiliesFromRanges: detected IPv6 address: %s", ipAddr)

			hasIPv6 = true
		}
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"detectIPFamiliesFromRanges: inspecting %d range(s): %v", len(ranges), ranges)

	for _, rangeStr := range ranges {
		rangeValue := strings.TrimSpace(rangeStr)

		parsedIP, _, err := net.ParseCIDR(rangeValue)
		if err == nil {
			markFamily(parsedIP)

			continue
		}

		// Fallback for Whereabouts range_start/range_end values emitted as "start-end"
		// and for single IP values without a CIDR prefix.
		ipCandidate := rangeValue
		if parts := strings.SplitN(rangeValue, "-", 2); len(parts) == 2 {
			ipCandidate = strings.TrimSpace(parts[0])
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"detectIPFamiliesFromRanges: %q is not a CIDR, extracted start IP %q from start-end range", rangeValue, ipCandidate)
		}

		if parsedFallbackIP := net.ParseIP(ipCandidate); parsedFallbackIP != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"detectIPFamiliesFromRanges: parsed %q as plain IP: %s", ipCandidate, parsedFallbackIP)
			markFamily(parsedFallbackIP)

			continue
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Skipping invalid IPAM range %q: %v", rangeStr, err)
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"detectIPFamiliesFromRanges: result — hasIPv4=%v, hasIPv6=%v", hasIPv4, hasIPv6)

	return hasIPv4, hasIPv6
}

// setupHeadlessService creates a headless service with ipFamilyPolicy determined from the NAD configuration.
func setupHeadlessService(svcName, namespace, svcLabel, svcPort, nadName string) {
	By(fmt.Sprintf("Checking that service %q doesn't exist in %q namespace",
		svcName, namespace))

	var ctx SpecContext

	svcOne, err := service.Pull(APIClient, svcName, namespace)
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to get service %q in %q namespace: %s",
			svcName, namespace, err)
	} else {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deleting service %q in %q namespace",
			svcName, namespace)

		delError := svcOne.Delete()

		Expect(delError).ToNot(HaveOccurred(), "Failed to delete service %q in %q namespace",
			svcName, namespace)
	}

	// create a headless service
	By(fmt.Sprintf("Creating headless service %q in %q namespace",
		svcName, namespace))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Defining headless service selector")

	svcLabelsMap := parseLabelsMap(svcLabel)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Defining headless service port")

	parsedPort, err := strconv.Atoi(svcPort)

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to parse port number: %v", svcPort))

	svcPortCr := corev1.ServicePort{
		Port:     int32(parsedPort),
		Protocol: corev1.ProtocolTCP,
	}

	svcOne = defineHeadlessService(svcName, namespace, svcLabelsMap, svcPortCr)

	ipFamilies, ipFamilyPolicy := determineIPFamilyPolicy(nadName, namespace)

	By(fmt.Sprintf("Setting ipFamilyPolicy to %q for NAD %q", ipFamilyPolicy, nadName))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("NAD %q: ipFamilies=%v, ipFamilyPolicy=%s",
		nadName, ipFamilies, ipFamilyPolicy)

	svcOne = svcOne.WithIPFamily(ipFamilies, ipFamilyPolicy)

	By(fmt.Sprintf("Creating headless service %q in %q namespace",
		svcName, namespace))

	Eventually(func() error {
		svcOne, err = svcOne.Create()
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to create headless service %q in %q namespace: %s",
				svcName, namespace, err)
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Headless service %q in %q namespace created",
			svcName, namespace)

		return err
	}).WithContext(ctx).WithPolling(15*time.Second).WithTimeout(1*time.Minute).Should(Succeed(),
		"Failed to create headless service %q in %q namespace", svcName, namespace)
}

// verifyInterPodCommunication validates network connectivity between all active pods via their whereabouts IPs.
func verifyInterPodCommunication(
	activePods []*pod.Builder,
	podWhereaboutsIPs map[string][]NetworkInterface,
	podsMapping map[string]string,
	parsedPort int) {
	By("Verifying inter-pod communication")

	var ctx SpecContext

	for podIndex, _pod := range activePods {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Running from %q to %q",
			_pod.Object.Name, podsMapping[_pod.Object.Name])

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod %q IP addresses: %+v",
			podsMapping[_pod.Object.Name], podWhereaboutsIPs[podsMapping[_pod.Object.Name]])

		for _, dstAddr := range podWhereaboutsIPs[podsMapping[_pod.Object.Name]][0].AddrInfo {
			if dstAddr.Family == ipv6Family && dstAddr.Scope == "link" {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Skipping link-local address %q", dstAddr.Local)

				continue
			}

			randomNumber := rand.Intn(3000)

			msgOne := fmt.Sprintf("Hello from %q to %q with random number %d",
				_pod.Object.Name, podsMapping[_pod.Object.Name], randomNumber)

			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Sending data from %q to %q",
				_pod.Object.Name, dstAddr.Local)

			targetAddr := fmt.Sprintf("%s %d", dstAddr.Local, parsedPort)

			sendDataOneCmd := []string{"/bin/bash", "-c",
				fmt.Sprintf("echo '%s' | nc %s", msgOne, targetAddr)}

			var (
				podOneResult bytes.Buffer
				err          error
			)

			timeStart := time.Now()

			Eventually(func() bool {
				podOneResult.Reset()

				podOneResult, err = _pod.ExecCommand(sendDataOneCmd, _pod.Definition.Spec.Containers[0].Name)

				return err == nil
			}).WithContext(ctx).WithPolling(10*time.Second).WithTimeout(1*time.Minute).Should(BeTrue(),
				"Failed to send data from pod %q to %q", _pod.Object.Name, targetAddr)

			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod %q result: %s", _pod.Object.Name, podOneResult.String())

			targetPod := activePods[len(activePods)-(podIndex+1)]

			By(fmt.Sprintf("Verifying message in pod %q", targetPod.Object.Name))

			verifyMsgInPodLogs(targetPod, msgOne, targetPod.Definition.Spec.Containers[0].Name, timeStart)
		}
	}
}

// checkPodIPv6Ready verifies that an IPv6 address is not in tentative or dadfailed state.
// Returns true if the address is ready for use, false if it's still in DAD process or failed.
func checkPodIPv6Ready(podObj *pod.Builder, interfaceName, ipv6Addr string) (bool, error) {
	if ipv6Addr == "" {
		return true, nil
	}

	// Use text format to get full interface details including tentative state
	cmdGetIPAddrText := []string{"/bin/sh", "-c", fmt.Sprintf("ip addr show dev %s", interfaceName)}

	output, err := podObj.ExecCommand(cmdGetIPAddrText, podObj.Definition.Spec.Containers[0].Name)
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to execute ip addr command in pod %q: %v",
			podObj.Object.Name, err)

		return false, err
	}

	scanner := bufio.NewScanner(strings.NewReader(output.String()))

	for scanner.Scan() {
		line := scanner.Text()

		// Look for lines containing the IPv6 address
		if strings.Contains(line, ipv6Addr) && strings.Contains(line, ipv6Family) {
			// Check if DAD failed (permanent failure - must check first)
			if strings.Contains(line, "dadfailed") {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("IPv6 address %s DAD failed for pod %q: %s",
					ipv6Addr, podObj.Object.Name, strings.TrimSpace(line))

				return false, fmt.Errorf("IPv6 DAD failed for address %s in pod %s", ipv6Addr, podObj.Object.Name)
			}

			// Check if the address is in tentative state (DAD in progress)
			if strings.Contains(line, "tentative") {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
					"IPv6 address %s is in tentative state (DAD in progress) for pod %q: %s",
					ipv6Addr, podObj.Object.Name, strings.TrimSpace(line))

				return false, nil
			}

			// Address found and not tentative - ready to use
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("IPv6 address %s is ready (not tentative) for pod %q",
				ipv6Addr, podObj.Object.Name)

			return true, nil
		}
	}

	// Check for scanning errors
	if err := scanner.Err(); err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Error scanning output for IPv6 address %s in pod %q: %v",
			ipv6Addr, podObj.Object.Name, err)

		return false, fmt.Errorf("error scanning output for IPv6 address %s in pod %s: %w",
			ipv6Addr, podObj.Object.Name, err)
	}

	// If we reach here, the address was not found in the output
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("IPv6 address %s not found in output for pod %q",
		ipv6Addr, podObj.Object.Name)

	return false, nil
}

// getPodWhereaboutsIPs gets the IP addresses for the given pod.
//
//nolint:gocognit
func getPodWhereaboutsIPs(activePods []*pod.Builder, interfaceName string) map[string][]NetworkInterface {
	podWhereaboutsIPs := make(map[string][]NetworkInterface)

	var ctx SpecContext

	cmdGetIPAddr := []string{"/bin/sh", "-c", fmt.Sprintf("ip -j addr show dev %s", interfaceName)}

	for _, _pod := range activePods {
		var networkInterface []NetworkInterface

		Eventually(func() error {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Executing command %q within pod %q in %q namespace",
				cmdGetIPAddr, _pod.Object.Name, _pod.Object.Namespace)

			addrBuffInfo, err := _pod.ExecCommand(cmdGetIPAddr, _pod.Definition.Spec.Containers[0].Name)
			if err != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to execute command within pod %q in %q namespace: %s",
					_pod.Object.Name, _pod.Object.Namespace, err)

				return fmt.Errorf("failed to execute command in pod: %w", err)
			}

			if addrBuffInfo.Len() == 0 {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Empty output from command within pod %q in %q namespace",
					_pod.Object.Name, _pod.Object.Namespace)

				return fmt.Errorf("empty output from command")
			}

			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Unmarshalling IP addresses")

			err = json.Unmarshal(addrBuffInfo.Bytes(), &networkInterface)
			if err != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to unmarshal IP addresses for pod %q in %q namespace: %s",
					_pod.Object.Name, _pod.Object.Namespace, err)

				return fmt.Errorf("failed to unmarshal IP addresses: %w", err)
			}

			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("IP addresses: %+v", networkInterface)

			// Check IPv6 addresses for tentative state (DAD must be complete)
			for _, addr := range networkInterface[0].AddrInfo {
				if addr.Family == ipv6Family && addr.Scope == "global" {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
						"Checking IPv6 address %s readiness for pod %q", addr.Local, _pod.Object.Name)

					ipv6Ready, err := checkPodIPv6Ready(_pod, interfaceName, addr.Local)
					if err != nil {
						// Check if this is a permanent DAD failure
						if strings.Contains(err.Error(), "IPv6 DAD failed") {
							klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
								"IPv6 address %s DAD failed for pod %q in %q namespace: %v",
								addr.Local, _pod.Object.Name, _pod.Object.Namespace, err)

							return StopTrying(fmt.Sprintf(
								"IPv6 DAD failed for address %s in pod %s/%s",
								addr.Local, _pod.Object.Namespace, _pod.Object.Name)).Wrap(err)
						}

						// For other errors (exec failures, scanner errors), continue retrying
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
							"Temporary error checking IPv6 address %s for pod %q in %q namespace: %v, will retry",
							addr.Local, _pod.Object.Name, _pod.Object.Namespace, err)

						return fmt.Errorf("failed to check IPv6 address: %w", err)
					}

					if !ipv6Ready {
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
							"IPv6 address %s not ready yet (tentative state) for pod %q in %q namespace, retrying...",
							addr.Local, _pod.Object.Name, _pod.Object.Namespace)

						return fmt.Errorf("IPv6 address not ready (tentative state)")
					}

					klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
						"IPv6 address %s is ready and not in tentative state for pod %q",
						addr.Local, _pod.Object.Name)
				}
			}

			podWhereaboutsIPs[_pod.Object.Name] = networkInterface

			return nil
		}).WithContext(ctx).WithPolling(15*time.Second).WithTimeout(3*time.Minute).Should(Succeed(),
			"Failed to get IP addresses for pod %q in %q namespace", _pod.Object.Name, _pod.Object.Namespace)
	}

	return podWhereaboutsIPs
}

// getActivePods gets the active pods with the given label and namespace.
//
//nolint:gocognit
func getActivePods(podLabel, namespace string) []*pod.Builder {
	By("Checking if pods are running")

	pods := findPodWithSelector(namespace, podLabel)

	Expect(pods).ToNot(BeEmpty(), "No pods found with selector %q in %q namespace",
		podLabel, namespace)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Found %d pods with selector %q in %q namespace",
		len(pods), podLabel, namespace)

	var activePods []*pod.Builder

	for _, _pod := range pods {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod %q in %q namespace is in phase %q",
			_pod.Object.Name, _pod.Object.Namespace, _pod.Object.Status.Phase)

		// Check if pod is marked for deletion first
		if _pod.Object.DeletionTimestamp != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod %q is marked for deletion, skipping", _pod.Object.Name)

			continue
		}

		switch _pod.Object.Status.Phase {
		case corev1.PodRunning:
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod %q is active(running and not marked for deletion)",
				_pod.Object.Name)

			activePods = append(activePods, _pod)
		case corev1.PodPending, corev1.PodSucceeded, corev1.PodFailed, corev1.PodUnknown:
			// Dump detailed status for non-running pods to help diagnose issues
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod %q is not running, dumping status details:",
				_pod.Object.Name)

			// Log pod conditions
			for _, condition := range _pod.Object.Status.Conditions {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Condition: Type=%s, Status=%s, Reason=%s, Message=%s",
					condition.Type, condition.Status, condition.Reason, condition.Message)
			}

			// Log container statuses
			for _, containerStatus := range _pod.Object.Status.ContainerStatuses {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  ContainerStatus: Name=%s, Ready=%t, RestartCount=%d",
					containerStatus.Name, containerStatus.Ready, containerStatus.RestartCount)

				if containerStatus.State.Waiting != nil {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    Waiting: Reason=%s, Message=%s",
						containerStatus.State.Waiting.Reason, containerStatus.State.Waiting.Message)
				}

				if containerStatus.State.Terminated != nil {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    Terminated: Reason=%s, Message=%s, ExitCode=%d",
						containerStatus.State.Terminated.Reason, containerStatus.State.Terminated.Message,
						containerStatus.State.Terminated.ExitCode)
				}
			}

			// Log init container statuses (often the cause of Pending)
			for _, initContainerStatus := range _pod.Object.Status.InitContainerStatuses {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  InitContainerStatus: Name=%s, Ready=%t, RestartCount=%d",
					initContainerStatus.Name, initContainerStatus.Ready, initContainerStatus.RestartCount)

				if initContainerStatus.State.Waiting != nil {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    Waiting: Reason=%s, Message=%s",
						initContainerStatus.State.Waiting.Reason, initContainerStatus.State.Waiting.Message)
				}
			}

			// Log scheduling info if available
			if _pod.Object.Status.NominatedNodeName != "" {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  NominatedNodeName: %s", _pod.Object.Status.NominatedNodeName)
			}

			if _pod.Object.Spec.NodeName == "" {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Pod has not been scheduled to a node yet")
			} else {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Pod scheduled to node: %s", _pod.Object.Spec.NodeName)
			}
		}
	}

	return activePods
}

// waitForWhereaboutsNetworkReady waits for Whereabouts network interfaces to be configured
// with IP addresses on all pods matching the given label selector.
// Simplified version that directly calls getPodWhereaboutsIPs (which has its own retry logic).
func waitForWhereaboutsNetworkReady(
	stLabel, namespace, interfaceName string,
	expectedPodCount int) error {
	By("Waiting for Whereabouts network interfaces to be ready")

	activePods := getActivePods(stLabel, namespace)

	if len(activePods) != expectedPodCount {
		return fmt.Errorf("expected %d pods, found %d", expectedPodCount, len(activePods))
	}

	// getPodWhereaboutsIPs already has Eventually with 3min timeout and IPv6 DAD checks
	// No need to wrap it in another Eventually block
	podWhereaboutsIPs := getPodWhereaboutsIPs(activePods, interfaceName)

	// Verify all pods have IP addresses assigned
	for _, _pod := range activePods {
		podIPs, exists := podWhereaboutsIPs[_pod.Object.Name]
		if !exists || len(podIPs) == 0 {
			return fmt.Errorf("pod %q has no Whereabouts IP addresses on interface %q",
				_pod.Object.Name, interfaceName)
		}

		if len(podIPs[0].AddrInfo) == 0 {
			return fmt.Errorf("pod %q has no IP addresses on interface %q",
				_pod.Object.Name, interfaceName)
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Pod %q has %d IP addresses on interface %q",
			_pod.Object.Name, len(podIPs[0].AddrInfo), interfaceName)
	}

	return nil
}

// logPodNetworkStatus logs detailed network status for pods matching the label selector.
// Includes pod phase, node, ready conditions, and Whereabouts IP addresses.
func logPodNetworkStatus(stLabel, namespace, interfaceName string) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("=== Pod Network Status ===")

	activePods := getActivePods(stLabel, namespace)

	for _, _pod := range activePods {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod: %s/%s",
			_pod.Object.Namespace, _pod.Object.Name)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Phase: %s", _pod.Object.Status.Phase)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Node: %s", _pod.Object.Spec.NodeName)

		// Log ready conditions
		for _, condition := range _pod.Object.Status.Conditions {
			if condition.Type == "Ready" {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Ready: %s (Reason: %s)",
					condition.Status, condition.Reason)

				break
			}
		}

		// Log Whereabouts IPs (if available)
		podWhereaboutsIPs := getPodWhereaboutsIPs([]*pod.Builder{_pod}, interfaceName)
		if podIPs, exists := podWhereaboutsIPs[_pod.Object.Name]; exists && len(podIPs) > 0 {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Whereabouts Interface: %s", interfaceName)

			for _, addr := range podIPs[0].AddrInfo {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    %s: %s/%d",
					addr.Family, addr.Local, addr.Prefixlen)
			}
		} else {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Whereabouts Interface: NOT CONFIGURED")
		}
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("=========================")
}

// verifyPodConnectivityWithRetry verifies inter-pod connectivity with retry logic.
// Returns error on failure, suitable for use in Eventually() blocks.
// This is an error-returning version of VerifyPodConnectivity logic.
func verifyPodConnectivityWithRetry(
	stLabel, namespace, interfaceName string,
	targetPort int,
	expectedReplicas int) error {
	// Get active pods
	activePods := getActivePods(stLabel, namespace)

	if len(activePods) != expectedReplicas {
		return fmt.Errorf("expected %d active pods, found %d",
			expectedReplicas, len(activePods))
	}

	// Get pod IP addresses
	podWhereaboutsIPs := getPodWhereaboutsIPs(activePods, interfaceName)
	if len(podWhereaboutsIPs) == 0 {
		return fmt.Errorf("no Whereabouts IP addresses found for pods")
	}

	// Set up pod mapping for connectivity test
	podOneName := activePods[0].Object.Name
	podTwoName := activePods[len(activePods)-1].Object.Name

	podsMapping := make(map[string]string)
	podsMapping[podOneName] = podTwoName
	podsMapping[podTwoName] = podOneName

	// Verify inter-pod communication with error returns
	return checkInterPodCommunicationWithError(activePods, podWhereaboutsIPs, podsMapping, targetPort)
}

// checkInterPodCommunicationWithError is an error-returning variant of verifyInterPodCommunication.
// Sends netcat messages between pods and verifies receipt in logs, returning errors instead of using Expect().
//
//nolint:funlen,gocognit
func checkInterPodCommunicationWithError(
	activePods []*pod.Builder,
	podWhereaboutsIPs map[string][]NetworkInterface,
	podsMapping map[string]string,
	parsedPort int) error {
	for podIndex, _pod := range activePods {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Checking connectivity from %q to %q",
			_pod.Object.Name, podsMapping[_pod.Object.Name])

		for _, dstAddr := range podWhereaboutsIPs[podsMapping[_pod.Object.Name]][0].AddrInfo {
			if dstAddr.Family == ipv6Family && dstAddr.Scope == "link" {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Skipping link-local address %q", dstAddr.Local)

				continue
			}

			randomNumber := rand.Intn(3000)
			msgOne := fmt.Sprintf("Hello from %q to %q with random number %d",
				_pod.Object.Name, podsMapping[_pod.Object.Name], randomNumber)

			targetAddr := fmt.Sprintf("%s %d", dstAddr.Local, parsedPort)
			sendDataOneCmd := []string{"/bin/bash", "-c",
				fmt.Sprintf("echo '%s' | nc %s", msgOne, targetAddr)}

			var (
				podOneResult bytes.Buffer
				execErr      error
			)

			timeStart := time.Now()

			// Send data with retry logic - use polling approach to avoid hard-fail assertions
			sendSuccess := false
			sendStartTime := time.Now()

			for time.Since(sendStartTime) < 1*time.Minute {
				podOneResult.Reset()

				podOneResult, execErr = _pod.ExecCommand(sendDataOneCmd, _pod.Definition.Spec.Containers[0].Name)
				if execErr == nil {
					sendSuccess = true

					break
				}

				time.Sleep(10 * time.Second)
			}

			if !sendSuccess {
				return fmt.Errorf("failed to send data from pod %q to %q after 1m: %w",
					_pod.Object.Name, targetAddr, execErr)
			}

			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod %q send result: %s",
				_pod.Object.Name, podOneResult.String())

			targetPod := activePods[len(activePods)-(podIndex+1)]

			// Verify message in logs - use polling approach to avoid hard-fail assertions
			var podLog string

			var logErr error

			logSuccess := false
			logStartTime := time.Now()

			for time.Since(logStartTime) < 1*time.Minute {
				logStartTimestamp := time.Since(timeStart)
				if logStartTimestamp.Abs().Seconds() < 1 {
					logStartTimestamp, _ = time.ParseDuration("1s")
				}

				podLog, logErr = targetPod.GetLog(logStartTimestamp, targetPod.Definition.Spec.Containers[0].Name)
				if logErr == nil {
					logSuccess = true

					break
				}

				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to get logs from pod %q: %v",
					targetPod.Definition.Name, logErr)

				time.Sleep(5 * time.Second)
			}

			if !logSuccess {
				return fmt.Errorf("failed to get logs from pod %q after 1m: %w", targetPod.Definition.Name, logErr)
			}

			// Check if message appears in logs
			if !strings.Contains(podLog, msgOne) {
				return fmt.Errorf("message %q not found in pod %q logs", msgOne, targetPod.Definition.Name)
			}

			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Successfully verified message in pod %q logs",
				targetPod.Definition.Name)
		}
	}

	return nil
}

// ensurePodConnectivityAfterPodTermination verifies inter-pod connectivity is restored after terminating a pod.
func ensurePodConnectivityAfterPodTermination(stLabel, namespace, targetPort string, stReplicas int) {
	By("Getting list of active pods")

	activePods := getActivePods(stLabel, namespace)

	Expect(len(activePods)).To(Equal(stReplicas),
		"Number of active pods is not equal to number of replicas")

	By("Generating random pod index")

	randomPodIndex := rand.Intn(len(activePods))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Random pod index: %d pod name: %q",
		randomPodIndex, activePods[randomPodIndex].Object.Name)

	terminatedPod := activePods[randomPodIndex]

	terminatedPodUID := terminatedPod.Object.UID

	By(fmt.Sprintf("Terminating pod %q in %q namespace with UUID: %q",
		terminatedPod.Object.Name, terminatedPod.Object.Namespace, terminatedPod.Object.UID))

	terminatedPod, err := terminatedPod.Delete()

	Expect(err).ToNot(HaveOccurred(), "Failed to delete pod %q in %q namespace",
		terminatedPod.Definition.Name, terminatedPod.Definition.Namespace)

	By("Waiting for new pod to be created")

	var ctx SpecContext

	Eventually(func() bool {
		activePods := getActivePods(stLabel, namespace)

		for _, _pod := range activePods {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Found pod %q in %q namespace with UUID: %q",
				_pod.Object.Name, _pod.Object.Namespace, _pod.Object.UID)

			if _pod.Object.UID == terminatedPodUID {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Found pod's UUID matches the one of the terminated pod")

				return false
			}
		}

		return len(activePods) == stReplicas
	}).WithContext(ctx).WithPolling(15*time.Second).WithTimeout(5*time.Minute).Should(BeTrue(),
		"New pod is not created")

	// Enhancement #1: Wait for Whereabouts network to be ready
	By("Waiting for Whereabouts network interfaces to be configured")

	err = waitForWhereaboutsNetworkReady(stLabel, namespace, interfaceName, stReplicas)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Whereabouts network not ready after pod termination: %v", err))

	// Enhancement #5: Log pod network status before connectivity test
	By("Logging pod network status for diagnostics")

	logPodNetworkStatus(stLabel, namespace, interfaceName)

	By("Verifying inter pod connectivity after pod termination")

	var parsedPort int

	parsedPort, err = strconv.Atoi(targetPort)

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to parse port number: %v", targetPort))

	// Enhancement #2: Wrap connectivity verification in retry logic
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Attempting connectivity verification with retry logic")

	Eventually(func() error {
		return verifyPodConnectivityWithRetry(stLabel, namespace, interfaceName, parsedPort, stReplicas)
	}).WithContext(ctx).WithPolling(15*time.Second).WithTimeout(2*time.Minute).Should(Succeed(),
		"Inter-pod connectivity verification failed after pod termination")
}

// ensurePodConnectivityAfterNodeDrain verifies inter-pod connectivity is restored after draining a node.
//
//nolint:funlen
func ensurePodConnectivityAfterNodeDrain(
	ctx SpecContext, stLabel, namespace, targetPort string, stReplicas int, sameNode bool) {
	By("Getting list of active pods")

	activePods := getActivePods(stLabel, namespace)

	Expect(len(activePods)).To(Equal(stReplicas),
		"Number of active pods is not equal to number of replicas")

	By("Generating random pod index")

	randomPodIndex := rand.Intn(len(activePods))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Random pod index: %d pod name: %q",
		randomPodIndex, activePods[randomPodIndex].Object.Name)

	terminatedPod := activePods[randomPodIndex]

	nodeToDrain := terminatedPod.Object.Spec.NodeName

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Node to drain: %q", nodeToDrain)

	By(fmt.Sprintf("Pulling in node %q", nodeToDrain))

	nodeObj, err := nodes.Pull(APIClient, nodeToDrain)

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve node %s object due to: %v", nodeToDrain, err))

	By(fmt.Sprintf("Cordoning node %q", nodeToDrain))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Cordoning node %q", nodeToDrain)

	err = nodeObj.Cordon()

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to cordon node %s due to: %v", nodeToDrain, err))

	defer UncordonNode(nodeObj, uncordonNodeInterval, uncordonNodeTimeout)

	time.Sleep(5 * time.Second)

	err = DrainNodeWithRetry(ctx, nodeObj)

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to drain node %s due to: %v", nodeToDrain, err))

	By("Waiting for new pod to be created")

	Eventually(func() bool {
		newActivePods := getActivePods(stLabel, namespace)

		found := false

		for _, newPod := range newActivePods {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Found pod %q in %q namespace with UUID: %q",
				newPod.Object.Name, newPod.Object.Namespace, newPod.Object.UID)

			if sameNode {
				for _, oldPod := range activePods {
					if newPod.Object.UID == oldPod.Object.UID {
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod %q in %q namespace matches old UUID: %q",
							newPod.Object.Name, newPod.Object.Namespace, newPod.Object.UID)

						found = true

						break
					}
				}
			} else if newPod.Object.UID == terminatedPod.Object.UID {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod %q in %q namespace matches old UUID: %q",
					newPod.Object.Name, newPod.Object.Namespace, terminatedPod.Object.UID)

				found = true

				break
			}
		}

		return !found && len(newActivePods) == stReplicas
	}).WithContext(ctx).WithPolling(15*time.Second).WithTimeout(10*time.Minute).Should(BeTrue(),
		"New pod is not created")

	// Enhancement #1: Wait for Whereabouts network to be ready
	By("Waiting for Whereabouts network interfaces to be configured after node drain")

	err = waitForWhereaboutsNetworkReady(stLabel, namespace, interfaceName, stReplicas)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Whereabouts network not ready after node drain: %v", err))

	// Enhancement #5: Log pod network status before connectivity test
	By("Logging pod network status for diagnostics")

	logPodNetworkStatus(stLabel, namespace, interfaceName)

	By("Verifying inter pod connectivity after node's drain")

	var parsedPort int

	parsedPort, err = strconv.Atoi(targetPort)

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to parse port number: %v", targetPort))

	// Enhancement #2: Wrap connectivity verification in retry logic
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Attempting connectivity verification with retry logic")

	Eventually(func() error {
		return verifyPodConnectivityWithRetry(stLabel, namespace, interfaceName, parsedPort, stReplicas)
	}).WithContext(ctx).WithPolling(15*time.Second).WithTimeout(2*time.Minute).Should(Succeed(),
		"Inter-pod connectivity verification failed after node drain")
}

// powerOnNodeWaitReady powers on a node via BMC and waits for it to reach Ready state.
func powerOnNodeWaitReady(bmcClient *bmc.BMC, nodeToPowerOff string, stopCh chan bool) {
	By("Stopping keepNodePoweredOff goroutine")

	stopCh <- true

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Closing stop channel")

	close(stopCh)

	By(fmt.Sprintf("Powering on node %q", nodeToPowerOff))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Powering on node %q", nodeToPowerOff)

	err := wait.PollUntilContextTimeout(context.TODO(), 15*time.Second, 6*time.Minute, true,
		func(context.Context) (bool, error) {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Checking power state of %q", nodeToPowerOff)

			powerState, err := bmcClient.SystemPowerState()
			if err != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to get power state of %q: %v",
					nodeToPowerOff, err)

				time.Sleep(1 * time.Second)

				return false, nil
			}

			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Node %q has power state %q",
				nodeToPowerOff, powerState)

			if powerState != "On" {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Powering on %q with power state %q",
					nodeToPowerOff, powerState)

				err = bmcClient.SystemPowerOn()
				if err != nil {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to power on %q: %v", nodeToPowerOff, err)

					return false, nil
				}
			}

			currentNode, err := nodes.Pull(APIClient, nodeToPowerOff)
			if err != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to pull node: %v", err)

				return false, nil
			}

			for _, condition := range currentNode.Object.Status.Conditions {
				if condition.Type == rdscoreparams.ConditionTypeReadyString {
					if condition.Status == rdscoreparams.ConstantTrueString {
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Node %q is Ready", currentNode.Definition.Name)

						return true, nil
					}
				}
			}

			return false, nil
		})
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to power on %q: %v", nodeToPowerOff, err)
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Successfully powered on %q", nodeToPowerOff)
}

// keepNodePoweredOff continuously monitors and powers off a node via BMC until signaled to stop.
func keepNodePoweredOff(bmcClient *bmc.BMC, nodeToPowerOff string, timeout time.Duration, stopCh chan bool) {
	By(fmt.Sprintf("Keeping node %q powered off", nodeToPowerOff))

	var stop = false

	for !stop {
		select {
		case <-stopCh:
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Stopping keepNodePoweredOff")

			stop = true
		case <-time.After(timeout):
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Timeout reached, stopping keepNodePoweredOff")

			stop = true
		default:
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Checking power state of %q", nodeToPowerOff)

			powerState, err := bmcClient.SystemPowerState()
			if err != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to get power state of %q: %v",
					nodeToPowerOff, err)

				time.Sleep(1 * time.Second)

				continue
			}

			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Node %q has power state %q",
				nodeToPowerOff, powerState)

			if powerState != "Off" {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Powering off %q with power state %q",
					nodeToPowerOff, powerState)

				err = bmcClient.SystemPowerOff()
				if err != nil {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to power off %q: %v",
						nodeToPowerOff, err)

					time.Sleep(1 * time.Second)

					continue
				}
			}

			time.Sleep(5 * time.Second)
		}
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("keepNodePoweredOff finished")
}

// ensurePodConnectivityAfterNodePowerOff verifies inter-pod connectivity is restored after powering off a node.
//
//nolint:gocognit,funlen
func ensurePodConnectivityAfterNodePowerOff(stLabel, namespace, targetPort string, stReplicas int, sameNode bool) {
	By("Getting list of active pods")

	activePods := getActivePods(stLabel, namespace)

	Expect(len(activePods)).To(Equal(stReplicas),
		"Number of active pods is not equal to number of replicas")

	// A random pod is selected from the list of active pods,
	// and the node it is running on is powered off.
	By("Generating random pod index")

	randomPodIndex := rand.Intn(len(activePods))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Random pod index: %d pod name: %q",
		randomPodIndex, activePods[randomPodIndex].Object.Name)

	terminatedPod := activePods[randomPodIndex]

	By(fmt.Sprintf("Getting node name for pod %q", terminatedPod.Object.Name))

	nodeToPowerOff := terminatedPod.Object.Spec.NodeName

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Node to power off: %q", nodeToPowerOff)

	By(fmt.Sprintf("Powering off node %q", nodeToPowerOff))

	var (
		bmcClient *bmc.BMC
		ctx       SpecContext
	)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Creating BMC client for node %s", nodeToPowerOff)

	if auth, ok := RDSCoreConfig.NodesCredentialsMap[nodeToPowerOff]; !ok {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"BMC Details for %q not found", nodeToPowerOff)
		Fail(fmt.Sprintf("BMC Details for %q not found", nodeToPowerOff))
	} else {
		bmcClient = bmc.New(auth.BMCAddress).
			WithRedfishUser(auth.Username, auth.Password).
			WithRedfishTimeout(6 * time.Minute)
	}

	stopCh := make(chan bool, 1)

	defer powerOnNodeWaitReady(bmcClient, nodeToPowerOff, stopCh)

	go keepNodePoweredOff(bmcClient, nodeToPowerOff, 15*time.Minute, stopCh)

	By(fmt.Sprintf("Waiting for node %q to get into NotReady state", nodeToPowerOff))

	Eventually(func() bool {
		currentNode, err := nodes.Pull(APIClient, nodeToPowerOff)
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to pull node: %v", err)

			return false
		}

		for _, condition := range currentNode.Object.Status.Conditions {
			if condition.Type == rdscoreparams.ConditionTypeReadyString {
				if condition.Status != rdscoreparams.ConstantTrueString {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Node %q is notReady", currentNode.Definition.Name)
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Reason: %s", condition.Reason)

					return true
				}
			}
		}

		return false
	}).WithContext(ctx).WithPolling(3*time.Second).WithTimeout(6*time.Minute).Should(BeTrue(),
		"Node %q hasn't reached NotReady state", nodeToPowerOff)

	By("Waiting for pods to be terminated due to a disruption")

	disruptedPods := make([]*pod.Builder, 0)

	Eventually(func() bool {
		newPods := findPodWithSelector(namespace, stLabel)

		disruptedPods = make([]*pod.Builder, 0)

		for _, _pod := range newPods {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Processing pod %q in %q namespace with UUID: %q",
				_pod.Object.Name, _pod.Object.Namespace, _pod.Object.UID)

			for _, condition := range _pod.Object.Status.Conditions {
				if condition.Type == corev1.DisruptionTarget {
					if condition.Status == rdscoreparams.ConstantTrueString {
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod %q is about to be terminated due to %q",
							_pod.Object.Name, condition.Message)

						disruptedPods = append(disruptedPods, _pod)
					}
				}
			}
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Found %d pods about to be terminated due to a disruption",
			len(disruptedPods))

		if sameNode {
			return len(disruptedPods) == stReplicas
		}

		return len(disruptedPods) == 1
	}).WithContext(ctx).WithPolling(15*time.Second).WithTimeout(10*time.Minute).Should(BeTrue(),
		"No pods are about to be terminated due to a disruption")

	By("Deleting pods that are about to be terminated due to a disruption")

	for _, _pod := range disruptedPods {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deleting pod %q", _pod.Object.Name)

		_pod, err := _pod.DeleteImmediate()
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to delete pod %q: %v", _pod.Definition.Name, err)
		}
	}

	By("Waiting for new pod(s) to be created")

	Eventually(func() bool {
		newActivePods := getActivePods(stLabel, namespace)

		found := false

		for _, newPod := range newActivePods {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Found pod %q in %q namespace with UUID: %q",
				newPod.Object.Name, newPod.Object.Namespace, newPod.Object.UID)

			if sameNode {
				for _, oldPod := range activePods {
					if newPod.Object.UID == oldPod.Object.UID {
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod %q in %q namespace matches old UUID: %q",
							newPod.Object.Name, newPod.Object.Namespace, newPod.Object.UID)

						found = true

						break
					}
				}
			} else if newPod.Object.UID == terminatedPod.Object.UID {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod %q in %q namespace matches old UUID: %q",
					newPod.Object.Name, newPod.Object.Namespace, terminatedPod.Object.UID)

				found = true

				break
			}
		}

		return !found && len(newActivePods) == stReplicas
	}).WithContext(ctx).WithPolling(15*time.Second).WithTimeout(10*time.Minute).Should(BeTrue(),
		"New pod is not created")

	// Enhancement #1: Wait for Whereabouts network to be ready
	By("Waiting for Whereabouts network interfaces to be configured after node power-on")

	var err error

	err = waitForWhereaboutsNetworkReady(stLabel, namespace, interfaceName, stReplicas)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Whereabouts network not ready after node power-on: %v", err))

	// Enhancement #5: Log pod network status before connectivity test
	By("Logging pod network status for diagnostics")

	logPodNetworkStatus(stLabel, namespace, interfaceName)

	By("Verifying inter pod connectivity after pod termination")

	var parsedPort int

	parsedPort, err = strconv.Atoi(targetPort)

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to parse port number: %v", targetPort))

	// Enhancement #2: Wrap connectivity verification in retry logic
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Attempting connectivity verification with retry logic")

	Eventually(func() error {
		return verifyPodConnectivityWithRetry(stLabel, namespace, interfaceName, parsedPort, stReplicas)
	}).WithContext(ctx).WithPolling(15*time.Second).WithTimeout(2*time.Minute).Should(Succeed(),
		"Inter-pod connectivity verification failed after node power-off")
}

// parseLabelsMap converts "key=value" string to map[string]string.
func parseLabelsMap(labelString string) map[string]string {
	parts := strings.Split(labelString, "=")
	if len(parts) != 2 {
		Fail(fmt.Sprintf("Invalid label format: %s. Expected 'key=value'", labelString))
	}

	return map[string]string{parts[0]: parts[1]}
}

// createStatefulsetBuilder creates and configures the basic statefulset.
func createStatefulsetBuilder(config StatefulsetConfig, svcLabelsMap map[string]string) *statefulset.Builder {
	By("Defining statefulset container")

	stContainer := defineStatefulsetContainer("my-st-container", config.Image, config.Command, nil, nil)
	Expect(stContainer).ToNot(BeNil(), "Failed to define statefulset container")

	By("Getting statefulset container config")

	stContainerCfg, err := stContainer.GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to get statefulset container config")

	By("Defining statefulset")

	stBuilder := statefulset.NewBuilder(APIClient, config.Name, RDSCoreConfig.WhereaboutNS, svcLabelsMap, stContainerCfg)

	// Add pod annotations
	nadMap := map[string]string{"k8s.v1.cni.cncf.io/networks": config.NAD}
	err = withPodAnnotations(stBuilder, nadMap)
	Expect(err).ToNot(HaveOccurred(), "Failed to add pod annotations to statefulset %q", config.Name)

	// Set replicas
	stBuilder.Definition.Spec.Replicas = &config.Replicas

	return stBuilder
}

// configureAffinity sets up pod affinity or anti-affinity based on configuration.
func configureAffinity(stBuilder *statefulset.Builder, config StatefulsetConfig, svcLabelsMap map[string]string) {
	By(fmt.Sprintf("Adding pod %s to statefulset %q",
		map[bool]string{true: "anti-affinity", false: "affinity"}[config.UseAntiAffinity],
		config.Name))

	var err error

	if config.UseAntiAffinity {
		err = withRequiredLabelPodAntiAffinity(stBuilder, svcLabelsMap,
			[]string{RDSCoreConfig.WhereaboutNS}, config.TopologyKey)
	} else {
		err = withRequiredLabelPodAffinity(stBuilder, svcLabelsMap,
			[]string{RDSCoreConfig.WhereaboutNS}, config.TopologyKey)
	}

	Expect(err).ToNot(HaveOccurred(), "Failed to configure pod affinity for statefulset %q", config.Name)
}

// setupServiceAccountAndRBAC handles service account and RBAC configuration.
func setupServiceAccountAndRBAC(config StatefulsetConfig) {
	By(fmt.Sprintf("Setting up service account %q", config.ServiceAccount))

	// Delete existing service account
	deleteServiceAccount(config.ServiceAccount, RDSCoreConfig.WhereaboutNS)

	// Create new service account
	createServiceAccount(config.ServiceAccount, RDSCoreConfig.WhereaboutNS)

	By(fmt.Sprintf("Setting up cluster role binding %q", config.RBACRole))

	// Delete existing cluster role binding
	deleteClusterRBAC(config.RBACRole)

	// Create new cluster role binding
	createClusterRBAC(config.ServiceAccount, config.RBACRole, config.ServiceAccount, RDSCoreConfig.WhereaboutNS)
}

// CreateWhereaboutsStatefulset creates a statefulset with whereabouts IPAM based on configuration.
func CreateWhereaboutsStatefulset(ctx SpecContext, config StatefulsetConfig) {
	By(fmt.Sprintf("Setting up statefulset with %s", config.Description))

	Expect(config.NAD).ToNot(BeEmpty(),
		"NetworkAttachmentDefinition must be set for statefulset %q", config.Name)

	// Cleanup existing statefulset FIRST (before deleting service)
	// This ensures pods fully terminate and release their endpoints
	// before the service and its EndpointSlices are deleted
	cleanupStatefulset(config.Name, RDSCoreConfig.WhereaboutNS, config.Label)

	// Setup headless service AFTER pod cleanup
	setupHeadlessService(config.ServiceName, RDSCoreConfig.WhereaboutNS, config.Label, config.Port, config.NAD)

	// Parse service labels
	svcLabelsMap := parseLabelsMap(config.Label)

	// Parse port
	parsedPort, err := strconv.Atoi(config.Port)
	Expect(err).ToNot(HaveOccurred(), "Failed to parse port number: %v", config.Port)

	// Create statefulset
	stBuilder := createStatefulsetBuilder(config, svcLabelsMap)

	// Configure affinity based on config
	configureAffinity(stBuilder, config, svcLabelsMap)

	// Setup service account and RBAC
	setupServiceAccountAndRBAC(config)

	// Set service account on statefulset
	stBuilder.Definition.Spec.Template.Spec.ServiceAccountName = config.ServiceAccount

	// Create and wait for statefulset
	createStatefulsetAndWaitReplicasReady(config.Name, RDSCoreConfig.WhereaboutNS, stBuilder)

	// Verify connectivity
	VerifyPodConnectivity(config.Label, RDSCoreConfig.WhereaboutNS, interfaceName, parsedPort)
}

// ReconcilerConfigState holds the original state of the reconciler configuration.
type ReconcilerConfigState struct {
	OriginalSchedule   string
	ConfigMapExisted   bool
	ScheduleKeyExisted bool
}

// ConfigureWhereaboutsReconciler configures the Whereabouts reconciler to run every 3 minutes
// and returns the original configuration state for later restoration.
//
//nolint:funlen
func ConfigureWhereaboutsReconciler() (*ReconcilerConfigState, error) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Configuring Whereabouts reconciler schedule to %q", WhereaboutsReconcilerSchedule)

	state := &ReconcilerConfigState{
		ConfigMapExisted:   false,
		ScheduleKeyExisted: false,
		OriginalSchedule:   "",
	}

	// 1. Check if ConfigMap exists
	configMap, err := configmap.Pull(APIClient, WhereaboutsReconcilerCMName, WhereaboutsReconcilerNamespace)
	if err != nil {
		// ConfigMap doesn't exist - create it
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"ConfigMap %q not found in namespace %q, creating it",
			WhereaboutsReconcilerCMName, WhereaboutsReconcilerNamespace)

		state.ConfigMapExisted = false

		createConfigMap(WhereaboutsReconcilerCMName, WhereaboutsReconcilerNamespace, map[string]string{
			WhereaboutsReconcilerKey: WhereaboutsReconcilerSchedule,
		})

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Successfully created ConfigMap %q with schedule %q",
			WhereaboutsReconcilerCMName, WhereaboutsReconcilerSchedule)

		return state, nil
	}

	// 2. ConfigMap exists - check if schedule key exists
	state.ConfigMapExisted = true

	if originalSchedule, ok := configMap.Object.Data[WhereaboutsReconcilerKey]; ok {
		// Schedule key exists - save original value
		state.ScheduleKeyExisted = true
		state.OriginalSchedule = originalSchedule

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"ConfigMap %q exists with schedule %q, saving original",
			WhereaboutsReconcilerCMName, originalSchedule)

		// Only update if different from target schedule
		if originalSchedule == WhereaboutsReconcilerSchedule {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Schedule is already set to %q, no update needed", WhereaboutsReconcilerSchedule)

			return state, nil
		}
	} else {
		// Schedule key doesn't exist
		state.ScheduleKeyExisted = false

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"ConfigMap %q exists but schedule key %q not found, adding it",
			WhereaboutsReconcilerCMName, WhereaboutsReconcilerKey)
	}

	// 3. Update ConfigMap with accelerated schedule
	if configMap.Object.Data == nil {
		configMap.Object.Data = make(map[string]string)
	}

	configMap.Object.Data[WhereaboutsReconcilerKey] = WhereaboutsReconcilerSchedule

	err = wait.PollUntilContextTimeout(
		context.TODO(),
		5*time.Second,
		1*time.Minute,
		true,
		func(ctx context.Context) (bool, error) {
			_, updateErr := configMap.Update()
			if updateErr != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
					"Failed to update ConfigMap, retrying: %v", updateErr)

				// Re-pull ConfigMap in case it was modified
				freshCM, pullErr := configmap.Pull(APIClient, WhereaboutsReconcilerCMName, WhereaboutsReconcilerNamespace)
				if pullErr != nil {
					return false, nil
				}

				if freshCM.Object.Data == nil {
					freshCM.Object.Data = make(map[string]string)
				}

				freshCM.Object.Data[WhereaboutsReconcilerKey] = WhereaboutsReconcilerSchedule
				configMap = freshCM

				return false, nil
			}

			return true, nil
		})
	if err != nil {
		return nil, fmt.Errorf("failed to update reconciler ConfigMap after retries: %w", err)
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Successfully configured reconciler schedule to %q", WhereaboutsReconcilerSchedule)

	return state, nil
}

// RestoreWhereaboutsReconciler restores the Whereabouts reconciler configuration to its original state.
//
//nolint:funlen
func RestoreWhereaboutsReconciler(state *ReconcilerConfigState) error {
	if state == nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"No reconciler state to restore (state is nil)")

		return nil
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Restoring Whereabouts reconciler configuration (existed=%v, had_key=%v, original=%q)",
		state.ConfigMapExisted, state.ScheduleKeyExisted, state.OriginalSchedule)

	// Case 1: ConfigMap didn't exist before - delete it
	if !state.ConfigMapExisted {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"ConfigMap %q was created by tests, deleting it",
			WhereaboutsReconcilerCMName)

		configMap, err := configmap.Pull(APIClient, WhereaboutsReconcilerCMName, WhereaboutsReconcilerNamespace)
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"ConfigMap %q not found during restoration (may have been deleted): %v",
				WhereaboutsReconcilerCMName, err)

			return nil // Already gone, nothing to restore
		}

		err = configMap.Delete()
		if err != nil {
			return fmt.Errorf("failed to delete ConfigMap %q: %w", WhereaboutsReconcilerCMName, err)
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Successfully deleted ConfigMap %q", WhereaboutsReconcilerCMName)

		return nil
	}

	// Case 2: ConfigMap existed but schedule key didn't - remove the key
	if !state.ScheduleKeyExisted {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Schedule key %q was added by tests, removing it", WhereaboutsReconcilerKey)

		configMap, err := configmap.Pull(APIClient, WhereaboutsReconcilerCMName, WhereaboutsReconcilerNamespace)
		if err != nil {
			return fmt.Errorf("failed to pull ConfigMap during restoration: %w", err)
		}

		if configMap.Object.Data != nil {
			delete(configMap.Object.Data, WhereaboutsReconcilerKey)

			_, err = configMap.Update()
			if err != nil {
				return fmt.Errorf("failed to remove schedule key from ConfigMap: %w", err)
			}
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Successfully removed schedule key %q", WhereaboutsReconcilerKey)

		return nil
	}

	// Case 3: Both ConfigMap and key existed - restore original schedule
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Restoring original schedule %q", state.OriginalSchedule)

	configMap, err := configmap.Pull(APIClient, WhereaboutsReconcilerCMName, WhereaboutsReconcilerNamespace)
	if err != nil {
		return fmt.Errorf("failed to pull ConfigMap during restoration: %w", err)
	}

	if configMap.Object.Data == nil {
		configMap.Object.Data = make(map[string]string)
	}

	configMap.Object.Data[WhereaboutsReconcilerKey] = state.OriginalSchedule

	err = wait.PollUntilContextTimeout(
		context.TODO(),
		5*time.Second,
		1*time.Minute,
		true,
		func(ctx context.Context) (bool, error) {
			_, updateErr := configMap.Update()
			if updateErr != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
					"Failed to restore ConfigMap, retrying: %v", updateErr)

				// Re-pull and retry
				freshCM, pullErr := configmap.Pull(APIClient, WhereaboutsReconcilerCMName, WhereaboutsReconcilerNamespace)
				if pullErr != nil {
					return false, nil
				}

				if freshCM.Object.Data == nil {
					freshCM.Object.Data = make(map[string]string)
				}

				freshCM.Object.Data[WhereaboutsReconcilerKey] = state.OriginalSchedule
				configMap = freshCM

				return false, nil
			}

			return true, nil
		})
	if err != nil {
		return fmt.Errorf("failed to restore ConfigMap after retries: %w", err)
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Successfully restored reconciler schedule to %q", state.OriginalSchedule)

	return nil
}

// VerifyWhereaboutsReconcilerHealth checks that the Whereabouts reconciler infrastructure is healthy.
// It verifies:
// - Reconciler ConfigMap exists and has valid cron schedule.
// - Reconciler pod exists and is running in openshift-multus namespace.
func VerifyWhereaboutsReconcilerHealth() error {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Verifying Whereabouts reconciler health")

	// 1. Check ConfigMap exists and has valid schedule
	configMap, err := configmap.Pull(APIClient, WhereaboutsReconcilerCMName, WhereaboutsReconcilerNamespace)
	if err != nil {
		return fmt.Errorf("reconciler ConfigMap %q not found in namespace %q: %w",
			WhereaboutsReconcilerCMName, WhereaboutsReconcilerNamespace, err)
	}

	schedule, ok := configMap.Object.Data[WhereaboutsReconcilerKey]
	if !ok || schedule == "" {
		return fmt.Errorf("reconciler schedule key %q not found in ConfigMap %q",
			WhereaboutsReconcilerKey, WhereaboutsReconcilerCMName)
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Reconciler ConfigMap found with schedule: %q", schedule)

	// 2. Check reconciler pod exists and is Running
	// Whereabouts reconciler pods are typically labeled with app=whereabouts-reconciler
	pods, err := pod.List(APIClient, WhereaboutsReconcilerNamespace,
		metav1.ListOptions{LabelSelector: "app=whereabouts-reconciler"})
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to list reconciler pods: %v", err)

		return fmt.Errorf("failed to list reconciler pods in namespace %q: %w",
			WhereaboutsReconcilerNamespace, err)
	}

	if len(pods) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("No reconciler pods found in namespace %q",
			WhereaboutsReconcilerNamespace)

		return fmt.Errorf("no reconciler pods found in namespace %q with label app=whereabouts-reconciler",
			WhereaboutsReconcilerNamespace)
	}

	runningPods := 0

	for _, podObj := range pods {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Reconciler pod %q status: %s",
			podObj.Object.Name, podObj.Object.Status.Phase)

		if podObj.Object.Status.Phase == corev1.PodRunning {
			runningPods++
		}
	}

	if runningPods == 0 {
		return fmt.Errorf("no reconciler pods are in Running state in namespace %q",
			WhereaboutsReconcilerNamespace)
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Found %d running reconciler pod(s)", runningPods)

	return nil
}

// WaitForReconcilerCycle waits for a Whereabouts reconciler cycle to complete.
// The reconciler runs every 3 minutes (*/3 * * * *), so we wait for 3 minutes + 30 second buffer.
func WaitForReconcilerCycle() error {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Waiting for Whereabouts reconciler cycle to complete")

	// Parse reconciler schedule from ConfigMap
	configMap, err := configmap.Pull(APIClient, WhereaboutsReconcilerCMName, WhereaboutsReconcilerNamespace)
	if err != nil {
		return fmt.Errorf("failed to get reconciler ConfigMap: %w", err)
	}

	schedule := configMap.Object.Data[WhereaboutsReconcilerKey]

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Reconciler schedule: %q", schedule)

	// For */3 * * * * schedule, wait for 3 minutes + 30 second buffer = 210 seconds
	waitDuration := 210 * time.Second

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Waiting %v for reconciler cycle", waitDuration)

	time.Sleep(waitDuration)

	// Verify reconciler pod has recent logs (optional check)
	pods, err := pod.List(APIClient, WhereaboutsReconcilerNamespace,
		metav1.ListOptions{LabelSelector: "app=whereabouts-reconciler"})
	if err == nil && len(pods) > 0 {
		for _, reconcilerPod := range pods {
			logDuration := 4 * time.Minute

			logs, err := reconcilerPod.GetLog(logDuration, "whereabouts")
			if err != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
					"Could not retrieve logs from reconciler pod %q: %v", reconcilerPod.Object.Name, err)

				continue
			}

			if logs == "" {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
					"No recent logs from reconciler pod %q in the last 4 minutes", reconcilerPod.Object.Name)
			} else {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
					"Reconciler pod %q has logs from the last 4 minutes", reconcilerPod.Object.Name)
			}
		}
	}

	return nil
}

// checkPodForDADFailure checks if a pod has any IPv6 addresses in dadfailed state.
func checkPodForDADFailure(podObj *pod.Builder) error {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Checking pod %q/%q for IPv6 DAD failures",
		podObj.Object.Namespace, podObj.Object.Name)

	if podObj.Object.Status.Phase != corev1.PodRunning || len(podObj.Definition.Spec.Containers) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Pod %q/%q: skipping DAD check - phase=%q, running=%v, containers=%d",
			podObj.Object.Namespace, podObj.Object.Name,
			podObj.Object.Status.Phase,
			podObj.Object.Status.Phase == corev1.PodRunning,
			len(podObj.Definition.Spec.Containers))

		return nil
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Pod %q/%q: executing 'ip addr show' in container %q",
		podObj.Object.Namespace, podObj.Object.Name,
		podObj.Definition.Spec.Containers[0].Name)

	output, err := podObj.ExecCommand([]string{"ip", "addr", "show"}, podObj.Definition.Spec.Containers[0].Name)
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Pod %q/%q: failed to execute 'ip addr show': %v (assuming no DAD issues)",
			podObj.Object.Namespace, podObj.Object.Name, err)

		return nil
	}

	if strings.Contains(output.String(), "dadfailed") {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Pod %q/%q: DADFAILED IPv6 addresses detected in output",
			podObj.Object.Namespace, podObj.Object.Name)

		return fmt.Errorf("pod %q has IPv6 addresses in dadfailed state: %s",
			podObj.Object.Name, output.String())
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Pod %q/%q: DAD check passed, no dadfailed addresses found",
		podObj.Object.Namespace, podObj.Object.Name)

	return nil
}

// extractIPsFromNetworkStatus parses network-status annotation and extracts IPs for the given network.
// Returns error if duplicate IPs are detected.
func extractIPsFromNetworkStatus(
	podObj *pod.Builder,
	networkName string,
	allocatedIPv4,
	allocatedIPv6 map[string]string) error {
	netStatus, ok := podObj.Object.Annotations["k8s.v1.cni.cncf.io/network-status"]
	if !ok {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Pod %q/%q: no network-status annotation, skipping IP extraction",
			podObj.Object.Namespace, podObj.Object.Name)

		return nil
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod %q network-status: %s", podObj.Object.Name, netStatus)

	var networkStatuses []map[string]interface{}
	if err := json.Unmarshal([]byte(netStatus), &networkStatuses); err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to parse network-status for pod %q: %v",
			podObj.Object.Name, err)

		return nil
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Pod %q/%q: searching for network matching %q in network-status annotation",
		podObj.Object.Namespace, podObj.Object.Name, networkName)

	ipCount := 0
	foundMatchingNetwork := false

	for _, netStatus := range networkStatuses {
		name, _ := netStatus["name"].(string)
		if !strings.Contains(name, networkName) {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Pod %q/%q: skipping network %q (does not match %q)",
				podObj.Object.Namespace, podObj.Object.Name, name, networkName)

			continue
		}

		foundMatchingNetwork = true

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Pod %q/%q: found matching network %q, extracting IPs",
			podObj.Object.Namespace, podObj.Object.Name, name)

		ips, ok := netStatus["ips"].([]interface{})
		if !ok {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Pod %q/%q: network %q has no 'ips' array or wrong type",
				podObj.Object.Namespace, podObj.Object.Name, name)

			continue
		}

		for _, ipInterface := range ips {
			ipStr, ok := ipInterface.(string)
			if !ok {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
					"Pod %q/%q: network %q has IP entry with unexpected type (expected string, got %T)",
					podObj.Object.Namespace, podObj.Object.Name, name, ipInterface)

				continue
			}

			if err := recordIPAllocation(ipStr, podObj.Object.Name, allocatedIPv4, allocatedIPv6); err != nil {
				return err
			}

			ipCount++
		}
	}

	if !foundMatchingNetwork {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Pod %q/%q: no network matching %q found in network-status annotation",
			podObj.Object.Namespace, podObj.Object.Name, networkName)
	} else {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Pod %q/%q: extracted %d IP(s) for network matching %q",
			podObj.Object.Namespace, podObj.Object.Name, ipCount, networkName)
	}

	return nil
}

// recordIPAllocation records an IP allocation and checks for duplicates.
func recordIPAllocation(ipStr, podName string, allocatedIPv4, allocatedIPv6 map[string]string) error {
	// Parse IP once and validate
	parsedIP := net.ParseIP(ipStr)
	if parsedIP == nil {
		return fmt.Errorf("invalid IP address %q for pod %q", ipStr, podName)
	}

	// Check if IPv4 or IPv6
	if parsedIP.To4() != nil {
		// IPv4
		if existingPod, exists := allocatedIPv4[ipStr]; exists {
			return fmt.Errorf("duplicate IPv4 address %q allocated to both pod %q and pod %q",
				ipStr, existingPod, podName)
		}

		allocatedIPv4[ipStr] = podName

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod %q allocated IPv4: %s", podName, ipStr)
	} else {
		// IPv6
		if existingPod, exists := allocatedIPv6[ipStr]; exists {
			return fmt.Errorf("duplicate IPv6 address %q allocated to both pod %q and pod %q",
				ipStr, existingPod, podName)
		}

		allocatedIPv6[ipStr] = podName

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod %q allocated IPv6: %s", podName, ipStr)
	}

	return nil
}

// VerifyIPAMStateConsistency checks Whereabouts IPAM state for consistency issues.
// It verifies:
// - No duplicate IP allocations across pods.
// - No pods have IPv6 addresses in dadfailed state.
func VerifyIPAMStateConsistency(namespace, networkName string) error {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Verifying IPAM state consistency in namespace %q for network %q", namespace, networkName)

	pods, err := pod.List(APIClient, namespace, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list pods in namespace %q: %w", namespace, err)
	}

	if len(pods) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("No pods found in namespace %q, IPAM state is clean", namespace)

		return nil
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Processing %d total pod(s) in namespace %q, filtering for Running non-deleted pods",
		len(pods), namespace)

	allocatedIPv4 := make(map[string]string) // IP -> PodName
	allocatedIPv6 := make(map[string]string) // IP -> PodName

	processedCount := 0
	skippedCount := 0

	for _, podObj := range pods {
		// Skip pods that are not running or being deleted
		if podObj.Object.Status.Phase != corev1.PodRunning || podObj.Object.DeletionTimestamp != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Skipping pod %q/%q: phase=%q, markedForDeletion=%v",
				podObj.Object.Namespace, podObj.Object.Name,
				podObj.Object.Status.Phase,
				podObj.Object.DeletionTimestamp != nil)

			skippedCount++

			continue
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Validating pod %q/%q: checking network %q IPs and DAD state",
			podObj.Object.Namespace, podObj.Object.Name, networkName)

		if err := extractIPsFromNetworkStatus(podObj, networkName, allocatedIPv4, allocatedIPv6); err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Pod %q/%q: IP extraction failed: %v",
				podObj.Object.Namespace, podObj.Object.Name, err)

			return err
		}

		if err := checkPodForDADFailure(podObj); err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Pod %q/%q: DAD failure check failed: %v",
				podObj.Object.Namespace, podObj.Object.Name, err)

			return err
		}

		processedCount++
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"IPAM state verification complete: processed %d pod(s), skipped %d pod(s), "+
			"found %d IPv4 and %d IPv6 addresses allocated",
		processedCount, skippedCount, len(allocatedIPv4), len(allocatedIPv6))

	return nil
}

// VerifyPodConnectivity verifies inter pod connectivity.
func VerifyPodConnectivity(stLabel, namespace, interfaceName string, targetPort int) {
	By("Verifying inter pod connectivity")

	By("Checking if pods are running and active")

	activePods := getActivePods(stLabel, namespace)

	Expect(len(activePods)).To(Equal(int(myStatefulsetTwoReplicas)),
		"Number of active pods is not equal to number of replicas")

	By("Checking pods IP addresses")

	podWhereaboutsIPs := getPodWhereaboutsIPs(activePods, interfaceName)
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("PodWhereaboutsIPs: %+v", podWhereaboutsIPs)

	podOneName := activePods[0].Object.Name
	podTwoName := activePods[len(activePods)-1].Object.Name

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod one %q", podOneName)
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod two %q", podTwoName)

	podsMapping := make(map[string]string)

	podsMapping[podOneName] = podTwoName
	podsMapping[podTwoName] = podOneName

	verifyInterPodCommunication(activePods, podWhereaboutsIPs, podsMapping, targetPort)
}

// CreateStatefulsetOnSameNode creates a statefulset with pods on the same node.
func CreateStatefulsetOnSameNode(ctx SpecContext) {
	config := SameNodeConfig
	// Set runtime configuration values
	config.Port = RDSCoreConfig.WhereaboutsSTOnePort
	config.Image = RDSCoreConfig.WhereaboutsSTImageOne
	config.Command = RDSCoreConfig.WhereaboutsSTOneCMD
	config.NAD = RDSCoreConfig.WhereaboutsSTOneNAD

	CreateWhereaboutsStatefulset(ctx, config)
}

// CreateStatefulsetOnDifferentNode creates a statefulset with pods on different nodes.
func CreateStatefulsetOnDifferentNode(ctx SpecContext) {
	config := DifferentNodeConfig
	// Set runtime configuration values
	config.Port = RDSCoreConfig.WhereaboutsSTTwoPort
	config.Image = RDSCoreConfig.WhereaboutsSTImageTwo
	config.Command = RDSCoreConfig.WhereaboutsSTTwoCMD
	config.NAD = RDSCoreConfig.WhereaboutsSTTwoNAD

	CreateWhereaboutsStatefulset(ctx, config)
}

// EnsurePodConnectivityBetweenDifferentNodesAfterPodTermination ensures inter pod connectivity
// between different nodes after one of the pods is terminated.
func EnsurePodConnectivityBetweenDifferentNodesAfterPodTermination(ctx SpecContext) {
	CreateStatefulsetOnDifferentNode(ctx)

	ensurePodConnectivityAfterPodTermination(myStatefulsetTwoLabel, RDSCoreConfig.WhereaboutNS,
		RDSCoreConfig.WhereaboutsSTTwoPort, myStatefulsetTwoReplicas)
}

// EnsurePodConnectivityOnSameNodeAfterPodTermination ensures inter pod connectivity
// on the same node after one of the pods is terminated.
func EnsurePodConnectivityOnSameNodeAfterPodTermination(ctx SpecContext) {
	CreateStatefulsetOnSameNode(ctx)

	ensurePodConnectivityAfterPodTermination(myStatefulsetOneLabel, RDSCoreConfig.WhereaboutNS,
		RDSCoreConfig.WhereaboutsSTOnePort, myStatefulsetOneReplicas)
}

// EnsurePodConnectivityBetweenDifferentNodesAfterNodeDrain ensures inter pod connectivity
// between different nodes after one of the nodes is drained.
func EnsurePodConnectivityBetweenDifferentNodesAfterNodeDrain(ctx SpecContext) {
	DeferCleanup(EnsureInNodeReadiness)

	CreateStatefulsetOnDifferentNode(ctx)

	ensurePodConnectivityAfterNodeDrain(ctx, myStatefulsetTwoLabel, RDSCoreConfig.WhereaboutNS,
		RDSCoreConfig.WhereaboutsSTTwoPort, myStatefulsetTwoReplicas, false)
}

// EnsurePodConnectivityOnSameNodeAfterNodeDrain ensures inter pod connectivity
// on the same node after one of the nodes is drained.
func EnsurePodConnectivityOnSameNodeAfterNodeDrain(ctx SpecContext) {
	DeferCleanup(EnsureInNodeReadiness)

	CreateStatefulsetOnSameNode(ctx)

	ensurePodConnectivityAfterNodeDrain(ctx, myStatefulsetOneLabel, RDSCoreConfig.WhereaboutNS,
		RDSCoreConfig.WhereaboutsSTOnePort, myStatefulsetOneReplicas, true)
}

// EnsurePodConnectivityOnSameNodeAfterNodePowerOff ensures inter pod connectivity
// on the same node after one of the nodes is powered off.
func EnsurePodConnectivityOnSameNodeAfterNodePowerOff(ctx SpecContext) {
	DeferCleanup(EnsureInNodeReadiness)

	CreateStatefulsetOnSameNode(ctx)

	ensurePodConnectivityAfterNodePowerOff(myStatefulsetOneLabel, RDSCoreConfig.WhereaboutNS,
		RDSCoreConfig.WhereaboutsSTOnePort, myStatefulsetOneReplicas, true)
}

// EnsurePodConnectivityBetweenDifferentNodesAfterNodePowerOff ensures inter pod connectivity
// between different nodes after one of the nodes is powered off.
func EnsurePodConnectivityBetweenDifferentNodesAfterNodePowerOff(ctx SpecContext) {
	DeferCleanup(EnsureInNodeReadiness)

	CreateStatefulsetOnDifferentNode(ctx)

	ensurePodConnectivityAfterNodePowerOff(myStatefulsetTwoLabel, RDSCoreConfig.WhereaboutNS,
		RDSCoreConfig.WhereaboutsSTTwoPort, myStatefulsetTwoReplicas, false)
}

// ValidatePodConnectivityOnSameNodeAfterClusterReboot validates inter pod connectivity
// on the same node after cluster reboot.
func ValidatePodConnectivityOnSameNodeAfterClusterReboot(ctx SpecContext) {
	By("Validating inter pod connectivity on the same node after cluster reboot")

	parsedPort, err := strconv.Atoi(RDSCoreConfig.WhereaboutsSTOnePort)

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to parse port number: %v", RDSCoreConfig.WhereaboutsSTOnePort))

	VerifyPodConnectivity(myStatefulsetOneLabel, RDSCoreConfig.WhereaboutNS, interfaceName, parsedPort)
}

// ValidatePodConnectivityBetweenDifferentNodesAfterClusterReboot validates inter pod connectivity
// between different nodes after cluster reboot.
func ValidatePodConnectivityBetweenDifferentNodesAfterClusterReboot(ctx SpecContext) {
	By("Validating inter pod connectivity between different nodes after cluster reboot")

	parsedPort, err := strconv.Atoi(RDSCoreConfig.WhereaboutsSTTwoPort)

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to parse port number: %v", RDSCoreConfig.WhereaboutsSTTwoPort))

	VerifyPodConnectivity(myStatefulsetTwoLabel, RDSCoreConfig.WhereaboutNS, interfaceName, parsedPort)
}
