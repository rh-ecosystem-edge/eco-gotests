package rdscorecommon

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"
	multus "gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
)

const (
	// Names of deployments.
	macvlanDeploy10Name = "rdscore-mcvlan1-one"
	macvlanDeploy11Name = "rdscore-mcvlan1-two"
	macvlanDeploy20Name = "rdscore-mcvlan2-one"
	macvlanDeploy21Name = "rdscore-mcvlan2-two"
	macvlanDeploy30Name = "rdscore-mcvlan3-one"
	macvlanDeploy31Name = "rdscore-mcvlan3-two"
	macvlanDeploy40Name = "rdscore-mcvlan4-one"
	macvlanDeploy41Name = "rdscore-mcvlan4-two"
	// ConfigMap names.
	macvlanDeploy1CMName = "rdscore-mcvlan1-config"
	macvlanDeploy2CMName = "rdscore-mcvlan2-config"
	macvlanDeploy3CMName = "rdscore-mcvlan3-config"
	macvlanDeploy4CMName = "rdscore-mcvlan4-config"
	// ServiceAccount names.
	macvlanDeploy1SAName = "rdscore-mcvlan-sa-one"
	macvlanDeploy2SAName = "rdscore-mcvlan-sa-two"
	macvlanDeploy3SAName = "rdscore-mcvlan-sa-three"
	macvlanDeploy4SAName = "rdscore-mcvlan-sa-four"
	// Container names within deployments.
	macvlanContainer1Name = "macvlan-one"
	macvlanContainer2Name = "macvlan-two"
	macvlanContainer3Name = "macvlan-three"
	macvlanContainer4Name = "macvlan-four"
	// Labels for deployments.
	macvlanDeploy10Label = "rdscore=mcvlan-deploy-one"
	macvlanDeploy11Label = "rdscore=mcvlan-deploy-two"
	macvlanDeploy20Label = "rdscore=mcvlan-deploy2-one"
	macvlanDeploy21Label = "rdscore=mcvlan-deploy2-two"
	macvlanDeploy30Label = "rdscore=mcvlan-deploy3-one"
	macvlanDeploy31Label = "rdscore=mcvlan-deploy3-two"
	macvlanDeploy40Label = "rdscore=mcvlan-deploy4-one"
	macvlanDeploy41Label = "rdscore=mcvlan-deploy4-two"
	// RBAC names for the deployments.
	macvlanDeployRBACName1 = "privileged-rdscore-mcvlan1"
	macvlanDeployRBACName2 = "privileged-rdscore-mcvlan2"
	macvlanDeployRBACName3 = "privileged-rdscore-mcvlan3"
	macvlanDeployRBACName4 = "privileged-rdscore-mcvlan4"
	// ClusterRole to use with RBAC.
	macvlanRBACRole1 = "system:openshift:scc:privileged"
	macvlanRBACRole2 = "system:openshift:scc:privileged"
	macvlanRBACRole3 = "system:openshift:scc:privileged"
	macvlanRBACRole4 = "system:openshift:scc:privileged"
)

func assertPodsAreGone(fNamespace, podLabel string) {
	By(fmt.Sprintf("Getting pod(s) matching selector %q", podLabel))

	var (
		podMatchingSelector []*pod.Builder
		err                 error
		ctx                 SpecContext
	)

	podOneSelector := metav1.ListOptions{
		LabelSelector: podLabel,
	}

	Eventually(func() bool {
		podMatchingSelector, err = pod.List(APIClient, fNamespace, podOneSelector)
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to list pods in %q namespace: %v",
				fNamespace, err)

			return false
		}

		if len(podMatchingSelector) != 0 {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Found %d pods matching label %q in namespace %q",
				len(podMatchingSelector), podLabel, fNamespace)

			return false
		}

		return true
	}).WithContext(ctx).WithPolling(15*time.Second).WithTimeout(5*time.Minute).Should(BeTrue(),
		fmt.Sprintf("Pods matching label %q still exist in %q namespace", podLabel, fNamespace))
}

// VerifyMacVlanOnDifferentNodes verifies connectivity between freshly deployed workloads that use
// same MACVLAN definition and are scheduled on different nodes.
//
//nolint:funlen
func VerifyMacVlanOnDifferentNodes() {
	deleteConfigMap(macvlanDeploy1CMName, RDSCoreConfig.MCVlanNSOne)
	deleteConfigMap(macvlanDeploy2CMName, RDSCoreConfig.MCVlanNSOne)

	createConfigMap(macvlanDeploy1CMName, RDSCoreConfig.MCVlanNSOne, RDSCoreConfig.MCVlanCMDataOne)
	createConfigMap(macvlanDeploy2CMName, RDSCoreConfig.MCVlanNSOne, RDSCoreConfig.MCVlanCMDataOne)

	deleteClusterRBAC(macvlanDeployRBACName1)
	deleteClusterRBAC(macvlanDeployRBACName2)

	deleteDeployments(macvlanDeploy10Name, RDSCoreConfig.MCVlanNSOne)
	deleteDeployments(macvlanDeploy11Name, RDSCoreConfig.MCVlanNSOne)

	assertPodsAreGone(RDSCoreConfig.MCVlanNSOne, macvlanDeploy10Label)
	assertPodsAreGone(RDSCoreConfig.MCVlanNSOne, macvlanDeploy11Label)

	deleteServiceAccount(macvlanDeploy1SAName, RDSCoreConfig.MCVlanNSOne)
	deleteServiceAccount(macvlanDeploy2SAName, RDSCoreConfig.MCVlanNSOne)

	By("Defining container configuration")

	mvOne := defineContainer(macvlanContainer1Name,
		RDSCoreConfig.MCVlanDeployImageOne, RDSCoreConfig.MCVlanDeplonOneCMD, map[string]string{}, map[string]string{})

	mvTwo := defineContainer(macvlanContainer2Name,
		RDSCoreConfig.MCVlanDeployImageOne, RDSCoreConfig.MCVlanDeplonTwoCMD, map[string]string{}, map[string]string{})

	By("Obtaining container definition")

	deployContainerCfg, err := mvOne.GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to get container definition")

	deployContainerTwoCfg, err := mvTwo.GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to get container definition")

	By("Defining deployment configuration")

	deployOne := defineMacVlanDeployment(macvlanDeploy10Name,
		RDSCoreConfig.MCVlanNSOne,
		macvlanDeploy10Label,
		RDSCoreConfig.MCVlanNADOneName,
		macvlanDeploy1CMName,
		deployContainerCfg,
		RDSCoreConfig.MCVlanDeployNodeSelectorOne)

	deployTwo := defineMacVlanDeployment(macvlanDeploy11Name,
		RDSCoreConfig.MCVlanNSOne,
		macvlanDeploy11Label,
		RDSCoreConfig.MCVlanNADOneName,
		macvlanDeploy2CMName,
		deployContainerTwoCfg,
		RDSCoreConfig.MCVlanDeployNodeSelectorTwo)

	By("Creating ServiceAccount for the deployment")

	createServiceAccount(macvlanDeploy1SAName, RDSCoreConfig.MCVlanNSOne)
	createServiceAccount(macvlanDeploy2SAName, RDSCoreConfig.MCVlanNSOne)

	By("Creating RBAC for SA")

	createClusterRBAC(macvlanDeployRBACName1, macvlanRBACRole1, macvlanDeploy1SAName, RDSCoreConfig.MCVlanNSOne)
	createClusterRBAC(macvlanDeployRBACName2, macvlanRBACRole2, macvlanDeploy2SAName, RDSCoreConfig.MCVlanNSOne)

	By("Assigning ServiceAccount to the deployment")

	deployOne = deployOne.WithServiceAccountName(macvlanDeploy1SAName)
	deployTwo = deployTwo.WithServiceAccountName(macvlanDeploy2SAName)

	if len(RDSCoreConfig.WlkdTolerationList) > 0 {
		By("Adding TaintToleration")

		for _, toleration := range RDSCoreConfig.WlkdTolerationList {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Adding toleration: %v", toleration)

			deployOne = deployOne.WithToleration(toleration)
			deployTwo = deployTwo.WithToleration(toleration)
		}
	}

	By("Creating a deployment")

	deployOne, err = deployOne.CreateAndWaitUntilReady(5 * time.Minute)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create deployment %s: %v", macvlanDeploy10Name, err))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deployment %q created and is Ready in %q namespace",
		deployOne.Definition.Name, deployOne.Definition.Namespace)

	deployTwo, err = deployTwo.CreateAndWaitUntilReady(5 * time.Minute)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create deployment %s: %v", macvlanDeploy11Name, err))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deployment %q created and is Ready in %q namespace",
		deployTwo.Definition.Name, deployTwo.Definition.Namespace)

	VerifyMACVLANConnectivityBetweenDifferentNodes()
}

// VerifyMACVLANConnectivityBetweenDifferentNodes verifies connectivity between workloads,
// using MACVLAN interfaces and running on different nodes.
func VerifyMACVLANConnectivityBetweenDifferentNodes() {
	verifySRIOVConnectivity(
		RDSCoreConfig.MCVlanNSOne,
		RDSCoreConfig.MCVlanNSOne,
		macvlanDeploy10Label,
		macvlanDeploy11Label,
		RDSCoreConfig.MCVlanDeploy1TargetAddress)

	verifySRIOVConnectivity(
		RDSCoreConfig.MCVlanNSOne,
		RDSCoreConfig.MCVlanNSOne,
		macvlanDeploy11Label,
		macvlanDeploy10Label,
		RDSCoreConfig.MCVlanDeploy2TargetAddress)

	verifySRIOVConnectivity(
		RDSCoreConfig.MCVlanNSOne,
		RDSCoreConfig.MCVlanNSOne,
		macvlanDeploy10Label,
		macvlanDeploy11Label,
		RDSCoreConfig.MCVlanDeploy1TargetAddressIPv6)

	verifySRIOVConnectivity(
		RDSCoreConfig.MCVlanNSOne,
		RDSCoreConfig.MCVlanNSOne,
		macvlanDeploy11Label,
		macvlanDeploy10Label,
		RDSCoreConfig.MCVlanDeploy2TargetAddressIPv6)
}

// VerifyMacVlanOnSameNode verifies connectivity between freshly deployed workloads that use
// same MACVLAN definition and are scheduled to the same node.
//
//nolint:funlen
func VerifyMacVlanOnSameNode() {
	deleteConfigMap(macvlanDeploy3CMName, RDSCoreConfig.MCVlanNSOne)
	deleteConfigMap(macvlanDeploy4CMName, RDSCoreConfig.MCVlanNSOne)

	createConfigMap(macvlanDeploy3CMName, RDSCoreConfig.MCVlanNSOne, RDSCoreConfig.MCVlanCMDataOne)
	createConfigMap(macvlanDeploy4CMName, RDSCoreConfig.MCVlanNSOne, RDSCoreConfig.MCVlanCMDataOne)

	deleteClusterRBAC(macvlanDeployRBACName3)
	deleteClusterRBAC(macvlanDeployRBACName4)

	deleteDeployments(macvlanDeploy20Name, RDSCoreConfig.MCVlanNSOne)
	deleteDeployments(macvlanDeploy21Name, RDSCoreConfig.MCVlanNSOne)

	assertPodsAreGone(RDSCoreConfig.MCVlanNSOne, macvlanDeploy20Label)
	assertPodsAreGone(RDSCoreConfig.MCVlanNSOne, macvlanDeploy21Label)

	deleteServiceAccount(macvlanDeploy3SAName, RDSCoreConfig.MCVlanNSOne)
	deleteServiceAccount(macvlanDeploy4SAName, RDSCoreConfig.MCVlanNSOne)

	By("Defining container configuration")

	mvOne := defineContainer(macvlanContainer3Name,
		RDSCoreConfig.MCVlanDeployImageOne, RDSCoreConfig.MCVlanDeplon3CMD, map[string]string{}, map[string]string{})

	mvTwo := defineContainer(macvlanContainer4Name,
		RDSCoreConfig.MCVlanDeployImageOne, RDSCoreConfig.MCVlanDeplon4CMD, map[string]string{}, map[string]string{})

	By("Obtaining container definition")

	deployContainerCfg, err := mvOne.GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to get container definition")

	deployContainerTwoCfg, err := mvTwo.GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to get container definition")

	By("Defining deployment configuration")

	deployOne := defineMacVlanDeployment(macvlanDeploy20Name,
		RDSCoreConfig.MCVlanNSOne,
		macvlanDeploy20Label,
		RDSCoreConfig.MCVlanNADOneName,
		macvlanDeploy3CMName,
		deployContainerCfg,
		RDSCoreConfig.MCVlanDeployNodeSelectorOne)

	deployTwo := defineMacVlanDeployment(macvlanDeploy21Name,
		RDSCoreConfig.MCVlanNSOne,
		macvlanDeploy21Label,
		RDSCoreConfig.MCVlanNADOneName,
		macvlanDeploy4CMName,
		deployContainerTwoCfg,
		RDSCoreConfig.MCVlanDeployNodeSelectorOne)

	By("Creating ServiceAccount for the deployment")

	createServiceAccount(macvlanDeploy3SAName, RDSCoreConfig.MCVlanNSOne)
	createServiceAccount(macvlanDeploy4SAName, RDSCoreConfig.MCVlanNSOne)

	By("Creating RBAC for SA")

	createClusterRBAC(macvlanDeployRBACName3, macvlanRBACRole3, macvlanDeploy3SAName, RDSCoreConfig.MCVlanNSOne)
	createClusterRBAC(macvlanDeployRBACName4, macvlanRBACRole4, macvlanDeploy4SAName, RDSCoreConfig.MCVlanNSOne)

	By("Assigning ServiceAccount to the deployment")

	deployOne = deployOne.WithServiceAccountName(macvlanDeploy3SAName)
	deployTwo = deployTwo.WithServiceAccountName(macvlanDeploy4SAName)

	if len(RDSCoreConfig.WlkdTolerationList) > 0 {
		By("Adding TaintToleration")

		for _, toleration := range RDSCoreConfig.WlkdTolerationList {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Adding toleration: %v", toleration)

			deployOne = deployOne.WithToleration(toleration)
			deployTwo = deployTwo.WithToleration(toleration)
		}
	}

	By("Creating a deployment")

	deployOne, err = deployOne.CreateAndWaitUntilReady(5 * time.Minute)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create deployment %s: %v", macvlanDeploy20Name, err))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deployment %q created and is Ready in %q namespace",
		deployOne.Definition.Name, deployOne.Definition.Namespace)

	deployTwo, err = deployTwo.CreateAndWaitUntilReady(5 * time.Minute)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create deployment %s: %v", macvlanDeploy21Name, err))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deployment %q created and is Ready in %q namespace",
		deployTwo.Definition.Name, deployTwo.Definition.Namespace)

	VerifyMACVLANConnectivityOnSameNode()
}

// VerifyMACVLANConnectivityOnSameNode verifies connectivity between workloads that use MACVLAN net.
func VerifyMACVLANConnectivityOnSameNode() {
	verifySRIOVConnectivity(
		RDSCoreConfig.MCVlanNSOne,
		RDSCoreConfig.MCVlanNSOne,
		macvlanDeploy20Label,
		macvlanDeploy21Label,
		RDSCoreConfig.MCVlanDeploy3TargetAddress)

	verifySRIOVConnectivity(
		RDSCoreConfig.MCVlanNSOne,
		RDSCoreConfig.MCVlanNSOne,
		macvlanDeploy21Label,
		macvlanDeploy20Label,
		RDSCoreConfig.MCVlanDeploy4TargetAddress)

	verifySRIOVConnectivity(
		RDSCoreConfig.MCVlanNSOne,
		RDSCoreConfig.MCVlanNSOne,
		macvlanDeploy20Label,
		macvlanDeploy21Label,
		RDSCoreConfig.MCVlanDeploy3TargetAddressIPv6)

	verifySRIOVConnectivity(
		RDSCoreConfig.MCVlanNSOne,
		RDSCoreConfig.MCVlanNSOne,
		macvlanDeploy21Label,
		macvlanDeploy20Label,
		RDSCoreConfig.MCVlanDeploy4TargetAddressIPv6)
}

func defineMacVlanDeployment(dName, nsName, dLabels, netDefName, volName string,
	dContainer *corev1.Container,
	nodeSelector map[string]string) *deployment.Builder {
	By("Defining deployment configuration")

	deploy := deployment.NewBuilder(APIClient,
		dName,
		nsName,
		map[string]string{strings.Split(dLabels, "=")[0]: strings.Split(dLabels, "=")[1]},
		*dContainer)

	By("Adding MACVLAN annotations")

	var networks []*multus.NetworkSelectionElement

	networks = append(networks,
		&multus.NetworkSelectionElement{
			Name:      netDefName,
			Namespace: nsName})

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("MACVlan networks:\n\t%#v", networks)

	deploy = deploy.WithSecondaryNetwork(networks)

	if len(nodeSelector) > 0 {
		By("Adding NodeSelector to the deployment")

		deploy = deploy.WithNodeSelector(nodeSelector)
	}

	By("Adding Volume to the deployment")

	volMode := new(int32)
	*volMode = 511

	volDefinition := corev1.Volume{
		Name: "configs",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				DefaultMode: volMode,
				LocalObjectReference: corev1.LocalObjectReference{
					Name: volName,
				},
			},
		},
	}

	deploy = deploy.WithVolume(volDefinition)

	By("Setting Replicas count")

	deploy = deploy.WithReplicas(int32(1))

	return deploy
}
