package sriov

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	"k8s.io/klog/v2"

	srIovV1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	intstrutil "k8s.io/apimachinery/pkg/util/intstr"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
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
		builder.errorMsg = "SriovNetworkPoolConfig 'name' cannot be empty"

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
		err := builder.apiClient.Create(context.TODO(), builder.Definition)
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

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
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

	klog.V(100).Infof(
		"Collecting SriovNetworkPoolConfig object %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	poolConfig := &srIovV1.SriovNetworkPoolConfig{}

	err := builder.apiClient.Get(context.TODO(), goclient.ObjectKey{
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

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
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

		return nil, errors.New("SriovNetworkPoolConfig 'name' cannot be empty")
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
