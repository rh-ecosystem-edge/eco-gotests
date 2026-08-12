package capoa

import (
	"context"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	hiveext "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/api/hiveextension/v1beta1"
	v1alpha3 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/capoa/controlplane/v1alpha3"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// OpenshiftAssistedControlPlaneBuilder provides a struct for the OpenshiftAssistedControlPlane object
// containing connection to the cluster and the resource definitions.
type OpenshiftAssistedControlPlaneBuilder struct {
	Definition *v1alpha3.OpenshiftAssistedControlPlane
	Object     *v1alpha3.OpenshiftAssistedControlPlane
	errorMsg   string
	apiClient  goclient.Client
}

// NewOpenshiftAssistedControlPlaneBuilder creates a new instance of OpenshiftAssistedControlPlaneBuilder.
func NewOpenshiftAssistedControlPlaneBuilder(
	apiClient *clients.Settings,
	name, nsname, baseDomain, distributionVersion string,
	replicas int32,
) *OpenshiftAssistedControlPlaneBuilder {
	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil
	}

	err := apiClient.AttachScheme(v1alpha3.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add CAPOA controlplane v1alpha3 scheme to client schemes")

		return nil
	}

	builder := &OpenshiftAssistedControlPlaneBuilder{
		apiClient: apiClient.Client,
		Definition: &v1alpha3.OpenshiftAssistedControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
			Spec: v1alpha3.OpenshiftAssistedControlPlaneSpec{
				Config: v1alpha3.OpenshiftAssistedControlPlaneConfigSpec{
					BaseDomain: baseDomain,
				},
				DistributionVersion: distributionVersion,
				Replicas:            replicas,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the OpenshiftAssistedControlPlane is empty")

		builder.errorMsg = "OpenshiftAssistedControlPlane 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the OpenshiftAssistedControlPlane is empty")

		builder.errorMsg = "OpenshiftAssistedControlPlane 'namespace' cannot be empty"

		return builder
	}

	if baseDomain == "" {
		klog.V(100).Info("The baseDomain of the OpenshiftAssistedControlPlane is empty")

		builder.errorMsg = "OpenshiftAssistedControlPlane 'baseDomain' cannot be empty"

		return builder
	}

	if distributionVersion == "" {
		klog.V(100).Info("The distributionVersion of the OpenshiftAssistedControlPlane is empty")

		builder.errorMsg = "OpenshiftAssistedControlPlane 'distributionVersion' cannot be empty"

		return builder
	}

	return builder
}

// PullOpenshiftAssistedControlPlane pulls an existing OpenshiftAssistedControlPlane from the cluster.
func PullOpenshiftAssistedControlPlane(
	apiClient *clients.Settings, name, nsname string,
) (*OpenshiftAssistedControlPlaneBuilder, error) {
	klog.V(100).Infof(
		"Pulling existing OpenshiftAssistedControlPlane %s under namespace %s from cluster", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil, fmt.Errorf("the apiClient is nil")
	}

	err := apiClient.AttachScheme(v1alpha3.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add CAPOA controlplane v1alpha3 scheme to client schemes")

		return nil, err
	}

	builder := &OpenshiftAssistedControlPlaneBuilder{
		apiClient: apiClient.Client,
		Definition: &v1alpha3.OpenshiftAssistedControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the OpenshiftAssistedControlPlane is empty")

		return nil, fmt.Errorf("OpenshiftAssistedControlPlane 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the OpenshiftAssistedControlPlane is empty")

		return nil, fmt.Errorf("OpenshiftAssistedControlPlane 'namespace' cannot be empty")
	}

	oacp, err := builder.Get()
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, fmt.Errorf(
				"OpenshiftAssistedControlPlane object %s does not exist in namespace %s", name, nsname)
		}

		return nil, err
	}

	builder.Definition = oacp
	builder.Object = oacp

	return builder, nil
}

// WithNetworkType sets the networkType for the control plane configuration.
func (builder *OpenshiftAssistedControlPlaneBuilder) WithNetworkType(
	networkType string,
) *OpenshiftAssistedControlPlaneBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting networkType %s on OpenshiftAssistedControlPlane %s in namespace %s",
		networkType, builder.Definition.Name, builder.Definition.Namespace)

	builder.Definition.Spec.Config.NetworkType = networkType

	return builder
}

// GetNetworkType returns the networkType from the object's spec.
func (builder *OpenshiftAssistedControlPlaneBuilder) GetNetworkType() string {
	if valid, _ := builder.validate(); !valid {
		return ""
	}

	if builder.Object != nil {
		return builder.Object.Spec.Config.NetworkType
	}

	return builder.Definition.Spec.Config.NetworkType
}

// WithAPIVIPs sets the API VIPs for the control plane configuration.
func (builder *OpenshiftAssistedControlPlaneBuilder) WithAPIVIPs(
	vips []string,
) *OpenshiftAssistedControlPlaneBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting apiVIPs on OpenshiftAssistedControlPlane %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	builder.Definition.Spec.Config.APIVIPs = vips

	return builder
}

// WithIngressVIPs sets the ingress VIPs for the control plane configuration.
func (builder *OpenshiftAssistedControlPlaneBuilder) WithIngressVIPs(
	vips []string,
) *OpenshiftAssistedControlPlaneBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting ingressVIPs on OpenshiftAssistedControlPlane %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	builder.Definition.Spec.Config.IngressVIPs = vips

	return builder
}

// WithPullSecretRef sets the pull secret reference for the control plane configuration.
func (builder *OpenshiftAssistedControlPlaneBuilder) WithPullSecretRef(
	name string,
) *OpenshiftAssistedControlPlaneBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting pullSecretRef %s on OpenshiftAssistedControlPlane %s in namespace %s",
		name, builder.Definition.Name, builder.Definition.Namespace)

	if name == "" {
		builder.errorMsg = "OpenshiftAssistedControlPlane pullSecretRef name cannot be empty"

		return builder
	}

	builder.Definition.Spec.Config.PullSecretRef = &corev1.LocalObjectReference{Name: name}

	return builder
}

// WithManifestsConfigMapRefs sets the manifests ConfigMap references.
func (builder *OpenshiftAssistedControlPlaneBuilder) WithManifestsConfigMapRefs(
	refs []hiveext.ManifestsConfigMapReference,
) *OpenshiftAssistedControlPlaneBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting manifestsConfigMapRefs on OpenshiftAssistedControlPlane %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	builder.Definition.Spec.Config.ManifestsConfigMapRefs = refs

	return builder
}

// WithMastersSchedulable sets whether control plane nodes are schedulable.
func (builder *OpenshiftAssistedControlPlaneBuilder) WithMastersSchedulable(
	schedulable bool,
) *OpenshiftAssistedControlPlaneBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting mastersSchedulable %v on OpenshiftAssistedControlPlane %s in namespace %s",
		schedulable, builder.Definition.Name, builder.Definition.Namespace)

	builder.Definition.Spec.Config.MastersSchedulable = schedulable

	return builder
}

// WithSSHAuthorizedKey sets the SSH authorized key for cluster node access.
func (builder *OpenshiftAssistedControlPlaneBuilder) WithSSHAuthorizedKey(
	key string,
) *OpenshiftAssistedControlPlaneBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting sshAuthorizedKey on OpenshiftAssistedControlPlane %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	builder.Definition.Spec.Config.SSHAuthorizedKey = key

	return builder
}

// WithMachineTemplate sets the machine template for the control plane.
func (builder *OpenshiftAssistedControlPlaneBuilder) WithMachineTemplate(
	template v1alpha3.OpenshiftAssistedControlPlaneMachineTemplate,
) *OpenshiftAssistedControlPlaneBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting machineTemplate on OpenshiftAssistedControlPlane %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	builder.Definition.Spec.MachineTemplate = template

	return builder
}

// WithReplicas sets the number of control plane replicas.
func (builder *OpenshiftAssistedControlPlaneBuilder) WithReplicas(
	replicas int32,
) *OpenshiftAssistedControlPlaneBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting replicas %d on OpenshiftAssistedControlPlane %s in namespace %s",
		replicas, builder.Definition.Name, builder.Definition.Namespace)

	builder.Definition.Spec.Replicas = replicas

	return builder
}

// WithClusterName sets the cluster name in the control plane configuration.
func (builder *OpenshiftAssistedControlPlaneBuilder) WithClusterName(
	name string,
) *OpenshiftAssistedControlPlaneBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting clusterName %s on OpenshiftAssistedControlPlane %s in namespace %s",
		name, builder.Definition.Name, builder.Definition.Namespace)

	builder.Definition.Spec.Config.ClusterName = name

	return builder
}

// Get fetches the defined OpenshiftAssistedControlPlane from the cluster.
func (builder *OpenshiftAssistedControlPlaneBuilder) Get() (
	*v1alpha3.OpenshiftAssistedControlPlane, error,
) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting OpenshiftAssistedControlPlane %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	oacp := &v1alpha3.OpenshiftAssistedControlPlane{}

	err := builder.apiClient.Get(logging.DiscardContext(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, oacp)
	if err != nil {
		return nil, err
	}

	return oacp, nil
}

// Exists checks if the defined OpenshiftAssistedControlPlane has already been created.
func (builder *OpenshiftAssistedControlPlaneBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if OpenshiftAssistedControlPlane %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Create generates an OpenshiftAssistedControlPlane on the cluster.
func (builder *OpenshiftAssistedControlPlaneBuilder) Create() (
	*OpenshiftAssistedControlPlaneBuilder, error,
) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the OpenshiftAssistedControlPlane %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	existing, err := builder.Get()
	if err == nil {
		builder.Object = existing

		return builder, nil
	}

	if !k8serrors.IsNotFound(err) {
		return builder, err
	}

	err = builder.apiClient.Create(logging.DiscardContext(), builder.Definition)
	if err == nil {
		builder.Object = builder.Definition
	}

	return builder, err
}

// Update modifies an existing OpenshiftAssistedControlPlane on the cluster.
func (builder *OpenshiftAssistedControlPlaneBuilder) Update(force bool) (
	*OpenshiftAssistedControlPlaneBuilder, error,
) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating OpenshiftAssistedControlPlane %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		return nil, fmt.Errorf("cannot update non-existent OpenshiftAssistedControlPlane")
	}

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err != nil {
		if force {
			klog.V(100).Infof(
				"%v", msg.FailToUpdateNotification("OpenshiftAssistedControlPlane",
					builder.Definition.Name, builder.Definition.Namespace))

			err = builder.DeleteAndWait(time.Second * 10)
			builder.Definition.ResourceVersion = ""

			if err != nil {
				klog.V(100).Infof(
					"%v", msg.FailToUpdateError("OpenshiftAssistedControlPlane",
						builder.Definition.Name, builder.Definition.Namespace))

				return nil, err
			}

			return builder.Create()
		}
	}

	if err == nil {
		builder.Object = builder.Definition
	}

	return builder, err
}

// Delete removes an OpenshiftAssistedControlPlane from the cluster.
func (builder *OpenshiftAssistedControlPlaneBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Deleting the OpenshiftAssistedControlPlane %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("OpenshiftAssistedControlPlane %s in namespace %s does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Definition)

	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("cannot delete OpenshiftAssistedControlPlane: %w", err)
	}

	builder.Object = nil

	return nil
}

// DeleteAndWait deletes an OpenshiftAssistedControlPlane and waits until it is removed from the cluster.
func (builder *OpenshiftAssistedControlPlaneBuilder) DeleteAndWait(timeout time.Duration) error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof(
		"Deleting OpenshiftAssistedControlPlane %s in namespace %s and waiting for the defined period until it is removed",
		builder.Definition.Name, builder.Definition.Namespace)

	if err := builder.Delete(); err != nil {
		return err
	}

	return wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			_, err := builder.Get()
			if k8serrors.IsNotFound(err) {
				return true, nil
			}

			return false, nil
		})
}

func (builder *OpenshiftAssistedControlPlaneBuilder) validate() (bool, error) {
	resourceCRD := "OpenshiftAssistedControlPlane"

	if builder == nil {
		klog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.Definition == nil {
		klog.V(100).Infof("The %s is undefined", resourceCRD)

		return false, fmt.Errorf("%s", msg.UndefinedCrdObjectErrString(resourceCRD))
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
