package rdscorecommon

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"
)

// defineBaseDeployment returns a base deployment.Builder for the given container,
// name, namespace, labels and replica count. The container is expected to be built
// by the caller. Callers decorate the returned builder (node selector, volumes,
// secondary networks, service account, tolerations, etc.) as needed.
func defineBaseDeployment(containerConfig *corev1.Container, deployName, deployNs string,
	deployLabels map[string]string, replicas int32) *deployment.Builder {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Defining deployment %q in %q ns", deployName, deployNs)

	if containerConfig == nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Container configuration for deployment %q is nil", deployName)

		return nil
	}

	deploy := deployment.NewBuilder(APIClient, deployName, deployNs, deployLabels, *containerConfig)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Setting Replicas count to %d for deployment %q", replicas, deployName)

	deploy = deploy.WithReplicas(replicas)

	return deploy
}
