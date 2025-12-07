package metallb

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/metallb/frrtypes"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// BGPSessionStateBuilder provides struct for BGPSessionState object which contains connection to cluster and
// BGPSessionState definitions.
type BGPSessionStateBuilder struct {
	Definition *frrtypes.BGPSessionState
	Object     *frrtypes.BGPSessionState
	apiClient  runtimeClient.Client
	errorMsg   string
}

// PullBGPSessionState retrieves an existing BGPSessionState object from the cluster.
func PullBGPSessionState(apiClient *clients.Settings, name string) (*BGPSessionStateBuilder, error) {
	klog.V(100).Infof("Pulling BGPSessionState object name:%s", name)

	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil, fmt.Errorf("the apiClient cannot be nil")
	}

	err := apiClient.AttachScheme(frrtypes.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add BGPSessionState scheme to client schemes")

		return nil, err
	}

	bgpSessionStateBuilder := &BGPSessionStateBuilder{
		apiClient: apiClient.Client,
		Definition: &frrtypes.BGPSessionState{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the BGPSessionState is empty")

		return nil, fmt.Errorf("BGPSessionState 'name' cannot be empty")
	}

	if !bgpSessionStateBuilder.Exists() {
		return nil, fmt.Errorf("BGPSessionState object %s does not exist", name)
	}

	bgpSessionStateBuilder.Definition = bgpSessionStateBuilder.Object

	return bgpSessionStateBuilder, nil
}

// PullBGPSessionStateByNodeAndPeer retrieves a BGPSessionState object by node name and peer IP.
// Since BGPSessionState names are auto-generated with random suffixes, this function
// lists all BGPSessionStates and filters by the node name and peer IP in the Status field.
func PullBGPSessionStateByNodeAndPeer(apiClient *clients.Settings, nodeName, peerIP string) (*BGPSessionStateBuilder, error) {
	klog.V(100).Infof("Pulling BGPSessionState object by node name:%s and peer IP:%s", nodeName, peerIP)

	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil, fmt.Errorf("the apiClient cannot be nil")
	}

	if nodeName == "" {
		klog.V(100).Info("The node name cannot be empty")

		return nil, fmt.Errorf("node name cannot be empty")
	}

	if peerIP == "" {
		klog.V(100).Info("The peer IP cannot be empty")

		return nil, fmt.Errorf("peer IP cannot be empty")
	}

	bgpSessionStates, err := ListBGPSessionState(apiClient)
	if err != nil {
		klog.V(100).Infof("Failed to list BGPSessionStates: %s", err.Error())

		return nil, fmt.Errorf("failed to list BGPSessionStates: %w", err)
	}

	for _, bgpSessionState := range bgpSessionStates {
		if bgpSessionState.Object != nil &&
			bgpSessionState.Object.Status.Node == nodeName &&
			bgpSessionState.Object.Status.Peer == peerIP {
			if valid, err := bgpSessionState.validate(); !valid {
				return nil, err
			}

			return bgpSessionState, nil
		}
	}

	return nil, fmt.Errorf("BGPSessionState for node %s and peer %s not found", nodeName, peerIP)
}

// Exists checks whether the given BGPSessionState exists.
func (builder *BGPSessionStateBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if BGPSessionState %s exists", builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Get returns BGPSessionState object if found.
func (builder *BGPSessionStateBuilder) Get() (*frrtypes.BGPSessionState, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Collecting BGPSessionState object %s", builder.Definition.Name)

	bgpSessionState := &frrtypes.BGPSessionState{}

	err := builder.apiClient.Get(logging.DiscardContext(), runtimeClient.ObjectKey{
		Name: builder.Definition.Name,
	}, bgpSessionState)
	if err != nil {
		klog.V(100).Infof("BGPSessionState object %s does not exist", builder.Definition.Name)

		return nil, err
	}

	return bgpSessionState, nil
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *BGPSessionStateBuilder) validate() (bool, error) {
	resourceCRD := "bgpsessionstate"

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
