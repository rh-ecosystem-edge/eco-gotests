package lso

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	corev1 "k8s.io/api/core/v1"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/golang/glog"
	lsov1alpha1 "github.com/openshift/local-storage-operator/api/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// LocalVolumeDiscoveryBuilder provides a struct for localVolumeDiscovery object from the cluster
// and a localVolumeDiscovery definition.
type LocalVolumeDiscoveryBuilder struct {
	// localVolumeDiscovery definition, used to create the localVolumeDiscovery object.
	Definition *lsov1alpha1.LocalVolumeDiscovery
	// Created localVolumeDiscovery object.
	Object *lsov1alpha1.LocalVolumeDiscovery
	// Used in functions that define or mutate localVolumeDiscovery definition. errorMsg is processed
	// before the localVolumeDiscovery object is created
	errorMsg string
	// api client to interact with the cluster.
	apiClient goclient.Client
}

// NewLocalVolumeDiscoveryBuilder creates new instance of LocalVolumeDiscoveryBuilder.
func NewLocalVolumeDiscoveryBuilder(apiClient *clients.Settings, name, nsname string) *LocalVolumeDiscoveryBuilder {
	glog.V(100).Infof("Initializing new localVolumeDiscovery structure with the following params: name: "+
		"%s, namespace: %s", name, nsname)

	if apiClient == nil {
		glog.V(100).Infof("localVolumeDiscovery 'apiClient' cannot be empty")

		return nil
	}

	err := apiClient.AttachScheme(lsov1alpha1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add lsov1alpha1 scheme to client schemes")

		return nil
	}

	builder := &LocalVolumeDiscoveryBuilder{
		apiClient: apiClient.Client,
		Definition: &lsov1alpha1.LocalVolumeDiscovery{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the localVolumeDiscovery is empty")

		builder.errorMsg = "localVolumeDiscovery 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		glog.V(100).Infof("The nsname of the localVolumeDiscovery is empty")

		builder.errorMsg = "localVolumeDiscovery 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// PullLocalVolumeDiscovery retrieves an existing localVolumeDiscovery object from the cluster.
func PullLocalVolumeDiscovery(apiClient *clients.Settings, name, nsname string) (*LocalVolumeDiscoveryBuilder, error) {
	glog.V(100).Infof(
		"Pulling localVolumeDiscovery object name: %s in namespace: %s", name, nsname)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient is empty")

		return nil, fmt.Errorf("localVolumeDiscovery 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(lsov1alpha1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add lsov1alpha1 scheme to client schemes")

		return nil, err
	}

	builder := &LocalVolumeDiscoveryBuilder{
		apiClient: apiClient.Client,
		Definition: &lsov1alpha1.LocalVolumeDiscovery{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the localVolumeDiscovery is empty")

		return nil, fmt.Errorf("localVolumeDiscovery 'name' cannot be empty")
	}

	if nsname == "" {
		glog.V(100).Infof("The namespace of the localVolumeDiscovery is empty")

		return nil, fmt.Errorf("localVolumeDiscovery 'nsname' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("localVolumeDiscovery object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get fetches existing localVolumeDiscovery from cluster.
func (builder *LocalVolumeDiscoveryBuilder) Get() (*lsov1alpha1.LocalVolumeDiscovery, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof("Pulling existing localVolumeDiscovery with name %s under namespace %s from cluster",
		builder.Definition.Name, builder.Definition.Namespace)

	lvd := &lsov1alpha1.LocalVolumeDiscovery{}

	err := builder.apiClient.Get(context.TODO(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, lvd)
	if err != nil {
		return nil, err
	}

	return lvd, nil
}

// Create makes a localVolumeDiscovery in the cluster and stores the created object in struct.
func (builder *LocalVolumeDiscoveryBuilder) Create() (*LocalVolumeDiscoveryBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Creating the localVolumeDiscovery %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error
	if !builder.Exists() {
		err = builder.apiClient.Create(context.TODO(), builder.Definition)
		if err == nil {
			builder.Object = builder.Definition
		}
	}

	return builder, err
}

// Delete removes localVolumeDiscovery from a cluster.
func (builder *LocalVolumeDiscoveryBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	glog.V(100).Infof("Deleting the localVolumeDiscovery %s from namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		glog.V(100).Infof("localVolumeDiscovery %s not found in the namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		return nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
	if err != nil {
		return fmt.Errorf("can not delete localVolumeDiscovery %s from namespace %s: %w",
			builder.Definition.Name, builder.Definition.Namespace, err)
	}

	builder.Object = nil

	return nil
}

// Exists checks whether the given localVolumeDiscovery exists.
func (builder *LocalVolumeDiscoveryBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Checking if localVolumeDiscovery %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// IsDiscovering check if the localVolumeDiscovery is Discovering.
func (builder *LocalVolumeDiscoveryBuilder) IsDiscovering(timeout time.Duration) bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Verify localVolumeDiscovery %s in namespace %s is in Discovering phase",
		builder.Definition.Name, builder.Definition.Namespace)

	err := wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			var err error

			phase, err := builder.GetPhase()
			if err != nil {
				glog.V(100).Infof("failed to get phase value for localVolumeDiscovery %s in namespace %s due to %w",
					builder.Definition.Name, builder.Definition.Namespace, err)

				return false, nil
			}

			return phase == "Discovering", nil
		})
	if err != nil {
		glog.V(100).Infof("localVolumeDiscovery %s in namespace %s is found not in the discovering state; %w",
			builder.Definition.Name, builder.Definition.Namespace, err)

		return false
	}

	return true
}

// GetPhase get current localVolumeDiscovery phase.
func (builder *LocalVolumeDiscoveryBuilder) GetPhase() (lsov1alpha1.DiscoveryPhase, error) {
	if valid, err := builder.validate(); !valid {
		return "", err
	}

	glog.V(100).Infof("Get %s localVolumeDiscovery in %s namespace phase",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		return "", fmt.Errorf("%s localVolumeDiscovery not found in %s namespace",
			builder.Definition.Name, builder.Definition.Namespace)
	}

	return builder.Object.Status.Phase, nil
}

// WithNodeSelector sets the localVolumeDiscovery's nodeSelector.
func (builder *LocalVolumeDiscoveryBuilder) WithNodeSelector(
	nodeSelector corev1.NodeSelector) *LocalVolumeDiscoveryBuilder {
	glog.V(100).Infof(
		"Adding nodeSelector %v to localVolumeDiscovery %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace, nodeSelector)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	builder.Definition.Spec.NodeSelector = &nodeSelector

	return builder
}

// WithTolerations sets the localVolumeDiscovery's generation.
func (builder *LocalVolumeDiscoveryBuilder) WithTolerations(
	tolerations []corev1.Toleration) *LocalVolumeDiscoveryBuilder {
	glog.V(100).Infof(
		"Adding tolerations %v to localVolumeDiscovery %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace, tolerations)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if len(tolerations) == 0 {
		glog.V(100).Infof("The tolerations list is empty")

		builder.errorMsg = "'tolerations' argument cannot be empty"

		return builder
	}

	builder.Definition.Spec.Tolerations = tolerations

	return builder
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *LocalVolumeDiscoveryBuilder) validate() (bool, error) {
	resourceCRD := "LocalVolumeDiscovery"

	if builder == nil {
		glog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.Definition == nil {
		glog.V(100).Infof("The %s is undefined", resourceCRD)

		return false, fmt.Errorf("%s", msg.UndefinedCrdObjectErrString(resourceCRD))
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
