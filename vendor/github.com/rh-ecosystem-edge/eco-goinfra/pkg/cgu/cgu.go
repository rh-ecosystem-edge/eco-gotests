package cgu

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/openshift-kni/cluster-group-upgrades-operator/pkg/api/clustergroupupgrades/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var conditionComplete = metav1.Condition{Type: "Succeeded", Status: metav1.ConditionTrue}

// CguBuilder provides struct for the cgu object containing connection to
// the cluster and the cgu definitions.
type CguBuilder struct {
	// cgu Definition, used to create the cgu object.
	Definition *v1alpha1.ClusterGroupUpgrade
	// created cgu object.
	Object *v1alpha1.ClusterGroupUpgrade
	// api client to interact with the cluster.
	apiClient goclient.Client
	// used to store latest error message upon defining or mutating application definition.
	errorMsg string
}

// NewCguBuilder creates a new instance of CguBuilder.
func NewCguBuilder(apiClient *clients.Settings, name, nsname string, maxConcurrency int) *CguBuilder {
	glog.V(100).Infof(
		"Initializing new CGU structure with the following params: name: %s, nsname: %s, maxConcurrency: %d",
		name, nsname, maxConcurrency)

	if apiClient == nil {
		glog.V(100).Info("The apiClient for the CGU is nil")

		return nil
	}

	err := apiClient.AttachScheme(v1alpha1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add cgu v1alpha1 scheme to client schemes")

		return nil
	}

	builder := &CguBuilder{
		apiClient: apiClient.Client,
		Definition: &v1alpha1.ClusterGroupUpgrade{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
			Spec: v1alpha1.ClusterGroupUpgradeSpec{
				RemediationStrategy: &v1alpha1.RemediationStrategySpec{
					MaxConcurrency: maxConcurrency,
				},
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the CGU is empty")

		builder.errorMsg = "CGU 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		glog.V(100).Infof("The namespace of the CGU is empty")

		builder.errorMsg = "CGU 'nsname' cannot be empty"

		return builder
	}

	if maxConcurrency < 1 {
		glog.V(100).Infof("The maxConcurrency of the CGU has a minimum of 1")

		builder.errorMsg = "CGU 'maxConcurrency' cannot be less than 1"

		return builder
	}

	return builder
}

// WithCluster appends a cluster to the clusters list in the CGU definition.
func (builder *CguBuilder) WithCluster(cluster string) *CguBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if cluster == "" {
		glog.V(100).Infof("The cluster to be added to the CGU is empty")

		builder.errorMsg = "cluster in CGU cluster spec cannot be empty"

		return builder
	}

	builder.Definition.Spec.Clusters = append(builder.Definition.Spec.Clusters, cluster)

	return builder
}

// WithManagedPolicy appends a policies to the managed policies list in the CGU definition.
func (builder *CguBuilder) WithManagedPolicy(policy string) *CguBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if policy == "" {
		glog.V(100).Infof("The policy to be added to the CGU's ManagedPolicies is empty")

		builder.errorMsg = "policy in CGU managedpolicies spec cannot be empty"

		return builder
	}

	builder.Definition.Spec.ManagedPolicies = append(builder.Definition.Spec.ManagedPolicies, policy)

	return builder
}

// WithCanary appends a canary to the RemediationStrategy canaries list in the CGU definition.
func (builder *CguBuilder) WithCanary(canary string) *CguBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if canary == "" {
		glog.V(100).Infof("The canary to be added to the CGU's RemediationStrategy is empty")

		builder.errorMsg = "canary in CGU remediationstrategy spec cannot be empty"

		return builder
	}

	builder.Definition.Spec.RemediationStrategy.Canaries = append(
		builder.Definition.Spec.RemediationStrategy.Canaries, canary)

	return builder
}

// Pull pulls existing cgu into CguBuilder struct.
func Pull(apiClient *clients.Settings, name, nsname string) (*CguBuilder, error) {
	glog.V(100).Infof("Pulling existing cgu name %s under namespace %s from cluster", name, nsname)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient is empty")

		return nil, fmt.Errorf("cgu 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(v1alpha1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add cgu v1alpha1 scheme to client schemes")

		return nil, err
	}

	builder := CguBuilder{
		apiClient: apiClient.Client,
		Definition: &v1alpha1.ClusterGroupUpgrade{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the cgu is empty")

		return nil, fmt.Errorf("cgu 'name' cannot be empty")
	}

	if nsname == "" {
		glog.V(100).Infof("The namespace of the cgu is empty")

		return nil, fmt.Errorf("cgu 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("cgu object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return &builder, nil
}

// Get returns ClusterGroupUpgrade object if found.
func (builder *CguBuilder) Get() (*v1alpha1.ClusterGroupUpgrade, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof(
		"Collecting clusterGroupUpgrade object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	clusterGroupUpgrade := &v1alpha1.ClusterGroupUpgrade{}

	err := builder.apiClient.Get(context.TODO(),
		goclient.ObjectKey{Name: builder.Definition.Name, Namespace: builder.Definition.Namespace},
		clusterGroupUpgrade)
	if err != nil {
		glog.V(100).Infof(
			"clusterGroupUpgrade object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		return nil, err
	}

	return clusterGroupUpgrade, nil
}

// Exists checks whether the given cgu exists.
func (builder *CguBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Checking if cgu %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Create makes a cgu in the cluster and stores the created object in struct.
func (builder *CguBuilder) Create() (*CguBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Creating the cgu %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error
	if !builder.Exists() {
		err = builder.apiClient.Create(context.TODO(), builder.Definition)
		if err != nil {
			glog.V(100).Infof("Failed to create clusterGroupUpgrade")

			return nil, err
		}
	}

	builder.Object = builder.Definition

	return builder, err
}

// Delete removes a cgu from a cluster.
func (builder *CguBuilder) Delete() (*CguBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Deleting the cgu %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		glog.V(100).Infof("cgu %s in namespace %s does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
	if err != nil {
		return builder, fmt.Errorf("can not delete cgu: %w", err)
	}

	builder.Object = nil

	return builder, nil
}

// Update renovates the existing cgu object with the cgu definition in builder.
func (builder *CguBuilder) Update(force bool) (*CguBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Updating the cgu object", builder.Definition.Name)

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err == nil {
		builder.Object = builder.Definition
	} else if force {
		glog.V(100).Infof(
			msg.FailToUpdateNotification("cgu", builder.Definition.Name))

		// Deleting the cgu may take time, so wait for it to be deleted before recreating. Otherwise,
		// the create happens before the delete finishes and this update results in just deletion.
		builder, err := builder.DeleteAndWait(time.Minute)
		builder.Definition.ResourceVersion = ""

		if err != nil {
			glog.V(100).Infof(
				msg.FailToUpdateError("cgu", builder.Definition.Name))

			return nil, err
		}

		return builder.Create()
	}

	return builder, err
}

// DeleteAndWait deletes the cgu object and waits until the cgu is deleted.
func (builder *CguBuilder) DeleteAndWait(timeout time.Duration) (*CguBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Deleting cgu %s in namespace %s and waiting for the defined period until it is removed",
		builder.Definition.Name, builder.Definition.Namespace)

	builder, err := builder.Delete()
	if err != nil {
		return builder, err
	}

	err = builder.WaitUntilDeleted(timeout)

	return builder, err
}

// WaitUntilDeleted waits for the duration of the defined timeout or until the cgu is deleted.
func (builder *CguBuilder) WaitUntilDeleted(timeout time.Duration) error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	glog.V(100).Infof(
		"Waiting for the defined period until cgu %s in namespace %s is deleted",
		builder.Definition.Name, builder.Definition.Namespace)

	return wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			_, err := builder.Get()
			if err == nil {
				glog.V(100).Infof("cgu %s/%s still present", builder.Definition.Name, builder.Definition.Namespace)

				return false, nil
			}

			if k8serrors.IsNotFound(err) {
				glog.V(100).Infof("cgu %s/%s is gone", builder.Definition.Name, builder.Definition.Namespace)

				return true, nil
			}

			glog.V(100).Infof("failed to get cgu %s/%s: %w", builder.Definition.Name, builder.Definition.Namespace, err)

			return false, err
		})
}

// WaitForCondition waits until the CGU has a condition that matches the expected, checking only the Type, Status,
// Reason, and Message fields. For the message field, it matches if the message contains the expected. Zero fields in
// the expected condition are ignored.
func (builder *CguBuilder) WaitForCondition(expected metav1.Condition, timeout time.Duration) (*CguBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	if !builder.Exists() {
		glog.V(100).Infof("The CGU does not exist on the cluster")

		return builder, fmt.Errorf(
			"cgu object %s does not exist in namespace %s", builder.Definition.Name, builder.Definition.Namespace)
	}

	err := wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			var err error

			builder.Object, err = builder.Get()
			if err != nil {
				glog.V(100).Info("failed to get cgu %s/%s: %w", builder.Definition.Name, builder.Definition.Namespace, err)

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

	return builder, err
}

// WaitUntilComplete waits the specified timeout for the CGU to complete.
func (builder *CguBuilder) WaitUntilComplete(timeout time.Duration) (*CguBuilder, error) {
	return builder.WaitForCondition(conditionComplete, timeout)
}

// WaitUntilClusterInState waits the specified timeout for a cluster in the CGU to be in the specified state.
func (builder *CguBuilder) WaitUntilClusterInState(cluster, state string, timeout time.Duration) (*CguBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	if cluster == "" {
		glog.V(100).Info("Cluster name cannot be empty")

		return nil, fmt.Errorf("cluster name cannot be empty")
	}

	if state == "" {
		glog.V(100).Info("State cannot be empty")

		return nil, fmt.Errorf("state cannot be empty")
	}

	glog.V(100).Infof(
		"Waiting until cluster %s on CGU %s in namespace %s is in state %s",
		cluster, builder.Definition.Name, builder.Definition.Namespace, state)

	if !builder.Exists() {
		return nil, fmt.Errorf(
			"cgu object %s does not exist in namespace %s", builder.Definition.Name, builder.Definition.Namespace)
	}

	var err error

	err = wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			builder.Object, err = builder.Get()
			if err != nil {
				return false, nil
			}

			status, ok := builder.Object.Status.Status.CurrentBatchRemediationProgress[cluster]
			if !ok {
				glog.V(100).Infof(
					"cluster %s not found in batch remediation progress for cgu %s in namespace %s",
					cluster, builder.Definition.Name, builder.Definition.Namespace)

				return false, nil
			}

			return status.State == state, nil
		})
	if err != nil {
		return nil, err
	}

	return builder, nil
}

// WaitUntilClusterComplete waits the specified timeout for a cluster in the CGU to complete remidation.
func (builder *CguBuilder) WaitUntilClusterComplete(cluster string, timeout time.Duration) (*CguBuilder, error) {
	return builder.WaitUntilClusterInState(cluster, v1alpha1.Completed, timeout)
}

// WaitUntilClusterInProgress waits the specified timeout for a cluster in the CGU to start remidation.
func (builder *CguBuilder) WaitUntilClusterInProgress(cluster string, timeout time.Duration) (*CguBuilder, error) {
	return builder.WaitUntilClusterInState(cluster, v1alpha1.InProgress, timeout)
}

// WaitUntilBackupStarts waits the specified timeout for the backup to start.
func (builder *CguBuilder) WaitUntilBackupStarts(timeout time.Duration) (*CguBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof(
		"Waiting for CGU %s in namespace %s to start backup", builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		glog.V(100).Infof("The CGU does not exist on the cluster")

		return builder, fmt.Errorf("%s", builder.errorMsg)
	}

	var err error

	err = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, timeout, true, func(context.Context) (bool, error) {
		builder.Object, err = builder.Get()
		if err != nil {
			glog.V(100).Infof(
				"Failed to get CGU %s in namespace %s due to: %w", builder.Definition.Name, builder.Definition.Namespace, err)

			return false, nil
		}

		return builder.Object.Status.Backup != nil, nil
	})
	if err == nil {
		return builder, nil
	}

	glog.V(100).Infof(
		"Failed to wait for CGU %s in namespace %s to start backup due to: %w",
		builder.Definition.Name, builder.Definition.Namespace, err)

	return nil, err
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *CguBuilder) validate() (bool, error) {
	resourceCRD := "cgu"

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
