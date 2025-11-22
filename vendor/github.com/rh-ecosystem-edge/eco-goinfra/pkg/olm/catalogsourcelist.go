package olm

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	oplmV1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/olm/operators/v1alpha1"
	"k8s.io/klog/v2"
)

// ListCatalogSources returns catalogsource inventory in the given namespace.
func ListCatalogSources(
	apiClient *clients.Settings,
	nsname string,
	options ...client.ListOptions) ([]*CatalogSourceBuilder, error) {
	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil, fmt.Errorf("failed to list catalogSource, 'apiClient' parameter is empty")
	}

	err := apiClient.AttachScheme(oplmV1alpha1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add oplmV1alpha1 scheme to client schemes")

		return nil, err
	}

	if nsname == "" {
		klog.V(100).Info("catalogsource 'nsname' parameter can not be empty")

		return nil, fmt.Errorf("failed to list catalogsource, 'nsname' parameter is empty")
	}

	passedOptions := client.ListOptions{}
	logMessage := fmt.Sprintf("Listing catalogsource in the namespace %s", nsname)

	if len(options) > 1 {
		klog.V(100).Info("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	klog.V(100).Infof("%v", logMessage)

	passedOptions.Namespace = nsname

	catalogSourceList := new(oplmV1alpha1.CatalogSourceList)

	err = apiClient.List(logging.DiscardContext(), catalogSourceList, &passedOptions)
	if err != nil {
		klog.V(100).Infof("Failed to list catalogsources in the namespace %s due to %s", nsname, err.Error())

		return nil, err
	}

	var catalogSourceObjects []*CatalogSourceBuilder

	for _, existingCatalogSource := range catalogSourceList.Items {
		copiedCatalogSource := existingCatalogSource
		catalogSourceBuilder := &CatalogSourceBuilder{
			apiClient:  apiClient.Client,
			Object:     &copiedCatalogSource,
			Definition: &copiedCatalogSource,
		}

		catalogSourceObjects = append(catalogSourceObjects, catalogSourceBuilder)
	}

	return catalogSourceObjects, nil
}
