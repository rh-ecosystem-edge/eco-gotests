package ibgu

import (
	"context"
	"fmt"
	"strings"
	"time"

	lcav1 "github.com/openshift-kni/lifecycle-agent/api/imagebasedupgrade/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/imagebasedgroupupgrades/v1alpha1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var conditionComplete = metav1.Condition{Type: "Progressing", Status: metav1.ConditionFalse, Reason: "Completed"}

// IbguBuilder provides struct for the ibgu object containing connection to
// the cluster and the ibgu definitions.
type IbguBuilder struct {
	// ibgu Definition, used to create the ibgu object.
	Definition *v1alpha1.ImageBasedGroupUpgrade
	// created ibgu object.
	Object *v1alpha1.ImageBasedGroupUpgrade
	// api client to interact with the cluster.
	apiClient goclient.Client
	// used to store latest error message upon defining or mutating application definition.
	errorMsg string
}

// NewIbguBuilder creates a new instance of IbguBuilder.
func NewIbguBuilder(
	apiClient *clients.Settings,
	name string,
	nsname string) *IbguBuilder {
	klog.V(100).Infof(
		"Initializing new ibgu structure with the following params: name: %s, nsname: %s", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient for the ibgu is nil")

		return nil
	}

	err := apiClient.AttachScheme(v1alpha1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add ibgu v1alpha1 scheme to client schemes")

		return nil
	}

	builder := &IbguBuilder{
		apiClient: apiClient.Client,
		Definition: &v1alpha1.ImageBasedGroupUpgrade{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
			Spec: v1alpha1.ImageBasedGroupUpgradeSpec{
				IBUSpec: lcav1.ImageBasedUpgradeSpec{},
				Plan:    []v1alpha1.PlanItem{},
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the ibgu is empty")

		builder.errorMsg = "ibgu 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the ibgu is empty")

		builder.errorMsg = "ibgu 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// WithClusterLabelSelectors appends labels to the ibgu clusterLabelSelectors.
func (builder *IbguBuilder) WithClusterLabelSelectors(labels map[string]string) *IbguBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Creating IBGU with %v cluster label selector", labels)

	if len(labels) == 0 {
		klog.V(100).Info("The 'labels' of the IBGU is empty")

		builder.errorMsg = "can not apply empty cluster label selectors to the IBGU"

		return builder
	}

	labelSelectors := []metav1.LabelSelector{
		{
			MatchLabels: labels,
		},
	}

	builder.Definition.Spec.ClusterLabelSelectors = labelSelectors

	return builder
}

// WithAutoRollbackOnFailure appends the AutoRollbackOnFailure InitMonitorTimeout to the ibuSpec.
func (builder *IbguBuilder) WithAutoRollbackOnFailure(initMonitorTimeout int) *IbguBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Creating IBGU with AutoRollbackOnFailure and InitMonitorTimeout set to %d seconds",
		initMonitorTimeout)

	if initMonitorTimeout < 0 {
		klog.V(100).Info("The 'initMonitorTimeout' parameter is undefined")

		builder.errorMsg = "initMonitorTimeout cannot be undefined"

		return builder
	}

	autoRollbackOnFailure := lcav1.AutoRollbackOnFailure{
		InitMonitorTimeoutSeconds: initMonitorTimeout,
	}

	builder.Definition.Spec.IBUSpec.AutoRollbackOnFailure = &autoRollbackOnFailure

	return builder
}

// WithSeedImageRef appends the SeedImageRef to the ibuSpec.
func (builder *IbguBuilder) WithSeedImageRef(seedImage string, seedVersion string) *IbguBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Creating IBGU with %s seed image and %s seed version", seedImage, seedVersion)

	if seedImage == "" {
		klog.V(100).Info("The 'seedImage' parameter is empty")

		builder.errorMsg = "seedImage cannot be empty"

		return builder
	}

	if seedVersion == "" {
		klog.V(100).Info("The 'seedVersion' parameter is empty")

		builder.errorMsg = "seedVersion cannot be empty"

		return builder
	}

	ibuSpec := lcav1.ImageBasedUpgradeSpec{
		SeedImageRef: lcav1.SeedImageRef{
			Image:   seedImage,
			Version: seedVersion,
		},
	}

	builder.Definition.Spec.IBUSpec = ibuSpec

	return builder
}

// WithOadpContent appends the oadpContent to the ibuSpec.
func (builder *IbguBuilder) WithOadpContent(name string, namespace string) *IbguBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Creating IBGU with OADP configmap %s in namespace %s", name, namespace)

	if name == "" {
		klog.V(100).Info("The 'name' parameter for OADP content is empty")

		builder.errorMsg = "oadp content name cannot be empty"

		return builder
	}

	if namespace == "" {
		klog.V(100).Info("The 'namespace' parameter for OADP content is empty")

		builder.errorMsg = "oadp content namespace cannot be empty"

		return builder
	}

	oadpContent := lcav1.ConfigMapRef{
		Name:      name,
		Namespace: namespace,
	}

	builder.Definition.Spec.IBUSpec.OADPContent = append(builder.Definition.Spec.IBUSpec.OADPContent, oadpContent)

	return builder
}

// WithPlan appends the plan to the ibgu.
func (builder *IbguBuilder) WithPlan(actions []string, maxConcurrency int, timeout int) *IbguBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof(
		"Creating IBGU with plan actions %v, maxConcurrency %d and timeout %d",
		actions,
		maxConcurrency,
		timeout,
	)

	if len(actions) == 0 {
		klog.V(100).Info("The 'actions' slice is empty")

		builder.errorMsg = "plan actions cannot be empty"

		return builder
	}

	if maxConcurrency <= 0 {
		klog.V(100).Infof("Invalid maxConcurrency value: %d", maxConcurrency)

		builder.errorMsg = "maxConcurrency must be greater than 0"

		return builder
	}

	if timeout <= 0 {
		klog.V(100).Infof("Invalid timeout value: %d", timeout)

		builder.errorMsg = "timeout must be greater than 0"

		return builder
	}

	plan := v1alpha1.PlanItem{
		Actions: actions,
		RolloutStrategy: v1alpha1.RolloutStrategy{
			MaxConcurrency: maxConcurrency,
			Timeout:        timeout,
		},
	}

	builder.Definition.Spec.Plan = append(builder.Definition.Spec.Plan, plan)

	return builder
}

// Get returns imagebasedgroupupgrade object if found.
func (builder *IbguBuilder) Get() (*v1alpha1.ImageBasedGroupUpgrade, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting imagebasedgroupupgrade %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	imagebasedgroupupgrade := &v1alpha1.ImageBasedGroupUpgrade{}

	err := builder.apiClient.Get(logging.DiscardContext(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, imagebasedgroupupgrade)
	if err != nil {
		klog.V(100).Infof("Failed to get imagebasedgroupupgrade %s in namespace %s: %v",
			builder.Definition.Name, builder.Definition.Namespace, err)

		return nil, err
	}

	return imagebasedgroupupgrade, nil
}

// Exists checks whether the given imagebasedgroupupgrade exists.
func (builder *IbguBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if imagebasedgroupupgrade %s in namespace %s exists",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Create makes an IBGU in the cluster and stores the created object in struct.
func (builder *IbguBuilder) Create() (*IbguBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating the imageasedgroupupgrade %s in namespace %s",
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

// Delete removes the IBGU from the cluster.
func (builder *IbguBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof("Deleting the ImageBasedGroupUpgrade %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Object)
	if err != nil {
		return err
	}

	builder.Object = nil

	return nil
}

// PullIbgu pulls existing ibgu into IbguBuilder struct.
func PullIbgu(apiClient *clients.Settings, name, nsname string) (*IbguBuilder, error) {
	klog.V(100).Infof("Pulling existing ibgu name %s under namespace %s from cluster", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("ibgu 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(v1alpha1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add ibgu v1alpha1 scheme to client schemes")

		return nil, err
	}

	builder := &IbguBuilder{
		apiClient: apiClient.Client,
		Definition: &v1alpha1.ImageBasedGroupUpgrade{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the ibgu is empty")

		return nil, fmt.Errorf("ibgu 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the ibgu is empty")

		return nil, fmt.Errorf("ibgu 'nsname' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("ibgu object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// DeleteAndWait deletes the ibgu object and waits until the ibgu is deleted.
func (builder *IbguBuilder) DeleteAndWait(timeout time.Duration) (*IbguBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Deleting ibgu %s in namespace %s and waiting for the defined period until it is removed",
		builder.Definition.Name, builder.Definition.Namespace)

	err := builder.Delete()
	if err != nil {
		return builder, err
	}

	err = builder.WaitUntilDeleted(timeout)

	return builder, err
}

// WaitUntilDeleted waits for the duration of the defined timeout or until the ibgu is deleted.
func (builder *IbguBuilder) WaitUntilDeleted(timeout time.Duration) error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof(
		"Waiting for the defined period until ibgu %s in namespace %s is deleted",
		builder.Definition.Name, builder.Definition.Namespace)

	return wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			_, err := builder.Get()
			if err == nil {
				klog.V(100).Infof("ibgu %s/%s still present", builder.Definition.Namespace, builder.Definition.Name)

				return false, nil
			}

			if k8serrors.IsNotFound(err) {
				klog.V(100).Infof("ibgu %s/%s is gone", builder.Definition.Namespace, builder.Definition.Name)

				return true, nil
			}

			klog.V(100).Infof("failed to get ibgu %s/%s: %v", builder.Definition.Namespace, builder.Definition.Name, err)

			return false, err
		})
}

// WaitForCondition waits until the IBGU has a condition that matches the expected, checking only the Type, Status,
// Reason, and Message fields. For the message field, it matches if the message contains the expected. Zero fields in
// the expected condition are ignored.
func (builder *IbguBuilder) WaitForCondition(expected metav1.Condition, timeout time.Duration) (*IbguBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	if !builder.Exists() {
		klog.V(100).Info("The IBGU does not exist on the cluster")

		return builder, fmt.Errorf(
			"ibgu object %s does not exist in namespace %s", builder.Definition.Name, builder.Definition.Namespace)
	}

	err := wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			var err error

			builder.Object, err = builder.Get()
			if err != nil {
				klog.V(100).Infof("failed to get ibgu %s/%s: %v", builder.Definition.Namespace, builder.Definition.Name, err)

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

// WaitUntilComplete waits the specified timeout for the IBGU to complete.
func (builder *IbguBuilder) WaitUntilComplete(timeout time.Duration) (*IbguBuilder, error) {
	return builder.WaitForCondition(conditionComplete, timeout)
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *IbguBuilder) validate() (bool, error) {
	resourceCRD := "ibgu"

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
