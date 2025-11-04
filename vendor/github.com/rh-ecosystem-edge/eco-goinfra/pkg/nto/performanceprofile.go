package nto //nolint:misspell
import (
	"context"
	"fmt"

	"k8s.io/utils/strings/slices"

	"github.com/golang/glog"
	performanceprofilev2 "github.com/openshift/cluster-node-tuning-operator/pkg/apis/performanceprofile/v2"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Builder provides a struct for PerformanceProfile object from the cluster and a PerformanceProfile definition.
type Builder struct {
	// PerformanceProfile definition, used to create the PerformanceProfile object.
	Definition *performanceprofilev2.PerformanceProfile
	// Created PerformanceProfile object.
	Object *performanceprofilev2.PerformanceProfile
	// Used to store latest error message upon defining or mutating PerformanceProfile definition.
	errorMsg string
	// api client to interact with the cluster.
	apiClient goclient.Client
}

// NewBuilder creates a new instance of Builder.
func NewBuilder(
	apiClient *clients.Settings, name, cpuIsolated, cpuReserved string, nodeSelector map[string]string) *Builder {
	glog.V(100).Infof(
		"Initializing new PerformanceProfile structure with the following params: "+
			"name: %s, cpu isolated: %s, cpu reserved %s, nodeSelector %v", name, cpuIsolated, cpuReserved, nodeSelector)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient cannot be nil")

		return nil
	}

	err := apiClient.AttachScheme(performanceprofilev2.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add node-tuning-operator v2 scheme to client schemes")

		return nil
	}

	isolatedCPUSet := performanceprofilev2.CPUSet(cpuIsolated)
	reservedCPUSet := performanceprofilev2.CPUSet(cpuReserved)

	builder := &Builder{
		apiClient: apiClient.Client,
		Definition: &performanceprofilev2.PerformanceProfile{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: performanceprofilev2.PerformanceProfileSpec{
				CPU: &performanceprofilev2.CPU{
					Isolated: &isolatedCPUSet,
					Reserved: &reservedCPUSet,
				},
				NodeSelector: nodeSelector,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the PerformanceProfile is empty")

		builder.errorMsg = "PerformanceProfile's name is empty"

		return builder
	}

	if cpuIsolated == "" {
		glog.V(100).Infof("Isolated CPU of the PerformanceProfile is empty")

		builder.errorMsg = "PerformanceProfile's 'cpuIsolated' is empty"

		return builder
	}

	if cpuReserved == "" {
		glog.V(100).Infof("Reserved CPU of the PerformanceProfile is empty")

		builder.errorMsg = "PerformanceProfile's 'cpuReserved' is empty"

		return builder
	}

	if len(nodeSelector) == 0 {
		glog.V(100).Infof("NodeSelector of the PerformanceProfile is empty")

		builder.errorMsg = "PerformanceProfile's 'nodeSelector' is empty"

		return builder
	}

	return builder
}

// Pull pulls existing PerformanceProfile from cluster.
func Pull(apiClient *clients.Settings, name string) (*Builder, error) {
	glog.V(100).Infof("Pulling existing PerformanceProfile name %s from cluster", name)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient is empty")

		return nil, fmt.Errorf("performanceProfile 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(performanceprofilev2.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add node-tuning-operator v2 scheme to client schemes")

		return nil, err
	}

	builder := &Builder{
		apiClient: apiClient.Client,
		Definition: &performanceprofilev2.PerformanceProfile{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the PerformanceProfile is empty")

		return nil, fmt.Errorf("performanceProfile 'name' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("PerformanceProfile object %s does not exist", name)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// WithHugePages defines the HugePages in the PerformanceProfile. hugePageSize allowed values are 2M, 1G.
func (builder *Builder) WithHugePages(hugePageSize string, hugePages []performanceprofilev2.HugePage) *Builder {
	glog.V(100).Infof("Adding hugePages to PerformanceProfile %s, size %s, hugePages %v",
		builder.Definition.Name, hugePageSize, hugePages)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if hugePageSize == "" {
		glog.V(100).Infof("The hugePageSize is empty")

		builder.errorMsg = "'hugePageSize' argument cannot be empty"

		return builder
	}

	allowedHugePageSize := []string{"2M", "1G"}
	if !slices.Contains(allowedHugePageSize, hugePageSize) {
		glog.V(100).Infof("'hugePageSize' has invalid parameter %s. Allowed parameters %v",
			hugePageSize, allowedHugePageSize)

		builder.errorMsg = fmt.Sprintf("'hugePageSize' argument is not in allowed list: %v", allowedHugePageSize)

		return builder
	}

	if len(hugePages) == 0 {
		glog.V(100).Infof("'hugePages' argument cannot be empty")

		builder.errorMsg = "'hugePages' argument cannot be empty"

		return builder
	}

	pageSize := performanceprofilev2.HugePageSize(hugePageSize)

	if builder.Definition.Spec.HugePages != nil {
		builder.Definition.Spec.HugePages.DefaultHugePagesSize = &pageSize
		builder.Definition.Spec.HugePages.Pages = hugePages

		return builder
	}

	builder.Definition.Spec.HugePages = &performanceprofilev2.HugePages{
		DefaultHugePagesSize: &pageSize,
		Pages:                hugePages,
	}

	return builder
}

// WithMachineConfigPoolSelector defines the MachineConfigPoolSelector in the PerformanceProfile.
func (builder *Builder) WithMachineConfigPoolSelector(machineConfigPoolSelector map[string]string) *Builder {
	glog.V(100).Infof("Adding MachineConfigPoolSelector %v to PerformanceProfile %s",
		machineConfigPoolSelector, builder.Definition.Name)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if len(machineConfigPoolSelector) == 0 {
		glog.V(100).Infof("'machineConfigPoolSelector' argument cannot be empty")

		builder.errorMsg = "'machineConfigPoolSelector' argument cannot be empty"

		return builder
	}

	builder.Definition.Spec.MachineConfigPoolSelector = machineConfigPoolSelector

	return builder
}

// WithNodeSelector defines the nodeSelector in the PerformanceProfile.
func (builder *Builder) WithNodeSelector(nodeSelector map[string]string) *Builder {
	glog.V(100).Infof("Adding nodeSelector %v to PerformanceProfile %s",
		nodeSelector, builder.Definition.Name)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if len(nodeSelector) == 0 {
		glog.V(100).Infof("'nodeSelector' argument cannot be empty")

		builder.errorMsg = "'nodeSelector' argument cannot be empty"

		return builder
	}

	builder.Definition.Spec.NodeSelector = nodeSelector

	return builder
}

// WithNumaTopology defines the NumaTopologyPolicy in the PerformanceProfile.
func (builder *Builder) WithNumaTopology(topologyPolicy string) *Builder {
	glog.V(100).Infof("Adding NumaTopologyPolicy %s to PerformanceProfile %s",
		topologyPolicy, builder.Definition.Name)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if topologyPolicy == "" {
		glog.V(100).Infof("The topologyPolicy is empty")

		builder.errorMsg = "'topologyPolicy' argument cannot be empty"

		return builder
	}

	allowedTopologyPolicies := []string{"best-effort", "restricted", "single-numa-node"}
	if !slices.Contains(allowedTopologyPolicies, topologyPolicy) {
		glog.V(100).Infof("'allowedTopologyPolicies' has invalid parameter %s. Allowed parameters %v",
			topologyPolicy, allowedTopologyPolicies)

		builder.errorMsg = fmt.Sprintf("'allowedTopologyPolicies' argument is not in allowed list %v",
			allowedTopologyPolicies)

		return builder
	}

	builder.Definition.Spec.NUMA = &performanceprofilev2.NUMA{TopologyPolicy: &topologyPolicy}

	return builder
}

// WithRTKernel defines the Real Time Kernel in the PerformanceProfile.
func (builder *Builder) WithRTKernel() *Builder {
	glog.V(100).Infof("Adding RTKernel flag to PerformanceProfile %s", builder.Definition.Name)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	trueFlag := true
	builder.Definition.Spec.RealTimeKernel = &performanceprofilev2.RealTimeKernel{Enabled: &trueFlag}

	return builder
}

// WithGloballyDisableIrqLoadBalancing defines the globallyDisableIrqLoadBalancing in the PerformanceProfile.
func (builder *Builder) WithGloballyDisableIrqLoadBalancing() *Builder {
	glog.V(100).Infof("Adding globallyDisableIrqLoadBalancing flag to PerformanceProfile %s",
		builder.Definition.Name)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	trueFlag := true
	builder.Definition.Spec.GloballyDisableIrqLoadBalancing = &trueFlag

	return builder
}

// WithWorkloadHints defines the Workload Hints in the PerformanceProfile.
func (builder *Builder) WithWorkloadHints(rtHint, perPodPowerMgmtHint, highPowerHint bool) *Builder {
	glog.V(100).Infof(
		"Adding WorkloadHints flags: RealTime=%t, PerPodPowerManagement=%t, HighPowerConsumption=%t to PerformanceProfile %s",
		rtHint, perPodPowerMgmtHint, highPowerHint, builder.Definition.Name)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if builder.Definition.Spec.WorkloadHints == nil {
		builder.Definition.Spec.WorkloadHints = &performanceprofilev2.WorkloadHints{
			RealTime:              &rtHint,
			PerPodPowerManagement: &perPodPowerMgmtHint,
			HighPowerConsumption:  &highPowerHint,
		}

		return builder
	}

	builder.Definition.Spec.WorkloadHints.RealTime = &rtHint
	builder.Definition.Spec.WorkloadHints.PerPodPowerManagement = &perPodPowerMgmtHint
	builder.Definition.Spec.WorkloadHints.HighPowerConsumption = &highPowerHint

	return builder
}

// WithAnnotations defines the annotations in the PerformanceProfile.
func (builder *Builder) WithAnnotations(annotations map[string]string) *Builder {
	glog.V(100).Infof("Adding annotations %v to the PerformanceProfile %s",
		annotations, builder.Definition.Name)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if len(annotations) == 0 {
		glog.V(100).Infof("'annotations' argument cannot be empty")

		builder.errorMsg = "'annotations' argument cannot be empty"

		return builder
	}

	builder.Definition.Annotations = annotations

	return builder
}

// WithNet defines the net in the PerformanceProfile.
func (builder *Builder) WithNet(userLevelNetworking bool, devices []performanceprofilev2.Device) *Builder {
	glog.V(100).Infof("Adding net field to the PerformanceProfile %s", builder.Definition.Name)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if len(devices) == 0 {
		glog.V(100).Infof("'net' argument cannot be empty")

		builder.errorMsg = "'net' argument cannot be empty"

		return builder
	}

	netField := performanceprofilev2.Net{
		UserLevelNetworking: &userLevelNetworking,
		Devices:             devices,
	}

	builder.Definition.Spec.Net = &netField

	return builder
}

// WithAdditionalKernelArgs defines the additionalKernelArgs in the PerformanceProfile.
func (builder *Builder) WithAdditionalKernelArgs(additionalKernelArgs []string) *Builder {
	glog.V(100).Infof("Adding additionalKernelArgs field to the PerformanceProfile %s",
		builder.Definition.Name)

	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if len(additionalKernelArgs) == 0 {
		glog.V(100).Infof("'additionalKernelArgs' argument cannot be empty")

		builder.errorMsg = "'additionalKernelArgs' argument cannot be empty"

		return builder
	}

	builder.Definition.Spec.AdditionalKernelArgs = additionalKernelArgs

	return builder
}

// Create the PerformanceProfile in the cluster and store the created object in Object.
func (builder *Builder) Create() (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Creating PerformanceProfile %s ", builder.Definition.Name)

	if !builder.Exists() {
		err := builder.apiClient.Create(context.TODO(), builder.Definition)
		if err != nil {
			return nil, err
		}

		builder.Object, err = builder.Get()
		if err != nil {
			return nil, err
		}
	}

	return builder, nil
}

// Exists checks whether the given PerformanceProfile exists.
func (builder *Builder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Checking if PerformanceProfile %s exists", builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Get fetches the defined PerformanceProfile from the cluster.
func (builder *Builder) Get() (*performanceprofilev2.PerformanceProfile, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof("Getting PerformanceProfile %s", builder.Definition.Name)

	module := &performanceprofilev2.PerformanceProfile{}

	err := builder.apiClient.Get(context.TODO(), goclient.ObjectKey{
		Name: builder.Definition.Name,
	}, module)
	if err != nil {
		return nil, err
	}

	return module, nil
}

// Delete removes the PerformanceProfile.
func (builder *Builder) Delete() (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Deleting PerformanceProfile %s", builder.Definition.Name)

	if !builder.Exists() {
		glog.V(100).Infof("performanceprofile %s cannot be deleted because it does not exist",
			builder.Definition.Name)

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
	if err != nil {
		return builder, err
	}

	builder.Object = nil

	return builder, nil
}

// Update renovates the existing PerformanceProfile object with the PerformanceProfile definition in builder.
func (builder *Builder) Update(force bool) (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Updating the PerformanceProfile object: %s", builder.Definition.Name)

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err != nil {
		if force {
			glog.V(100).Infof(
				"Failed to update the PerformanceProfile object %s. "+
					"Note: Force flag set, executed delete/create methods instead", builder.Definition.Name)

			builder, err := builder.Delete()
			if err != nil {
				glog.V(100).Infof(
					"Failed to update the PerformanceProfile object %s, "+
						"due to error in delete function", builder.Definition.Name)

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

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *Builder) validate() (bool, error) {
	resourceCRD := "PerformanceProfile"

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
