package olm

import (
	"context"
	"fmt"

	operatorv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/olm/package-server/operators/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
)

// ListPackageManifest returns PackageManifest inventory in the given namespace.
func ListPackageManifest(
	apiClient *clients.Settings,
	nsname string,
	options ...client.ListOptions) ([]*PackageManifestBuilder, error) {
	if nsname == "" {
		glog.V(100).Infof("packagemanifest 'nsname' parameter can not be empty")

		return nil, fmt.Errorf("failed to list packagemanifests, 'nsname' parameter is empty")
	}

	if apiClient == nil {
		glog.V(100).Infof("The apiClient cannot be nil")

		return nil, fmt.Errorf("failed to list packageManifest, 'apiClient' parameter is empty")
	}

	err := apiClient.AttachScheme(operatorv1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add packageManifest scheme to client schemes")

		return nil, err
	}

	passedOptions := client.ListOptions{}
	logMessage := fmt.Sprintf("Listing PackageManifests in the namespace %s", nsname)

	if len(options) > 1 {
		glog.V(100).Infof("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	glog.V(100).Infof(logMessage)

	pkgManifestList := new(operatorv1.PackageManifestList)

	err = apiClient.List(context.TODO(), pkgManifestList, &passedOptions)
	if err != nil {
		glog.V(100).Infof("Failed to list PackageManifests in the namespace %s due to %s",
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
