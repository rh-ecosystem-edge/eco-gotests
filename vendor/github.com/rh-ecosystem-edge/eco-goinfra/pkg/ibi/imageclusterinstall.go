package ibi

import (
	"fmt"
	"net"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	ibiv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/imagebasedinstall/api/hiveextensions/v1alpha1"
	hivev1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/imagebasedinstall/hive/api/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ImageClusterInstallBuilder provides struct for the imageclusterinstall object containing connection to
// the cluster and the imageclusterinstall definitions.
type ImageClusterInstallBuilder struct {
	Definition *ibiv1alpha1.ImageClusterInstall
	Object     *ibiv1alpha1.ImageClusterInstall
	errorMsg   string
	apiClient  goclient.Client
}

// NewImageClusterInstallBuilder creates a new instance of ImageClusterInstallBuilder.
func NewImageClusterInstallBuilder(
	apiClient *clients.Settings, name, nsname, imageset string) *ImageClusterInstallBuilder {
	klog.V(100).Infof(
		"Initializing new imageclusterinstall structure with the following params: "+
			"name: %s, namespace: %s, imageset: %s",
		name, nsname, imageset)

	if apiClient == nil {
		return nil
	}

	err := apiClient.AttachScheme(ibiv1alpha1.AddToScheme)
	if err != nil {
		klog.V(100).Infof(
			"Failed to add ibiv1alpha1 scheme to client schemes")

		return nil
	}

	builder := &ImageClusterInstallBuilder{
		apiClient: apiClient.Client,
		Definition: &ibiv1alpha1.ImageClusterInstall{
			Spec: ibiv1alpha1.ImageClusterInstallSpec{
				ImageSetRef: hivev1.ClusterImageSetReference{
					Name: imageset,
				},
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the imageclusterinstall is empty")

		builder.errorMsg = "imageclusterinstall 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the imageclusterinstall is empty")

		builder.errorMsg = "imageclusterinstall 'nsname' cannot be empty"

		return builder
	}

	if imageset == "" {
		klog.V(100).Info("The imageset of the imageclusterinstall is empty")

		builder.errorMsg = "imageclusterinstall 'imageset' cannot be empty"

		return builder
	}

	return builder
}

// PullImageClusterInstall retrieves an existing imageclusterinstall from the cluster.
func PullImageClusterInstall(apiClient *clients.Settings, name, nsname string) (*ImageClusterInstallBuilder, error) {
	klog.V(100).Infof(
		"Pulling existing imageclusterinstall with name %s from namespace %s", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is nil")

		return nil, fmt.Errorf("apiClient cannot be nil")
	}

	err := apiClient.AttachScheme(ibiv1alpha1.AddToScheme)
	if err != nil {
		klog.V(100).Infof(
			"Failed to add ibiv1alpha1 scheme to client schemes")

		return nil, fmt.Errorf("failed to add ibiv1alpha1 to client schemes")
	}

	builder := &ImageClusterInstallBuilder{
		apiClient: apiClient.Client,
		Definition: &ibiv1alpha1.ImageClusterInstall{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the imageclusterinstall is empty")

		return nil, fmt.Errorf("imageclusterinstall 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the imageclusterinstall is empty")

		return nil, fmt.Errorf("imageclusterinstall 'nsname' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("imageclusterinstall object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// WithHostname sets hostname of installed node.
func (builder *ImageClusterInstallBuilder) WithHostname(hostname string) *ImageClusterInstallBuilder {
	if valid, _ := builder.validate(); !valid {
		return nil
	}

	if hostname == "" {
		klog.V(100).Info("The imageclusterinstall hostname is empty")

		builder.errorMsg = "imageclusterinstall hostname cannot be empty"

		return builder
	}

	builder.Definition.Spec.Hostname = hostname

	return builder
}

// WithClusterDeployment links imageclusterinstall to an existing cluster deployment.
func (builder *ImageClusterInstallBuilder) WithClusterDeployment(
	clusterDeploymentName string) *ImageClusterInstallBuilder {
	if valid, _ := builder.validate(); !valid {
		return nil
	}

	if clusterDeploymentName == "" {
		klog.V(100).Info("The imageclusterinstall clusterdeployment is empty")

		builder.errorMsg = "imageclusterinstall clusterdeployment cannot be empty"

		return builder
	}

	if builder.Definition.Spec.ClusterDeploymentRef == nil {
		builder.Definition.Spec.ClusterDeploymentRef = &corev1.LocalObjectReference{}
	}

	builder.Definition.Spec.ClusterDeploymentRef.Name = clusterDeploymentName

	return builder
}

// WithExtraManifests includes manifests via configmap name.
func (builder *ImageClusterInstallBuilder) WithExtraManifests(extraManifestsName string) *ImageClusterInstallBuilder {
	if valid, _ := builder.validate(); !valid {
		return nil
	}

	if extraManifestsName == "" {
		klog.V(100).Info("The imageclusterinstall extramanifest is empty")

		builder.errorMsg = "imageclusterinstall extramanifest cannot be empty"

		return builder
	}

	builder.Definition.Spec.ExtraManifestsRefs =
		append(builder.Definition.Spec.ExtraManifestsRefs, corev1.LocalObjectReference{
			Name: extraManifestsName,
		})

	return builder
}

// WithCABundle sets a CA bundle via configmap name.
func (builder *ImageClusterInstallBuilder) WithCABundle(caBundleConfigMapName string) *ImageClusterInstallBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if caBundleConfigMapName == "" {
		klog.V(100).Info("The imageclusterinstall cabundle is empty")

		builder.errorMsg = "imageclusterinstall cabundle cannot be empty"

		return builder
	}

	builder.Definition.Spec.CABundleRef = &corev1.LocalObjectReference{Name: caBundleConfigMapName}

	return builder
}

// WithMachineNetwork specifies the machine network where nodes will be installed.
func (builder *ImageClusterInstallBuilder) WithMachineNetwork(network string) *ImageClusterInstallBuilder {
	if valid, _ := builder.validate(); !valid {
		return nil
	}

	if _, _, err := net.ParseCIDR(network); err != nil {
		klog.V(100).Info("The machinenetwork is not a properly formatted IP network address")

		builder.errorMsg = "imageclusterinstall machinenetwork incorrectly formatted"

		return builder
	}

	builder.Definition.Spec.MachineNetwork = network

	return builder
}

// WithSSHKey adds specified ssh key to authorized_keys of installed nodes.
func (builder *ImageClusterInstallBuilder) WithSSHKey(sshKey string) *ImageClusterInstallBuilder {
	if valid, _ := builder.validate(); !valid {
		return nil
	}

	if sshKey == "" {
		klog.V(100).Info("The imageclusterinstall sshkey is empty")

		builder.errorMsg = "imageclusterinstall sshkey cannot be empty"

		return builder
	}

	builder.Definition.Spec.SSHKey = sshKey

	return builder
}

// GetCompletedCondition returns Completed condition from imageclusterinstall.
func (builder *ImageClusterInstallBuilder) GetCompletedCondition() (*hivev1.ClusterInstallCondition, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting Completed condition from imageclusterinstall %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	return builder.getCondition(hivev1.ClusterInstallCompleted)
}

// GetFailedCondition returns Failed condition from imageclusterinstall.
func (builder *ImageClusterInstallBuilder) GetFailedCondition() (*hivev1.ClusterInstallCondition, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting Failed condition from imageclusterinstall %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	return builder.getCondition(hivev1.ClusterInstallFailed)
}

// GetRequirementsMetCondition returns RequirementsMet condition from imageclusterinstall.
func (builder *ImageClusterInstallBuilder) GetRequirementsMetCondition() (*hivev1.ClusterInstallCondition, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting RequirementsMet condition from imageclusterinstall %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	return builder.getCondition(hivev1.ClusterInstallRequirementsMet)
}

// GetStoppedCondition returns Stopped condition from imageclusterinstall.
func (builder *ImageClusterInstallBuilder) GetStoppedCondition() (*hivev1.ClusterInstallCondition, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting Stopped condition from imageclusterinstall %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	return builder.getCondition(hivev1.ClusterInstallStopped)
}

// Get fetches the defined imageclusterinstall from the cluster.
func (builder *ImageClusterInstallBuilder) Get() (*ibiv1alpha1.ImageClusterInstall, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting imageclusterinstall %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	imageClusterInstall := &ibiv1alpha1.ImageClusterInstall{}

	err := builder.apiClient.Get(logging.DiscardContext(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, imageClusterInstall)
	if err != nil {
		return nil, err
	}

	return imageClusterInstall, nil
}

// Create generates an imageclusterinstall on the cluster.
func (builder *ImageClusterInstallBuilder) Create() (*ImageClusterInstallBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the imageclusterinstall %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error
	if !builder.Exists() {
		err = builder.apiClient.Create(logging.DiscardContext(), builder.Definition)
		if err == nil {
			builder.Object = builder.Definition
		}
	}

	return builder, err
}

// Update modifies an existing imageclusterinstall on the cluster.
func (builder *ImageClusterInstallBuilder) Update(force bool) (*ImageClusterInstallBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating imageclusterinstall %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("imageclusterinstall %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		return builder, fmt.Errorf("cannot update non-existent imageclusterinstall")
	}

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err != nil {
		if force {
			klog.V(100).Infof("%v", msg.FailToUpdateNotification("imageclusterinstall", builder.Definition.Name, builder.Definition.Namespace))

			err := builder.Delete()
			if err != nil {
				klog.V(100).Infof("%v", msg.FailToUpdateError("imageclusterinstall", builder.Definition.Name, builder.Definition.Namespace))

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

// Delete removes an imageclusterinstall from the cluster.
func (builder *ImageClusterInstallBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Deleting the imageclusterinstall %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		return fmt.Errorf("imageclusterinstall cannot be deleted because it does not exist")
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return fmt.Errorf("cannot delete imageclusterinstall: %w", err)
	}

	builder.Object = nil

	return nil
}

// Exists checks if the defined imageclusterinstall has already been created.
func (builder *ImageClusterInstallBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if imageclusterinstall %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

func (builder *ImageClusterInstallBuilder) getCondition(
	conditionType hivev1.ClusterInstallConditionType) (*hivev1.ClusterInstallCondition, error) {
	if !builder.Exists() {
		return nil, fmt.Errorf("cannot get condition from non-existent imageclusterinstall")
	}

	for _, condition := range builder.Object.Status.Conditions {
		if condition.Type == conditionType {
			return &condition, nil
		}
	}

	return nil, fmt.Errorf("cannot find %s condition in imageclusterinstall status", conditionType)
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *ImageClusterInstallBuilder) validate() (bool, error) {
	resourceCRD := "ImageClusterInstall"

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
