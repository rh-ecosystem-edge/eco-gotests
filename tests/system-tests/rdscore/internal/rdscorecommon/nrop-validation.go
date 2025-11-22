package rdscorecommon

import (
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"
)

const (
	// Names of deployments.
	nropDeploy1Name = "rdscore-nrop-one"
	nropDeploy2Name = "rdscore-nrop-two"
	// Container names within deployments.
	nropContainerOneName = "nrop-one"
	nropContainerTwoName = "nrop-two"
	// Labels for deployments.
	nropDeployOneLabel = "rds-core-nrop=nrop-deploy-one"
	nropDeployTwoLabel = "rds-core-nrop=nrop-deploy-two"
)

func defineNROPContainer(cName, cImage string, cCmd []string,
	cRequests, cLimits map[string]string) *pod.ContainerBuilder {
	nContainer := pod.NewContainerBuilder(cName, cImage, cCmd)

	if len(cRequests) != 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Processing container's requests")

		containerRequests := corev1.ResourceList{}

		for key, val := range cRequests {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Parsing container's request: %q - %q", key, val)

			containerRequests[corev1.ResourceName(key)] = resource.MustParse(val)
		}

		nContainer = nContainer.WithCustomResourcesRequests(containerRequests)
	}

	if len(cLimits) != 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Processing container's limits")

		containerLimits := corev1.ResourceList{}

		for key, val := range cLimits {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Parsing container's limit: %q - %q", key, val)

			containerLimits[corev1.ResourceName(key)] = resource.MustParse(val)
		}

		nContainer = nContainer.WithCustomResourcesLimits(containerLimits)
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("%q container's  definition:\n%#v", cName, nContainer)

	return nContainer
}

func defineNROPDeployment(containerConfig *corev1.Container, deployName, deployNs, deployScheduler string,
	deployLabels, nodeSelector map[string]string) *deployment.Builder {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Defining deployment %q in %q ns", deployName, deployNs)

	By("Defining NROP deployment")

	deployNROP := deployment.NewBuilder(APIClient, deployName, deployNs, deployLabels, *containerConfig)

	By("Adding NodeSelector to the deployment")

	deployNROP = deployNROP.WithNodeSelector(nodeSelector)

	By("Setting Replicas count")

	deployNROP = deployNROP.WithReplicas(int32(1))

	By("Setting Scheduler")

	deployNROP = deployNROP.WithSchedulerName(deployScheduler)

	if len(RDSCoreConfig.WlkdNROPTolerationList) > 0 {
		By("Adding TaintToleration")

		for _, toleration := range RDSCoreConfig.WlkdNROPTolerationList {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Adding toleration: %v", toleration)

			deployNROP = deployNROP.WithToleration(toleration)
		}
	}

	return deployNROP
}

// VerifyNROPWorkload deploys workload with NROP scheduler.
func VerifyNROPWorkload(ctx SpecContext) {
	By("Checking NROP deployment doesn't exist")

	deleteDeployments(nropDeploy1Name, RDSCoreConfig.WlkdNROPOneNS)

	By(fmt.Sprintf("Ensuring pods from %q deployment are gone", sriovDeploy1OneName))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Ensuring pods from %q deployment in %q namespace are gone",
		nropDeploy1Name, RDSCoreConfig.WlkdNROPOneNS)

	Eventually(func() bool {
		oldPods, _ := pod.List(APIClient, RDSCoreConfig.WlkdNROPOneNS,
			metav1.ListOptions{LabelSelector: nropDeployOneLabel})

		return len(oldPods) == 0
	}).WithContext(ctx).WithPolling(3*time.Second).WithTimeout(1*time.Minute).Should(
		BeTrue(), "pods matching label() still present")

	By("Defining container configuration")

	deployContainer := defineNROPContainer(nropContainerOneName, RDSCoreConfig.WlkdNROPDeployOneImage,
		RDSCoreConfig.WlkdNROPDeployOneCmd, RDSCoreConfig.WldkNROPDeployOneResRequests,
		RDSCoreConfig.WldkNROPDeployOneResLimits)

	By("Reseting SecurityContext")

	deployContainer = deployContainer.WithSecurityContext(&corev1.SecurityContext{RunAsGroup: nil, RunAsUser: nil})

	By("Obtaining container definition")

	deployContainerCfg, err := deployContainer.GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to get container definition")

	By("Defining 1st deployment configuration")

	deployOneLabels := map[string]string{
		strings.Split(nropDeployOneLabel, "=")[0]: strings.Split(nropDeployOneLabel, "=")[1]}

	deploy := defineNROPDeployment(deployContainerCfg,
		nropDeploy1Name,
		RDSCoreConfig.WlkdNROPOneNS,
		RDSCoreConfig.NROPSchedulerName,
		deployOneLabels,
		RDSCoreConfig.WlkdNROPDeployOneSelector)

	By("Creating NROP deployment one")

	_, err = deploy.CreateAndWaitUntilReady(5 * time.Minute)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create deployment %s: %v", nropDeploy1Name, err))
}

// VerifyNROPWorkloadAvailable verifies NUMA-aware workload is in Ready state.
func VerifyNROPWorkloadAvailable(ctx SpecContext) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Verify NUMA-aware workload is Ready")

	var (
		deploy *deployment.Builder
		err    error
	)

	Eventually(func() bool {
		deploy, err = deployment.Pull(APIClient, nropDeploy1Name, RDSCoreConfig.WlkdNROPOneNS)
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Error pulling deployment %q from %q namespace",
				nropDeploy1Name, RDSCoreConfig.WlkdNROPOneNS)

			return false
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Successfully pulled in deployment %q from %q namespace",
			nropDeploy1Name, RDSCoreConfig.WlkdNROPOneNS)

		return true
	}).WithContext(ctx).WithPolling(3*time.Second).WithTimeout(1*time.Minute).Should(BeTrue(),
		fmt.Sprintf("Error retrieving deployment %q from %q namespace",
			nropDeploy1Name, RDSCoreConfig.WlkdNROPOneNS))

	By("Asserting deployment is Ready")
	Expect(deploy.IsReady(1*time.Minute)).To(BeTrue(),
		fmt.Sprintf("Deployment %q in %q namespace is not READY",
			nropDeploy1Name, RDSCoreConfig.WlkdNROPOneNS))

	By("Asserting pods are READY")

	nropPods := findPodWithSelector(RDSCoreConfig.WlkdNROPOneNS, nropDeployOneLabel)

	Expect(len(nropPods)).NotTo(BeZero(),
		fmt.Sprintf("Failed to find pods matching %q label", nropDeployOneLabel))

	for _, _pod := range nropPods {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Processing pod %q", _pod.Definition.Name)

		Expect(_pod.WaitUntilReady(1*time.Minute)).ShouldNot(HaveOccurred(),
			fmt.Sprintf("Pod %q in %q namespace isn't READY",
				_pod.Definition.Name, _pod.Definition.Namespace))
	}
}
