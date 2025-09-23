package nad

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	nadV1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// List returns NADs inventory in the given namespace.
func List(apiClient *clients.Settings, nsname string) ([]*Builder, error) {
	if apiClient == nil {
		glog.V(100).Infof("The apiClient is empty")

		return nil, fmt.Errorf("nadList 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(nadV1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add nad v1 scheme to client schemes")

		return nil, fmt.Errorf("failed to add nad v1 scheme to client schemes")
	}

	if nsname == "" {
		glog.V(100).Infof("nad 'nsname' parameter can not be empty")

		return nil, fmt.Errorf("failed to list NADs, 'nsname' parameter is empty")
	}

	logMessage := fmt.Sprintf("Listing NADs in the nsname %s", nsname)

	glog.V(100).Infof(logMessage)

	nadList := &nadV1.NetworkAttachmentDefinitionList{}

	err = apiClient.List(context.TODO(), nadList, &goclient.ListOptions{Namespace: nsname})
	if err != nil {
		glog.V(100).Infof("Failed to list NADs in namespace: %s due to %s",
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
