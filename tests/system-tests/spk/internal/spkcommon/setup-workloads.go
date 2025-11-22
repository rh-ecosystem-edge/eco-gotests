package spkcommon

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/service"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/spk/internal/spkinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/spk/internal/spkparams"
)

var (
	// SPKWorkloadCMData contains data for ConfigMap.
	SPKWorkloadCMData = map[string]string{
		"index.html": "SPK-DEFAULT-WEB-PAGE-GOLANG",
	}
)

const (
	// SPKBackendCMName configMap's name.
	SPKBackendCMName = "spk-cm"
	// SPKBackendSVCName service's name.
	SPKBackendSVCName = "f5-hello-world"
	// SPKBackendSelector labels used by deployment and service.
	SPKBackendSelector = "system-test=spk-demo-workload"
	// SPKBackendSVCPort service port.
	SPKBackendSVCPort = int32(8080)
	// SPKBackendSVCTargetPort service's target port.
	SPKBackendSVCTargetPort = int32(8080)
	// SPKBackendSVCProtocol service's protocol.
	SPKBackendSVCProtocol = corev1.Protocol("TCP")
	// SPKBackendDeployName deployment's name.
	SPKBackendDeployName = "spk-hello-world"
	// SPKBackendContainerName container's name.
	SPKBackendContainerName = "spk-httpd"

	// SPKBackendUDPSVCName name for service for UDP testing.
	SPKBackendUDPSVCName = "f5-udp-svc"
	// SPKBackendUDPSelector labels used by deployment and service.
	SPKBackendUDPSelector = "systemtest-udp-app=qe-udp-server"
	// SPKBackendUDPSVCPort service port.
	SPKBackendUDPSVCPort = int32(8080)
	// SPKBackendUDPSVCTargetPort service's target port.
	SPKBackendUDPSVCTargetPort = int32(8080)
	// SPKBackendUDPSVCProtocol service's protocol.
	SPKBackendUDPSVCProtocol = corev1.Protocol("UDP")
	// SPKBackendUDPDeployName deployment's name.
	SPKBackendUDPDeployName = "udp-mock-server"
	// SPKBackendUDPContainerName container's name.
	SPKBackendUDPContainerName = "udp-server"
)

func deleteConfigMap(cmName, nsName string) {
	klog.V(spkparams.SPKLogLevel).Infof("Assert ConfigMap %q exists in %q namespace",
		cmName, nsName)

	if cmBuilder, err := configmap.Pull(
		APIClient, cmName, nsName); err == nil {
		klog.V(spkparams.SPKLogLevel).Infof("configMap %q found, deleting", cmName)

		var ctx SpecContext

		Eventually(func() bool {
			err := cmBuilder.Delete()
			if err != nil {
				klog.V(spkparams.SPKLogLevel).Infof("Error deleting configMap %q : %v",
					cmName, err)

				return false
			}

			klog.V(spkparams.SPKLogLevel).Infof("Deleted configMap %q in %q namespace",
				cmName, nsName)

			return true
		}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(1*time.Minute).Should(BeTrue(),
			"Failed to delete configMap")
	}
}

func createConfigMap(cmName, nsName string, data map[string]string) {
	klog.V(spkparams.SPKLogLevel).Infof("Create ConfigMap %q in %q namespace",
		cmName, nsName)

	cmBuilder := configmap.NewBuilder(APIClient, cmName, nsName)
	cmBuilder.WithData(data)

	var ctx SpecContext

	Eventually(func() bool {
		cmResult, err := cmBuilder.Create()
		if err != nil {
			klog.V(spkparams.SPKLogLevel).Infof("Error creating ConfigMap %q in %q namespace",
				cmName, nsName)

			return false
		}

		klog.V(spkparams.SPKLogLevel).Infof("Created ConfigMap %q in %q namespace",
			cmResult.Definition.Name, nsName)

		return true
	}).WithContext(ctx).WithPolling(5*time.Second).WithPolling(1*time.Minute).Should(BeTrue(),
		"Failed to crete configMap")
}

func createSVC() {
	klog.V(spkparams.SPKLogLevel).Infof("Creating Service %q", SPKBackendSVCName)

	svcSelector := map[string]string{
		strings.Split(SPKBackendSelector, "=")[0]: strings.Split(SPKBackendSelector, "=")[1],
	}

	klog.V(spkparams.SPKLogLevel).Infof("Defining ServicePort")

	svcPort, err := service.DefineServicePort(SPKBackendSVCPort,
		SPKBackendSVCTargetPort, SPKBackendSVCProtocol)

	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to define ServicePort: %v", err))

	klog.V(spkparams.SPKLogLevel).Infof("Creating Service Builder")

	svcDemo := service.NewBuilder(APIClient, SPKBackendSVCName, SPKConfig.Namespace, svcSelector, *svcPort)

	var ctx SpecContext

	Eventually(func() bool {
		svcDemo, err = svcDemo.Create()
		if err != nil {
			klog.V(spkparams.SPKLogLevel).Infof("Error creating service: %v", err)

			return false
		}

		klog.V(spkparams.SPKLogLevel).Infof("Created service: %q in %q namespace",
			svcDemo.Definition.Name, svcDemo.Definition.Namespace)

		return true
	}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(1*time.Minute).Should(BeTrue(),
		"Failed to create service")
}

func createUDPSVC() {
	klog.V(spkparams.SPKLogLevel).Infof("Creating Service %q", SPKBackendUDPSVCName)

	svcSelector := map[string]string{
		strings.Split(SPKBackendUDPSelector, "=")[0]: strings.Split(SPKBackendUDPSelector, "=")[1],
	}

	klog.V(spkparams.SPKLogLevel).Infof("Defining ServicePort")

	svcPort, err := service.DefineServicePort(SPKBackendUDPSVCPort,
		SPKBackendUDPSVCTargetPort, SPKBackendUDPSVCProtocol)

	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to define ServicePort: %v", err))

	klog.V(spkparams.SPKLogLevel).Infof("Creating Service Builder")

	svcDemo := service.NewBuilder(APIClient, SPKBackendUDPSVCName, SPKConfig.Namespace, svcSelector, *svcPort)

	By("Setting service type to NodePort")

	svcDemo = svcDemo.WithNodePort()

	By("Resetting NodePort value")

	svcDemo.Definition.Spec.Ports[0].NodePort = int32(31225)

	By("Setting IPFamily")

	ipFamily := []corev1.IPFamily{"IPv4", "IPv6"}
	ipStackFamily := corev1.IPFamilyPolicyPreferDualStack

	svcDemo = svcDemo.WithIPFamily(ipFamily, ipStackFamily)

	klog.V(spkparams.SPKLogLevel).Infof("Service:\n%v\n", svcDemo.Definition)

	var ctx SpecContext

	Eventually(func() bool {
		svcDemo, err = svcDemo.Create()
		if err != nil {
			klog.V(spkparams.SPKLogLevel).Infof("Error creating service: %v", err)

			return false
		}

		klog.V(spkparams.SPKLogLevel).Infof("Created service: %q in %q namespace",
			svcDemo.Definition.Name, svcDemo.Definition.Namespace)

		return true
	}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(1*time.Minute).Should(BeTrue(),
		"Failed to create service")
}

func deleteSVC(svcName, svcNS string) {
	klog.V(spkparams.SPKLogLevel).Infof("Deleting Service %q in %q namespace",
		svcName, svcNS)

	svcDemo, err := service.Pull(APIClient, svcName, svcNS)

	if err != nil && svcDemo == nil {
		klog.V(spkparams.SPKLogLevel).Infof("Service %q not found in %q namespace",
			svcName, svcNS)

		return
	}

	var ctx SpecContext

	Eventually(func() bool {
		err := svcDemo.Delete()
		if err != nil {
			klog.V(spkparams.SPKLogLevel).Infof("Error deleting service: %v", err)

			return false
		}

		klog.V(spkparams.SPKLogLevel).Infof("Deleted service %q in %q namespace",
			svcName, svcNS)

		return true
	}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(1*time.Minute).Should(BeTrue(),
		"Failed to delete service")
}

func deleteDemoDeployment() {
	klog.V(spkparams.SPKLogLevel).Infof("Deleting deployment %q in %q namespace",
		SPKBackendDeployName, SPKConfig.Namespace)

	deploy, _ := deployment.Pull(APIClient, SPKBackendDeployName, SPKConfig.Namespace)

	if deploy == nil {
		klog.V(spkparams.SPKLogLevel).Infof("Deployment %q not found in %q namespace",
			SPKBackendDeployName, SPKConfig.Namespace)

		return
	}

	err := deploy.DeleteAndWait(5 * time.Minute)

	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to delete deployment: %v", err))
}

func deleteDeployment(dName, dNamespace string) {
	klog.V(spkparams.SPKLogLevel).Infof("Deleting deployment %q in %q namespace",
		dName, dNamespace)

	deploy, _ := deployment.Pull(APIClient, dName, dNamespace)

	if deploy == nil {
		klog.V(spkparams.SPKLogLevel).Infof("Deployment %q not found in %q namespace",
			dName, dNamespace)

		return
	}

	err := deploy.DeleteAndWait(5 * time.Minute)

	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to delete deployment: %v", err))
}

//nolint:funlen
func createBackendDeployment() {
	klog.V(spkparams.SPKLogLevel).Infof("Create deployment %q in %q namespace",
		SPKBackendDeployName, SPKConfig.Namespace)

	By("Defining container configuration")

	klog.V(spkparams.SPKLogLevel).Infof("Defining container configuration")

	deployContainer := pod.NewContainerBuilder(SPKBackendContainerName, SPKConfig.BackendContainerImage,
		[]string{"/bin/bash", "-c", "httpd -D FOREGROUND"})

	By("Resetting SCC")

	klog.V(spkparams.SPKLogLevel).Infof("Setting SCC")

	deployContainer = deployContainer.WithSecurityContext(&corev1.SecurityContext{RunAsGroup: nil, RunAsUser: nil})

	By("Adding VolumeMount to container")

	volMount := corev1.VolumeMount{
		Name:      "web-page",
		MountPath: "/opt/rh/httpd24/root/var/www/html",
		ReadOnly:  false,
	}

	deployContainer = deployContainer.WithVolumeMount(volMount)

	By("Obtaining container definition")

	klog.V(spkparams.SPKLogLevel).Infof("Obtaining contaienr configuration for deployment")

	deployContainerCfg, err := deployContainer.GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to get container config")

	// NOTE(yprokule): image has entry point that does not require command to be set.
	klog.V(spkparams.SPKLogLevel).Infof("Reseting container's command")

	deployContainerCfg.Command = nil

	By("Defining deployment configuration")

	deployLabels := map[string]string{
		strings.Split(SPKBackendSelector, "=")[0]: strings.Split(SPKBackendSelector, "=")[1],
	}

	var deploy *deployment.Builder

	klog.V(spkparams.SPKLogLevel).Infof("Defining deployment %q", SPKBackendDeployName)

	deploy = deployment.NewBuilder(APIClient,
		SPKBackendDeployName,
		SPKConfig.Namespace,
		deployLabels,
		*deployContainerCfg)

	By("Adding Volume to the deployment")

	klog.V(spkparams.SPKLogLevel).Infof("Defining VolumeDefinition")

	volMode := new(int32)
	*volMode = 511

	volDefinition := corev1.Volume{
		Name: "web-page",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				DefaultMode: volMode,
				LocalObjectReference: corev1.LocalObjectReference{
					Name: SPKBackendCMName,
				},
			},
		},
	}

	klog.V(spkparams.SPKLogLevel).Infof("Volume definition:\n%#v", volDefinition)

	deploy = deploy.WithVolume(volDefinition)

	By("Setting Replicas count")

	deploy = deploy.WithReplicas(int32(1))

	By("Creating deployment")

	klog.V(spkparams.SPKLogLevel).Infof("Creating deployment %q in %q namespace",
		SPKBackendDeployName, SPKConfig.Namespace)

	deploy, err = deploy.CreateAndWaitUntilReady(300 * time.Second)

	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create deployment: %v", err))

	klog.V(spkparams.SPKLogLevel).Infof("Created deployment %q in %q namespace",
		deploy.Definition.Name, deploy.Definition.Namespace)
}

func createBackendUDPDeployment() {
	klog.V(spkparams.SPKLogLevel).Infof("Create deployment %q in %q namespace",
		SPKBackendUDPDeployName, SPKConfig.Namespace)

	By("Defining container configuration")

	klog.V(spkparams.SPKLogLevel).Infof("Defining container configuration")

	deployContainer := pod.NewContainerBuilder(SPKBackendUDPContainerName, SPKConfig.BackendUDPContainerImage,
		[]string{"/bin/bash", "-c", "/opt/local/bin/demo-udp-server.bin 8080"})

	By("Resetting SCC")

	klog.V(spkparams.SPKLogLevel).Infof("Setting SCC")

	deployContainer = deployContainer.WithSecurityContext(&corev1.SecurityContext{RunAsGroup: nil, RunAsUser: nil})

	By("Obtaining container definition")

	klog.V(spkparams.SPKLogLevel).Infof("Obtaining contaienr configuration for deployment")

	deployContainerCfg, err := deployContainer.GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to get container config")

	By("Defining deployment configuration")

	deployLabels := map[string]string{
		strings.Split(SPKBackendUDPSelector, "=")[0]: strings.Split(SPKBackendUDPSelector, "=")[1],
	}

	var deploy *deployment.Builder

	klog.V(spkparams.SPKLogLevel).Infof("Defining deployment %q", SPKBackendDeployName)

	deploy = deployment.NewBuilder(APIClient,
		SPKBackendUDPDeployName,
		SPKConfig.Namespace,
		deployLabels,
		*deployContainerCfg)

	By("Setting Replicas count")

	deploy = deploy.WithReplicas(int32(1))

	By("Creating deployment")

	klog.V(spkparams.SPKLogLevel).Infof("Creating deployment %q in %q namespace",
		SPKBackendUDPDeployName, SPKConfig.Namespace)

	deploy, err = deploy.CreateAndWaitUntilReady(300 * time.Second)

	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create deployment: %v", err))

	klog.V(spkparams.SPKLogLevel).Infof("Created deployment %q in %q namespace",
		deploy.Definition.Name, deploy.Definition.Namespace)
}

// SetupSPKBackendWorkload creates workload that is used in SPK Ingress testing.
func SetupSPKBackendWorkload() {
	deleteConfigMap(SPKBackendCMName, SPKConfig.Namespace)
	createConfigMap(SPKBackendCMName, SPKConfig.Namespace, SPKWorkloadCMData)
	deleteSVC(SPKBackendSVCName, SPKConfig.Namespace)
	createSVC()
	deleteDemoDeployment()
	createBackendDeployment()
}

// SetupSPKBackendUDPWorkload creates workload that is used in SPK Ingress testing.
func SetupSPKBackendUDPWorkload() {
	deleteSVC(SPKBackendUDPSVCName, SPKConfig.Namespace)
	createUDPSVC()
	deleteDeployment(SPKBackendUDPDeployName, SPKConfig.Namespace)
	createBackendUDPDeployment()
}
