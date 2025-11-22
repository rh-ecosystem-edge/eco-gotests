package rdscorecommon

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
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
	// WhereaboutsReconcilcerCMName is the name of the whereabouts reconciler configmap.
	WhereaboutsReconcilcerCMName = "whereabouts-config"

	myHeadlessSvcOne            = "rds-st-one-headless-1"
	myStatefulsetOne            = "rds-st-one"
	myStatefulsetOneLabel       = "app=rds-st-one"
	myStatefulsetOneReplicas    = 2
	myStatefulsetOneSA          = "rds-st-one-sa"
	myStatefulsetOneRBACRole    = "system:openshift:scc:nonroot-v2"
	myStatefulsetOneTopologyKey = "kubernetes.io/hostname"
	// interfaceName is the name of the network interface inside the pod.
	interfaceName = "net1"

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
			pods, err := pod.List(APIClient, RDSCoreConfig.WhereaboutNS, metav1.ListOptions{
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

func setupHeadlessService(svcName, namespace, svcLabel, svcPort string) {
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

	By("Setting ipFamilyPolicy to 'RequireDualStack'")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Setting ipFamilyPolicy to 'RequireDualStack'")

	svcOne = svcOne.WithIPFamily([]corev1.IPFamily{"IPv4", "IPv6"},
		corev1.IPFamilyPolicyRequireDualStack)

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
			if dstAddr.Family == "inet6" && dstAddr.Scope == "link" {
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

// getPodWhereaboutsIPs gets the IP addresses for the given pod.
func getPodWhereaboutsIPs(activePods []*pod.Builder, interfaceName string) map[string][]NetworkInterface {
	podWhereaboutsIPs := make(map[string][]NetworkInterface)

	var ctx SpecContext

	cmdGetIPAddr := []string{"/bin/sh", "-c", fmt.Sprintf("ip -j addr show dev %s", interfaceName)}

	for _, _pod := range activePods {
		var networkInterface []NetworkInterface

		Eventually(func() bool {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Executing command %q within pod %q in %q namespace",
				cmdGetIPAddr, _pod.Object.Name, _pod.Object.Namespace)

			addrBuffInfo, err := _pod.ExecCommand(cmdGetIPAddr, _pod.Definition.Spec.Containers[0].Name)
			if err != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to execute command within pod %q in %q namespace: %s",
					_pod.Object.Name, _pod.Object.Namespace, err)

				return false
			}

			if addrBuffInfo.Len() == 0 {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Empty output from command within pod %q in %q namespace",
					_pod.Object.Name, _pod.Object.Namespace)

				return false
			}

			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Unmarshalling IP addresses")

			err = json.Unmarshal(addrBuffInfo.Bytes(), &networkInterface)
			if err != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to unmarshal IP addresses for pod %q in %q namespace: %s",
					_pod.Object.Name, _pod.Object.Namespace, err)

				return false
			}

			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("IP addresses: %+v", networkInterface)

			podWhereaboutsIPs[_pod.Object.Name] = networkInterface

			return true
		}).WithContext(ctx).WithPolling(10*time.Second).WithTimeout(1*time.Minute).Should(BeTrue(),
			"Failed to get IP addresses for pod %q in %q namespace", _pod.Object.Name, _pod.Object.Namespace)
	}

	return podWhereaboutsIPs
}

// getActivePods gets the active pods with the given label and namespace.
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

		if _pod.Object.Status.Phase == corev1.PodRunning && _pod.Object.DeletionTimestamp == nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod %q is active(running and not marked for deletion)",
				_pod.Object.Name)

			activePods = append(activePods, _pod)
		} else if _pod.Object.DeletionTimestamp != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod %q is marked for deletion, skipping", _pod.Object.Name)
		}
	}

	return activePods
}

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

	By("Verifying inter pod connectivity after pod termination")

	parsedPort, err := strconv.Atoi(targetPort)

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to parse port number: %v", targetPort))

	VerifyPodConnectivity(stLabel, namespace, interfaceName, parsedPort)
}

//nolint:funlen
func ensurePodConnectivityAfterNodeDrain(stLabel, namespace, targetPort string, stReplicas int, sameNode bool) {
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

	By(fmt.Sprintf("Draining node %q", nodeToDrain))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Draining node %q", nodeToDrain)

	time.Sleep(5 * time.Second)

	err = nodeObj.Drain()

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to drain node %s due to: %v", nodeToDrain, err))

	By("Waiting for new pod to be created")

	var ctx SpecContext

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
	}).WithContext(ctx).WithPolling(15*time.Second).WithTimeout(5*time.Minute).Should(BeTrue(),
		"New pod is not created")

	By("Verifying inter pod connectivity after node's drain")

	parsedPort, err := strconv.Atoi(targetPort)

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to parse port number: %v", targetPort))

	VerifyPodConnectivity(stLabel, namespace, interfaceName, parsedPort)
}

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

	By("Verifying inter pod connectivity after pod termination")

	parsedPort, err := strconv.Atoi(targetPort)

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to parse port number: %v", targetPort))

	VerifyPodConnectivity(stLabel, namespace, interfaceName, parsedPort)
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

	configureWhereaboutsIPReconciler()

	// Setup headless service
	setupHeadlessService(config.ServiceName, RDSCoreConfig.WhereaboutNS, config.Label, config.Port)

	// Cleanup existing statefulset
	cleanupStatefulset(config.Name, RDSCoreConfig.WhereaboutNS, config.Label)

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

// configureWhereaboutsIPReconciler configures whereabouts IP reconciler to run every 3 minutes.
func configureWhereaboutsIPReconciler() {
	By(fmt.Sprintf("Checking if configmap %q exists in %q namespace",
		WhereaboutsReconcilcerCMName, WhereaboutsReconcilerNamespace))

	var ctx SpecContext

	cmWhereabouts, err := configmap.Pull(APIClient, WhereaboutsReconcilcerCMName, WhereaboutsReconcilerNamespace)
	if err == nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Configmap %q exists in %q namespace, updating it",
			WhereaboutsReconcilcerCMName, WhereaboutsReconcilerNamespace)

		if oldSchedule, ok := cmWhereabouts.Object.Data[WhereaboutsReconcilerKey]; ok {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Key %q already exists in configmap %q in %q namespace, updating it",
				WhereaboutsReconcilerKey, WhereaboutsReconcilcerCMName, WhereaboutsReconcilerNamespace)

			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Old schedule: %q", oldSchedule)

			cmWhereabouts.Object.Data[WhereaboutsReconcilerKey] = WhereaboutsReconcilerSchedule
		} else {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Key %q does not exist in configmap %q in %q namespace, adding it",
				WhereaboutsReconcilerKey, WhereaboutsReconcilcerCMName, WhereaboutsReconcilerNamespace)

			cmWhereabouts.Object.Data[WhereaboutsReconcilerKey] = WhereaboutsReconcilerSchedule
		}

		By(fmt.Sprintf("Updating configmap %q in %q namespace",
			WhereaboutsReconcilcerCMName, WhereaboutsReconcilerNamespace))

		Eventually(func() error {
			_, err := cmWhereabouts.Update()

			return err
		}).WithContext(ctx).WithPolling(15*time.Second).WithTimeout(1*time.Minute).Should(Succeed(),
			"Failed to update configmap %q in %q namespace", WhereaboutsReconcilcerCMName, WhereaboutsReconcilerNamespace)
	} else {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Configmap %q does not exist in %q namespace, creating it",
			WhereaboutsReconcilcerCMName, WhereaboutsReconcilerNamespace)

		By(fmt.Sprintf("Configuring whereabouts reconciler with configmap %q in %q namespace",
			WhereaboutsReconcilcerCMName, WhereaboutsReconcilerNamespace))

		createConfigMap(WhereaboutsReconcilcerCMName, WhereaboutsReconcilerNamespace, map[string]string{
			WhereaboutsReconcilerKey: WhereaboutsReconcilerSchedule,
		})
	}
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
	CreateStatefulsetOnDifferentNode(ctx)

	ensurePodConnectivityAfterNodeDrain(myStatefulsetTwoLabel, RDSCoreConfig.WhereaboutNS,
		RDSCoreConfig.WhereaboutsSTTwoPort, myStatefulsetTwoReplicas, false)
}

// EnsurePodConnectivityOnSameNodeAfterNodeDrain ensures inter pod connectivity
// on the same node after one of the nodes is drained.
func EnsurePodConnectivityOnSameNodeAfterNodeDrain(ctx SpecContext) {
	CreateStatefulsetOnSameNode(ctx)

	ensurePodConnectivityAfterNodeDrain(myStatefulsetOneLabel, RDSCoreConfig.WhereaboutNS,
		RDSCoreConfig.WhereaboutsSTOnePort, myStatefulsetOneReplicas, true)
}

// EnsurePodConnectivityOnSameNodeAfterNodePowerOff ensures inter pod connectivity
// on the same node after one of the nodes is powered off.
func EnsurePodConnectivityOnSameNodeAfterNodePowerOff(ctx SpecContext) {
	CreateStatefulsetOnSameNode(ctx)

	ensurePodConnectivityAfterNodePowerOff(myStatefulsetOneLabel, RDSCoreConfig.WhereaboutNS,
		RDSCoreConfig.WhereaboutsSTOnePort, myStatefulsetOneReplicas, true)
}

// EnsurePodConnectivityBetweenDifferentNodesAfterNodePowerOff ensures inter pod connectivity
// between different nodes after one of the nodes is powered off.
func EnsurePodConnectivityBetweenDifferentNodesAfterNodePowerOff(ctx SpecContext) {
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
