package sriov

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	"k8s.io/klog/v2"

	srIovV1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	errEmptyName = "SriovNetworkPoolConfig 'name' cannot be empty"
)

// PoolConfigBuilder provides struct for SriovNetworkPoolConfig object containing connection to the cluster
// and the SriovNetworkPoolConfig definitions.
type PoolConfigBuilder struct {
	// SriovNetworkPoolConfig definition. Used to create SriovNetworkPoolConfig object.
	Definition *srIovV1.SriovNetworkPoolConfig
	// Created sriovNetworkPoolConfig object.
	Object *srIovV1.SriovNetworkPoolConfig
	// Used in functions that define or mutate SriovNetworkPoolConfig definition.
	// errorMsg is processed before the SriovNetworkPoolConfig object is created.
	errorMsg string
	// apiClient opens api connection to the cluster.
	apiClient goclient.Client
}

// NewPoolConfigBuilder creates a new instance of PoolConfigBuilder.
func NewPoolConfigBuilder(apiClient *clients.Settings, name, nsname string) *PoolConfigBuilder {
	klog.V(100).Infof(
		"Initializing new SriovNetworkPoolConfig structure with the name %s in the namespace %s", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil
	}

	err := apiClient.AttachScheme(srIovV1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add sriovv1 scheme to client schemes")

		return nil
	}

	builder := &PoolConfigBuilder{
		apiClient: apiClient.Client,
		Definition: &srIovV1.SriovNetworkPoolConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		}}

	if name == "" {
		builder.errorMsg = errEmptyName

		return builder
	}

	if nsname == "" {
		builder.errorMsg = "SriovNetworkPoolConfig 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// Create generates an SriovNetworkPoolConfig in the cluster and stores the created object in struct.
func (builder *PoolConfigBuilder) Create() (*PoolConfigBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof(
		"Creating the SriovNetworkPoolConfig %s in namespace %s", builder.Definition.Name,
		builder.Definition.Namespace)

	if !builder.Exists() {
		err := builder.apiClient.Create(logging.DiscardContext(), builder.Definition)
		if err != nil {
			return nil, err
		}
	}

	builder.Object = builder.Definition

	return builder, nil
}

// Delete removes an SriovNetworkPoolConfig object.
func (builder *PoolConfigBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Deleting the SriovNetworkPoolConfig object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("SriovNetworkPoolConfig %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Definition)
	if err != nil {
		return err
	}

	builder.Object = nil

	return nil
}

// Exists checks whether the given SriovNetworkPoolConfig object exists in the cluster.
func (builder *PoolConfigBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if SriovNetworkPoolConfig %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Get returns SriovNetworkPoolConfig object if found.
func (builder *PoolConfigBuilder) Get() (*srIovV1.SriovNetworkPoolConfig, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	return builder.get(logging.DiscardContext())
}

func (builder *PoolConfigBuilder) get(ctx context.Context) (*srIovV1.SriovNetworkPoolConfig, error) {
	klog.V(100).Infof(
		"Collecting SriovNetworkPoolConfig object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	poolConfig := &srIovV1.SriovNetworkPoolConfig{}

	err := builder.apiClient.Get(logging.WithDiscardLogger(ctx), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, poolConfig)
	if err != nil {
		klog.V(100).Infof("Failed to get SriovNetworkPoolConfig %s in namespace %s", builder.Definition.Name,
			builder.Definition.Namespace)

		return nil, err
	}

	return poolConfig, nil
}

// Update renovates the existing SriovNetworkPoolConfig object with the new definition in builder.
func (builder *PoolConfigBuilder) Update() (*PoolConfigBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating the SriovNetworkPoolConfig object %s in namespace %s", builder.Definition.Name,
		builder.Definition.Namespace)

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err != nil {
		klog.V(100).Infof("Failed to update SriovNetworkPoolConfig %s in namespace %s", builder.Definition.Name,
			builder.Definition.Namespace)

		return nil, err
	}

	builder.Object = builder.Definition

	return builder, nil
}

// WithNodeSelector sets nodeSelector in the SriovNetworkPoolConfig definition.
func (builder *PoolConfigBuilder) WithNodeSelector(nodeSelector map[string]string) *PoolConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating SriovNetworkPoolConfig %s in namespace %s with Node selector: %v", builder.Definition.Name,
		builder.Definition.Namespace, nodeSelector)

	if len(nodeSelector) == 0 {
		builder.errorMsg = "SriovNetworkPoolConfig 'nodeSelector' cannot be empty map"

		return builder
	}

	builder.Definition.Spec.NodeSelector = &metav1.LabelSelector{MatchLabels: nodeSelector}

	return builder
}

// WithMaxUnavailable sets MaxUnavailable in the SriovNetworkPoolConfig definition.
func (builder *PoolConfigBuilder) WithMaxUnavailable(maxUnavailable intstrutil.IntOrString) *PoolConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating SriovNetworkPoolConfig %s in namespace %s with MaxUnavailable: %v", builder.Definition.Name,
		builder.Definition.Namespace, maxUnavailable)

	if maxUnavailable.Type == intstrutil.String {
		if strings.HasSuffix(maxUnavailable.StrVal, "%") {
			i := strings.TrimSuffix(maxUnavailable.StrVal, "%")

			value, err := strconv.Atoi(i)
			if err != nil {
				builder.errorMsg = fmt.Sprintf("invalid value %q: %v", maxUnavailable.StrVal, err)

				return builder
			}

			if value > 100 || value < 1 {
				builder.errorMsg = fmt.Sprintf("invalid value: percentage needs to be between 1 and 100: %v", maxUnavailable)

				return builder
			}
		} else {
			builder.errorMsg = fmt.Sprintf("invalid type: strings needs to be a percentage: %v", maxUnavailable)

			return builder
		}
	} else {
		if maxUnavailable.IntValue() < 0 {
			builder.errorMsg = fmt.Sprintf("negative number is not allowed: %v", maxUnavailable)

			return builder
		}
	}

	builder.Definition.Spec.MaxUnavailable = &maxUnavailable

	return builder
}

// WithRDMAMode method sets rdmaMode to shared/exclusive.
func (builder *PoolConfigBuilder) WithRDMAMode(mode string) *PoolConfigBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if mode == "" {
		klog.V(100).Info("PoolConfig RdmaMode cannot be empty")

		builder.errorMsg = "rdmaMode cannot be empty"

		return builder
	}

	if mode != "shared" && mode != "exclusive" {
		klog.V(100).Info("Invalid RdmaMode. Acceptable values: shared or exclusive")

		builder.errorMsg = "invalid value for rdmaMode. It should be 'shared' or 'exclusive'"

		return builder
	}

	builder.Definition.Spec.RdmaMode = mode

	return builder
}

// PullPoolConfig pulls existing SriovNetworkPoolConfig from cluster.
func PullPoolConfig(apiClient *clients.Settings, name, nsname string) (*PoolConfigBuilder, error) {
	klog.V(100).Infof("Pulling existing SriovNetworkPoolConfig name %s under namespace %s from cluster", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("SriovNetworkPoolConfig 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(srIovV1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add sriovv1 scheme to client schemes")

		return nil, err
	}

	builder := PoolConfigBuilder{
		apiClient: apiClient.Client,
		Definition: &srIovV1.SriovNetworkPoolConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the SriovNetworkPoolConfig is empty")

		return nil, errors.New(errEmptyName)
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the SriovNetworkPoolConfig is empty")

		return nil, errors.New("SriovNetworkPoolConfig 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("SriovNetworkPoolConfig object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return &builder, nil
}

// WaitUntilStable waits until timeout for a generation-fresh Ready=True
// PoolConfig. Progressing may be absent; fail only if it is present, current,
// and True.
func (builder *PoolConfigBuilder) WaitUntilStable(timeout time.Duration) error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Waiting up to %s until SriovNetworkPoolConfig %s/%s is stable",
		timeout, builder.Definition.Namespace, builder.Definition.Name)

	return wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			pool, err := builder.get(ctx)
			if err != nil {
				klog.V(100).Infof("Failed to get SriovNetworkPoolConfig %s/%s: %v",
					builder.Definition.Namespace, builder.Definition.Name, err)

				return false, nil
			}

			builder.Object = pool

			return isPoolConfigStable(pool), nil
		})
}

// WaitForCondition waits until timeout for a generation-fresh condition matching expected.
func (builder *PoolConfigBuilder) WaitForCondition(expected metav1.Condition, timeout time.Duration) error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	return waitForCondition(
		"SriovNetworkPoolConfig", builder.Definition.Namespace, builder.Definition.Name,
		expected, timeout, func(ctx context.Context) ([]metav1.Condition, int64, error) {
			pool, err := builder.get(ctx)
			if err != nil {
				return nil, 0, err
			}

			builder.Object = pool

			return pool.Status.Conditions, pool.GetGeneration(), nil
		})
}

// GetCondition returns the named condition from the last fetched object, or nil if missing.
func (builder *PoolConfigBuilder) GetCondition(conditionType string) *metav1.Condition {
	if builder == nil || builder.Object == nil {
		return nil
	}

	return meta.FindStatusCondition(builder.Object.Status.Conditions, conditionType)
}

// GetMatchedNodeCount returns status.matchedNodeCount from the last fetched object.
func (builder *PoolConfigBuilder) GetMatchedNodeCount() *int32 {
	if builder == nil || builder.Object == nil {
		return nil
	}

	return builder.Object.Status.MatchedNodeCount
}

// GetReadyNodeCount returns status.readyNodeCount from the last fetched object.
func (builder *PoolConfigBuilder) GetReadyNodeCount() *int32 {
	if builder == nil || builder.Object == nil {
		return nil
	}

	return builder.Object.Status.ReadyNodeCount
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *PoolConfigBuilder) validate() (bool, error) {
	resourceCRD := "SriovNetworkPoolConfig"

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
