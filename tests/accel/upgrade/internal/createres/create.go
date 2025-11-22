package createres

import (
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/route"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/service"

	upgradeinittools "github.com/rh-ecosystem-edge/eco-gotests/tests/accel/internal/accelinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/accel/upgrade/internal/upgradeparams"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

// Workload creates a workload with test image.
func Workload(apiClient *clients.Settings, workloadImage string) (*deployment.Builder, error) {
	klog.V(90).Infof("Creating Deployment %q", upgradeparams.DeploymentName)

	containerConfig, err := pod.NewContainerBuilder(upgradeparams.DeploymentName, upgradeinittools.
		AccelConfig.IBUWorkloadImage, []string{"/hello-openshift"}).WithPorts(
		[]corev1.ContainerPort{{Name: "http", ContainerPort: 8080}}).
		WithSecurityContext(upgradeparams.DefaultSC).GetContainerCfg()
	if err != nil {
		return nil, fmt.Errorf("failed to get containerConfig with error: %w", err)
	}

	workloadDeployment, err := deployment.NewBuilder(
		upgradeinittools.HubAPIClient, upgradeparams.DeploymentName, upgradeparams.TestNamespaceName, map[string]string{
			"app": upgradeparams.DeploymentName,
		}, *containerConfig).WithLabel("app", upgradeparams.DeploymentName).CreateAndWaitUntilReady(time.Second * 120)
	if err != nil {
		return nil, fmt.Errorf("failed to create workload with error: %w", err)
	}

	return workloadDeployment, nil
}

// Service creates a service for a workload.
// Return nil on success, otherwise returns an error.
func Service(apiClient *clients.Settings, port int32) (*service.Builder, error) {
	klog.V(90).Infof("Creating Service %q", upgradeparams.DeploymentName)

	klog.V(90).Infof("Defining ServicePort")

	svcPort, err := service.DefineServicePort(
		upgradeparams.ServicePort,
		upgradeparams.ServicePort,
		corev1.Protocol("TCP"))
	if err != nil {
		klog.V(90).Infof("Error defining service port: %v", err)

		return nil, err
	}

	klog.V(90).Infof("Creating Service Builder")

	svcDemo, err := service.NewBuilder(apiClient,
		upgradeparams.DeploymentName,
		upgradeparams.TestNamespaceName,
		upgradeparams.ContainerLabelsMap,
		*svcPort).Create()
	if err != nil {
		klog.V(90).Infof("Error creating service: %v", err)

		return nil, err
	}

	klog.V(90).Infof("Created service: %q in %q namespace",
		svcDemo.Definition.Name, svcDemo.Definition.Namespace)

	return svcDemo, nil
}

// WorkloadRoute creates a route for the workload service.
func WorkloadRoute(apiClient *clients.Settings) (*route.Builder, error) {
	workloadRoute, err := route.NewBuilder(
		apiClient, upgradeparams.DeploymentName, upgradeparams.TestNamespaceName, upgradeparams.DeploymentName).Create()
	if err != nil {
		klog.V(90).Infof("Error creating route: %v", err)

		return nil, err
	}

	return workloadRoute, err
}
