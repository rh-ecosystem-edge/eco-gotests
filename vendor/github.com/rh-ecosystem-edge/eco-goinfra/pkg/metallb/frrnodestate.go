package metallb

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/metallb/frrtypes"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// FrrNodeStateBuilder provides struct for FrrNodeState object which contains connection to cluster and
// frrconfiguration definitions.
type FrrNodeStateBuilder struct {
	Definition *frrtypes.FRRNodeState
	Object     *frrtypes.FRRNodeState
	apiClient  runtimeClient.Client
	errorMsg   string
}

// PullFrrNodeState retrieves an existing FrrNodeState object from the cluster.
func PullFrrNodeState(apiClient *clients.Settings, name string) (*FrrNodeStateBuilder, error) {
	glog.V(100).Infof("Pulling FrrNodeState object name:%s", name)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient cannot be nil")

		return nil, fmt.Errorf("the apiClient cannot be nil")
	}

	err := apiClient.AttachScheme(frrtypes.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add FrrNodeState scheme to client schemes")

		return nil, err
	}

	frrStateBuilder := &FrrNodeStateBuilder{
		apiClient: apiClient.Client,
		Definition: &frrtypes.FRRNodeState{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the FrrNodeState is empty")

		return nil, fmt.Errorf("frrNodeState 'name' cannot be empty")
	}

	if !frrStateBuilder.Exists() {
		return nil, fmt.Errorf("frrNodeState object %s does not exist", name)
	}

	frrStateBuilder.Definition = frrStateBuilder.Object

	return frrStateBuilder, nil
}

// Exists checks whether the given FRRNodeState exists.
func (builder *FrrNodeStateBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Checking if FrrNodeState %s exists", builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Get returns FrrNodeState object if found.
func (builder *FrrNodeStateBuilder) Get() (*frrtypes.FRRNodeState, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof("Collecting FrrNodeState object %s", builder.Definition.Name)

	frrNodeState := &frrtypes.FRRNodeState{}

	err := builder.apiClient.Get(context.TODO(), runtimeClient.ObjectKey{
		Name: builder.Definition.Name,
	}, frrNodeState)
	if err != nil {
		glog.V(100).Infof("FrrNodeState object %s does not exist", builder.Definition.Name)

		return nil, err
	}

	return frrNodeState, nil
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *FrrNodeStateBuilder) validate() (bool, error) {
	resourceCRD := "frrnodestate"

	if builder == nil {
		glog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.apiClient == nil {
		glog.V(100).Infof("The %s builder apiclient is nil", resourceCRD)

		return false, fmt.Errorf("%s builder cannot have nil apiClient", resourceCRD)
	}

	if builder.errorMsg != "" {
		glog.V(100).Infof("The %s builder has error message: %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
