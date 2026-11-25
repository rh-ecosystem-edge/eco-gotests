package sriov

import (
	"context"
	"fmt"
	"time"

	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"

	srIovV1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const (
	nodeStatePollInterval = time.Second
	syncStatusSucceeded   = "Succeeded"
)

// NetworkNodeStateBuilder provides struct for SriovNetworkNodeState object which contains connection to cluster and
// SriovNetworkNodeState definitions.
type NetworkNodeStateBuilder struct {
	// Dynamically discovered SriovNetworkNodeState object.
	Objects *srIovV1.SriovNetworkNodeState
	// apiClient opens api connection to the cluster.
	apiClient runtimeClient.Client
	// nodeName defines on what node SriovNetworkNodeState resource should be queried.
	nodeName string
	// nsName defines SrIov operator namespace.
	nsName string
	// errorMsg used in discovery function before sending api request to cluster.
	errorMsg string
}

// NewNetworkNodeStateBuilder creates new instance of NetworkNodeStateBuilder.
func NewNetworkNodeStateBuilder(apiClient *clients.Settings, nodeName, nsname string) *NetworkNodeStateBuilder {
	klog.V(100).Infof(
		"Initializing new NetworkNodeStateBuilder structure with the following params: %s, %s",
		nodeName, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil
	}

	err := apiClient.AttachScheme(srIovV1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add sriovv1 scheme to client schemes")

		return nil
	}

	builder := &NetworkNodeStateBuilder{
		apiClient: apiClient.Client,
		nodeName:  nodeName,
		nsName:    nsname,
	}

	if nodeName == "" {
		klog.V(100).Info("The name of the nodeName is empty")

		builder.errorMsg = "SriovNetworkNodeState 'nodeName' is empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the SriovNetworkNodeState is empty")

		builder.errorMsg = "SriovNetworkNodeState 'nsname' is empty"

		return builder
	}

	return builder
}

// Discover method gets the SriovNetworkNodeState items and stores them in the NetworkNodeStateBuilder struct.
func (builder *NetworkNodeStateBuilder) Discover() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	return builder.discover(logging.DiscardContext())
}

func (builder *NetworkNodeStateBuilder) discover(ctx context.Context) error {
	klog.V(100).Infof("Getting the SriovNetworkNodeState object in namespace %s for node %s",
		builder.nsName, builder.nodeName)

	nodeNetworkState := &srIovV1.SriovNetworkNodeState{}

	err := builder.apiClient.Get(logging.WithDiscardLogger(ctx),
		runtimeClient.ObjectKey{Name: builder.nodeName, Namespace: builder.nsName}, nodeNetworkState)
	if err == nil {
		builder.Objects = nodeNetworkState
	}

	return err
}

// GetUpNICs returns a list of SrIov interfaces in UP state.
func (builder *NetworkNodeStateBuilder) GetUpNICs() (srIovV1.InterfaceExts, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Collection of sriov interfaces in UP state for node %s", builder.nodeName)

	sriovNics, err := builder.GetNICs()
	if err != nil {
		klog.V(100).Infof("Error to discover sriov interfaces for node %s", builder.nodeName)

		return nil, err
	}

	var sriovNicsUp srIovV1.InterfaceExts

	for _, nic := range sriovNics {
		if nic.LinkSpeed != "" && nic.LinkSpeed != "-1 Mb/s" {
			klog.V(100).Infof("Interface %s is UP on node %s. Append to list", nic.Name, builder.nodeName)
			sriovNicsUp = append(sriovNicsUp, nic)
		}
	}

	klog.V(100).Infof("Collected sriov UP interfaces list %v for node %s",
		builder.Objects.Status.Interfaces, builder.nodeName)

	return sriovNicsUp, nil
}

// GetNICs returns a list of SrIov interfaces.
func (builder *NetworkNodeStateBuilder) GetNICs() (srIovV1.InterfaceExts, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	if err := builder.Discover(); err != nil {
		klog.V(100).Infof("Error to discover sriov interfaces for node %s", builder.nodeName)

		return nil, err
	}

	klog.V(100).Infof("Collected sriov interfaces list %v for node %s",
		builder.Objects.Status.Interfaces, builder.nodeName)

	return builder.Objects.Status.Interfaces, nil
}

// WaitUntilSyncStatus waits until timeout or until the requested syncStatus is reported.
func (builder *NetworkNodeStateBuilder) WaitUntilSyncStatus(syncStatus string, timeout time.Duration) error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Waiting up to %s until SriovNetworkNodeState %s/%s has syncStatus %s",
		timeout, builder.nsName, builder.nodeName, syncStatus)

	if syncStatus == "" {
		klog.V(100).Info("The syncStatus parameter is empty")

		return fmt.Errorf("syncStatus cannot be empty")
	}

	return wait.PollUntilContextTimeout(
		context.TODO(), nodeStatePollInterval, timeout, true, func(ctx context.Context) (bool, error) {
			err := builder.discover(ctx)
			if err != nil {
				klog.V(100).Infof("Failed to get SriovNetworkNodeState %s/%s: %v",
					builder.nsName, builder.nodeName, err)

				return false, nil
			}

			return builder.Objects.Status.SyncStatus == syncStatus, nil
		})
}

// WaitUntilStable waits until timeout for generation-fresh Ready=True and
// Progressing!=True, Draining not True, and syncStatus Succeeded. Draining is
// not required to be generation-fresh: the operator only updates it when drain
// state changes. Use WaitUntilSyncStatus for the syncStatus-only path.
func (builder *NetworkNodeStateBuilder) WaitUntilStable(timeout time.Duration) error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Waiting up to %s until SriovNetworkNodeState %s/%s is stable",
		timeout, builder.nsName, builder.nodeName)

	return wait.PollUntilContextTimeout(
		context.TODO(), nodeStatePollInterval, timeout, true, func(ctx context.Context) (bool, error) {
			err := builder.discover(ctx)
			if err != nil {
				klog.V(100).Infof("Failed to get SriovNetworkNodeState %s/%s: %v",
					builder.nsName, builder.nodeName, err)

				return false, nil
			}

			return isNodeStateStable(builder.Objects), nil
		})
}

// WaitForCondition waits until timeout for a generation-fresh condition matching expected.
func (builder *NetworkNodeStateBuilder) WaitForCondition(expected metav1.Condition, timeout time.Duration) error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	return waitForCondition(
		"SriovNetworkNodeState", builder.nsName, builder.nodeName,
		expected, timeout, func(ctx context.Context) ([]metav1.Condition, int64, error) {
			if err := builder.discover(ctx); err != nil {
				return nil, 0, err
			}

			return builder.Objects.Status.Conditions, builder.Objects.GetGeneration(), nil
		})
}

// GetCondition returns the named condition from the last discovered object, or nil if missing.
func (builder *NetworkNodeStateBuilder) GetCondition(conditionType string) *metav1.Condition {
	if builder == nil || builder.Objects == nil {
		return nil
	}

	return meta.FindStatusCondition(builder.Objects.Status.Conditions, conditionType)
}

// GetNumVFs returns num-vfs under the given interface.
func (builder *NetworkNodeStateBuilder) GetNumVFs(sriovInterfaceName string) (int, error) {
	klog.V(100).Infof("Getting num-vfs under interface %s from SriovNetworkNodeState %s",
		sriovInterfaceName, builder.nodeName)

	interf, err := builder.findInterfaceByName(sriovInterfaceName)
	if err != nil {
		return 0, err
	}

	return interf.NumVfs, nil
}

// GetDriverName returns driver name under the given interface.
func (builder *NetworkNodeStateBuilder) GetDriverName(sriovInterfaceName string) (string, error) {
	klog.V(100).Infof("Getting driver name for interface %s from SriovNetworkNodeState %s",
		sriovInterfaceName, builder.nodeName)

	interf, err := builder.findInterfaceByName(sriovInterfaceName)
	if err != nil {
		return "", err
	}

	return interf.Driver, nil
}

// GetPciAddress returns PciAddress under the given interface.
func (builder *NetworkNodeStateBuilder) GetPciAddress(sriovInterfaceName string) (string, error) {
	klog.V(100).Infof("Getting PCI address for interface %s from SriovNetworkNodeState %s",
		sriovInterfaceName, builder.nodeName)

	interf, err := builder.findInterfaceByName(sriovInterfaceName)
	if err != nil {
		return "", err
	}

	return interf.PciAddress, nil
}

// GetTotalVFs returns total VFs under the given interface.
func (builder *NetworkNodeStateBuilder) GetTotalVFs(sriovInterfaceName string) (int, error) {
	klog.V(100).Infof("Getting totalvfs under interface %s from SriovNetworkNodeState %s",
		sriovInterfaceName, builder.nodeName)

	interf, err := builder.findInterfaceByName(sriovInterfaceName)
	if err != nil {
		return 0, err
	}

	return interf.TotalVfs, nil
}

func (builder *NetworkNodeStateBuilder) findInterfaceByName(sriovInterfaceName string) (*srIovV1.InterfaceExt, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	if err := builder.Discover(); err != nil {
		klog.V(100).Infof("Error to discover sriov network node state for node %s", builder.nodeName)

		builder.errorMsg = "failed to discover sriov network node state"

		return nil, err
	}

	if sriovInterfaceName == "" {
		klog.V(100).Info("The sriovInterface can not be empty string")

		builder.errorMsg = "the sriovInterface is an empty sting"

		return nil, fmt.Errorf("sriovInterface can not be empty string")
	}

	for _, interf := range builder.Objects.Status.Interfaces {
		if interf.Name == sriovInterfaceName {
			return &interf, nil
		}
	}

	return nil, fmt.Errorf("interface %s was not found", sriovInterfaceName)
}

func isNodeStateStable(nodeState *srIovV1.SriovNetworkNodeState) bool {
	if nodeState == nil {
		return false
	}

	ready := meta.FindStatusCondition(nodeState.Status.Conditions, srIovV1.ConditionReady)
	progressing := meta.FindStatusCondition(nodeState.Status.Conditions, srIovV1.ConditionProgressing)
	draining := meta.FindStatusCondition(nodeState.Status.Conditions, srIovV1.ConditionDraining)

	if !isConditionCurrent(nodeState, ready) || ready.Status != metav1.ConditionTrue {
		logUnstableNodeState(nodeState, "Ready is missing, stale, or not True")

		return false
	}

	if !isConditionCurrent(nodeState, progressing) || progressing.Status == metav1.ConditionTrue {
		logUnstableNodeState(nodeState, "Progressing is missing, stale, or True")

		return false
	}

	// The operator only refreshes Draining when drain state changes. A spec
	// update that does not drain leaves Draining at an older observedGeneration
	// (DrainNotNeeded). Fail only when Draining is True.
	if draining != nil && draining.Status == metav1.ConditionTrue {
		logUnstableNodeState(nodeState, "Draining is True")

		return false
	}

	if nodeState.Status.SyncStatus != syncStatusSucceeded {
		logUnstableNodeState(nodeState, "conditions are stable but syncStatus is not Succeeded")

		return false
	}

	return true
}

func isConditionCurrent(nodeState *srIovV1.SriovNetworkNodeState, condition *metav1.Condition) bool {
	if nodeState == nil {
		return false
	}

	return isConditionGenerationCurrent(nodeState.GetGeneration(), condition)
}

func logUnstableNodeState(nodeState *srIovV1.SriovNetworkNodeState, reason string) {
	if nodeState == nil {
		return
	}

	klog.V(100).Infof("SriovNetworkNodeState %s/%s is not stable: %s; syncStatus=%q lastSyncError=%q",
		nodeState.Namespace, nodeState.Name, reason, nodeState.Status.SyncStatus, nodeState.Status.LastSyncError)
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *NetworkNodeStateBuilder) validate() (bool, error) {
	resourceCRD := "SriovNetworkNodeState"

	if builder == nil {
		klog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.apiClient == nil {
		klog.V(100).Infof("The %s builder apiclient is nil", resourceCRD)

		return false, fmt.Errorf("%s builder cannot have nil apiClient", resourceCRD)
	}

	if builder.errorMsg != "" {
		klog.V(100).Infof("The %s builder has error message: %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
