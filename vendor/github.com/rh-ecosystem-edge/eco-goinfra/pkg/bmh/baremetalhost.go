package bmh

import (
	"context"
	"time"

	goclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/util/wait"

	"fmt"

	bmhv1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	"golang.org/x/exp/slices"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BmhBuilder provides struct for the bmh object containing connection to
// the cluster and the bmh definitions.
type BmhBuilder struct {
	Definition *bmhv1alpha1.BareMetalHost
	Object     *bmhv1alpha1.BareMetalHost
	apiClient  goclient.Client
	errorMsg   string
}

// AdditionalOptions additional options for bmh object.
type AdditionalOptions func(builder *BmhBuilder) (*BmhBuilder, error)

// NewBuilder creates a new instance of BmhBuilder.
func NewBuilder(
	apiClient *clients.Settings, name, nsname, bmcAddress, bmcSecretName, bootMacAddress, bootMode string) *BmhBuilder {
	if apiClient == nil {
		glog.V(100).Infof("The apiClient cannot be nil")

		return nil
	}

	err := apiClient.AttachScheme(bmhv1alpha1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add bmhv1alpha1 scheme to client schemes")

		return nil
	}

	builder := &BmhBuilder{
		apiClient: apiClient.Client,
		Definition: &bmhv1alpha1.BareMetalHost{
			Spec: bmhv1alpha1.BareMetalHostSpec{

				BMC: bmhv1alpha1.BMCDetails{
					Address:                        bmcAddress,
					CredentialsName:                bmcSecretName,
					DisableCertificateVerification: true,
				},
				BootMode:              bmhv1alpha1.BootMode(bootMode),
				BootMACAddress:        bootMacAddress,
				Online:                true,
				ExternallyProvisioned: false,
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the baremetalhost is empty")

		builder.errorMsg = "BMH 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		glog.V(100).Infof("The namespace of the baremetalhost is empty")

		builder.errorMsg = "BMH 'nsname' cannot be empty"

		return builder
	}

	if bmcAddress == "" {
		glog.V(100).Infof("The bootmacaddress of the baremetalhost is empty")

		builder.errorMsg = "BMH 'bmcAddress' cannot be empty"

		return builder
	}

	if bmcSecretName == "" {
		glog.V(100).Infof("The bmcsecret of the baremetalhost is empty")

		builder.errorMsg = "BMH 'bmcSecretName' cannot be empty"

		return builder
	}

	bootModeAcceptable := []string{"UEFI", "UEFISecureBoot", "legacy"}
	if !slices.Contains(bootModeAcceptable, bootMode) {
		builder.errorMsg = "not acceptable 'bootMode' value"

		return builder
	}

	if bootMacAddress == "" {
		glog.V(100).Infof("The bootmacaddress of the baremetalhost is empty")

		builder.errorMsg = "BMH 'bootMacAddress' cannot be empty"

		return builder
	}

	return builder
}

// WithRootDeviceDeviceName sets rootDeviceHints DeviceName to specified value.
func (builder *BmhBuilder) WithRootDeviceDeviceName(deviceName string) *BmhBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if deviceName == "" {
		glog.V(100).Infof("The baremetalhost rootDeviceHint deviceName is empty")

		builder.errorMsg = "the baremetalhost rootDeviceHint deviceName cannot be empty"

		return builder
	}

	if builder.Definition.Spec.RootDeviceHints == nil {
		builder.Definition.Spec.RootDeviceHints = &bmhv1alpha1.RootDeviceHints{}
	}

	builder.Definition.Spec.RootDeviceHints.DeviceName = deviceName

	return builder
}

// WithRootDeviceHTCL sets rootDeviceHints HTCL to specified value.
func (builder *BmhBuilder) WithRootDeviceHTCL(hctl string) *BmhBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if hctl == "" {
		glog.V(100).Infof("The baremetalhost rootDeviceHint hctl is empty")

		builder.errorMsg = "the baremetalhost rootDeviceHint hctl cannot be empty"

		return builder
	}

	if builder.Definition.Spec.RootDeviceHints == nil {
		builder.Definition.Spec.RootDeviceHints = &bmhv1alpha1.RootDeviceHints{}
	}

	builder.Definition.Spec.RootDeviceHints.HCTL = hctl

	return builder
}

// WithRootDeviceModel sets rootDeviceHints Model to specified value.
func (builder *BmhBuilder) WithRootDeviceModel(model string) *BmhBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if model == "" {
		glog.V(100).Infof("The baremetalhost rootDeviceHint model is empty")

		builder.errorMsg = "the baremetalhost rootDeviceHint model cannot be empty"

		return builder
	}

	if builder.Definition.Spec.RootDeviceHints == nil {
		builder.Definition.Spec.RootDeviceHints = &bmhv1alpha1.RootDeviceHints{}
	}

	builder.Definition.Spec.RootDeviceHints.Model = model

	return builder
}

// WithRootDeviceVendor sets rootDeviceHints Vendor to specified value.
func (builder *BmhBuilder) WithRootDeviceVendor(vendor string) *BmhBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if vendor == "" {
		glog.V(100).Infof("The baremetalhost rootDeviceHint vendor is empty")

		builder.errorMsg = "the baremetalhost rootDeviceHint vendor cannot be empty"

		return builder
	}

	if builder.Definition.Spec.RootDeviceHints == nil {
		builder.Definition.Spec.RootDeviceHints = &bmhv1alpha1.RootDeviceHints{}
	}

	builder.Definition.Spec.RootDeviceHints.Model = vendor

	return builder
}

// WithRootDeviceSerialNumber sets rootDeviceHints serialNumber to specified value.
func (builder *BmhBuilder) WithRootDeviceSerialNumber(serialNumber string) *BmhBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if serialNumber == "" {
		glog.V(100).Infof("The baremetalhost rootDeviceHint serialNumber is empty")

		builder.errorMsg = "the baremetalhost rootDeviceHint serialNumber cannot be empty"

		return builder
	}

	if builder.Definition.Spec.RootDeviceHints == nil {
		builder.Definition.Spec.RootDeviceHints = &bmhv1alpha1.RootDeviceHints{}
	}

	builder.Definition.Spec.RootDeviceHints.SerialNumber = serialNumber

	return builder
}

// WithRootDeviceMinSizeGigabytes sets rootDeviceHints MinSizeGigabytes to specified value.
func (builder *BmhBuilder) WithRootDeviceMinSizeGigabytes(size int) *BmhBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if size < 0 {
		glog.V(100).Infof("The baremetalhost rootDeviceHint size is less than 0")

		builder.errorMsg = "the baremetalhost rootDeviceHint size cannot be less than 0"

		return builder
	}

	if builder.Definition.Spec.RootDeviceHints == nil {
		builder.Definition.Spec.RootDeviceHints = &bmhv1alpha1.RootDeviceHints{}
	}

	builder.Definition.Spec.RootDeviceHints.MinSizeGigabytes = size

	return builder
}

// WithRootDeviceWWN sets rootDeviceHints WWN to specified value.
func (builder *BmhBuilder) WithRootDeviceWWN(wwn string) *BmhBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if wwn == "" {
		glog.V(100).Infof("The baremetalhost rootDeviceHint wwn is empty")

		builder.errorMsg = "the baremetalhost rootDeviceHint wwn cannot be empty"

		return builder
	}

	if builder.Definition.Spec.RootDeviceHints == nil {
		builder.Definition.Spec.RootDeviceHints = &bmhv1alpha1.RootDeviceHints{}
	}

	builder.Definition.Spec.RootDeviceHints.WWN = wwn

	return builder
}

// WithRootDeviceWWNWithExtension sets rootDeviceHints WWNWithExtension to specified value.
func (builder *BmhBuilder) WithRootDeviceWWNWithExtension(wwnWithExtension string) *BmhBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if wwnWithExtension == "" {
		glog.V(100).Infof("The baremetalhost rootDeviceHint wwnWithExtension is empty")

		builder.errorMsg = "the baremetalhost rootDeviceHint wwnWithExtension cannot be empty"

		return builder
	}

	if builder.Definition.Spec.RootDeviceHints == nil {
		builder.Definition.Spec.RootDeviceHints = &bmhv1alpha1.RootDeviceHints{}
	}

	builder.Definition.Spec.RootDeviceHints.WWNWithExtension = wwnWithExtension

	return builder
}

// WithRootDeviceWWNVendorExtension sets rootDeviceHint WWNVendorExtension to specified value.
func (builder *BmhBuilder) WithRootDeviceWWNVendorExtension(wwnVendorExtension string) *BmhBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if wwnVendorExtension == "" {
		glog.V(100).Infof("The baremetalhost rootDeviceHint wwnVendorExtension is empty")

		builder.errorMsg = "the baremetalhost rootDeviceHint wwnVendorExtension cannot be empty"

		return builder
	}

	if builder.Definition.Spec.RootDeviceHints == nil {
		builder.Definition.Spec.RootDeviceHints = &bmhv1alpha1.RootDeviceHints{}
	}

	builder.Definition.Spec.RootDeviceHints.WWNVendorExtension = wwnVendorExtension

	return builder
}

// WithRootDeviceRotationalDisk sets rootDeviceHint Rotational to specified value.
func (builder *BmhBuilder) WithRootDeviceRotationalDisk(rotational bool) *BmhBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if builder.Definition.Spec.RootDeviceHints == nil {
		builder.Definition.Spec.RootDeviceHints = &bmhv1alpha1.RootDeviceHints{}
	}

	builder.Definition.Spec.RootDeviceHints.Rotational = &rotational

	return builder
}

// WithOptions creates bmh with generic mutation options.
func (builder *BmhBuilder) WithOptions(options ...AdditionalOptions) *BmhBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("Setting bmh additional options")

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

// Pull pulls existing baremetalhost from cluster.
func Pull(apiClient *clients.Settings, name, nsname string) (*BmhBuilder, error) {
	glog.V(100).Infof("Pulling existing baremetalhost name %s under namespace %s from cluster", name, nsname)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient is empty")

		return nil, fmt.Errorf("baremetalhost 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(bmhv1alpha1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add bmhv1alpha1 scheme to client schemes")

		return nil, err
	}

	builder := &BmhBuilder{
		apiClient: apiClient.Client,
		Definition: &bmhv1alpha1.BareMetalHost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the baremetalhost is empty")

		return nil, fmt.Errorf("baremetalhost 'name' cannot be empty")
	}

	if nsname == "" {
		glog.V(100).Infof("The namespace of the baremetalhost is empty")

		return nil, fmt.Errorf("baremetalhost 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("baremetalhost object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Create makes a bmh in the cluster and stores the created object in struct.
func (builder *BmhBuilder) Create() (*BmhBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Creating the baremetalhost %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error
	if !builder.Exists() {
		err = builder.apiClient.Create(context.TODO(), builder.Definition)
		if err == nil {
			builder.Object = builder.Definition
		}
	}

	return builder, err
}

// Delete removes bmh from a cluster.
func (builder *BmhBuilder) Delete() (*BmhBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Deleting the baremetalhost %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		glog.V(100).Infof("bmh %s namespace: %s cannot be deleted because it does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
	if err != nil {
		return builder, fmt.Errorf("can not delete bmh: %w", err)
	}

	builder.Object = nil

	return builder, nil
}

// Get returns bmh object if found.
func (builder *BmhBuilder) Get() (*bmhv1alpha1.BareMetalHost, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof("Getting baremetalhost %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	bmh := &bmhv1alpha1.BareMetalHost{}

	err := builder.apiClient.Get(context.TODO(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, bmh)
	if err != nil {
		return nil, err
	}

	return bmh, nil
}

// Exists checks whether the given bmh exists.
func (builder *BmhBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Checking if baremetalhost %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// GetBmhOperationalState returns the current OperationalStatus of the bmh.
func (builder *BmhBuilder) GetBmhOperationalState() bmhv1alpha1.OperationalStatus {
	if valid, _ := builder.validate(); !valid {
		return ""
	}

	glog.V(100).Infof("Pull OperationalStatus value for %s baremetalhost within %s namespace",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		return ""
	}

	return builder.Object.Status.OperationalStatus
}

// GetBmhPowerOnStatus checks BareMetalHost PowerOn status.
func (builder *BmhBuilder) GetBmhPowerOnStatus() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Pull PoweredOn value for %s baremetalhost within %s namespace",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		return false
	}

	return builder.Object.Status.PoweredOn
}

// CreateAndWaitUntilProvisioned creates bmh object and waits until bmh is provisioned.
func (builder *BmhBuilder) CreateAndWaitUntilProvisioned(timeout time.Duration) (*BmhBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof(`Creating the baremetalhost %s in namespace %s and 
	waiting for the defined period until it is created`,
		builder.Definition.Name, builder.Definition.Namespace)

	builder, err := builder.Create()
	if err != nil {
		return nil, err
	}

	err = builder.WaitUntilProvisioned(timeout)

	return builder, err
}

// WaitUntilProvisioned waits for timeout duration or until bmh is provisioned.
func (builder *BmhBuilder) WaitUntilProvisioned(timeout time.Duration) error {
	return builder.WaitUntilInStatus(bmhv1alpha1.StateProvisioned, timeout)
}

// WaitUntilProvisioning waits for timeout duration or until bmh is provisioning.
func (builder *BmhBuilder) WaitUntilProvisioning(timeout time.Duration) error {
	return builder.WaitUntilInStatus(bmhv1alpha1.StateProvisioning, timeout)
}

// WaitUntilReady waits for timeout duration or until bmh is ready.
func (builder *BmhBuilder) WaitUntilReady(timeout time.Duration) error {
	return builder.WaitUntilInStatus(bmhv1alpha1.StateReady, timeout)
}

// WaitUntilAvailable waits for timeout duration or until bmh is available.
func (builder *BmhBuilder) WaitUntilAvailable(timeout time.Duration) error {
	return builder.WaitUntilInStatus(bmhv1alpha1.StateAvailable, timeout)
}

// WaitUntilInStatus waits for timeout duration or until bmh gets to a specific status.
func (builder *BmhBuilder) WaitUntilInStatus(status bmhv1alpha1.ProvisioningState, timeout time.Duration) error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	return wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			var err error

			builder.Object, err = builder.Get()
			if err != nil {
				return false, nil
			}

			if builder.Object.Status.Provisioning.State == status {
				return true, nil
			}

			return false, err
		})
}

// DeleteAndWaitUntilDeleted delete bmh object and waits until deleted.
func (builder *BmhBuilder) DeleteAndWaitUntilDeleted(timeout time.Duration) (*BmhBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof(`Deleting baremetalhost %s in namespace %s and 
	waiting for the defined period until it is removed`,
		builder.Definition.Name, builder.Definition.Namespace)

	builder, err := builder.Delete()
	if err != nil {
		return builder, err
	}

	err = builder.WaitUntilDeleted(timeout)

	return nil, err
}

// WaitUntilDeleted waits for timeout duration or until bmh is deleted.
func (builder *BmhBuilder) WaitUntilDeleted(timeout time.Duration) error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	err := wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, false, func(ctx context.Context) (bool, error) {
			_, err := builder.Get()
			if err == nil {
				glog.V(100).Infof("bmh %s/%s still present",
					builder.Definition.Namespace,
					builder.Definition.Name)

				return false, nil
			}

			if k8serrors.IsNotFound(err) {
				glog.V(100).Infof("bmh %s/%s is gone",
					builder.Definition.Namespace,
					builder.Definition.Name)

				return true, nil
			}

			glog.V(100).Infof("failed to get bmh %s/%s: %v",
				builder.Definition.Namespace,
				builder.Definition.Name, err)

			return false, err
		})

	return err
}

// WaitUntilAnnotationExists waits up to the specified timeout until the annotation exists.
func (builder *BmhBuilder) WaitUntilAnnotationExists(annotation string, timeout time.Duration) (*BmhBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	if annotation == "" {
		glog.V(100).Info("BMH annotation key cannot be empty")

		return nil, fmt.Errorf("bmh annotation key cannot be empty")
	}

	glog.V(100).Infof(
		"Waiting until BMH %s in namespace %s has annotation %s",
		builder.Definition.Name, builder.Definition.Namespace, annotation)

	if !builder.Exists() {
		return nil, fmt.Errorf(
			"baremetalhost object %s does not exist in namespace %s", builder.Definition.Name, builder.Definition.Namespace)
	}

	var err error

	err = wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			builder.Object, err = builder.Get()
			if err != nil {
				glog.V(100).Infof("failed to get bmh %s/%s: %v", builder.Definition.Namespace, builder.Definition.Name, err)

				return false, nil
			}

			if _, ok := builder.Object.Annotations[annotation]; !ok {
				return false, nil
			}

			return true, nil
		})
	if err != nil {
		return nil, err
	}

	return builder, nil
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *BmhBuilder) validate() (bool, error) {
	resourceCRD := "BareMetalHost"

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
