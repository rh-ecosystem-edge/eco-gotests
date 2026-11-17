package cgu

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/cgu/clustergroupupgrades/v1alpha1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

var (
	conditionComplete           = metav1.Condition{Type: "Succeeded", Status: metav1.ConditionTrue}
	errInvalidCguMaxConcurrency = fmt.Errorf("CGU 'maxConcurrency' cannot be less than 1")
	errClusterNameEmpty         = errors.New("cluster name cannot be empty")
	errStateEmpty               = errors.New("state cannot be empty")
)

type cguObjectNotExistsError struct {
	name      string
	namespace string
}

func (e cguObjectNotExistsError) Error() string {
	return fmt.Sprintf("cgu object %s does not exist in namespace %s", e.name, e.namespace)
}

func errCguObjectNotExists(name, namespace string) error {
	return cguObjectNotExistsError{name: name, namespace: namespace}
}

// CguBuilder provides struct for the cgu object containing connection to
// the cluster and the cgu definitions.
type CguBuilder struct {
	common.EmbeddableBuilder[v1alpha1.ClusterGroupUpgrade, *v1alpha1.ClusterGroupUpgrade]
	common.EmbeddableCreator[v1alpha1.ClusterGroupUpgrade, CguBuilder, *v1alpha1.ClusterGroupUpgrade, *CguBuilder]
	common.EmbeddableDeleteReturner[v1alpha1.ClusterGroupUpgrade, CguBuilder, *v1alpha1.ClusterGroupUpgrade, *CguBuilder]
	common.EmbeddableForceUpdater[v1alpha1.ClusterGroupUpgrade, CguBuilder, *v1alpha1.ClusterGroupUpgrade, *CguBuilder]
}

// AttachMixins wires the embedded CRUD mixins to this builder instance.
func (builder *CguBuilder) AttachMixins() {
	builder.EmbeddableCreator.SetBase(builder)
	builder.EmbeddableDeleteReturner.SetBase(builder)
	builder.EmbeddableForceUpdater.SetBase(builder)
}

// GetGVK returns the ClusterGroupUpgrade GVK for this builder.
func (builder *CguBuilder) GetGVK() schema.GroupVersionKind {
	return v1alpha1.SchemeGroupVersion.WithKind("ClusterGroupUpgrade")
}

// NewCguBuilder creates a new instance of CguBuilder.
func NewCguBuilder(apiClient *clients.Settings, name, nsname string, maxConcurrency int) *CguBuilder {
	klog.V(100).Infof(
		"Initializing new CGU structure with the following params: name: %s, nsname: %s, maxConcurrency: %d",
		name, nsname, maxConcurrency)

	builder := common.NewNamespacedBuilder[v1alpha1.ClusterGroupUpgrade, CguBuilder](
		apiClient, v1alpha1.AddToScheme, name, nsname)
	if builder.GetError() != nil {
		return builder
	}

	if maxConcurrency < 1 {
		klog.V(100).Info("The maxConcurrency of the CGU has a minimum of 1")

		builder.SetError(errInvalidCguMaxConcurrency)

		return builder
	}

	builder.Definition.Spec = v1alpha1.ClusterGroupUpgradeSpec{
		RemediationStrategy: &v1alpha1.RemediationStrategySpec{
			MaxConcurrency: maxConcurrency,
		},
	}

	return builder
}

// WithCluster appends a cluster to the clusters list in the CGU definition.
func (builder *CguBuilder) WithCluster(cluster string) *CguBuilder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	if cluster == "" {
		klog.V(100).Info("The cluster to be added to the CGU is empty")

		builder.SetError(fmt.Errorf("cluster in CGU cluster spec cannot be empty"))

		return builder
	}

	builder.Definition.Spec.Clusters = append(builder.Definition.Spec.Clusters, cluster)

	return builder
}

// WithManagedPolicy appends a policies to the managed policies list in the CGU definition.
func (builder *CguBuilder) WithManagedPolicy(policy string) *CguBuilder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	if policy == "" {
		klog.V(100).Info("The policy to be added to the CGU's ManagedPolicies is empty")

		builder.SetError(fmt.Errorf("policy in CGU managedpolicies spec cannot be empty"))

		return builder
	}

	builder.Definition.Spec.ManagedPolicies = append(builder.Definition.Spec.ManagedPolicies, policy)

	return builder
}

// WithCanary appends a canary to the RemediationStrategy canaries list in the CGU definition.
func (builder *CguBuilder) WithCanary(canary string) *CguBuilder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	if canary == "" {
		klog.V(100).Info("The canary to be added to the CGU's RemediationStrategy is empty")

		builder.SetError(fmt.Errorf("canary in CGU remediationstrategy spec cannot be empty"))

		return builder
	}

	builder.Definition.Spec.RemediationStrategy.Canaries = append(
		builder.Definition.Spec.RemediationStrategy.Canaries, canary)

	return builder
}

// Pull pulls existing cgu into CguBuilder struct.
func Pull(apiClient *clients.Settings, name, nsname string) (*CguBuilder, error) {
	klog.V(100).Infof("Pulling existing cgu name %s under namespace %s from cluster", name, nsname)

	return common.PullNamespacedBuilder[v1alpha1.ClusterGroupUpgrade, CguBuilder](
		context.TODO(), apiClient, v1alpha1.AddToScheme, name, nsname)
}

// DeleteAndWait deletes the cgu object and waits until the cgu is deleted.
func (builder *CguBuilder) DeleteAndWait(timeout time.Duration) (*CguBuilder, error) {
	if err := common.Validate(builder); err != nil {
		return builder, err
	}

	klog.V(100).Infof("Deleting cgu %s in namespace %s and waiting for the defined period until it is removed",
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
	if err := common.Validate(builder); err != nil {
		return err
	}

	klog.V(100).Infof(
		"Waiting for the defined period until cgu %s in namespace %s is deleted",
		builder.Definition.Name, builder.Definition.Namespace)

	return wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			_, err := builder.Get()
			if err == nil {
				klog.V(100).Infof("cgu %s/%s still present", builder.Definition.Name, builder.Definition.Namespace)

				return false, nil
			}

			if k8serrors.IsNotFound(err) {
				klog.V(100).Infof("cgu %s/%s is gone", builder.Definition.Name, builder.Definition.Namespace)

				return true, nil
			}

			klog.V(100).Infof("failed to get cgu %s/%s: %v", builder.Definition.Name, builder.Definition.Namespace, err)

			return false, err
		})
}

// WaitForCondition waits until the CGU has a condition that matches the expected, checking only the Type, Status,
// Reason, and Message fields. For the message field, it matches if the message contains the expected. Zero fields in
// the expected condition are ignored.
func (builder *CguBuilder) WaitForCondition(expected metav1.Condition, timeout time.Duration) (*CguBuilder, error) {
	if err := common.Validate(builder); err != nil {
		return builder, err
	}

	if !builder.Exists() {
		klog.V(100).Info("The CGU does not exist on the cluster")

		return builder, errCguObjectNotExists(builder.Definition.Name, builder.Definition.Namespace)
	}

	err := wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			var err error

			builder.Object, err = builder.Get()
			if err != nil {
				klog.V(100).Infof("failed to get cgu %s/%s: %v", builder.Definition.Name, builder.Definition.Namespace, err)

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
	if err := common.Validate(builder); err != nil {
		return nil, err
	}

	if cluster == "" {
		klog.V(100).Info("Cluster name cannot be empty")

		return nil, errClusterNameEmpty
	}

	if state == "" {
		klog.V(100).Info("State cannot be empty")

		return nil, errStateEmpty
	}

	klog.V(100).Infof(
		"Waiting until cluster %s on CGU %s in namespace %s is in state %s",
		cluster, builder.Definition.Name, builder.Definition.Namespace, state)

	if !builder.Exists() {
		return nil, errCguObjectNotExists(builder.Definition.Name, builder.Definition.Namespace)
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
				klog.V(100).Infof(
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
	if err := common.Validate(builder); err != nil {
		return builder, err
	}

	klog.V(100).Infof(
		"Waiting for CGU %s in namespace %s to start backup", builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Info("The CGU does not exist on the cluster")

		return builder, errCguObjectNotExists(builder.Definition.Name, builder.Definition.Namespace)
	}

	var err error

	err = wait.PollUntilContextTimeout(context.TODO(), 3*time.Second, timeout, true, func(context.Context) (bool, error) {
		builder.Object, err = builder.Get()
		if err != nil {
			klog.V(100).Infof(
				"Failed to get CGU %s in namespace %s due to: %v", builder.Definition.Name, builder.Definition.Namespace, err)

			return false, nil
		}

		return builder.Object.Status.Backup != nil, nil
	})
	if err == nil {
		return builder, nil
	}

	klog.V(100).Infof(
		"Failed to wait for CGU %s in namespace %s to start backup due to: %v",
		builder.Definition.Name, builder.Definition.Namespace, err)

	return nil, err
}
