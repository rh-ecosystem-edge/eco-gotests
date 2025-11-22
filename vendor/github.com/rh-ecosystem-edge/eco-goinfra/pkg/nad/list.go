package nad

import (
	"fmt"

	nadV1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// List returns NADs inventory in the given namespace.
func List(apiClient *clients.Settings, nsname string) ([]*Builder, error) {
	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("nadList 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(nadV1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add nad v1 scheme to client schemes")

		return nil, fmt.Errorf("failed to add nad v1 scheme to client schemes")
	}

	if nsname == "" {
		klog.V(100).Info("nad 'nsname' parameter can not be empty")

		return nil, fmt.Errorf("failed to list NADs, 'nsname' parameter is empty")
	}

	logMessage := fmt.Sprintf("Listing NADs in the nsname %s", nsname)

	klog.V(100).Infof("%v", logMessage)

	nadList := &nadV1.NetworkAttachmentDefinitionList{}

	err = apiClient.List(logging.DiscardContext(), nadList, &goclient.ListOptions{Namespace: nsname})
	if err != nil {
		klog.V(100).Infof("Failed to list NADs in namespace: %s due to %s",
			nsname, err.Error())

		return nil, err
	}

	var nadObjects []*Builder

	for _, nadObj := range nadList.Items {
		networkAttachmentDefinition := nadObj
		nadBuilder := &Builder{
			apiClient:  apiClient.Client,
			Definition: &networkAttachmentDefinition,
			Object:     &networkAttachmentDefinition,
		}

		nadObjects = append(nadObjects, nadBuilder)
	}

	return nadObjects, err
}
