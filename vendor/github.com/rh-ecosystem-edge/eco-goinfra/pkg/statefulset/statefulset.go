package statefulset

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
)

// Builder provides struct for statefulset object containing connection to the cluster and the statefulset definitions.
type Builder struct {
	// StatefulSet definition. Used to create the statefulset object.
	Definition *appsv1.StatefulSet
	// Created statefulset object
	Object *appsv1.StatefulSet
	// Used in functions that define or mutate statefulset definition. errorMsg is processed before the statefulset
	// object is created.
	errorMsg  string
	apiClient *clients.Settings
}

// AdditionalOptions additional options for StatefulSet object.
type AdditionalOptions func(builder *Builder) (*Builder, error)

// NewBuilder creates a new instance of Builder.
func NewBuilder(
	apiClient *clients.Settings,
	name string,
	nsname string,
	labels map[string]string,
	containerSpec *corev1.Container) *Builder {
	glog.V(100).Infof(
		"Initializing new statefulset structure with the following params: "+
			"name: %s, namespace: %s, labels: %s, containerSpec %v",
		name, nsname, labels, containerSpec)

	builder := &Builder{
		apiClient: apiClient,
		Definition: &appsv1.StatefulSet{
			Spec: appsv1.StatefulSetSpec{
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

	builder.WithAdditionalContainerSpecs([]corev1.Container{*containerSpec})

	if name == "" {
		glog.V(100).Infof("The name of the statefulset is empty")

		builder.errorMsg = "statefulset 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		glog.V(100).Infof("The namespace of the statefulset is empty")

		builder.errorMsg = "statefulset 'namespace' cannot be empty"

		return builder
	}

	if labels == nil {
		glog.V(100).Infof("There are no labels for the statefulset")

		builder.errorMsg = "statefulset 'labels' cannot be empty"

		return builder
	}

	return builder
}

// WithAdditionalContainerSpecs appends a list of container specs to the statefulset definition.
func (builder *Builder) WithAdditionalContainerSpecs(specs []corev1.Container) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Appending a list of container specs %v to statefulset %s in namespace %s",
		specs, builder.Definition.Name, builder.Definition.Namespace)

	if specs == nil {
		glog.V(100).Infof("The container specs are empty")

		builder.errorMsg = "cannot accept nil or empty list as container specs"

		return builder
	}

	if builder.Definition.Spec.Template.Spec.Containers == nil {
		builder.Definition.Spec.Template.Spec.Containers = specs

		return builder
	}

	builder.Definition.Spec.Template.Spec.Containers = append(builder.Definition.Spec.Template.Spec.Containers, specs...)

	return builder
}

// WithOptions creates StatefulSet with generic mutation options.
func (builder *Builder) WithOptions(options ...AdditionalOptions) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting StatefulSet additional options")

	for _, option := range options {
		if option != nil {
			builder, err := option(builder)
			if err != nil {
				glog.V(100).Infof("Error occurred in mutation function")

				builder.errorMsg = err.Error()

				return builder
			}
		}
	}

	return builder
}

// WithPodAnnotations sets annotations on the pod template for pods managed by the statefulset.
func (builder *Builder) WithPodAnnotations(annotations map[string]string) *Builder {
	if valid, _ := builder.validate(); !valid {
		glog.V(100).Infof("Failed to validate StatefulSet builder")

		return builder
	}

	glog.V(100).Infof("Setting pod annotations %v for statefulset %s in namespace %s",
		annotations, builder.Definition.Name, builder.Definition.Namespace)

	if len(annotations) == 0 {
		glog.V(100).Infof("The pod annotations are empty")

		builder.errorMsg = "cannot accept nil or empty annotations"

		return builder
	}

	if builder.Definition.Spec.Template.Annotations == nil {
		builder.Definition.Spec.Template.Annotations = make(map[string]string)
	}

	for key, value := range annotations {
		builder.Definition.Spec.Template.Annotations[key] = value
	}

	return builder
}

// Pull loads an existing statefulset into Builder struct.
func Pull(apiClient *clients.Settings, name, nsname string) (*Builder, error) {
	glog.V(100).Infof("Pulling existing statefulset name: %s under namespace: %s", name, nsname)

	builder := Builder{
		apiClient: apiClient,
		Definition: &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		builder.errorMsg = "statefulset 'name' cannot be empty"

		return nil, fmt.Errorf("statefulset 'name' cannot be empty")
	}

	if nsname == "" {
		builder.errorMsg = "statefulset 'namespace' cannot be empty"

		return nil, fmt.Errorf("statefulset 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("statefulset object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return &builder, nil
}

// Create generates a statefulset in cluster and stores the created object in struct.
func (builder *Builder) Create() (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Creating statefulset %s in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

	var err error
	if !builder.Exists() {
		builder.Object, err = builder.apiClient.StatefulSets(builder.Definition.Namespace).Create(
			context.TODO(), builder.Definition, metav1.CreateOptions{})
	}

	return builder, err
}

// Exists checks whether the given statefulset exists.
func (builder *Builder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Checking if statefulset %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.apiClient.StatefulSets(builder.Definition.Namespace).Get(
		context.TODO(), builder.Definition.Name, metav1.GetOptions{})

	return err == nil || !k8serrors.IsNotFound(err)
}

// Delete a statefulset from the cluster.
func (builder *Builder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	glog.V(100).Infof("Deleting statefulset %s in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		glog.V(100).Infof("Statefulset %s cannot be deleted because it does not exist", builder.Definition.Name)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.StatefulSets(builder.Definition.Namespace).Delete(
		context.TODO(), builder.Definition.Name, metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	builder.Object = nil

	return nil
}

// IsReady periodically checks if statefulset is in ready status.
func (builder *Builder) IsReady(timeout time.Duration) bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Running periodic check until statefulset %s in namespace %s is ready",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		return false
	}

	err := wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			var err error

			builder.Object, err = builder.apiClient.StatefulSets(builder.Definition.Namespace).Get(
				context.TODO(), builder.Definition.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}

			if builder.Object.Status.ReadyReplicas > 0 && builder.Object.Status.Replicas == builder.Object.Status.ReadyReplicas {
				return true, nil
			}

			return false, nil
		})

	return err == nil
}

// GetGVR returns pod's GroupVersionResource which could be used for Clean function.
func GetGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *Builder) validate() (bool, error) {
	resourceCRD := "StatefulSet"

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
