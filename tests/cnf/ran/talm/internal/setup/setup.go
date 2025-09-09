package setup

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranhelper"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/talm/internal/tsparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CreateTalmTestNamespace creates the test namespace on the hub and the spokes, if present.
func CreateTalmTestNamespace() error {
	_, err := namespace.NewBuilder(Spoke1APIClient, tsparams.TestNamespace).Create()
	if err != nil {
		return err
	}

	if HubAPIClient != nil {
		_, err := namespace.NewBuilder(HubAPIClient, tsparams.TestNamespace).Create()
		if err != nil {
			return err
		}
	}

	// spoke 2 may be optional depending on what tests are running
	if Spoke2APIClient != nil {
		_, err := namespace.NewBuilder(Spoke2APIClient, tsparams.TestNamespace).Create()
		if err != nil {
			return err
		}
	}

	return nil
}

// VerifyTalmIsInstalled checks that talm pod and container is present and that CGUs can be fetched.
func VerifyTalmIsInstalled() error {
	// Check for talm pods
	talmPods, err := pod.List(
		HubAPIClient,
		ranparam.OpenshiftOperatorNamespace,
		metav1.ListOptions{LabelSelector: tsparams.TalmPodLabelSelector})
	if err != nil {
		return err
	}

	// Check if any pods exist
	if len(talmPods) == 0 {
		return fmt.Errorf("unable to find talm pod")
	}

	// Check each pod for the talm container
	for _, talmPod := range talmPods {
		if !talmPod.IsHealthy() {
			return fmt.Errorf("talm pod %s is not healthy", talmPod.Definition.Name)
		}

		if !ranhelper.DoesContainerExistInPod(talmPod, ranparam.TalmContainerName) {
			return fmt.Errorf("talm pod defined but talm container does not exist")
		}
	}

	return nil
}
