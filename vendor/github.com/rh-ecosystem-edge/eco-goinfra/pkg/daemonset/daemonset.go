package daemonset

import (
	"context"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	appsv1Typed "k8s.io/client-go/kubernetes/typed/apps/v1"
	"k8s.io/klog/v2"
)

// Builder provides struct for daemonset object containing connection to the cluster and the daemonset definitions.
type Builder struct {
	// Daemonset definition. Used to create a daemonset object.
	Definition *appsv1.DaemonSet
	// Created daemonset object.
	Object *appsv1.DaemonSet
	// Used in functions that define or mutate daemonset definition. errorMsg is processed before the daemonset
	// object is created.
	errorMsg  string
	apiClient appsv1Typed.DaemonSetInterface
}

// AdditionalOptions additional options for daemonset object.
type AdditionalOptions func(builder *Builder) (*Builder, error)

var retryInterval = time.Second * 3

// NewBuilder creates a new instance of Builder.
func NewBuilder(
	apiClient *clients.Settings, name, nsname string, labels map[string]string, containerSpec corev1.Container) *Builder {
	klog.V(100).Infof(
		"Initializing new daemonset structure with the following params: "+
			"name: %s, namespace: %s, labels: %s, containerSpec %v",
		name, nsname, labels, containerSpec)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is nil")

		return nil
	}

	builder := &Builder{
		apiClient: apiClient.DaemonSets(nsname),
		Definition: &appsv1.DaemonSet{
			Spec: appsv1.DaemonSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: labels,
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: labels,
					},
				},
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	builder.WithAdditionalContainerSpecs([]corev1.Container{containerSpec})

	if name == "" {
		klog.V(100).Info("The name of the daemonset is empty")

		builder.errorMsg = "daemonset 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the daemonset is empty")

		builder.errorMsg = "daemonset 'namespace' cannot be empty"

		return builder
	}

	if len(labels) == 0 {
		klog.V(100).Info("There are no labels for the daemonset")

		builder.errorMsg = "daemonset 'labels' cannot be empty"

		return builder
	}

	return builder
}

// Pull loads an existing daemonSet into the Builder struct.
func Pull(apiClient *clients.Settings, name, nsname string) (*Builder, error) {
	klog.V(100).Infof("Pulling existing daemonset name:%s under namespace:%s", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is nil")

		return nil, fmt.Errorf("apiClient cannot be nil")
	}

	builder := &Builder{
		apiClient: apiClient.DaemonSets(nsname),
		Definition: &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the daemonset is empty")

		return nil, fmt.Errorf("daemonset name cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the daemonset is empty")

		return nil, fmt.Errorf("daemonset namespace cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("daemonset object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// WithNodeSelector applies nodeSelector to the daemonset definition.
func (builder *Builder) WithNodeSelector(selector map[string]string) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Applying nodeSelector %s to daemonset %s in namespace %s",
		selector, builder.Definition.Name, builder.Definition.Namespace)

	if len(selector) == 0 {
		klog.V(100).Info("The nodeselector is empty")

		builder.errorMsg = "cannot accept empty map as nodeselector"

		return builder
	}

	builder.Definition.Spec.Template.Spec.NodeSelector = selector

	return builder
}

// WithHostNetwork applies HostNetwork to daemonset definition.
func (builder *Builder) WithHostNetwork() *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Enabling hostnetwork flag to daemonset %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	builder.Definition.Spec.Template.Spec.HostNetwork = true

	return builder
}

// WithPodAffinity applies pod's Affinity to daemonset definition.
func (builder *Builder) WithPodAffinity(podAffinity *corev1.Affinity) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if podAffinity == nil {
		klog.V(100).Info("The Affinity parameter is empty")

		builder.errorMsg = "affinity parameter is empty"

		return builder
	}

	klog.V(100).Infof("Adding pod affinity to daemonset %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	builder.Definition.Spec.Template.Spec.Affinity = podAffinity

	return builder
}

// WithVolume defines Volume of daemonset under PodTemplateSpec.
func (builder *Builder) WithVolume(dsVolume corev1.Volume) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if dsVolume.Name == "" {
		klog.V(100).Info("The Volume name parameter is empty")

		builder.errorMsg = "Volume name parameter is empty"

		return builder
	}

	klog.V(100).Infof("Adding volume %s for daemonset %s pod template in namespace %s",
		dsVolume.Name, builder.Definition.Name, builder.Definition.Namespace)

	builder.Definition.Spec.Template.Spec.Volumes = append(
		builder.Definition.Spec.Template.Spec.Volumes,
		dsVolume)

	return builder
}

// WithAdditionalContainerSpecs appends a list of container specs to the daemonset definition.
func (builder *Builder) WithAdditionalContainerSpecs(specs []corev1.Container) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Appending a list of container specs %v to daemonset %s in namespace %s",
		specs, builder.Definition.Name, builder.Definition.Namespace)

	if len(specs) == 0 {
		klog.V(100).Info("The container specs are empty")

		builder.errorMsg = "cannot accept empty list as container specs"

		return builder
	}

	if builder.Definition.Spec.Template.Spec.Containers == nil {
		builder.Definition.Spec.Template.Spec.Containers = specs
	} else {
		builder.Definition.Spec.Template.Spec.Containers = append(builder.Definition.Spec.Template.Spec.Containers, specs...)
	}

	return builder
}

// WithOptions creates daemonset with generic mutation options.
func (builder *Builder) WithOptions(options ...AdditionalOptions) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Info("Setting daemonset additional options")

	for _, option := range options {
		if option != nil {
			builder, err := option(builder)
			if err != nil {
				klog.V(100).Info("Error occurred in mutation function")

				builder.errorMsg = err.Error()

				return builder
			}
		}
	}

	return builder
}

// Create builds daemonset in the cluster and stores the created object in struct.
func (builder *Builder) Create() (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating daemonset %s in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

	var err error
	if !builder.Exists() {
		builder.Object, err = builder.apiClient.Create(
			logging.DiscardContext(), builder.Definition, metav1.CreateOptions{})
	}

	return builder, err
}

// Update renovates the existing daemonset object with daemonset definition in builder.
func (builder *Builder) Update() (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating daemonset %s in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.apiClient.Update(
		logging.DiscardContext(), builder.Definition, metav1.UpdateOptions{})

	return builder, err
}

// Delete removes the daemonset.
func (builder *Builder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Deleting daemonset %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(
		logging.DiscardContext(), builder.Definition.Name, metav1.DeleteOptions{})

	if err != nil && !k8serrors.IsNotFound(err) {
		return err
	}

	builder.Object = nil

	return nil
}

// CreateAndWaitUntilReady creates a daemonset in the cluster and waits until the daemonset is available.
func (builder *Builder) CreateAndWaitUntilReady(timeout time.Duration) (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating daemonset %s in namespace %s and waiting for the defined period until it is ready",
		builder.Definition.Name, builder.Definition.Namespace)

	_, err := builder.Create()
	if err != nil {
		klog.V(100).Infof("Failed to create daemonset. Error is: '%s'", err.Error())

		return nil, err
	}

	// Polls every retryInterval to determine if daemonset is available.
	err = wait.PollUntilContextTimeout(
		context.TODO(), retryInterval, timeout, true, func(ctx context.Context) (bool, error) {
			builder.Object, err = builder.apiClient.Get(
				logging.DiscardContext(), builder.Definition.Name, metav1.GetOptions{})
			if err != nil {
				return false, nil
			}

			// Check if the daemonset is ready.
			//nolint:gocritic
			if builder.Object.Status.DesiredNumberScheduled == builder.Object.Status.CurrentNumberScheduled &&
				builder.Object.Status.DesiredNumberScheduled == builder.Object.Status.NumberReady &&
				builder.Object.Status.DesiredNumberScheduled == builder.Object.Status.NumberAvailable &&
				builder.Object.Status.NumberUnavailable == 0 &&
				builder.Object.Status.NumberReady > 0 {
				return true, nil
			}

			// If none of the conditions above are met, evaluate the daemonset conditions.
			for _, condition := range builder.Object.Status.Conditions {
				if condition.Type == "Available" {
					return condition.Status == "True", nil
				}
			}

			return false, err
		})
	if err == nil {
		return builder, nil
	}

	return nil, err
}

// DeleteAndWait deletes a daemonset and waits until it is removed from the cluster.
func (builder *Builder) DeleteAndWait(timeout time.Duration) error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Deleting daemonset %s in namespace %s and waiting for the defined period until it is removed",
		builder.Definition.Name, builder.Definition.Namespace)

	if err := builder.Delete(); err != nil {
		return err
	}

	// Polls the daemonset every retryInterval until it is removed.
	return wait.PollUntilContextTimeout(
		context.TODO(), retryInterval, timeout, true, func(ctx context.Context) (bool, error) {
			_, err := builder.apiClient.Get(
				logging.DiscardContext(), builder.Definition.Name, metav1.GetOptions{})
			if k8serrors.IsNotFound(err) {
				return true, nil
			}

			return false, nil
		})
}

// Exists checks whether the given daemonset exists.
func (builder *Builder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if daemonset %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.apiClient.Get(
		logging.DiscardContext(), builder.Definition.Name, metav1.GetOptions{})

	return err == nil || !k8serrors.IsNotFound(err)
}

// IsReady waits for the daemonset to reach expected number of pods in Ready state.
func (builder *Builder) IsReady(timeout time.Duration) bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Running periodic check until daemonset %s in namespace %s is ready or "+
		"timeout %s exceeded", builder.Definition.Name, builder.Definition.Namespace, timeout.String())

	// Polls every retryInterval to determine if daemonset is available.
	err := wait.PollUntilContextTimeout(
		context.TODO(), retryInterval, timeout, true, func(ctx context.Context) (bool, error) {
			var err error

			builder.Object, err = builder.apiClient.Get(
				logging.DiscardContext(), builder.Definition.Name, metav1.GetOptions{})
			if err != nil {
				klog.V(100).Infof("Failed to get daemonset from cluster. Error is: '%s'", err.Error())

				return false, nil
			}

			if builder.Object.Status.NumberReady == builder.Object.Status.DesiredNumberScheduled {
				return true, nil
			}

			if builder.Object.Status.NumberReady == builder.Object.Status.UpdatedNumberScheduled &&
				builder.Object.Status.UpdatedNumberScheduled != 0 {
				return true, nil
			}

			return false, err
		})

	return err == nil
}

// GetGVR returns the GroupVersionResource for the daemonset.
func GetGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "daemonsets",
	}
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *Builder) validate() (bool, error) {
	resourceCRD := "DaemonSet"

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
