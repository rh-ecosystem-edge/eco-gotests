package oran

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/golang/glog"
	provisioningv1alpha1 "github.com/openshift-kni/oran-o2ims/api/provisioning/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterTemplateBuilder provides a struct to inferface with ClusterTemplate resources on a specific cluster.
type ClusterTemplateBuilder struct {
	// Definition of the ClusterTemplate used to create the resource.
	Definition *provisioningv1alpha1.ClusterTemplate
	// Object of the ClusterTemplate as it is on the cluster.
	Object *provisioningv1alpha1.ClusterTemplate
	// apiClient used to interact with the cluster.
	apiClient runtimeclient.Client
	// errorMsg used to store latest error message from functions that do not return errors.
	errorMsg string
}

// PullClusterTemplate pulls an existing ClusterTemplate into a ClusterTemplateBuilder struct.
func PullClusterTemplate(apiClient *clients.Settings, name, nsname string) (*ClusterTemplateBuilder, error) {
	glog.V(100).Infof("Pulling existing ClusterTemplate %s in namespace %s from cluster", name, nsname)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient of the ClusterTemplate is nil")

		return nil, fmt.Errorf("clusterTemplate 'apiClient' cannot be nil")
	}

	err := apiClient.AttachScheme(provisioningv1alpha1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add provisioning v1alpha1 scheme to client schemes: %v", err)

		return nil, err
	}

	builder := &ClusterTemplateBuilder{
		apiClient: apiClient.Client,
		Definition: &provisioningv1alpha1.ClusterTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		glog.V(100).Info("The name of the ClusterTemplate is empty")

		return nil, fmt.Errorf("clusterTemplate 'name' cannot be empty")
	}

	if nsname == "" {
		glog.V(100).Info("The nsname of the ClusterTemplate is empty")

		return nil, fmt.Errorf("clusterTemplate 'nsname' cannot be empty")
	}

	if !builder.Exists() {
		glog.V(100).Info("The ClusterTemplate %s does not exist in namespace %s", name, nsname)

		return nil, fmt.Errorf("clusterTemplate object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get returns the ClusterTemplate object if found.
func (builder *ClusterTemplateBuilder) Get() (*provisioningv1alpha1.ClusterTemplate, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof("Getting ClusterTemplate object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	clusterTemplate := &provisioningv1alpha1.ClusterTemplate{}

	err := builder.apiClient.Get(context.TODO(), runtimeclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, clusterTemplate)
	if err != nil {
		glog.V(100).Infof("Failed to get ClusterTemplate object %s in namespace %s: %v",
			builder.Definition.Name, builder.Definition.Namespace, err)

		return nil, err
	}

	return clusterTemplate, nil
}

// Exists checks whether this ClusterTemplate exists on the cluster.
func (builder *ClusterTemplateBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Checking if ClusterTemplate %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// WaitForCondition waits up to the provided timeout for a condition matching expected. It checks only the Type, Status,
// Reason, and Message fields. For the message, it matches if the message contains the expected. Zero fields in the
// expected condition are ignored.
func (builder *ClusterTemplateBuilder) WaitForCondition(
	expected metav1.Condition, timeout time.Duration) (*ClusterTemplateBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof("Waiting up to %s until ClusterTemplate %s in namespace %s has condition %v",
		timeout, builder.Definition.Name, builder.Definition.Namespace, expected)

	if !builder.Exists() {
		glog.V(100).Infof("ClusterTemplate %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		return nil, fmt.Errorf("cannot wait for non-existent ClusterTemplate")
	}

	err := wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			var err error

			builder.Object, err = builder.Get()
			if err != nil {
				glog.V(100).Infof("Failed to get ClusterTemplate %s in namespace %s: %v",
					builder.Definition.Name, builder.Definition.Namespace, err)

				return false, nil
			}

			builder.Definition = builder.Object

			for _, condition := range builder.Object.Status.Conditions {
				if expected.Type != "" && condition.Type != expected.Type {
					continue
				}

				if expected.Status != "" && condition.Status != expected.Status {
					continue
				}

				if expected.Reason != "" && condition.Reason != expected.Reason {
					continue
				}

				if expected.Message != "" && !strings.Contains(condition.Message, expected.Message) {
					continue
				}

				return true, nil
			}

			return false, nil
		})
	if err != nil {
		return nil, err
	}

	return builder, nil
}

// validate checks that the builder, definition, and apiClient are properly initialized and there is no errorMsg.
func (builder *ClusterTemplateBuilder) validate() (bool, error) {
	resourceCRD := "clusterTemplate"

	if builder == nil {
		glog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.Definition == nil {
		glog.V(100).Infof("The %s is uninitialized", resourceCRD)

		return false, fmt.Errorf("%s", msg.UndefinedCrdObjectErrString(resourceCRD))
	}

	if builder.apiClient == nil {
		glog.V(100).Infof("The %s builder apiClient is nil", resourceCRD)

		return false, fmt.Errorf("%s builder cannot have nil apiClient", resourceCRD)
	}

	if builder.errorMsg != "" {
		glog.V(100).Infof("The %s builder has error message %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
