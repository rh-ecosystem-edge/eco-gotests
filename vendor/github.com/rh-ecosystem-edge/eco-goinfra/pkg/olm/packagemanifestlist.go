package olm

import (
	"fmt"

	operatorv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/olm/package-server/operators/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"k8s.io/klog/v2"
)

// ListPackageManifest returns PackageManifest inventory in the given namespace.
func ListPackageManifest(
	apiClient *clients.Settings,
	nsname string,
	options ...client.ListOptions) ([]*PackageManifestBuilder, error) {
	if nsname == "" {
		klog.V(100).Info("packagemanifest 'nsname' parameter can not be empty")

		return nil, fmt.Errorf("failed to list packagemanifests, 'nsname' parameter is empty")
	}

	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil, fmt.Errorf("failed to list packageManifest, 'apiClient' parameter is empty")
	}

	err := apiClient.AttachScheme(operatorv1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add packageManifest scheme to client schemes")

		return nil, err
	}

	passedOptions := client.ListOptions{}
	logMessage := fmt.Sprintf("Listing PackageManifests in the namespace %s", nsname)

	if len(options) > 1 {
		klog.V(100).Info("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	klog.V(100).Infof("%v", logMessage)

	pkgManifestList := new(operatorv1.PackageManifestList)

	err = apiClient.List(logging.DiscardContext(), pkgManifestList, &passedOptions)
	if err != nil {
		klog.V(100).Infof("Failed to list PackageManifests in the namespace %s due to %s",
			nsname, err.Error())

		return nil, err
	}

	var pkgManifestObjects []*PackageManifestBuilder

	for _, runningPkgManifest := range pkgManifestList.Items {
		copiedPkgManifest := runningPkgManifest
		pkgManifestBuilder := &PackageManifestBuilder{
			apiClient:  apiClient.Client,
			Object:     &copiedPkgManifest,
			Definition: &copiedPkgManifest,
		}

		pkgManifestObjects = append(pkgManifestObjects, pkgManifestBuilder)
	}

	return pkgManifestObjects, nil
}
