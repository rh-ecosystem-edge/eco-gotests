package sriovhelper

import (
	"context"
	"fmt"
	"time"

	nadV1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ValidateSriovInterfaces checks that the requested interfaces (from env) exist on every worker
// in workerNodeList. This ensures multi-worker tests do not fail later when scheduling on a worker
// that does not expose the requested PF names.
func ValidateSriovInterfaces(workerNodeList []*nodes.Builder, requestedNumber int) error {
	requestedSriovInterfaceList, err := NetConfig.GetSriovInterfaces(requestedNumber)
	if err != nil {
		return err
	}

	for _, worker := range workerNodeList {
		availableUpSriovInterfaces, err := sriov.NewNetworkNodeStateBuilder(
			APIClient, worker.Definition.Name, NetConfig.SriovOperatorNamespace).GetUpNICs()
		if err != nil {
			return fmt.Errorf("failed to get SR-IOV devices from node %s: %w", worker.Definition.Name, err)
		}

		var validCount int

		for _, availableUpSriovInterface := range availableUpSriovInterfaces {
			for _, requestedSriovInterface := range requestedSriovInterfaceList {
				if availableUpSriovInterface.Name == requestedSriovInterface {
					validCount++

					break
				}
			}
		}

		if validCount < requestedNumber {
			return fmt.Errorf("requested interfaces %v are not all present on node %s (found %d of %d)",
				requestedSriovInterfaceList, worker.Definition.Name, validCount, requestedNumber)
		}
	}

	return nil
}

// DiscoverInterfaceUnderTestDeviceID discovers device ID for a given SR-IOV interface on a worker node.
func DiscoverInterfaceUnderTestDeviceID(srIovInterfaceUnderTest, workerNodeName string) string {
	sriovInterfaces, err := sriov.NewNetworkNodeStateBuilder(
		APIClient, workerNodeName, NetConfig.SriovOperatorNamespace).GetUpNICs()
	if err != nil {
		klog.V(90).Infof("Failed to discover device ID for network interface %s: %v",
			srIovInterfaceUnderTest, err)

		return ""
	}

	for _, srIovInterface := range sriovInterfaces {
		if srIovInterface.Name == srIovInterfaceUnderTest {
			return srIovInterface.DeviceID
		}
	}

	return ""
}

// CreateSriovNetworkAndWaitForNADCreation creates a SriovNetwork and waits for NAD creation on the test namespace.
func CreateSriovNetworkAndWaitForNADCreation(sNet *sriov.NetworkBuilder, timeout time.Duration) error {
	klog.V(90).Infof("Creating SriovNetwork %s and waiting for net-attach-def to be created", sNet.Definition.Name)

	sriovNetwork, err := sNet.Create()
	if err != nil {
		return err
	}

	return WaitForNADCreation(sriovNetwork.Object.Name, TargetNamespaceOf(sriovNetwork), timeout)
}

// WaitForNADCreation waits for the NAD to be created.
func WaitForNADCreation(name, namespace string, timeout time.Duration) error {
	if err := APIClient.AttachScheme(nadV1.AddToScheme); err != nil {
		return fmt.Errorf("failed to add NAD scheme to client: %w", err)
	}

	return wait.PollUntilContextTimeout(context.TODO(),
		netparam.DefaultRetryInterval, timeout, true, func(ctx context.Context) (bool, error) {
			var testNAD nadV1.NetworkAttachmentDefinition

			err := APIClient.Client.Get(ctx, k8sclient.ObjectKey{Name: name, Namespace: namespace}, &testNAD)
			if err == nil {
				return true, nil
			}

			if k8serrors.IsNotFound(err) {
				klog.V(100).Infof("NAD %s in namespace %s not found yet: %v", name, namespace, err)

				return false, nil
			}

			return false, err
		})
}

// TargetNamespaceOf returns the target namespace of a SriovNetwork.
// If the target namespace is not set, it returns the namespace of the SriovNetwork.
func TargetNamespaceOf(sriovNetwork *sriov.NetworkBuilder) string {
	if sriovNetwork.Object.Spec.NetworkNamespace != "" {
		return sriovNetwork.Object.Spec.NetworkNamespace
	}

	return sriovNetwork.Object.Namespace
}
