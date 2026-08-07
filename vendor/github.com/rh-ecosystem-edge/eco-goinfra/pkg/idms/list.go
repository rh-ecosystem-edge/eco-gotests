package idms

import (
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"k8s.io/klog/v2"

	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ListImageDigestMirrorSets returns a cluster-wide imagedigestmirrorset inventory.
func ListImageDigestMirrorSets(
	apiClient *clients.Settings,
	options ...runtimeClient.ListOptions) ([]*Builder, error) {
	passedOptions := runtimeClient.ListOptions{}
	logMessage := "Listing all imagedigestmirrorsets"

	if apiClient == nil {
		klog.V(100).Info("The apiClient is nil")

		return nil, fmt.Errorf("apiClient cannot be nil")
	}

	if err := apiClient.AttachScheme(configv1.AddToScheme); err != nil {
		klog.V(100).Infof(
			"Failed to add configv1 scheme to client schemes")

		return nil, fmt.Errorf("failed to add configv1 to client schemes")
	}

	if len(options) > 1 {
		klog.V(100).Info("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	klog.V(100).Infof("%v", logMessage)

	imageDigestMirrorSets := new(configv1.ImageDigestMirrorSetList)

	err := apiClient.List(logging.DiscardContext(), imageDigestMirrorSets, &passedOptions)
	if err != nil {
		klog.V(100).Infof("Failed to list all imageDigestMirrorSets due to %s", err.Error())

		return nil, err
	}

	var IDMSObjects []*Builder

	for _, idms := range imageDigestMirrorSets.Items {
		copiedIDMS := idms
		idmsBuilder := &Builder{
			apiClient:  apiClient.Client,
			Object:     &copiedIDMS,
			Definition: &copiedIDMS,
		}

		IDMSObjects = append(IDMSObjects, idmsBuilder)
	}

	return IDMSObjects, nil
}
