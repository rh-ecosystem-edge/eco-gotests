package clusteroperator

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/golang/glog"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	fiveScds time.Duration = 5 * time.Second
	isTrue                 = "True"
)

// Builder provides struct for clusterOperator object.
type Builder struct {
	// ClusterOperator definition. Used to create a clusterOperator object.
	Definition *configv1.ClusterOperator
	// Created clusterOperator object.
	Object *configv1.ClusterOperator
	// apiClient opens api connection to the cluster.
	apiClient goclient.Client
	// Used in functions that define or mutate clusterOperator definition. errorMsg is processed before the
	// ClusterOperator object is created.
	errorMsg string
}

// Pull loads an existing clusterOperator into Builder struct.
func Pull(apiClient *clients.Settings, clusterOperatorName string) (*Builder, error) {
	glog.V(100).Infof("Pulling existing clusterOperator: %s", clusterOperatorName)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient is empty")

		return nil, fmt.Errorf("clusterOperator 'apiClient' cannot be empty")
	}

	builder := &Builder{
		apiClient: apiClient.Client,
		Definition: &configv1.ClusterOperator{
			ObjectMeta: metav1.ObjectMeta{
				Name: clusterOperatorName,
			},
		},
	}

	if clusterOperatorName == "" {
		glog.V(100).Infof("The name of the clusterOperator is empty")

		return nil, fmt.Errorf("clusterOperator 'clusterOperatorName' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("clusterOperator object %s does not exist", clusterOperatorName)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get fetches existing clusterOperator from cluster.
func (builder *Builder) Get() (*configv1.ClusterOperator, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof("Getting existing clusterOperator with name %s from cluster", builder.Definition.Name)

	clusterOperatorObj := &configv1.ClusterOperator{}

	err := builder.apiClient.Get(context.TODO(), goclient.ObjectKey{
		Name: builder.Definition.Name,
	}, clusterOperatorObj)
	if err != nil {
		glog.V(100).Infof("Failed to get clusterOperator object %s from cluster due to: %w",
			builder.Definition.Name, err)

		return nil, err
	}

	return clusterOperatorObj, nil
}

// Exists checks whether the given clusterOperator exists.
func (builder *Builder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Checking if clusterOperator %s exists", builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// IsAvailable check if the clusterOperator is available.
func (builder *Builder) IsAvailable() bool {
	if !builder.Exists() {
		return false
	}

	glog.V(100).Infof("Verify the availability of %s clusterOperator", builder.Definition.Name)

	for _, condition := range builder.Object.Status.Conditions {
		if condition.Type == "Available" {
			return condition.Status == isTrue
		}
	}

	return false
}

// IsDegraded checks if the clusterOperator is degraded.
func (builder *Builder) IsDegraded() bool {
	if !builder.Exists() {
		return false
	}

	glog.V(100).Infof("Check if %s clusterOperator is degraded", builder.Definition.Name)

	for _, condition := range builder.Object.Status.Conditions {
		if condition.Type == "Degraded" {
			return condition.Status == isTrue
		}
	}

	return false
}

// IsProgressing checks if the clusterOperator is progressing.
func (builder *Builder) IsProgressing() bool {
	if !builder.Exists() {
		return false
	}

	glog.V(100).Infof("Check if %s clusterOperator is progressing", builder.Definition.Name)

	for _, condition := range builder.Object.Status.Conditions {
		if condition.Type == "Progressing" {
			return condition.Status == isTrue
		}
	}

	return false
}

// GetConditionReason returns the specific condition type's reason value or an empty string if it does not exist.
func (builder *Builder) GetConditionReason(conditionType configv1.ClusterStatusConditionType) string {
	if valid, _ := builder.validate(); !valid {
		return ""
	}

	glog.V(100).Infof("Get %s clusterOperator %v condition reason if exists",
		builder.Definition.Name, conditionType)

	err := builder.WaitUntilConditionTrue(conditionType, time.Second)
	if err != nil {
		return ""
	}

	for _, condition := range builder.Object.Status.Conditions {
		if condition.Type == conditionType {
			return condition.Reason
		}
	}

	return ""
}

// WaitUntilAvailable waits for timeout duration or until clusterOperator is Available.
func (builder *Builder) WaitUntilAvailable(timeout time.Duration) error {
	return builder.WaitUntilConditionTrue("Available", timeout)
}

// WaitUntilProgressing waits for timeout duration or until clusterOperator is Progressing.
func (builder *Builder) WaitUntilProgressing(timeout time.Duration) error {
	return builder.WaitUntilConditionTrue("Progressing", timeout)
}

// WaitUntilConditionTrue waits for timeout duration or until clusterOperator gets to a specific status.
func (builder *Builder) WaitUntilConditionTrue(
	conditionType configv1.ClusterStatusConditionType, timeout time.Duration) error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	if !builder.Exists() {
		return fmt.Errorf("%s clusterOperator not found", builder.Definition.Name)
	}

	return wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			var err error

			builder.Object, err = builder.Get()
			if err != nil {
				return false, nil
			}

			for _, condition := range builder.Object.Status.Conditions {
				if condition.Type == conditionType {
					return condition.Status == isTrue, nil
				}
			}

			return false, err
		})
}

// HasDesiredVersion checks if an operator has a desiredVersion.
func (builder *Builder) HasDesiredVersion(desiredVersion string) (bool, error) {
	if valid, err := builder.validate(); !valid {
		return false, err
	}

	if desiredVersion == "" {
		return false, fmt.Errorf("desiredVersion can't be empty")
	}

	if builder.Object.Status.Versions == nil {
		return false, fmt.Errorf("undefined cluster operator status versions")
	}

	for _, operatorVersion := range builder.Object.Status.Versions {
		glog.V(100).Infof("Testing %s and %s", operatorVersion.Version, desiredVersion)

		if operatorVersion.Version == desiredVersion {
			glog.V(100).Infof("The clusterOperator %s version matches the desired version %s",
				builder.Definition.Name, desiredVersion)

			return true, nil
		}

		glog.V(100).Infof(
			"The clusterOperator %s doesn't have the desired version %s, but has version %s. Continue lookup",
			builder.Definition.Name, desiredVersion, operatorVersion.Version)
	}

	glog.V(100).Infof("lookup failed.The clusterOperator %s doesn't have the desired version %s",
		builder.Definition.Name, desiredVersion)

	return false, nil
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *Builder) validate() (bool, error) {
	resourceCRD := "ClusterOperator"

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
