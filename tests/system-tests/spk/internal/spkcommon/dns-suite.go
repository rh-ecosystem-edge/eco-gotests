package spkcommon

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/spk/internal/spkinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/spk/internal/spkparams"
)

const (
	wlkdDeployLabel      = "systemtest-test=spk-dns-nat46"
	wlkdContainerName    = "qe-spk"
	wlkdDCILabel         = "app=mttool"
	wlkdDCIContainerName = "network-multitool"
	tmmLabel             = "app=f5-tmm"
	ingressDataLabel     = "app=spk-data-f5ingress"
	ingressDNSLabel      = "app=spk-dns46-f5ingress"
)

// VerifyDNSResolutionFromNewDeploy asserts DNS resolution from within a newly created deployment.
//
//nolint:funlen
func VerifyDNSResolutionFromNewDeploy(ctx SpecContext) {
	By("Asserting deployment exists")

	var (
		appPods []*pod.Builder
		err     error
	)

	deploy, _ := deployment.Pull(APIClient, SPKConfig.WorkloadDeploymentName, SPKConfig.Namespace)

	if deploy != nil {
		klog.V(spkparams.SPKLogLevel).Infof("Existing deployment %q found. Removing...",
			SPKConfig.WorkloadDeploymentName)

		err := deploy.DeleteAndWait(300 * time.Second)
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("failed to delete deployment %q", SPKConfig.WorkloadDeploymentName))
	}

	By("Checking that pods backed by deployment are gone")

	klog.V(spkparams.SPKLogLevel).Infof("Checking for pods with label(s) %q \n", wlkdDeployLabel)

	Eventually(func() bool {
		oldPods, err := pod.List(APIClient, SPKConfig.Namespace,
			metav1.ListOptions{LabelSelector: wlkdDeployLabel})
		if err != nil {
			klog.V(spkparams.SPKLogLevel).Infof("Error listing pods: %v", err)

			return false
		}

		klog.V(spkparams.SPKLogLevel).Infof("Found %d pods matching label %q ", len(oldPods), wlkdDeployLabel)

		return len(oldPods) == 0
	}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(5*time.Minute).Should(BeTrue(),
		fmt.Sprintf("Theare are pods matching label %q in %q namespace", wlkdDeployLabel, SPKConfig.Namespace))

	By("Defining container configuration")

	klog.V(spkparams.SPKLogLevel).Infof("Defining container configuration")
	// NOTE(yprokule): image has entry point that does not require command to be set.
	deployContainer := pod.NewContainerBuilder(wlkdContainerName, SPKConfig.WorkloadContainerImage,
		[]string{"/bin/bash", "-c", "sleep infinity"})

	By("Setting SCC")

	klog.V(spkparams.SPKLogLevel).Infof("Setting SCC")

	deployContainer = deployContainer.WithSecurityContext(&corev1.SecurityContext{RunAsGroup: nil, RunAsUser: nil})

	By("Obtaining container definition")

	klog.V(spkparams.SPKLogLevel).Infof("Obtaining contaienr configuration for deployment")

	deployContainerCfg, err := deployContainer.GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to get container config")

	By("Defining deployment configuration")

	klog.V(spkparams.SPKLogLevel).Infof("Defining deployment %q", SPKConfig.WorkloadDeploymentName)
	deploy = deployment.NewBuilder(APIClient,
		SPKConfig.WorkloadDeploymentName,
		SPKConfig.Namespace,
		map[string]string{strings.Split(wlkdDeployLabel, "=")[0]: strings.Split(wlkdDeployLabel, "=")[1]},
		*deployContainerCfg)

	By("Creating deployment")

	klog.V(spkparams.SPKLogLevel).Infof("Creating deployment %q in %q namespace",
		SPKConfig.WorkloadDeploymentName, SPKConfig.Namespace)

	_, err = deploy.CreateAndWaitUntilReady(300 * time.Second)
	Expect(err).ToNot(HaveOccurred(), "failed to create deployment")

	By("Finding pod backed by deployment")

	klog.V(spkparams.SPKLogLevel).Infof("Looking for pods from deployment %q in %q namespace",
		SPKConfig.WorkloadDeploymentName, SPKConfig.Namespace)

	Eventually(func() bool {
		appPods, err = pod.List(APIClient, SPKConfig.Namespace,
			metav1.ListOptions{LabelSelector: wlkdDeployLabel})
		if err != nil {
			klog.V(spkparams.SPKLogLevel).Infof("Failed to list pods: %v", err)

			return false
		}

		klog.V(spkparams.SPKLogLevel).Infof("Found %d pods matching label %q",
			len(appPods), wlkdDeployLabel)

		return true
	}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(5*time.Minute).Should(BeTrue(),
		fmt.Sprintf("Failed to find pods matching label %q", wlkdDeployLabel))

	verifyDNSResolution(deploy.Definition.Name, deploy.Definition.Namespace, wlkdDeployLabel,
		wlkdContainerName, "6m", "20s")
}

// VerifyDNSResolutionFromExistingDeploy verifies DNS resolution from existing deployment.
func VerifyDNSResolutionFromExistingDeploy(ctx SpecContext) {
	klog.V(spkparams.SPKLogLevel).Infof("*** VerifyDNSResolutionFromExistingDeploy ***")
	verifyDNSResolution(SPKConfig.WorkloadDCIDeploymentName, SPKConfig.Namespace, wlkdDCILabel,
		wlkdDCIContainerName, "3m", "5s")
}

// VerifyDNSResolutionAfterTMMScaleUpDownExisting verifies DNS resolution from existing deployment
// after TMM deployment is scaled down and then up.
func VerifyDNSResolutionAfterTMMScaleUpDownExisting(ctx SpecContext) {
	klog.V(spkparams.SPKLogLevel).Infof("*** VerifyDNSResolutionAfterTMMScaleUpDownExisting ***")

	scaleDownDeployment(SPKConfig.SPKDataTMMDeployName, SPKConfig.SPKDataNS, tmmLabel)
	scaleUpDeployment(SPKConfig.SPKDataTMMDeployName, SPKConfig.SPKDataNS, tmmLabel, 1)
	scaleDownDeployment(SPKConfig.SPKDnsTMMDeployName, SPKConfig.SPKDnsNS, tmmLabel)
	scaleUpDeployment(SPKConfig.SPKDnsTMMDeployName, SPKConfig.SPKDnsNS, tmmLabel, 1)
	verifyDNSResolution(SPKConfig.WorkloadDCIDeploymentName, SPKConfig.Namespace, wlkdDCILabel,
		wlkdDCIContainerName, "6m", "10s")
}

// VerifyDNSResolutionWithMultipleTMMsExisting verifies DNS resolution with multiple instances of TMM controller.
func VerifyDNSResolutionWithMultipleTMMsExisting(ctx SpecContext) {
	klog.V(spkparams.SPKLogLevel).Infof("*** VerifyDNSResolutionWithMultipleTMMsExisting ***")

	scaleUpDeployment(SPKConfig.SPKDataTMMDeployName, SPKConfig.SPKDataNS, tmmLabel, 2)
	scaleUpDeployment(SPKConfig.SPKDnsTMMDeployName, SPKConfig.SPKDnsNS, tmmLabel, 2)
	verifyDNSResolution(SPKConfig.WorkloadDCIDeploymentName, SPKConfig.Namespace, wlkdDCILabel,
		wlkdDCIContainerName, "3m", "15s")
}

// VerifyIngressScaleDownUp verifies DNS resolution from existing deployment after Ingress scale down and up.
func VerifyIngressScaleDownUp(ctx SpecContext) {
	klog.V(spkparams.SPKLogLevel).Infof("*** VerifyIngressScaleDownUp ***")

	scaleDownDeployment(SPKConfig.SPKDataIngressDeployName, SPKConfig.SPKDataNS, ingressDataLabel)
	scaleDownDeployment(SPKConfig.SPKDnsIngressDeployName, SPKConfig.SPKDnsNS, ingressDNSLabel)
	scaleUpDeployment(SPKConfig.SPKDataIngressDeployName, SPKConfig.SPKDataNS, ingressDataLabel, 1)
	scaleUpDeployment(SPKConfig.SPKDnsIngressDeployName, SPKConfig.SPKDnsNS, ingressDNSLabel, 1)
	verifyDNSResolution(SPKConfig.WorkloadDCIDeploymentName, SPKConfig.Namespace, wlkdDCILabel,
		wlkdDCIContainerName, "7m", "15s")
}

//nolint:funlen
func verifyDNSResolution(deployName, deployNS, deployLabel, containerName, waitDuration, pollInterval string) {
	By("Asserting deployment exists")

	klog.V(spkparams.SPKLogLevel).Infof("Check deployment %q exists in %q namespace",
		deployName, deployNS)

	deploy, _ := deployment.Pull(APIClient, deployName, deployNS)

	if deploy == nil {
		Skip(fmt.Sprintf("Deployment %q not found in %q ns",
			deployName,
			deployNS))
	}

	var (
		appPods  []*pod.Builder
		err      error
		ctx      SpecContext
		output   bytes.Buffer
		rGroup   = `((2[0-4][0-9])|(25[05]))|(1[0-9][0-9])|([1-9][0-9])|([1-9])`
		ipRegexp = `(` + rGroup + `\.){3}` + rGroup
	)

	By("Parsing timeout value")

	retryDuration, err := time.ParseDuration(waitDuration)

	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to parse duration: %q", waitDuration))

	By("Parsing retry value")

	pollDuration, err := time.ParseDuration(pollInterval)

	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to parse duration: %q", waitDuration))

	By("Finding pod backed by deployment")

	Eventually(func() bool {
		appPods, err = pod.List(APIClient, deployNS,
			metav1.ListOptions{LabelSelector: deployLabel})
		if err != nil {
			klog.V(spkparams.SPKLogLevel).Infof("Failed to list pods: %v", err)

			return false
		}

		klog.V(spkparams.SPKLogLevel).Infof("Found %d pods matching label %q",
			len(appPods), deployLabel)

		return true
	}).WithContext(ctx).WithPolling(pollDuration).WithTimeout(retryDuration).Should(BeTrue(),
		fmt.Sprintf("Failed to find pods matching label %q", deployLabel))

	for _, _pod := range appPods {
		By("Running DNS resolution from within a pod")

		cmdDig := fmt.Sprintf("dig -t A %s +short", SPKConfig.WorkloadTestURL)
		klog.V(spkparams.SPKLogLevel).Infof("Running command %q from within a pod %q",
			cmdDig, _pod.Definition.Name)

		ipRegObj := regexp.MustCompile(ipRegexp)

		Eventually(func() bool {
			output, err := _pod.ExecCommand([]string{"/bin/sh", "-c", cmdDig}, containerName)
			if err != nil {
				klog.V(spkparams.SPKLogLevel).Infof("Failed to run command: %v", err)

				return false
			}

			if !ipRegObj.MatchString(output.String()) {
				klog.V(spkparams.SPKLogLevel).Infof("Command's output doesn't match regexp: %q", ipRegexp)
				klog.V(spkparams.SPKLogLevel).Infof("Command's Output:\n%v\n", output.String())

				return false
			}

			klog.V(spkparams.SPKLogLevel).Infof("Command's output matches regexp: %q", ipRegexp)
			klog.V(spkparams.SPKLogLevel).Infof("Command's Output:\n%v\n", output.String())

			return true
		}).WithContext(ctx).WithPolling(pollDuration).WithTimeout(retryDuration).Should(BeTrue(),
			"Error performing DNS lookup from within pod")

		By("Access outside URL")

		targetURL := fmt.Sprintf("http://%s:%s", SPKConfig.WorkloadTestURL, SPKConfig.WorkloadTestPort)
		cmd := fmt.Sprintf("curl -Ls --max-time 5 -o /dev/null -w '%%{http_code}' %s", targetURL)

		klog.V(spkparams.SPKLogLevel).Infof("Running command %q from within a pod %q",
			cmd, _pod.Definition.Name)

		klog.V(spkparams.SPKLogLevel).Infof("Resetting output buffer")
		output.Reset()

		Eventually(func() bool {
			output, err = _pod.ExecCommand([]string{"/bin/sh", "-c", cmd}, containerName)
			if err != nil {
				klog.V(spkparams.SPKLogLevel).Infof("Failed to run command: %v", err)

				return false
			}

			klog.V(spkparams.SPKLogLevel).Infof("Command's Output:\n%v\n", output.String())

			codesPattern := "200 404"

			return strings.Contains(codesPattern, strings.Trim(output.String(), "'"))
		}).WithContext(ctx).WithPolling(pollDuration).WithTimeout(retryDuration).Should(BeTrue(),
			"Error fetching outside URL from within pod")
	}
}

func scaleDownDeployment(deployName, deployNS, deployLabel string) {
	By("Asserting deployment exists")

	deployData, err := deployment.Pull(APIClient, deployName, deployNS)

	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to get deployment %q in %q namespace",
		deployName, deployNS))

	By("Setting replicas count to 0")

	klog.V(spkparams.SPKLogLevel).Infof("Setting replicas count to 0 for deployment %q in %q namespace",
		deployData.Definition.Name, deployData.Definition.Namespace)

	deployData = deployData.WithReplicas(0)

	By("Updating deployment")

	klog.V(spkparams.SPKLogLevel).Infof("Updating deployment %q in %q namespace",
		deployData.Definition.Name, deployData.Definition.Namespace)

	var (
		ctx     SpecContext
		appPods []*pod.Builder
	)

	Eventually(func() bool {
		deployData, err = deployData.Update()
		if err != nil {
			klog.V(spkparams.SPKLogLevel).Infof("Error updating deployment %q: %v", deployData.Definition.Name, err)

			return false
		}

		klog.V(spkparams.SPKLogLevel).Infof("Updated deployment %q in %q namespace",
			deployData.Definition.Name, deployData.Definition.Namespace)

		return true
	}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(5*time.Minute).Should(BeTrue(),
		fmt.Sprintf("Failed to scale down deployment %q in %q namespace", deployName, deployNS))

	By(fmt.Sprintf("Wating for all pods from %q deployment in %q namespace to go away",
		deployData.Definition.Name, deployData.Definition.Namespace))

	klog.V(spkparams.SPKLogLevel).Infof("Assert pods from deployment %q are gone", deployData.Definition.Name)

	Eventually(func() bool {
		appPods, err = pod.List(APIClient, deployNS,
			metav1.ListOptions{LabelSelector: deployLabel})
		if err != nil {
			klog.V(spkparams.SPKLogLevel).Infof("Failed to list pods: %v", err)

			return false
		}

		klog.V(spkparams.SPKLogLevel).Infof("Found %d pods matching label %q", len(appPods), deployLabel)

		return len(appPods) == 0
	}).WithContext(ctx).WithPolling(3*time.Second).WithTimeout(5*time.Minute).Should(BeTrue(),
		fmt.Sprintf("Pods matching label %q still exist in %q namespace", deployLabel, deployNS))

	klog.V(spkparams.SPKLogLevel).Infof("Pods matching label %q in %q namespace are gone", deployLabel, deployNS)
}

func scaleUpDeployment(deployName, deployNS, deployLabel string, replicas int32) {
	By("Asserting deployment exists")

	deployData, err := deployment.Pull(APIClient, deployName, deployNS)

	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to get deployment %q in %q namespace",
		deployName, deployNS))

	if *deployData.Object.Spec.Replicas == replicas {
		klog.V(spkparams.SPKLogLevel).Infof("Amount of replicas is the same, nothing to do here")

		return
	}

	By(fmt.Sprintf("Setting replicas count to %d", replicas))

	klog.V(spkparams.SPKLogLevel).Infof("Setting replicas count to %d for deployment %q in %q namespace",
		replicas, deployData.Definition.Name, deployData.Definition.Namespace)

	deployData = deployData.WithReplicas(replicas)

	By("Updating deployment")

	klog.V(spkparams.SPKLogLevel).Infof("Updating deployment %q in %q namespace",
		deployData.Definition.Name, deployData.Definition.Namespace)

	var (
		ctx     SpecContext
		appPods []*pod.Builder
	)

	Eventually(func() bool {
		deployData, err = deployData.Update()
		if err != nil {
			klog.V(spkparams.SPKLogLevel).Infof("Error updating deployment %q: %v", deployData.Definition.Name, err)

			return false
		}

		klog.V(spkparams.SPKLogLevel).Infof("Updated deployment %q in %q namespace",
			deployData.Definition.Name, deployData.Definition.Namespace)

		return true
	}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(5*time.Minute).Should(BeTrue(),
		fmt.Sprintf("Failed to scale up deployment %q in %q namespace", deployName, deployNS))

	By(fmt.Sprintf("Wating for new pods from %q deployment in %q namespace to appear",
		deployData.Definition.Name, deployData.Definition.Namespace))

	klog.V(spkparams.SPKLogLevel).Infof("Assert new pods from deployment %q are created", deployData.Definition.Name)

	Eventually(func() bool {
		appPods, err = pod.List(APIClient, deployNS,
			metav1.ListOptions{LabelSelector: deployLabel})
		if err != nil {
			klog.V(spkparams.SPKLogLevel).Infof("Failed to list pods: %v", err)

			return false
		}

		klog.V(spkparams.SPKLogLevel).Infof("Found %d pods matching label %q", len(appPods), deployLabel)

		return len(appPods) == int(replicas)
	}).WithContext(ctx).WithPolling(3*time.Second).WithTimeout(5*time.Minute).Should(BeTrue(),
		fmt.Sprintf("Pods matching label %q not found in %q namespace", deployLabel, deployNS))

	klog.V(spkparams.SPKLogLevel).Infof("Pods matching label %q in %q namespace are created", deployLabel, deployNS)

	By("Waiting for deployment to be Ready")

	klog.V(spkparams.SPKLogLevel).Infof("Wait for deployment %q to be Ready", deployData.Definition.Name)

	if !deployData.IsReady(5 * time.Minute) {
		klog.V(spkparams.SPKLogLevel).Infof("Deployment %q in %q namespace isn't Ready after 5 minutes",
			deployData.Definition.Name, deployData.Definition.Namespace)

		Fail(fmt.Sprintf("Deployment %q in %q namespace hasn't reached Ready state",
			deployData.Definition.Name, deployData.Definition.Namespace))
	}
}

func deletePodMatchingLabel(nsName, labelSelector, waitDuration string) {
	var (
		ctx     SpecContext
		oldPods []*pod.Builder
		err     error
	)

	delDuration, err := time.ParseDuration(waitDuration)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to parse duration: %q", waitDuration))

	Eventually(func() bool {
		oldPods, err = pod.List(APIClient, nsName,
			metav1.ListOptions{LabelSelector: labelSelector})
		if err != nil {
			klog.V(spkparams.SPKLogLevel).Infof("Error listing pods in %q namespace: %v", nsName, err)

			return false
		}

		klog.V(spkparams.SPKLogLevel).Infof("Found %d pods matching label %q ", len(oldPods), labelSelector)

		return true
	}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(5*time.Minute).Should(BeTrue(),
		fmt.Sprintf("Error listing pods in %q namespace", nsName))

	if len(oldPods) == 0 {
		klog.V(spkparams.SPKLogLevel).Infof("No pods matching label %q found in %q namespace",
			labelSelector, nsName)
	}

	for _, _pod := range oldPods {
		klog.V(spkparams.SPKLogLevel).Infof("Deleting pod %q in %q namspace",
			_pod.Definition.Name, _pod.Definition.Namespace)

		_pod, err = _pod.DeleteAndWait(delDuration)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to delete pod %q: %v", _pod.Definition.Name, err))
	}
}

// VerifyDNSResolutionAfterIngressPodIsDeleteExistinDeploy assert DNS resolution from existing deployment,
// after SPK Ingress pods are deleted.
func VerifyDNSResolutionAfterIngressPodIsDeleteExistinDeploy(ctx SpecContext) {
	deletePodMatchingLabel(SPKConfig.SPKDataNS, ingressDataLabel, "3m")
	deletePodMatchingLabel(SPKConfig.SPKDnsNS, ingressDNSLabel, "3m")
	verifyDNSResolution(SPKConfig.WorkloadDCIDeploymentName, SPKConfig.Namespace,
		wlkdDCILabel, wlkdDCIContainerName, "6m", "15s")
}

// VerifyDNSResolutionAfterIngressPodIsDeleteNewDeploy assert DNS resolution from new deployment,
// after SPK Ingress pods are delete.
func VerifyDNSResolutionAfterIngressPodIsDeleteNewDeploy(ctx SpecContext) {
	deletePodMatchingLabel(SPKConfig.SPKDataNS, ingressDataLabel, "3m")
	deletePodMatchingLabel(SPKConfig.SPKDnsNS, ingressDNSLabel, "3m")
	VerifyDNSResolutionFromNewDeploy(ctx)
}

// VerifyDNSResolutionAfterTMMPodIsDeletedExistingDeploy assert DNS resolution from existing deployment,
// after SPK TMM pod(s) are deleted.
func VerifyDNSResolutionAfterTMMPodIsDeletedExistingDeploy(ctx SpecContext) {
	deletePodMatchingLabel(SPKConfig.SPKDataNS, tmmLabel, "5m")
	deletePodMatchingLabel(SPKConfig.SPKDnsNS, tmmLabel, "5m")
	verifyDNSResolution(SPKConfig.WorkloadDCIDeploymentName, SPKConfig.Namespace,
		wlkdDCILabel, wlkdDCIContainerName, "6m", "15s")
}

// VerifyDNSResolutionAfterTMMPodIsDeletedNewDeploy assert DNS resolution from new deployment,
// after SPK TMM pod(s) are deleted.
func VerifyDNSResolutionAfterTMMPodIsDeletedNewDeploy(ctx SpecContext) {
	deletePodMatchingLabel(SPKConfig.SPKDataNS, tmmLabel, "5m")
	deletePodMatchingLabel(SPKConfig.SPKDnsNS, tmmLabel, "5m")
	VerifyDNSResolutionFromNewDeploy(ctx)
}

// ResetTMMReplicas sets TMM replica count to 1.
func ResetTMMReplicas(ctx SpecContext) {
	klog.V(spkparams.SPKLogLevel).Infof("*** Resetting TMM replicas to 1 ***")

	scaleUpDeployment(SPKConfig.SPKDataTMMDeployName, SPKConfig.SPKDataNS, tmmLabel, 1)
	scaleUpDeployment(SPKConfig.SPKDnsTMMDeployName, SPKConfig.SPKDnsNS, tmmLabel, 1)
}
