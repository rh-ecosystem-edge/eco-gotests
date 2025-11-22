package rdscorecommon

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"
	multus "gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

const (
	// Names of deployments.
	ipvlanDeploy10Name = "rdscore-ipvlan1-one"
	ipvlanDeploy11Name = "rdscore-ipvlan1-two"
	ipvlanDeploy20Name = "rdscore-ipvlan2-one"
	ipvlanDeploy21Name = "rdscore-ipvlan2-two"
	// ConfigMap names.
	ipvlanDeploy1CMName = "rdscore-ipvlan1-config"
	ipvlanDeploy2CMName = "rdscore-ipvlan2-config"
	ipvlanDeploy3CMName = "rdscore-ipvlan3-config"
	ipvlanDeploy4CMName = "rdscore-ipvlan4-config"
	// ServiceAccount names.
	ipvlanDeploy1SAName = "rdscore-ipvlan-sa-one"
	ipvlanDeploy2SAName = "rdscore-ipvlan-sa-two"
	ipvlanDeploy3SAName = "rdscore-ipvlan-sa-three"
	ipvlanDeploy4SAName = "rdscore-ipvlan-sa-four"
	// Container names within deployments.
	ipvlanContainer1Name = "ipvlan-one"
	ipvlanContainer2Name = "ipvlan-two"
	ipvlanContainer3Name = "ipvlan-three"
	ipvlanContainer4Name = "ipvlan-four"
	// Labels for deployments.
	ipvlanDeploy10Label = "rdscore=ipvlan-deploy-one"
	ipvlanDeploy11Label = "rdscore=ipvlan-deploy-two"
	ipvlanDeploy20Label = "rdscore=ipvlan-deploy2-one"
	ipvlanDeploy21Label = "rdscore=ipvlan-deploy2-two"
	// RBAC names for the deployments.
	ipvlanDeployRBACName1 = "privileged-rdscore-ipvlan1"
	ipvlanDeployRBACName2 = "privileged-rdscore-ipvlan2"
	ipvlanDeployRBACName3 = "privileged-rdscore-ipvlan3"
	ipvlanDeployRBACName4 = "privileged-rdscore-ipvlan4"
	// ClusterRole to use with RBAC.
	ipvlanRBACRole1 = "system:openshift:scc:privileged"
	ipvlanRBACRole2 = "system:openshift:scc:privileged"
	ipvlanRBACRole3 = "system:openshift:scc:privileged"
	ipvlanRBACRole4 = "system:openshift:scc:privileged"
)

// VerifyIPVlanOnDifferentNodes verifies connectivity between freshly deployed workloads that use
// same IPVLAN definition and are scheduled on different nodes.
//
//nolint:funlen
func VerifyIPVlanOnDifferentNodes() {
	deleteConfigMap(ipvlanDeploy1CMName, RDSCoreConfig.IPVlanNSOne)
	deleteConfigMap(ipvlanDeploy2CMName, RDSCoreConfig.IPVlanNSOne)

	createConfigMap(ipvlanDeploy1CMName, RDSCoreConfig.IPVlanNSOne, RDSCoreConfig.IPVlanCMDataOne)
	createConfigMap(ipvlanDeploy2CMName, RDSCoreConfig.IPVlanNSOne, RDSCoreConfig.IPVlanCMDataOne)

	deleteClusterRBAC(ipvlanDeployRBACName1)
	deleteClusterRBAC(ipvlanDeployRBACName2)

	deleteDeployments(ipvlanDeploy10Name, RDSCoreConfig.IPVlanNSOne)
	deleteDeployments(ipvlanDeploy11Name, RDSCoreConfig.IPVlanNSOne)

	assertPodsAreGone(RDSCoreConfig.IPVlanNSOne, ipvlanDeploy10Label)
	assertPodsAreGone(RDSCoreConfig.IPVlanNSOne, ipvlanDeploy11Label)

	deleteServiceAccount(ipvlanDeploy1SAName, RDSCoreConfig.IPVlanNSOne)
	deleteServiceAccount(ipvlanDeploy2SAName, RDSCoreConfig.IPVlanNSOne)

	By("Defining container configuration")

	ivOne := defineContainer(ipvlanContainer1Name,
		RDSCoreConfig.IPVlanDeployImageOne, RDSCoreConfig.IPVlanDeplonOneCMD, map[string]string{}, map[string]string{})

	ivTwo := defineContainer(ipvlanContainer2Name,
		RDSCoreConfig.IPVlanDeployImageOne, RDSCoreConfig.IPVlanDeplonTwoCMD, map[string]string{}, map[string]string{})

	By("Obtaining container definition")

	deployContainerCfg, err := ivOne.GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to get container definition")

	deployContainerTwoCfg, err := ivTwo.GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to get container definition")

	By("Defining deployment configuration")

	deployOne := defineIPVlanDeployment(ipvlanDeploy10Name,
		RDSCoreConfig.IPVlanNSOne,
		ipvlanDeploy10Label,
		RDSCoreConfig.IPVlanNADOneName,
		ipvlanDeploy1CMName,
		deployContainerCfg,
		RDSCoreConfig.IPVlanDeployNodeSelectorOne)

	deployTwo := defineIPVlanDeployment(ipvlanDeploy11Name,
		RDSCoreConfig.IPVlanNSOne,
		ipvlanDeploy11Label,
		RDSCoreConfig.IPVlanNADTwoName,
		ipvlanDeploy2CMName,
		deployContainerTwoCfg,
		RDSCoreConfig.IPVlanDeployNodeSelectorTwo)

	By("Creating ServiceAccount for the deployment")

	createServiceAccount(ipvlanDeploy1SAName, RDSCoreConfig.IPVlanNSOne)
	createServiceAccount(ipvlanDeploy2SAName, RDSCoreConfig.IPVlanNSOne)

	By("Creating RBAC for SA")

	createClusterRBAC(ipvlanDeployRBACName1, ipvlanRBACRole1, ipvlanDeploy1SAName, RDSCoreConfig.IPVlanNSOne)
	createClusterRBAC(ipvlanDeployRBACName2, ipvlanRBACRole2, ipvlanDeploy2SAName, RDSCoreConfig.IPVlanNSOne)

	By("Assigning ServiceAccount to the deployment")

	deployOne = deployOne.WithServiceAccountName(ipvlanDeploy1SAName)
	deployTwo = deployTwo.WithServiceAccountName(ipvlanDeploy2SAName)

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
		fmt.Sprintf("Failed to create deployment %s: %v", ipvlanDeploy10Name, err))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deployment %q created and is Ready in %q namespace",
		deployOne.Definition.Name, deployOne.Definition.Namespace)

	deployTwo, err = deployTwo.CreateAndWaitUntilReady(5 * time.Minute)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create deployment %s: %v", ipvlanDeploy11Name, err))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deployment %q created and is Ready in %q namespace",
		deployTwo.Definition.Name, deployTwo.Definition.Namespace)

	VerifyIPVLANConnectivityBetweenDifferentNodes()
}

// VerifyIPVLANConnectivityBetweenDifferentNodes verifies connectivity between workloads,
// using IPVLAN interfaces and running on different nodes.
func VerifyIPVLANConnectivityBetweenDifferentNodes() {
	verifySRIOVConnectivity(
		RDSCoreConfig.IPVlanNSOne,
		RDSCoreConfig.IPVlanNSOne,
		ipvlanDeploy10Label,
		ipvlanDeploy11Label,
		RDSCoreConfig.IPVlanDeploy1TargetAddress)

	verifySRIOVConnectivity(
		RDSCoreConfig.IPVlanNSOne,
		RDSCoreConfig.IPVlanNSOne,
		ipvlanDeploy11Label,
		ipvlanDeploy10Label,
		RDSCoreConfig.IPVlanDeploy2TargetAddress)

	verifySRIOVConnectivity(
		RDSCoreConfig.IPVlanNSOne,
		RDSCoreConfig.IPVlanNSOne,
		ipvlanDeploy10Label,
		ipvlanDeploy11Label,
		RDSCoreConfig.IPVlanDeploy1TargetAddressIPv6)

	verifySRIOVConnectivity(
		RDSCoreConfig.IPVlanNSOne,
		RDSCoreConfig.IPVlanNSOne,
		ipvlanDeploy11Label,
		ipvlanDeploy10Label,
		RDSCoreConfig.IPVlanDeploy2TargetAddressIPv6)
}

// VerifyIPVlanOnSameNode verifies connectivity between freshly deployed workloads that use
// same IPVLAN definition and are scheduled to the same node.
//
//nolint:funlen
func VerifyIPVlanOnSameNode() {
	deleteConfigMap(ipvlanDeploy3CMName, RDSCoreConfig.IPVlanNSOne)
	deleteConfigMap(ipvlanDeploy4CMName, RDSCoreConfig.IPVlanNSOne)

	createConfigMap(ipvlanDeploy3CMName, RDSCoreConfig.IPVlanNSOne, RDSCoreConfig.IPVlanCMDataOne)
	createConfigMap(ipvlanDeploy4CMName, RDSCoreConfig.IPVlanNSOne, RDSCoreConfig.IPVlanCMDataOne)

	deleteClusterRBAC(ipvlanDeployRBACName3)
	deleteClusterRBAC(ipvlanDeployRBACName4)

	deleteDeployments(ipvlanDeploy20Name, RDSCoreConfig.IPVlanNSOne)
	deleteDeployments(ipvlanDeploy21Name, RDSCoreConfig.IPVlanNSOne)

	assertPodsAreGone(RDSCoreConfig.IPVlanNSOne, ipvlanDeploy20Label)
	assertPodsAreGone(RDSCoreConfig.IPVlanNSOne, ipvlanDeploy21Label)

	deleteServiceAccount(ipvlanDeploy3SAName, RDSCoreConfig.IPVlanNSOne)
	deleteServiceAccount(ipvlanDeploy4SAName, RDSCoreConfig.IPVlanNSOne)

	By("Defining container configuration")

	ivOne := defineContainer(ipvlanContainer3Name,
		RDSCoreConfig.IPVlanDeployImageOne, RDSCoreConfig.IPVlanDeplon3CMD, map[string]string{}, map[string]string{})

	ivTwo := defineContainer(ipvlanContainer4Name,
		RDSCoreConfig.IPVlanDeployImageOne, RDSCoreConfig.IPVlanDeplon4CMD, map[string]string{}, map[string]string{})

	By("Obtaining container definition")

	deployContainerCfg, err := ivOne.GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to get container definition")

	deployContainerTwoCfg, err := ivTwo.GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to get container definition")

	By("Defining deployment configuration")

	deployOne := defineIPVlanDeployment(ipvlanDeploy20Name,
		RDSCoreConfig.IPVlanNSOne,
		ipvlanDeploy20Label,
		RDSCoreConfig.IPVlanNADThreeName,
		ipvlanDeploy3CMName,
		deployContainerCfg,
		RDSCoreConfig.IPVlanDeployNodeSelectorOne)

	deployTwo := defineIPVlanDeployment(ipvlanDeploy21Name,
		RDSCoreConfig.IPVlanNSOne,
		ipvlanDeploy21Label,
		RDSCoreConfig.IPVlanNADFourName,
		ipvlanDeploy4CMName,
		deployContainerTwoCfg,
		RDSCoreConfig.IPVlanDeployNodeSelectorOne)

	By("Creating ServiceAccount for the deployment")

	createServiceAccount(ipvlanDeploy3SAName, RDSCoreConfig.IPVlanNSOne)
	createServiceAccount(ipvlanDeploy4SAName, RDSCoreConfig.IPVlanNSOne)

	By("Creating RBAC for SA")

	createClusterRBAC(ipvlanDeployRBACName3, ipvlanRBACRole3, ipvlanDeploy3SAName, RDSCoreConfig.IPVlanNSOne)
	createClusterRBAC(ipvlanDeployRBACName4, ipvlanRBACRole4, ipvlanDeploy4SAName, RDSCoreConfig.IPVlanNSOne)

	By("Assigning ServiceAccount to the deployment")

	deployOne = deployOne.WithServiceAccountName(ipvlanDeploy3SAName)
	deployTwo = deployTwo.WithServiceAccountName(ipvlanDeploy4SAName)

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
		fmt.Sprintf("Failed to create deployment %s: %v", ipvlanDeploy20Name, err))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deployment %q created and is Ready in %q namespace",
		deployOne.Definition.Name, deployOne.Definition.Namespace)

	deployTwo, err = deployTwo.CreateAndWaitUntilReady(5 * time.Minute)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create deployment %s: %v", ipvlanDeploy21Name, err))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deployment %q created and is Ready in %q namespace",
		deployTwo.Definition.Name, deployTwo.Definition.Namespace)

	VerifyIPVLANConnectivityOnSameNode()
}

// VerifyIPVLANConnectivityOnSameNode verifies connectivity between workloads that use IPVLAN net.
func VerifyIPVLANConnectivityOnSameNode() {
	verifySRIOVConnectivity(
		RDSCoreConfig.IPVlanNSOne,
		RDSCoreConfig.IPVlanNSOne,
		ipvlanDeploy20Label,
		ipvlanDeploy21Label,
		RDSCoreConfig.IPVlanDeploy3TargetAddress)

	verifySRIOVConnectivity(
		RDSCoreConfig.IPVlanNSOne,
		RDSCoreConfig.IPVlanNSOne,
		ipvlanDeploy21Label,
		ipvlanDeploy20Label,
		RDSCoreConfig.IPVlanDeploy4TargetAddress)

	verifySRIOVConnectivity(
		RDSCoreConfig.IPVlanNSOne,
		RDSCoreConfig.IPVlanNSOne,
		ipvlanDeploy20Label,
		ipvlanDeploy21Label,
		RDSCoreConfig.IPVlanDeploy3TargetAddressIPv6)

	verifySRIOVConnectivity(
		RDSCoreConfig.IPVlanNSOne,
		RDSCoreConfig.IPVlanNSOne,
		ipvlanDeploy21Label,
		ipvlanDeploy20Label,
		RDSCoreConfig.IPVlanDeploy4TargetAddressIPv6)
}

func defineIPVlanDeployment(dName, nsName, dLabels, netDefName, volName string,
	dContainer *corev1.Container,
	nodeSelector map[string]string) *deployment.Builder {
	By("Defining deployment configuration")

	deploy := deployment.NewBuilder(APIClient,
		dName,
		nsName,
		map[string]string{strings.Split(dLabels, "=")[0]: strings.Split(dLabels, "=")[1]},
		*dContainer)

	By("Adding IPVLAN annotations")

	var networks []*multus.NetworkSelectionElement

	networks = append(networks,
		&multus.NetworkSelectionElement{
			Name:      netDefName,
			Namespace: nsName})

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("IPVlan networks:\n\t%#v", networks)

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
