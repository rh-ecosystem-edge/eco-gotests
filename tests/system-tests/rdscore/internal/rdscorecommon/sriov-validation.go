package rdscorecommon

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/rbac"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/serviceaccount"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	srIovV1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	multus "gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"
)

const (
	// SR-IOV operator namespace.
	sriovNS = "openshift-sriov-network-operator"
	// Names of deployments.
	sriovDeploy1OneName = "rdscore-sriov-one"
	sriovDeploy1TwoName = "rdscore-sriov-two"
	sriovDeploy2OneName = "rdscore-sriov2-one"
	sriovDeploy2TwoName = "rdscore-sriov2-two"
	sriovDeploy3OneName = "rdscore-sriov3-one"
	sriovDeploy3TwoName = "rdscore-sriov3-two"
	sriovDeploy4OneName = "rdscore-sriov4-one"
	sriovDeploy4TwoName = "rdscore-sriov4-two"
	// ConfigMap names.
	sriovDeploy1CMName = "rdscore-sriov-config"
	sriovDeploy2CMName = "rdscore-sriov2-config"
	sriovDeploy3CMName = "rdscore-sriov3-config"
	sriovDeploy4CMName = "rdscore-sriov4-config"
	// ServiceAccount names.
	sriovDeploy1SAName = "rdscore-sriov-sa-one"
	sriovDeploy2SAName = "rdscore-sriov-sa-two"
	sriovDeploy3SAName = "rdscore-sriov-sa-3"
	sriovDeploy4SAName = "rdscore-sriov-sa-4"
	// Container names within deployments.
	sriovContainerOneName = "sriov-one"
	sriovContainerTwoName = "sriov-two"
	// Labels for deployments.
	sriovDeployOneLabel  = "rds-core=sriov-deploy-one"
	sriovDeployTwoLabel  = "rds-core=sriov-deploy-two"
	sriovDeploy2OneLabel = "rds-core=sriov-deploy2-one"
	sriovDeploy2TwoLabel = "rds-core=sriov-deploy2-two"
	sriovDeploy3OneLabel = "rds-core=sriov-deploy3-one"
	sriovDeploy3TwoLabel = "rds-core=sriov-deploy3-two"
	sriovDeploy4OneLabel = "rds-core=sriov-deploy4-one"
	sriovDeploy4TwoLabel = "rds-core=sriov-deploy4-two"
	// RBAC names for the deployments.
	sriovDeployRBACName  = "privileged-rdscore-sriov"
	sriovDeployRBACName2 = "privileged-rdscore-sriov2"
	sriovDeployRBACName3 = "privileged-rdscore-sriov3"
	sriovDeployRBACName4 = "privileged-rdscore-sriov4"
	// ClusterRole to use with RBAC.
	sriovRBACRole  = "system:openshift:scc:privileged"
	sriovRBACRole2 = "system:openshift:scc:privileged"
	sriovRBACRole3 = "system:openshift:scc:privileged"
	sriovRBACRole4 = "system:openshift:scc:privileged"
)

func getSRIOVOperatorConfig() (*srIovV1.SriovOperatorConfig, error) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Retrieving SR-IOV Operator config")

	sriovConfigBuiler := sriov.NewOperatorConfigBuilder(APIClient, sriovNS)

	Expect(sriovConfigBuiler).ToNot(BeNil(), "Failed to initialize SR-IOV Operator Config structure")

	var (
		sriovConfig *srIovV1.SriovOperatorConfig
		err         error
	)

	err = wait.PollUntilContextTimeout(context.TODO(),
		5*time.Second,
		1*time.Minute,
		true,
		func(ctx context.Context) (bool, error) {
			var getErr error

			sriovConfig, getErr = sriovConfigBuiler.Get()
			if getErr != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Error retrieving SR-IOV Operator config: %v", getErr)

				return false, nil
			}

			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Retrieved SR-IOV Operator config")

			return true, nil
		})
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Error retrieving SR-IOV Operator config: %v", err)

		return &srIovV1.SriovOperatorConfig{}, err
	}

	return sriovConfig, nil
}

//nolint:unparam
func getSRIOVConfigOption(sriovConfig *srIovV1.SriovOperatorConfig, option string) (bool, bool) {
	var (
		featureEnabled bool
		optionFound    bool
	)

	featureEnabled, optionFound = sriovConfig.Spec.FeatureGates[option]

	if !optionFound {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Feature %q is not defined in the SR-IOV operator config", option)

		return optionFound, optionFound
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Feature %q is defined with value: %v", option, featureEnabled)

	return featureEnabled, optionFound
}

func createServiceAccount(saName, nsName string) {
	By(fmt.Sprintf("Creating ServiceAccount %q in %q namespace",
		saName, nsName))
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Creating SA %q in %q namespace",
		saName, nsName)

	deploySa := serviceaccount.NewBuilder(APIClient, saName, nsName)

	var ctx SpecContext

	Eventually(func() bool {
		deploySa, err := deploySa.Create()
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Error creating SA %q in %q namespace: %v",
				saName, nsName, err)

			return false
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Created SA %q in %q namespace",
			deploySa.Definition.Name, deploySa.Definition.Namespace)

		return true
	}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(1*time.Minute).Should(BeTrue(),
		fmt.Sprintf("Failed to create ServiceAccount %q in %q namespace", saName, nsName))
}

func deleteServiceAccount(saName, nsName string) {
	By("Removing Service Account")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Assert SA %q exists in %q namespace",
		saName, nsName)

	var ctx SpecContext

	if deploySa, err := serviceaccount.Pull(
		APIClient, saName, nsName); err == nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("ServiceAccount %q found in %q namespace",
			saName, nsName)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deleting ServiceAccount %q in %q namespace",
			saName, nsName)

		Eventually(func() bool {
			err := deploySa.Delete()
			if err != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Error deleting ServiceAccount %q in %q namespace: %v",
					saName, nsName, err)

				return false
			}

			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deleted ServiceAccount %q in %q namespace",
				saName, nsName)

			return true
		}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(1*time.Minute).Should(BeTrue(),
			fmt.Sprintf("Failed to delete ServiceAccount %q from %q ns", saName, nsName))
	} else {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("ServiceAccount %q not found in %q namespace",
			saName, nsName)
	}
}

func deleteClusterRBAC(rbacName string) {
	By("Deleting Cluster RBAC")

	var ctx SpecContext

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Assert ClusterRoleBinding %q exists", rbacName)

	if crbSa, err := rbac.PullClusterRoleBinding(
		APIClient,
		rbacName); err == nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("ClusterRoleBinding %q found. Deleting...", rbacName)

		Eventually(func() bool {
			err := crbSa.Delete()
			if err != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Error deleting ClusterRoleBinding %q : %v",
					rbacName, err)

				return false
			}

			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deleted ClusterRoleBinding %q", rbacName)

			return true
		}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(1*time.Minute).Should(BeTrue(),
			"Failed to delete Cluster RBAC")
	}
}

func createClusterRBAC(rbacName, clusterRole, saName, nsName string) {
	By("Creating RBAC for SA")

	var ctx SpecContext

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Creating ClusterRoleBinding %q", rbacName)
	crbSa := rbac.NewClusterRoleBindingBuilder(APIClient,
		rbacName,
		clusterRole,
		rbacv1.Subject{
			Name:      saName,
			Kind:      "ServiceAccount",
			Namespace: nsName,
		})

	Eventually(func() bool {
		crbSa, err := crbSa.Create()
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Error Creating ClusterRoleBinding %q : %v", crbSa.Definition.Name, err)

			return false
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("ClusterRoleBinding %q created:\n\t%v",
			crbSa.Definition.Name, crbSa)

		return true
	}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(1*time.Minute).Should(BeTrue(),
		"Failed to create ClusterRoleBinding")
}

func deleteConfigMap(cmName, nsName string) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Assert ConfigMap %q exists in %q namespace",
		cmName, nsName)

	if cmBuilder, err := configmap.Pull(
		APIClient, cmName, nsName); err == nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("configMap %q found, deleting", cmName)

		var ctx SpecContext

		Eventually(func() bool {
			err := cmBuilder.Delete()
			if err != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Error deleting configMap %q : %v",
					cmName, err)

				return false
			}

			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deleted configMap %q in %q namespace",
				cmName, nsName)

			return true
		}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(1*time.Minute).Should(BeTrue(),
			"Failed to delete configMap")
	}
}

func createConfigMap(cmName, nsName string, data map[string]string) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Create ConfigMap %q in %q namespace",
		cmName, nsName)

	cmBuilder := configmap.NewBuilder(APIClient, cmName, nsName)
	cmBuilder.WithData(data)

	var ctx SpecContext

	Eventually(func() bool {
		cmResult, err := cmBuilder.Create()
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Error creating ConfigMap %q in %q namespace",
				cmName, nsName)

			return false
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Created ConfigMap %q in %q namespace",
			cmResult.Definition.Name, nsName)

		return true
	}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(1*time.Minute).Should(BeTrue(),
		"Failed to crete configMap")
}

func deleteDeployments(dName, nsName string) {
	By(fmt.Sprintf("Removing test deployment %q from %q ns", dName, nsName))

	if deploy, err := deployment.Pull(APIClient, dName, nsName); err == nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deleting deployment %q from %q namespace",
			deploy.Definition.Name, nsName)

		err = deploy.DeleteAndWait(300 * time.Second)
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("failed to delete deployment %q", dName))
	} else {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deployment %q not found in %q namespace",
			dName, nsName)
	}
}

func findPodWithSelector(fNamespace, podLabel string) []*pod.Builder {
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

		if len(podMatchingSelector) == 0 {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Found 0 pods matching label %q in namespace %q",
				podLabel, fNamespace)

			return false
		}

		return true
	}).WithContext(ctx).WithPolling(15*time.Second).WithTimeout(5*time.Minute).Should(BeTrue(),
		fmt.Sprintf("Failed to find pod matching label %q in %q namespace", podLabel, fNamespace))

	return podMatchingSelector
}

func defineContainer(cName, cImage string, cCmd []string, cRequests, cLimits map[string]string) *pod.ContainerBuilder {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Creating container %q", cName)
	deployContainer := pod.NewContainerBuilder(cName, cImage, cCmd)

	By("Defining SecurityContext")

	var trueFlag = true

	userUID := new(int64)

	*userUID = 0

	secContext := &corev1.SecurityContext{
		RunAsUser:  userUID,
		Privileged: &trueFlag,
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
		Capabilities: &corev1.Capabilities{
			Add: []corev1.Capability{"NET_RAW", "NET_ADMIN", "SYS_ADMIN", "IPC_LOCK"},
		},
	}

	By("Setting SecurityContext")

	deployContainer = deployContainer.WithSecurityContext(secContext)
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Container One definition: %#v", deployContainer)

	By("Dropping ALL security capability")

	deployContainer = deployContainer.WithDropSecurityCapabilities([]string{"ALL"}, true)

	By("Adding VolumeMount to container")

	volMount := corev1.VolumeMount{
		Name:      "configs",
		MountPath: "/opt/net/",
		ReadOnly:  false,
	}

	deployContainer = deployContainer.WithVolumeMount(volMount)

	if len(cRequests) != 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Processing container's requests")

		containerRequests := corev1.ResourceList{}

		for key, val := range cRequests {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Parsing container's request: %q - %q", key, val)

			containerRequests[corev1.ResourceName(key)] = resource.MustParse(val)
		}

		deployContainer = deployContainer.WithCustomResourcesRequests(containerRequests)
	}

	if len(cLimits) != 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Processing container's limits")

		containerLimits := corev1.ResourceList{}

		for key, val := range cLimits {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Parsing container's limit: %q - %q", key, val)

			containerLimits[corev1.ResourceName(key)] = resource.MustParse(val)
		}

		deployContainer = deployContainer.WithCustomResourcesLimits(containerLimits)
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("%q container's  definition:\n%#v", cName, deployContainer)

	return deployContainer
}

func defineDeployment(containerConfig *corev1.Container, deployName, deployNs, sriovNet, cmName, saName string,
	deployLabels, nodeSelector map[string]string) *deployment.Builder {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Defining deployment %q in %q ns", deployName, deployNs)

	deploy := deployment.NewBuilder(APIClient, deployName, deployNs, deployLabels, *containerConfig)

	By("Defining SR-IOV annotations")

	var networksOne []*multus.NetworkSelectionElement

	networksOne = append(networksOne,
		&multus.NetworkSelectionElement{
			Name: sriovNet})

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("SR-IOV networks: %#v", networksOne)

	By("Adding SR-IOV annotations")

	deploy = deploy.WithSecondaryNetwork(networksOne)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("SR-IOV deploy one: %#v",
		deploy.Definition.Spec.Template.ObjectMeta.Annotations)

	By("Adding NodeSelector to the deployment")

	deploy = deploy.WithNodeSelector(nodeSelector)

	By("Adding Volume to the deployment")

	volMode := new(int32)
	*volMode = 511

	volDefinition := corev1.Volume{
		Name: "configs",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				DefaultMode: volMode,
				LocalObjectReference: corev1.LocalObjectReference{
					Name: cmName,
				},
			},
		},
	}

	deploy = deploy.WithVolume(volDefinition)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("SR-IOV One Volume:\n %v",
		deploy.Definition.Spec.Template.Spec.Volumes)

	By(fmt.Sprintf("Assigning ServiceAccount %q to the deployment", saName))

	deploy = deploy.WithServiceAccountName(saName)

	By("Setting Replicas count")

	deploy = deploy.WithReplicas(int32(1))

	if len(RDSCoreConfig.WlkdTolerationList) > 0 {
		By("Adding TaintToleration")

		for _, toleration := range RDSCoreConfig.WlkdTolerationList {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Adding toleration: %v", toleration)

			deploy = deploy.WithToleration(toleration)
		}
	}

	return deploy
}

func verifyMsgInPodLogs(podObj *pod.Builder, msg, cName string, timeSpan time.Time) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Parsing duration %q", timeSpan)

	var (
		podLog string
		err    error
		ctx    SpecContext
	)

	Eventually(func() bool {
		logStartTimestamp := time.Since(timeSpan)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("\tTime duration is %s", logStartTimestamp)

		if logStartTimestamp.Abs().Seconds() < 1 {
			logStartTimestamp, err = time.ParseDuration("1s")
			Expect(err).ToNot(HaveOccurred(), "Failed to parse time duration")
		}

		podLog, err = podObj.GetLog(logStartTimestamp, cName)
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to get logs from pod %q: %v", podObj.Definition.Name, err)

			return false
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Logs from pod %s:\n%s", podObj.Definition.Name, podLog)

		return true
	}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(1*time.Minute).Should(BeTrue(),
		fmt.Sprintf("Failed to get logs from pod %q", podObj.Definition.Name))

	Expect(podLog).Should(ContainSubstring(msg))
}

func verifySRIOVConnectivity(nsOneName, nsTwoName, deployOneLabels, deployTwoLabels, targetAddr string) {
	var (
		podOneResult bytes.Buffer
		err          error
		ctx          SpecContext
	)

	By("Getting pods backed by deployment")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Looking for pod(s) matching label %q in %q namespace",
		deployOneLabels, nsOneName)

	podOneList := findPodWithSelector(nsOneName, deployOneLabels)
	Expect(len(podOneList)).To(Equal(1), "Expected only one pod")

	podOne := podOneList[0]
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod one is %q on node %q",
		podOne.Definition.Name, podOne.Definition.Spec.NodeName)

	By(fmt.Sprintf("Waiting for pod %q to get Ready", podOne.Definition.Name))

	err = podOne.WaitUntilReady(3 * time.Minute)

	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Pod %q in %q ns is not Ready",
		podOne.Definition.Name, podOne.Definition.Namespace))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Looking for pod(s) matching label %q in %q namespace",
		deployTwoLabels, nsTwoName)

	podTwoList := findPodWithSelector(nsTwoName, deployTwoLabels)
	Expect(len(podTwoList)).To(Equal(1), "Expected only one pod")

	podTwo := podTwoList[0]
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod two is %q on node %q",
		podTwo.Definition.Name, podTwo.Definition.Spec.NodeName)

	By(fmt.Sprintf("Waiting for pod %q to get Ready", podTwo.Definition.Name))

	err = podTwo.WaitUntilReady(3 * time.Minute)

	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Pod %q in %q ns is not Ready",
		podTwo.Definition.Name, podTwo.Definition.Namespace))

	By("Sending data from pod one to pod two")

	msgOne := fmt.Sprintf("Running from pod %s(%s) at %d",
		podOne.Definition.Name,
		podOne.Definition.Spec.NodeName,
		time.Now().Unix())

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Sending msg %q from pod %s",
		msgOne, podOne.Definition.Name)

	sendDataOneCmd := []string{"/bin/bash", "-c",
		fmt.Sprintf("echo '%s' | nc %s", msgOne, targetAddr)}

	timeStart := time.Now()

	Eventually(func() bool {
		podOneResult, err = podOne.ExecCommand(sendDataOneCmd, podOne.Definition.Spec.Containers[0].Name)
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to run command within pod: %v", sendDataOneCmd)
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to run command within pod: %v", err)

			return false
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Successfully run command %v within container", sendDataOneCmd)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Successfully run command within container %q",
			podOne.Definition.Spec.Containers[0].Name)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Result: %v - %s", podOneResult, &podOneResult)

		return true
	}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(5*time.Minute).Should(BeTrue(),
		fmt.Sprintf("Failed to send data from pod %s", podOne.Definition.Name))

	verifyMsgInPodLogs(podTwo, msgOne, podTwo.Definition.Spec.Containers[0].Name, timeStart)
}

// VerifySRIOVWorkloadsOnSameNode deploy worklods with SRIOV interfaces on the same node
//
//nolint:funlen
func VerifySRIOVWorkloadsOnSameNode(ctx SpecContext) {
	By("Retrieving SR-IOV Operator config")

	SriovOperatorConfig, oerr := getSRIOVOperatorConfig()

	Expect(oerr).ToNot(HaveOccurred(), "Failed to retrieved SR-IOV Operator Config")

	By("Checking resourceInjectorMatchCondition is set")

	optionSet, ok := getSRIOVConfigOption(SriovOperatorConfig, "resourceInjectorMatchCondition")

	if !ok || !optionSet {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'resourceInjectorMatchCondition' not defined or disabled")

		Skip("Option 'resourceInjectorMatchCondition' not defined or enabled")
	}

	By("Checking SR-IOV deployments don't exist")

	deleteDeployments(sriovDeploy1OneName, RDSCoreConfig.WlkdSRIOVOneNS)
	deleteDeployments(sriovDeploy1TwoName, RDSCoreConfig.WlkdSRIOVOneNS)

	By(fmt.Sprintf("Ensuring pods from %q deployment are gone", sriovDeploy1OneName))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Ensuring pods from %q deployment in %q namespace are gone",
		sriovDeploy1OneName, RDSCoreConfig.WlkdSRIOVOneNS)

	Eventually(func() bool {
		oldPods, _ := pod.List(APIClient, RDSCoreConfig.WlkdSRIOVOneNS,
			metav1.ListOptions{LabelSelector: sriovDeployOneLabel})

		return len(oldPods) == 0
	}, 6*time.Minute, 3*time.Second).WithContext(ctx).Should(BeTrue(), "pods matching label() still present")

	By(fmt.Sprintf("Ensuring pods from %q deployment are gone", sriovDeploy1TwoName))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Ensuring pods from %q deployment in %q namespace are gone",
		sriovDeploy1TwoName, RDSCoreConfig.WlkdSRIOVOneNS)

	Eventually(func() bool {
		oldPods, _ := pod.List(APIClient, RDSCoreConfig.WlkdSRIOVOneNS,
			metav1.ListOptions{LabelSelector: sriovDeployTwoLabel})

		return len(oldPods) == 0
	}, 6*time.Minute, 3*time.Second).WithContext(ctx).Should(BeTrue(), "pods matching label() still present")

	By("Removing ConfigMap")

	deleteConfigMap(sriovDeploy1CMName, RDSCoreConfig.WlkdSRIOVOneNS)

	By("Creating ConfigMap")

	createConfigMap(sriovDeploy1CMName,
		RDSCoreConfig.WlkdSRIOVOneNS, RDSCoreConfig.WlkdSRIOVConfigMapDataOne)

	By("Removing ServiceAccount")
	deleteServiceAccount(sriovDeploy1SAName, RDSCoreConfig.WlkdSRIOVOneNS)

	By("Creating ServiceAccount")
	createServiceAccount(sriovDeploy1SAName, RDSCoreConfig.WlkdSRIOVOneNS)

	By("Remoing Cluster RBAC")
	deleteClusterRBAC(sriovDeployRBACName)

	By("Creating Cluster RBAC")
	createClusterRBAC(sriovDeployRBACName, sriovRBACRole,
		sriovDeploy1SAName, RDSCoreConfig.WlkdSRIOVOneNS)

	By("Defining container configuration")

	deployContainer := defineContainer(sriovContainerOneName, RDSCoreConfig.WlkdSRIOVDeployOneImage,
		RDSCoreConfig.WlkdSRIOVDeployOneCmd, RDSCoreConfig.WldkSRIOVDeployOneResRequests,
		RDSCoreConfig.WldkSRIOVDeployOneResLimits)

	deployContainerTwo := defineContainer(sriovContainerTwoName, RDSCoreConfig.WlkdSRIOVDeployTwoImage,
		RDSCoreConfig.WlkdSRIOVDeployTwoCmd, RDSCoreConfig.WldkSRIOVDeployTwoResRequests,
		RDSCoreConfig.WldkSRIOVDeployTwoResLimits)

	By("Obtaining container definition")

	deployContainerCfg, err := deployContainer.GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to get container definition")

	deployContainerTwoCfg, err := deployContainerTwo.GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to get container definition")

	By("Defining 1st deployment configuration")

	deployOneLabels := map[string]string{
		strings.Split(sriovDeployOneLabel, "=")[0]: strings.Split(sriovDeployOneLabel, "=")[1]}

	deploy := defineDeployment(deployContainerCfg,
		sriovDeploy1OneName,
		RDSCoreConfig.WlkdSRIOVOneNS,
		RDSCoreConfig.WlkdSRIOVNetOne,
		sriovDeploy1CMName,
		sriovDeploy1SAName,
		deployOneLabels,
		RDSCoreConfig.WlkdSRIOVDeployOneSelector)

	By("Creating deployment one")

	deploy, err = deploy.CreateAndWaitUntilReady(5 * time.Minute)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create deployment %s: %v", sriovDeploy1OneName, err))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deployment %q created in %q namespace",
		deploy.Definition.Name, deploy.Definition.Namespace)

	By("Defining 2nd deployment")

	deployTwoLabels := map[string]string{
		strings.Split(sriovDeployTwoLabel, "=")[0]: strings.Split(sriovDeployTwoLabel, "=")[1]}

	deployTwo := defineDeployment(deployContainerTwoCfg,
		sriovDeploy1TwoName,
		RDSCoreConfig.WlkdSRIOVOneNS,
		RDSCoreConfig.WlkdSRIOVNetOne,
		sriovDeploy1CMName,
		sriovDeploy1SAName,
		deployTwoLabels,
		RDSCoreConfig.WlkdSRIOVDeployOneSelector)

	By("Creating 2nd deployment")

	deployTwo, err = deployTwo.CreateAndWaitUntilReady(5 * time.Minute)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create deployment %s: %v", sriovDeploy1TwoName, err))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deployment %q created in %q namespace",
		deployTwo.Definition.Name, deployTwo.Definition.Namespace)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Verify connectivity between SR-IOV workloads on the same node")

	addressesList := []string{RDSCoreConfig.WlkdSRIOVDeployOneTargetAddress,
		RDSCoreConfig.WlkdSRIOVDeployOneTargetAddressIPv6}

	for _, targetAddress := range addressesList {
		if targetAddress == "" {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Skipping empty address %q", targetAddress)

			continue
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Access workload via %q", targetAddress)

		verifySRIOVConnectivity(
			RDSCoreConfig.WlkdSRIOVOneNS,
			RDSCoreConfig.WlkdSRIOVOneNS,
			sriovDeployOneLabel,
			sriovDeployTwoLabel,
			targetAddress)
	}

	addressesList = []string{RDSCoreConfig.WlkdSRIOVDeployTwoTargetAddress,
		RDSCoreConfig.WlkdSRIOVDeployTwoTargetAddressIPv6}

	for _, targetAddress := range addressesList {
		if targetAddress == "" {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Skipping empty address %q", targetAddress)

			continue
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Access workload via %q", targetAddress)

		verifySRIOVConnectivity(
			RDSCoreConfig.WlkdSRIOVOneNS,
			RDSCoreConfig.WlkdSRIOVOneNS,
			sriovDeployTwoLabel,
			sriovDeployOneLabel,
			targetAddress)
	}
}

// VerifySRIOVWorkloadsOnDifferentNodes deploy worklods with SRIOV interfaces on the same node
// Test config:
//
//	Same SR-IOV network
//	Same Namespace
//	Different nodes
//
//nolint:funlen
func VerifySRIOVWorkloadsOnDifferentNodes(ctx SpecContext) {
	if strings.TrimSpace(RDSCoreConfig.WlkdSRIOVNet21) == "" || strings.TrimSpace(RDSCoreConfig.WlkdSRIOVNet22) == "" {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("SR-IOV networks cannot be empty")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("SRIOV Network 1: %s", RDSCoreConfig.WlkdSRIOVNet21)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("SRIOV Network 2: %s", RDSCoreConfig.WlkdSRIOVNet22)

		Skip("SR-IOV networks cannot be empty")
	}

	if strings.TrimSpace(RDSCoreConfig.WlkdSRIOVNet21) != strings.TrimSpace(RDSCoreConfig.WlkdSRIOVNet22) {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("SR-IOV networks are not the same")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("SRIOV Network 1: %s", RDSCoreConfig.WlkdSRIOVNet21)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("SRIOV Network 2: %s", RDSCoreConfig.WlkdSRIOVNet22)

		Skip("SR-IOV networks are not the same")
	}

	By("Retrieving SR-IOV Operator config")

	SriovOperatorConfig, oerr := getSRIOVOperatorConfig()

	Expect(oerr).ToNot(HaveOccurred(), "Failed to retrieved SR-IOV Operator Config")

	By("Checking resourceInjectorMatchCondition is set")

	optionSet, ok := getSRIOVConfigOption(SriovOperatorConfig, "resourceInjectorMatchCondition")

	if !ok || !optionSet {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'resourceInjectorMatchCondition' not defined or disabled")

		Skip("Option 'resourceInjectorMatchCondition' not defined or enabled")
	}

	By("Checking SR-IOV deployments don't exist")

	deleteDeployments(sriovDeploy2OneName, RDSCoreConfig.WlkdSRIOVOneNS)
	deleteDeployments(sriovDeploy2TwoName, RDSCoreConfig.WlkdSRIOVOneNS)

	By(fmt.Sprintf("Ensuring pods from %q deployment are gone", sriovDeploy2OneName))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Ensuring pods from %q deployment in %q namespace are gone",
		sriovDeploy2OneName, RDSCoreConfig.WlkdSRIOVOneNS)

	Eventually(func() bool {
		oldPods, _ := pod.List(APIClient, RDSCoreConfig.WlkdSRIOVOneNS,
			metav1.ListOptions{LabelSelector: sriovDeploy2OneLabel})

		return len(oldPods) == 0
	}, 6*time.Minute, 3*time.Second).WithContext(ctx).Should(BeTrue(), "pods matching label() still present")

	By(fmt.Sprintf("Ensuring pods from %q deployment are gone", sriovDeploy2TwoName))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Ensuring pods from %q deployment in %q namespace are gone",
		sriovDeploy2TwoName, RDSCoreConfig.WlkdSRIOVOneNS)

	Eventually(func() bool {
		oldPods, _ := pod.List(APIClient, RDSCoreConfig.WlkdSRIOVOneNS,
			metav1.ListOptions{LabelSelector: sriovDeploy2TwoLabel})

		return len(oldPods) == 0
	}, 6*time.Minute, 3*time.Second).WithContext(ctx).Should(BeTrue(), "pods matching label() still present")

	By("Removing ConfigMap")

	deleteConfigMap(sriovDeploy2CMName, RDSCoreConfig.WlkdSRIOVOneNS)

	By("Creating ConfigMap")

	createConfigMap(sriovDeploy2CMName,
		RDSCoreConfig.WlkdSRIOVOneNS, RDSCoreConfig.WlkdSRIOVConfigMapDataOne)

	By("Removing ServiceAccount")
	deleteServiceAccount(sriovDeploy2SAName, RDSCoreConfig.WlkdSRIOVOneNS)

	By("Creating ServiceAccount")
	createServiceAccount(sriovDeploy2SAName, RDSCoreConfig.WlkdSRIOVOneNS)

	By("Remoing Cluster RBAC")
	deleteClusterRBAC(sriovDeployRBACName2)

	By("Creating Cluster RBAC")
	createClusterRBAC(sriovDeployRBACName2, sriovRBACRole2,
		sriovDeploy2SAName, RDSCoreConfig.WlkdSRIOVOneNS)

	By("Defining container configuration")

	deployContainer := defineContainer(sriovContainerOneName, RDSCoreConfig.WlkdSRIOVDeployOneImage,
		RDSCoreConfig.WlkdSRIOVDeploy2OneCmd, RDSCoreConfig.WldkSRIOVDeployOneResRequests,
		RDSCoreConfig.WldkSRIOVDeployOneResLimits)

	deployContainerTwo := defineContainer(sriovContainerTwoName, RDSCoreConfig.WlkdSRIOVDeployTwoImage,
		RDSCoreConfig.WlkdSRIOVDeploy2TwoCmd, RDSCoreConfig.WldkSRIOVDeployOneResRequests,
		RDSCoreConfig.WldkSRIOVDeployOneResLimits)

	By("Obtaining container definition")

	deployContainerCfg, err := deployContainer.GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to get container definition")

	deployContainerTwoCfg, err := deployContainerTwo.GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to get container definition")

	By("Defining 1st deployment configuration")

	deployOneLabels := map[string]string{
		strings.Split(sriovDeploy2OneLabel, "=")[0]: strings.Split(sriovDeploy2OneLabel, "=")[1]}

	deploy := defineDeployment(deployContainerCfg,
		sriovDeploy2OneName,
		RDSCoreConfig.WlkdSRIOVOneNS,
		RDSCoreConfig.WlkdSRIOVNet21,
		sriovDeploy2CMName,
		sriovDeploy2SAName,
		deployOneLabels,
		RDSCoreConfig.WlkdSRIOVDeployOneSelector)

	By("Creating deployment one")

	deploy, err = deploy.CreateAndWaitUntilReady(5 * time.Minute)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create deployment %s: %v", sriovDeploy2OneName, err))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deployment %q in %q ns is Ready",
		deploy.Definition.Name, deploy.Definition.Namespace)

	By("Defining 2nd deployment")

	deployTwoLabels := map[string]string{
		strings.Split(sriovDeploy2TwoLabel, "=")[0]: strings.Split(sriovDeploy2TwoLabel, "=")[1]}

	deployTwo := defineDeployment(deployContainerTwoCfg,
		sriovDeploy2TwoName,
		RDSCoreConfig.WlkdSRIOVOneNS,
		RDSCoreConfig.WlkdSRIOVNet22,
		sriovDeploy2CMName,
		sriovDeploy2SAName,
		deployTwoLabels,
		RDSCoreConfig.WlkdSRIOVDeployTwoSelector)

	By("Creating 2nd deployment")

	deployTwo, err = deployTwo.CreateAndWaitUntilReady(5 * time.Minute)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create deployment %s: %v", sriovDeploy2TwoName, err))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deployment %q in %q ns is Ready",
		deployTwo.Definition.Name, deployTwo.Definition.Namespace)

	addressesList := []string{RDSCoreConfig.WlkdSRIOVDeploy2OneTargetAddress,
		RDSCoreConfig.WlkdSRIOVDeploy2OneTargetAddressIPv6}

	for _, targetAddress := range addressesList {
		if targetAddress == "" {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Skipping empty address %q", targetAddress)

			continue
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Access workload via %q", targetAddress)

		verifySRIOVConnectivity(
			RDSCoreConfig.WlkdSRIOVOneNS,
			RDSCoreConfig.WlkdSRIOVOneNS,
			sriovDeploy2OneLabel,
			sriovDeploy2TwoLabel,
			targetAddress)
	}

	addressesList = []string{RDSCoreConfig.WlkdSRIOVDeploy2TwoTargetAddress,
		RDSCoreConfig.WlkdSRIOVDeploy2TwoTargetAddressIPv6}

	for _, targetAddress := range addressesList {
		if targetAddress == "" {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Skipping empty address %q", targetAddress)

			continue
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Access workload via %q", targetAddress)

		verifySRIOVConnectivity(
			RDSCoreConfig.WlkdSRIOVOneNS,
			RDSCoreConfig.WlkdSRIOVOneNS,
			sriovDeploy2TwoLabel,
			sriovDeploy2OneLabel,
			targetAddress)
	}
}

// VerifySRIOVConnectivityBetweenDifferentNodes test connectivity after cluster's reboot.
func VerifySRIOVConnectivityBetweenDifferentNodes(ctx SpecContext) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Verify connectivity between SR-IOV workloads on different node")

	addressesList := []string{RDSCoreConfig.WlkdSRIOVDeploy2OneTargetAddress,
		RDSCoreConfig.WlkdSRIOVDeploy2OneTargetAddressIPv6}

	for _, targetAddress := range addressesList {
		if targetAddress == "" {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Skipping empty address %q", targetAddress)

			continue
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Access workload via %q", targetAddress)

		verifySRIOVConnectivity(
			RDSCoreConfig.WlkdSRIOVOneNS,
			RDSCoreConfig.WlkdSRIOVOneNS,
			sriovDeploy2OneLabel,
			sriovDeploy2TwoLabel,
			targetAddress)
	}

	addressesList = []string{RDSCoreConfig.WlkdSRIOVDeploy2TwoTargetAddress,
		RDSCoreConfig.WlkdSRIOVDeploy2TwoTargetAddressIPv6}

	for _, targetAddress := range addressesList {
		if targetAddress == "" {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Skipping empty address %q", targetAddress)

			continue
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Access workload via %q", targetAddress)

		verifySRIOVConnectivity(
			RDSCoreConfig.WlkdSRIOVOneNS,
			RDSCoreConfig.WlkdSRIOVOneNS,
			sriovDeploy2TwoLabel,
			sriovDeploy2OneLabel,
			targetAddress)
	}
}

// VerifySRIOVConnectivityOnSameNode tests connectivity after cluster's reboot.
func VerifySRIOVConnectivityOnSameNode(ctx SpecContext) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Verify connectivity between SR-IOV workloads on the same node")

	addressesList := []string{RDSCoreConfig.WlkdSRIOVDeployOneTargetAddress,
		RDSCoreConfig.WlkdSRIOVDeployOneTargetAddressIPv6}

	for _, targetAddress := range addressesList {
		if targetAddress == "" {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Skipping empty address %q", targetAddress)

			continue
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Access workload via %q", targetAddress)

		verifySRIOVConnectivity(
			RDSCoreConfig.WlkdSRIOVOneNS,
			RDSCoreConfig.WlkdSRIOVOneNS,
			sriovDeployOneLabel,
			sriovDeployTwoLabel,
			targetAddress)
	}

	addressesList = []string{RDSCoreConfig.WlkdSRIOVDeployTwoTargetAddress,
		RDSCoreConfig.WlkdSRIOVDeployTwoTargetAddressIPv6}

	for _, targetAddress := range addressesList {
		if targetAddress == "" {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Skipping empty address %q", targetAddress)

			continue
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Access workload via %q", targetAddress)

		verifySRIOVConnectivity(
			RDSCoreConfig.WlkdSRIOVOneNS,
			RDSCoreConfig.WlkdSRIOVOneNS,
			sriovDeployTwoLabel,
			sriovDeployOneLabel,
			targetAddress)
	}
}

// VerifySRIOVConnectivityOnSameNodeAndDifferentNets verifies connectivity between workloads
// scheduled on the same node and on different SR-IOV networks.
func VerifySRIOVConnectivityOnSameNodeAndDifferentNets() {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Verify connectivity between SR-IOV workloads on the same node and different SR-IOV networks")

	addressesList := []string{RDSCoreConfig.WlkdSRIOVDeploy3OneTargetAddress,
		RDSCoreConfig.WlkdSRIOVDeploy3OneTargetAddressIPv6}

	for _, targetAddress := range addressesList {
		if targetAddress == "" {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Skipping empty address %q", targetAddress)

			continue
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Access workload via %q", targetAddress)

		verifySRIOVConnectivity(
			RDSCoreConfig.WlkdSRIOV3NS,
			RDSCoreConfig.WlkdSRIOV3NS,
			sriovDeploy3OneLabel,
			sriovDeploy3TwoLabel,
			targetAddress)
	}

	addressesList = []string{RDSCoreConfig.WlkdSRIOVDeploy3TwoTargetAddress,
		RDSCoreConfig.WlkdSRIOVDeploy3TwoTargetAddressIPv6}

	for _, targetAddress := range addressesList {
		if targetAddress == "" {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Skipping empty address %q", targetAddress)

			continue
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Access workload via %q", targetAddress)

		verifySRIOVConnectivity(
			RDSCoreConfig.WlkdSRIOV3NS,
			RDSCoreConfig.WlkdSRIOV3NS,
			sriovDeploy3TwoLabel,
			sriovDeploy3OneLabel,
			targetAddress)
	}
}

// VerifySRIOVWorkloadsOnSameNodeDifferentNet deploy worklods with different SRIOV networks on the same node
//
//nolint:funlen
func VerifySRIOVWorkloadsOnSameNodeDifferentNet(ctx SpecContext) {
	if strings.TrimSpace(RDSCoreConfig.WlkdSRIOVNet31) == "" || strings.TrimSpace(RDSCoreConfig.WlkdSRIOVNet32) == "" {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("SR-IOV networks cannot be empty")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("SRIOV Network 1: %s", RDSCoreConfig.WlkdSRIOVNet31)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("SRIOV Network 2: %s", RDSCoreConfig.WlkdSRIOVNet32)

		Skip("SR-IOV networks cannot be empty")
	}

	if strings.TrimSpace(RDSCoreConfig.WlkdSRIOVNet31) == strings.TrimSpace(RDSCoreConfig.WlkdSRIOVNet32) {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("SR-IOV networks are the same but should be different")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("SRIOV Network 1: %s", RDSCoreConfig.WlkdSRIOVNet31)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("SRIOV Network 2: %s", RDSCoreConfig.WlkdSRIOVNet32)

		Skip("SR-IOV networks are the same but should be different")
	}

	By("Retrieving SR-IOV Operator config")

	SriovOperatorConfig, oerr := getSRIOVOperatorConfig()

	Expect(oerr).ToNot(HaveOccurred(), "Failed to retrieved SR-IOV Operator Config")

	By("Checking resourceInjectorMatchCondition is set")

	optionSet, ok := getSRIOVConfigOption(SriovOperatorConfig, "resourceInjectorMatchCondition")

	if !ok || !optionSet {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'resourceInjectorMatchCondition' not defined or disabled")

		Skip("Option 'resourceInjectorMatchCondition' not defined or enabled")
	}

	By("Checking SR-IOV deployments don't exist")

	deleteDeployments(sriovDeploy3OneName, RDSCoreConfig.WlkdSRIOV3NS)
	deleteDeployments(sriovDeploy3TwoName, RDSCoreConfig.WlkdSRIOV3NS)

	By(fmt.Sprintf("Ensuring pods from %q deployment are gone", sriovDeploy3OneName))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Ensuring pods from %q deployment in %q namespace are gone",
		sriovDeploy3OneName, RDSCoreConfig.WlkdSRIOV3NS)

	Eventually(func() bool {
		oldPods, _ := pod.List(APIClient, RDSCoreConfig.WlkdSRIOV3NS,
			metav1.ListOptions{LabelSelector: sriovDeploy3OneLabel})

		return len(oldPods) == 0
	}).WithContext(ctx).WithPolling(3*time.Second).WithTimeout(6*time.Minute).Should(BeTrue(),
		"pods matching label() still present")

	By(fmt.Sprintf("Ensuring pods from %q deployment are gone", sriovDeploy3TwoName))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Ensuring pods from %q deployment in %q namespace are gone",
		sriovDeploy3TwoName, RDSCoreConfig.WlkdSRIOV3NS)

	Eventually(func() bool {
		oldPods, _ := pod.List(APIClient, RDSCoreConfig.WlkdSRIOV3NS,
			metav1.ListOptions{LabelSelector: sriovDeploy3TwoLabel})

		return len(oldPods) == 0
	}).WithContext(ctx).WithPolling(3*time.Second).WithTimeout(6*time.Minute).Should(BeTrue(),
		"pods matching label() still present")

	By("Removing ConfigMap")

	deleteConfigMap(sriovDeploy3CMName, RDSCoreConfig.WlkdSRIOV3NS)

	By("Creating ConfigMap")

	createConfigMap(sriovDeploy3CMName,
		RDSCoreConfig.WlkdSRIOV3NS, RDSCoreConfig.WlkdSRIOVConfigMapData3)

	By("Removing ServiceAccount")

	deleteServiceAccount(sriovDeploy3SAName, RDSCoreConfig.WlkdSRIOV3NS)

	By("Creating ServiceAccount")

	createServiceAccount(sriovDeploy3SAName, RDSCoreConfig.WlkdSRIOV3NS)

	By("Removing Cluster RBAC")

	deleteClusterRBAC(sriovDeployRBACName3)

	By("Creating Cluster RBAC")

	createClusterRBAC(sriovDeployRBACName3, sriovRBACRole3,
		sriovDeploy3SAName, RDSCoreConfig.WlkdSRIOV3NS)

	By("Defining container configuration")

	deployContainer := defineContainer(sriovContainerOneName, RDSCoreConfig.WlkdSRIOVDeploy3Image,
		RDSCoreConfig.WlkdSRIOVDeploy3OneCmd, RDSCoreConfig.WldkSRIOVDeploy3OneResRequests,
		RDSCoreConfig.WldkSRIOVDeploy3OneResLimits)

	deployContainerTwo := defineContainer(sriovContainerTwoName, RDSCoreConfig.WlkdSRIOVDeploy3Image,
		RDSCoreConfig.WlkdSRIOVDeploy3TwoCmd, RDSCoreConfig.WldkSRIOVDeploy3TwoResRequests,
		RDSCoreConfig.WldkSRIOVDeploy3TwoResLimits)

	By("Obtaining container definition")

	deployContainerCfg, err := deployContainer.GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to get container definition")

	deployContainerTwoCfg, err := deployContainerTwo.GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to get container definition")

	By("Defining 1st deployment configuration")

	deployOneLabels := map[string]string{
		strings.Split(sriovDeploy3OneLabel, "=")[0]: strings.Split(sriovDeploy3OneLabel, "=")[1]}

	deploy := defineDeployment(deployContainerCfg,
		sriovDeploy3OneName,
		RDSCoreConfig.WlkdSRIOV3NS,
		RDSCoreConfig.WlkdSRIOVNet31,
		sriovDeploy3CMName,
		sriovDeploy3SAName,
		deployOneLabels,
		RDSCoreConfig.WlkdSRIOVDeploy3OneSelector)

	By("Creating deployment one")

	deploy, err = deploy.CreateAndWaitUntilReady(5 * time.Minute)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create deployment %s: %v", sriovDeploy3OneName, err))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deployment %q created in %q namespace",
		deploy.Definition.Name, deploy.Definition.Namespace)

	By("Defining 2nd deployment")

	deployTwoLabels := map[string]string{
		strings.Split(sriovDeploy3TwoLabel, "=")[0]: strings.Split(sriovDeploy3TwoLabel, "=")[1]}

	deployTwo := defineDeployment(deployContainerTwoCfg,
		sriovDeploy3TwoName,
		RDSCoreConfig.WlkdSRIOV3NS,
		RDSCoreConfig.WlkdSRIOVNet32,
		sriovDeploy3CMName,
		sriovDeploy3SAName,
		deployTwoLabels,
		RDSCoreConfig.WlkdSRIOVDeploy3OneSelector)

	By("Creating 2nd deployment")

	deployTwo, err = deployTwo.CreateAndWaitUntilReady(5 * time.Minute)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create deployment %s: %v", sriovDeploy3TwoName, err))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deployment %q created in %q namespace",
		deployTwo.Definition.Name, deployTwo.Definition.Namespace)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Verify connectivity between SR-IOV workloads on the same node")

	addressesList := []string{RDSCoreConfig.WlkdSRIOVDeploy3OneTargetAddress,
		RDSCoreConfig.WlkdSRIOVDeploy3OneTargetAddressIPv6}

	for _, targetAddress := range addressesList {
		if targetAddress == "" {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Skipping empty address %q", targetAddress)

			continue
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Access workload via %q", targetAddress)

		verifySRIOVConnectivity(
			RDSCoreConfig.WlkdSRIOV3NS,
			RDSCoreConfig.WlkdSRIOV3NS,
			sriovDeploy3OneLabel,
			sriovDeploy3TwoLabel,
			targetAddress)
	}

	addressesList = []string{RDSCoreConfig.WlkdSRIOVDeploy3TwoTargetAddress,
		RDSCoreConfig.WlkdSRIOVDeploy3TwoTargetAddressIPv6}

	for _, targetAddress := range addressesList {
		if targetAddress == "" {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Skipping empty address %q", targetAddress)

			continue
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Access workload via %q", targetAddress)

		verifySRIOVConnectivity(
			RDSCoreConfig.WlkdSRIOV3NS,
			RDSCoreConfig.WlkdSRIOV3NS,
			sriovDeploy3TwoLabel,
			sriovDeploy3OneLabel,
			targetAddress)
	}
}

// VerifySRIOVConnectivityOnDifferentNodesAndDifferentNetworks verifies connectivity between workloads
// running on different SR-IOV networks and different nodes.
func VerifySRIOVConnectivityOnDifferentNodesAndDifferentNetworks() {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Verify connectivity between SR-IOV workloads on different nodes and different SR-IOV networks")

	addressesList := []string{RDSCoreConfig.WlkdSRIOVDeploy4OneTargetAddress,
		RDSCoreConfig.WlkdSRIOVDeploy4OneTargetAddressIPv6}

	for _, targetAddress := range addressesList {
		if targetAddress == "" {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Skipping empty address %q", targetAddress)

			continue
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Access workload via %q", targetAddress)

		verifySRIOVConnectivity(
			RDSCoreConfig.WlkdSRIOV4NS,
			RDSCoreConfig.WlkdSRIOV4NS,
			sriovDeploy4OneLabel,
			sriovDeploy4TwoLabel,
			targetAddress)
	}

	addressesList = []string{RDSCoreConfig.WlkdSRIOVDeploy4TwoTargetAddress,
		RDSCoreConfig.WlkdSRIOVDeploy4TwoTargetAddressIPv6}

	for _, targetAddress := range addressesList {
		if targetAddress == "" {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Skipping empty address %q", targetAddress)

			continue
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Access workload via %q", targetAddress)

		verifySRIOVConnectivity(
			RDSCoreConfig.WlkdSRIOV4NS,
			RDSCoreConfig.WlkdSRIOV4NS,
			sriovDeploy4TwoLabel,
			sriovDeploy4OneLabel,
			targetAddress)
	}
}

// VerifySRIOVWorkloadsOnDifferentNodesDifferentNet deploy worklods with different SRIOV networks on different nodes
//
//nolint:funlen
func VerifySRIOVWorkloadsOnDifferentNodesDifferentNet(ctx SpecContext) {
	if strings.TrimSpace(RDSCoreConfig.WlkdSRIOVNet41) == "" || strings.TrimSpace(RDSCoreConfig.WlkdSRIOVNet42) == "" {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("SR-IOV networks cannot be empty")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("SRIOV Network 1: %s", RDSCoreConfig.WlkdSRIOVNet41)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("SRIOV Network 2: %s", RDSCoreConfig.WlkdSRIOVNet42)

		Skip("SR-IOV networks cannot be empty")
	}

	if strings.TrimSpace(RDSCoreConfig.WlkdSRIOVNet41) == strings.TrimSpace(RDSCoreConfig.WlkdSRIOVNet42) {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("SR-IOV networks are the same but should be different")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("SRIOV Network 1: %s", RDSCoreConfig.WlkdSRIOVNet41)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("SRIOV Network 2: %s", RDSCoreConfig.WlkdSRIOVNet42)

		Skip("SR-IOV networks are the same but should be different")
	}

	By("Retrieving SR-IOV Operator config")

	SriovOperatorConfig, oerr := getSRIOVOperatorConfig()

	Expect(oerr).ToNot(HaveOccurred(), "Failed to retrieved SR-IOV Operator Config")

	By("Checking resourceInjectorMatchCondition is set")

	optionSet, ok := getSRIOVConfigOption(SriovOperatorConfig, "resourceInjectorMatchCondition")

	if !ok || !optionSet {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'resourceInjectorMatchCondition' not defined or disabled")

		Skip("Option 'resourceInjectorMatchCondition' not defined or enabled")
	}

	By("Checking SR-IOV deployments don't exist")

	deleteDeployments(sriovDeploy4OneName, RDSCoreConfig.WlkdSRIOV4NS)
	deleteDeployments(sriovDeploy4TwoName, RDSCoreConfig.WlkdSRIOV4NS)

	By(fmt.Sprintf("Ensuring pods from %q deployment are gone", sriovDeploy4OneName))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Ensuring pods from %q deployment in %q namespace are gone",
		sriovDeploy4OneName, RDSCoreConfig.WlkdSRIOV4NS)

	Eventually(func() bool {
		oldPods, _ := pod.List(APIClient, RDSCoreConfig.WlkdSRIOV4NS,
			metav1.ListOptions{LabelSelector: sriovDeploy4OneLabel})

		return len(oldPods) == 0
	}).WithContext(ctx).WithPolling(3*time.Second).WithTimeout(6*time.Minute).Should(BeTrue(),
		"pods matching label() still present")

	By(fmt.Sprintf("Ensuring pods from %q deployment are gone", sriovDeploy4TwoName))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Ensuring pods from %q deployment in %q namespace are gone",
		sriovDeploy4TwoName, RDSCoreConfig.WlkdSRIOV4NS)

	Eventually(func() bool {
		oldPods, _ := pod.List(APIClient, RDSCoreConfig.WlkdSRIOV4NS,
			metav1.ListOptions{LabelSelector: sriovDeploy4TwoLabel})

		return len(oldPods) == 0
	}).WithContext(ctx).WithPolling(3*time.Second).WithTimeout(6*time.Minute).Should(BeTrue(),
		"pods matching label() still present")

	By("Removing ConfigMap")

	deleteConfigMap(sriovDeploy4CMName, RDSCoreConfig.WlkdSRIOV4NS)

	By("Creating ConfigMap")

	createConfigMap(sriovDeploy4CMName,
		RDSCoreConfig.WlkdSRIOV4NS, RDSCoreConfig.WlkdSRIOVConfigMapData4)

	By("Removing ServiceAccount")

	deleteServiceAccount(sriovDeploy4SAName, RDSCoreConfig.WlkdSRIOV4NS)

	By("Creating ServiceAccount")

	createServiceAccount(sriovDeploy4SAName, RDSCoreConfig.WlkdSRIOV4NS)

	By("Removing Cluster RBAC")

	deleteClusterRBAC(sriovDeployRBACName4)

	By("Creating Cluster RBAC")

	createClusterRBAC(sriovDeployRBACName4, sriovRBACRole4,
		sriovDeploy4SAName, RDSCoreConfig.WlkdSRIOV4NS)

	By("Defining container configuration")

	deployContainer := defineContainer(sriovContainerOneName, RDSCoreConfig.WlkdSRIOVDeploy4Image,
		RDSCoreConfig.WlkdSRIOVDeploy4OneCmd, RDSCoreConfig.WldkSRIOVDeploy4OneResRequests,
		RDSCoreConfig.WldkSRIOVDeploy4OneResLimits)

	deployContainerTwo := defineContainer(sriovContainerTwoName, RDSCoreConfig.WlkdSRIOVDeploy4Image,
		RDSCoreConfig.WlkdSRIOVDeploy4TwoCmd, RDSCoreConfig.WldkSRIOVDeploy4TwoResRequests,
		RDSCoreConfig.WldkSRIOVDeploy4TwoResLimits)

	By("Obtaining container definition")

	deployContainerCfg, err := deployContainer.GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to get container definition")

	deployContainerTwoCfg, err := deployContainerTwo.GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to get container definition")

	By("Defining 1st deployment configuration")

	deployOneLabels := map[string]string{
		strings.Split(sriovDeploy4OneLabel, "=")[0]: strings.Split(sriovDeploy4OneLabel, "=")[1]}

	deploy := defineDeployment(deployContainerCfg,
		sriovDeploy4OneName,
		RDSCoreConfig.WlkdSRIOV4NS,
		RDSCoreConfig.WlkdSRIOVNet41,
		sriovDeploy4CMName,
		sriovDeploy4SAName,
		deployOneLabels,
		RDSCoreConfig.WlkdSRIOVDeploy4OneSelector)

	By("Creating deployment one")

	deploy, err = deploy.CreateAndWaitUntilReady(5 * time.Minute)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create deployment %s: %v", sriovDeploy4OneName, err))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deployment %q created in %q namespace",
		deploy.Definition.Name, deploy.Definition.Namespace)

	By("Defining 2nd deployment")

	deployTwoLabels := map[string]string{
		strings.Split(sriovDeploy4TwoLabel, "=")[0]: strings.Split(sriovDeploy4TwoLabel, "=")[1]}

	deployTwo := defineDeployment(deployContainerTwoCfg,
		sriovDeploy4TwoName,
		RDSCoreConfig.WlkdSRIOV4NS,
		RDSCoreConfig.WlkdSRIOVNet42,
		sriovDeploy4CMName,
		sriovDeploy4SAName,
		deployTwoLabels,
		RDSCoreConfig.WlkdSRIOVDeploy4TwoSelector)

	By("Creating 2nd deployment")

	deployTwo, err = deployTwo.CreateAndWaitUntilReady(5 * time.Minute)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create deployment %s: %v", sriovDeploy4TwoName, err))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deployment %q created in %q namespace",
		deployTwo.Definition.Name, deployTwo.Definition.Namespace)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Verify connectivity between SR-IOV workloads on the same node")

	addressesList := []string{RDSCoreConfig.WlkdSRIOVDeploy4OneTargetAddress,
		RDSCoreConfig.WlkdSRIOVDeploy4OneTargetAddressIPv6}

	for _, targetAddress := range addressesList {
		if targetAddress == "" {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Skipping empty address %q", targetAddress)

			continue
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Access workload via %q", targetAddress)

		verifySRIOVConnectivity(
			RDSCoreConfig.WlkdSRIOV4NS,
			RDSCoreConfig.WlkdSRIOV4NS,
			sriovDeploy4OneLabel,
			sriovDeploy4TwoLabel,
			targetAddress)
	}

	addressesList = []string{RDSCoreConfig.WlkdSRIOVDeploy4TwoTargetAddress,
		RDSCoreConfig.WlkdSRIOVDeploy4TwoTargetAddressIPv6}

	for _, targetAddress := range addressesList {
		if targetAddress == "" {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Skipping empty address %q", targetAddress)

			continue
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Access workload via %q", targetAddress)

		verifySRIOVConnectivity(
			RDSCoreConfig.WlkdSRIOV4NS,
			RDSCoreConfig.WlkdSRIOV4NS,
			sriovDeploy4TwoLabel,
			sriovDeploy4OneLabel,
			targetAddress)
	}
}

// VerifySRIOVSuite container that contains tests for SR-IOV verification.
func VerifySRIOVSuite() {
	Describe(
		"SR-IOV verification",
		Label(rdscoreparams.LabelValidateSRIOV), func() {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("*******************************************")
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("*** Starting SR-IOV RDS Core Test Suite ***")
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("*******************************************")

			It("Verifices SR-IOV workloads on the same node",
				Label("sriov-same-node"), reportxml.ID("71949"), MustPassRepeatedly(3),
				VerifySRIOVWorkloadsOnSameNode)

			It("Verifices SR-IOV workloads on different nodes",
				Label("sriov-different-node"), reportxml.ID("71950"), MustPassRepeatedly(3),
				VerifySRIOVWorkloadsOnDifferentNodes)
		})
}
