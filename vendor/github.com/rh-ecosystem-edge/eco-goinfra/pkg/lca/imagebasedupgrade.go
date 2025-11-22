package lca

import (
	"context"
	"fmt"
	"slices"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"

	lcav1 "github.com/openshift-kni/lifecycle-agent/api/imagebasedupgrade/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
)

const (
	isTrue     = "True"
	isFalse    = "False"
	isComplete = "Completed"
	ibuName    = "upgrade"
	idle       = "Idle"
	disabled   = "Disabled"
)

// ImageBasedUpgradeBuilder provides struct for the imagebasedupgrade object containing connection to
// the cluster and the imagebasedupgrade definitions.
type ImageBasedUpgradeBuilder struct {
	// ImageBasedUpgrade definition. Used to store the imagebasedupgrade object.
	Definition *lcav1.ImageBasedUpgrade

	// Created imagebasedupgrade object.
	Object *lcav1.ImageBasedUpgrade
	// Used in functions that define or mutate the imagebasedupgrade definition.
	// errorMsg is processed before the imagebasedupgrade object is created
	errorMsg  string
	apiClient goclient.Client
}

// AdditionalOptions additional options for imagebasedupgrade object.
type AdditionalOptions func(builder *ImageBasedUpgradeBuilder) (*ImageBasedUpgradeBuilder, error)

// WithOptions creates imagebasedupgrade with generic mutation options.
func (builder *ImageBasedUpgradeBuilder) WithOptions(options ...AdditionalOptions) *ImageBasedUpgradeBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Info("Setting imagebasedupgrade additional options")

	for _, option := range options {
		if option != nil {
			builder, err := option(builder)
			if err != nil {
				klog.V(100).Info("Error occurred in mutation function")

				builder.errorMsg = err.Error()

				return builder
			}
		}
	}

	return builder
}

// PullImageBasedUpgrade pulls existing imagebasedupgrade from cluster.
func PullImageBasedUpgrade(apiClient *clients.Settings) (*ImageBasedUpgradeBuilder, error) {
	klog.V(100).Infof("Pulling existing imagebasedupgrade name %s from cluster", ibuName)

	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil, fmt.Errorf("the apiClient is nil")
	}

	err := apiClient.AttachScheme(lcav1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add lsa v1 scheme to client schemes")

		return nil, err
	}

	builder := &ImageBasedUpgradeBuilder{
		apiClient: apiClient.Client,
		Definition: &lcav1.ImageBasedUpgrade{
			ObjectMeta: metav1.ObjectMeta{
				Name: ibuName,
			},
		},
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("imagebasedupgrade object %s does not exist", ibuName)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Update modifies the imagebasedupgrade resource on the cluster
// to match what is defined in the local definition of the builder.
func (builder *ImageBasedUpgradeBuilder) Update() (*ImageBasedUpgradeBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating imagebasedupgrade %s",
		builder.Definition.Name)

	if !builder.Exists() {
		klog.V(100).Infof("imagebasedupgrade %s does not exist",
			builder.Definition.Name)

		return nil, fmt.Errorf("unable to update non-existing imagebasedupgrade")
	}

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err == nil {
		// Wait for the IBU to reconcile after it is updated.
		err = wait.PollUntilContextTimeout(
			context.TODO(), time.Second*2, time.Second*10, true, func(ctx context.Context) (bool, error) {
				klog.V(100).Infof("Waiting for imagebasedupgrade %s to finish reconciling",
					builder.Definition.Name)

				ibu, err := builder.Get()
				if err != nil {
					return false, err
				}

				if ibu.Generation == ibu.Status.ObservedGeneration {
					builder.Object = ibu

					return true, nil
				}

				return false, nil
			})
		if err == nil {
			builder.Definition = builder.Object
		}
	}

	return builder, err
}

// Delete removes the existing imagebasedupgrade from a cluster.
// Note that a new imagebasedupgrade with the specs from the deleted
// one is created instantly upon deletion.
func (builder *ImageBasedUpgradeBuilder) Delete() (*ImageBasedUpgradeBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Deleting the imagebasedupgrade %s",
		builder.Definition.Name)

	if !builder.Exists() {
		klog.V(100).Infof("imagebasedupgrade %s cannot be deleted because it does not exist",
			builder.Definition.Name)

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
	if err != nil {
		return builder, fmt.Errorf("can not delete imagebasedupgrade: %w", err)
	}

	builder.Object = nil

	return builder, nil
}

// Get returns imagebasedupgrade object if found.
func (builder *ImageBasedUpgradeBuilder) Get() (*lcav1.ImageBasedUpgrade, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting imagebasedupgrade %s",
		builder.Definition.Name)

	imagebasedupgrade := &lcav1.ImageBasedUpgrade{}

	err := builder.apiClient.Get(context.TODO(), goclient.ObjectKey{
		Name: builder.Definition.Name,
	}, imagebasedupgrade)
	if err != nil {
		return nil, err
	}

	return imagebasedupgrade, err
}

// Exists checks whether the given imagebasedupgrade exists.
func (builder *ImageBasedUpgradeBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if imagebasedupgrade %s",
		builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// WithSeedImage sets the seed image used by the imagebasedupgrade.
func (builder *ImageBasedUpgradeBuilder) WithSeedImage(
	seedImage string) *ImageBasedUpgradeBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting image %s in imagebasedupgrade", seedImage)

	builder.Definition.Spec.SeedImageRef.Image = seedImage

	return builder
}

// WithExtraManifests adds extraManifests to be used by the imagebasedupgrade.
// This is used to create/configure resources during upgrade.
func (builder *ImageBasedUpgradeBuilder) WithExtraManifests(
	extraManifestsConfigMapName, extraManifestsConfigMapNamespace string) *ImageBasedUpgradeBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Appending extraManifests's configmap name %s in namespace %s to the imagebasedupgrade",
		extraManifestsConfigMapName, extraManifestsConfigMapNamespace)

	builder.Definition.Spec.ExtraManifests = append(builder.Definition.Spec.ExtraManifests,
		lcav1.ConfigMapRef{Name: extraManifestsConfigMapName, Namespace: extraManifestsConfigMapNamespace})

	return builder
}

// WithOadpContent adds oadpContent to be used by the imagebasedupgrade.
// This is used for backup/restore during upgrade.
func (builder *ImageBasedUpgradeBuilder) WithOadpContent(
	oadpContentConfigMapName, oadpContentConfigMapNamespace string) *ImageBasedUpgradeBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Appending oadpContent's configmap name %s in namespace %s to the imagebasedupgrade",
		oadpContentConfigMapName, oadpContentConfigMapNamespace)

	builder.Definition.Spec.OADPContent = append(builder.Definition.Spec.OADPContent,
		lcav1.ConfigMapRef{Name: oadpContentConfigMapName, Namespace: oadpContentConfigMapNamespace})

	return builder
}

// AutoRollbackOnFailureInitMonitorTimeoutSeconds allows controlling
// the timeout for the upgrade to complete before the rollback.
// Set to 1800 seconds by default.
func (builder *ImageBasedUpgradeBuilder) AutoRollbackOnFailureInitMonitorTimeoutSeconds(
	seconds uint) *ImageBasedUpgradeBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting timeout for InitMonitor to %d seconds in imagebasedupgrade", seconds)

	builder.Definition.Spec.AutoRollbackOnFailure.InitMonitorTimeoutSeconds = int(seconds)

	return builder
}

// AutoRollbackOnFailureDisabledInitMonitor allows disabling the watchdog
// triggering a rollback upon upgrade failure within the set timeout.
func (builder *ImageBasedUpgradeBuilder) AutoRollbackOnFailureDisabledInitMonitor() *ImageBasedUpgradeBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Info("Setting the Init Monitor for Auto Rollback on failure to Disabled in imagebasedupgrade")

	if builder.Definition.Annotations == nil {
		builder.Definition.Annotations = make(map[string]string)
	}

	builder.Definition.Annotations["auto-rollback-on-failure.lca.openshift.io/init-monitor"] = disabled

	return builder
}

// AutoRollbackOnFailureDisableForPostReboot allows controlling
// AutoRollback on failure for post reboot stage.
func (builder *ImageBasedUpgradeBuilder) AutoRollbackOnFailureDisableForPostReboot() *ImageBasedUpgradeBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Info("Setting Auto Rollback on failure for post reboot to Disabled in imagebasedupgrade")

	if builder.Definition.Annotations == nil {
		builder.Definition.Annotations = make(map[string]string)
	}

	builder.Definition.Annotations["auto-rollback-on-failure.lca.openshift.io/post-reboot-config"] = disabled

	return builder
}

// AutoRollbackOnFailureDisableForUpgradeCompletion allows controlling
// AutoRollback on failure for upgrade completion stage. Enabled by default.
func (builder *ImageBasedUpgradeBuilder) AutoRollbackOnFailureDisableForUpgradeCompletion() *ImageBasedUpgradeBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Info("Setting Auto Rollback on failure for upgrade completion to Disabled in imagebasedupgrade")

	if builder.Definition.Annotations == nil {
		builder.Definition.Annotations = make(map[string]string)
	}

	builder.Definition.Annotations["auto-rollback-on-failure.lca.openshift.io/upgrade-completion"] = disabled

	return builder
}

// WithSeedImageVersion sets the seed image version used by the imagebasedupgrade.
func (builder *ImageBasedUpgradeBuilder) WithSeedImageVersion(
	seedImageVersion string) *ImageBasedUpgradeBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting seed image version %s in imagebasedupgrade", seedImageVersion)

	builder.Definition.Spec.SeedImageRef.Version = seedImageVersion

	return builder
}

// WithSeedImagePullSecretRef sets the imagebasedupgrade with reference to the pull-secret
// for pulling the seed image.
func (builder *ImageBasedUpgradeBuilder) WithSeedImagePullSecretRef(
	pullSecretName string) *ImageBasedUpgradeBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting pull-secret %s in imagebasedupgrade for pulling the seed image", pullSecretName)

	builder.Definition.Spec.SeedImageRef.PullSecretRef = &lcav1.PullSecretRef{Name: pullSecretName}

	return builder
}

// WaitUntilStageComplete waits the specified timeout for the imagebasedupgrade to complete
// actions for the provided stage.
//
//nolint:gocognit
func (builder *ImageBasedUpgradeBuilder) WaitUntilStageComplete(stage string) (*ImageBasedUpgradeBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Waiting for imagebasedupgrade %s to set stage %s",
		builder.Definition.Name,
		stage)

	if !builder.Exists() {
		klog.V(100).Info("The imagebasedupgrade does not exist on the cluster")

		return builder, fmt.Errorf("%s", builder.errorMsg)
	}

	if !slices.Contains([]string{idle, "Prep", "Upgrade", "Rollback"}, stage) {
		return builder, fmt.Errorf("wrong stage selected for imagebasedupgrade")
	}

	// Polls periodically to determine if imagebasedupgrade is in desired state.
	var err error

	err = wait.PollUntilContextTimeout(
		context.TODO(), time.Second*3, time.Minute*30, true, func(ctx context.Context) (bool, error) {
			builder.Object, err = builder.Get()
			if err != nil {
				return false, nil
			}

			builder.Definition = builder.Object

			for _, condition := range builder.Object.Status.Conditions {
				switch stage {
				case idle:
					if condition.Status == isTrue && condition.Type == idle {
						return true, nil
					}

				case "Prep":
					if condition.Status == isFalse && condition.Type == "PrepInProgress" &&
						condition.Message == "Prep completed" && condition.Reason == isComplete {
						return true, nil
					}
				case "Upgrade":
					if condition.Status == isFalse && condition.Type == "UpgradeInProgress" &&
						condition.Message == "Upgrade completed" && condition.Reason == isComplete {
						return true, nil
					}

				case "Rollback":
					if condition.Status == isFalse && condition.Type == "RollbackInProgress" &&
						condition.Message == "Rollback completed" && condition.Reason == isComplete {
						return true, nil
					}

				default:
					return false, fmt.Errorf("wrong stage selected for imagebasedupgrade")
				}
			}

			return false, nil
		})
	if err == nil {
		return builder, nil
	}

	return nil, err
}

// WithStage sets the stage used by the imagebasedupgrade.
func (builder *ImageBasedUpgradeBuilder) WithStage(
	stage string) *ImageBasedUpgradeBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	klog.V(100).Infof("Setting stage %s in imagebasedupgrade", stage)
	builder.Definition.Spec.Stage = lcav1.ImageBasedUpgradeStage(stage)

	return builder
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *ImageBasedUpgradeBuilder) validate() (bool, error) {
	resourceCRD := "ImageBasedUpgrade"

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
