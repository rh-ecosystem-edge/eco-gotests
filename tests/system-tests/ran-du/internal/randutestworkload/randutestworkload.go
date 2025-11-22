package randutestworkload

import (
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/statefulset"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/systemtestsinittools"
	"k8s.io/klog/v2"
)

// CleanNameSpace function removes all objects inside the namespace plus sriov networks whose
// NetworkNamespace spec matches the namespace.
func CleanNameSpace(cleanTimeout time.Duration, nsname string) error {
	err := namespace.NewBuilder(APIClient, nsname).
		CleanObjects(cleanTimeout, deployment.GetGVR(), statefulset.GetGVR())
	if err != nil {
		klog.V(100).Infof("Failed to clean up objects in namespace: %s", nsname)

		return err
	}

	err = namespace.NewBuilder(APIClient, nsname).DeleteAndWait(cleanTimeout)
	if err != nil {
		klog.V(100).Infof("Failed to remove namespace: %s", nsname)

		return err
	}

	return sriov.CleanAllNetworksByTargetNamespace(APIClient,
		SystemTestsTestConfig.SriovOperatorNamespace,
		nsname)
}
