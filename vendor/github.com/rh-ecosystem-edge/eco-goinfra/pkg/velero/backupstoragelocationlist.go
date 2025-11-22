package velero

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ListBackupStorageLocationBuilder returns backupstoragelocation inventory in the given namespace.
func ListBackupStorageLocationBuilder(
	apiClient *clients.Settings, nsname string, options ...client.ListOptions) ([]*BackupStorageLocationBuilder, error) {
	if apiClient == nil {
		klog.V(100).Info("The apiClient cannot be nil")

		return nil, fmt.Errorf("the apiClient is nil")
	}

	err := apiClient.AttachScheme(velerov1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add nmstate v1 scheme to client schemes")

		return nil, err
	}

	if nsname == "" {
		klog.V(100).Info("backupstoragelocation 'nsname' parameter can not be empty")

		return nil, fmt.Errorf("failed to list backupstoragelocations, 'nsname' parameter is empty")
	}

	logMessage := fmt.Sprintf("Listing backupstoragelocations in the nsname %s", nsname)
	passedOptions := client.ListOptions{}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	} else if len(options) > 1 {
		klog.V(100).Info("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	klog.V(100).Infof("%v", logMessage)

	bslList := &velerov1.BackupStorageLocationList{}

	err = apiClient.List(logging.DiscardContext(), bslList, &passedOptions)
	if err != nil {
		klog.V(100).Infof("Failed to list backupstoragelocations in the nsname %s due to %s", nsname, err.Error())

		return nil, err
	}

	var bslObjects []*BackupStorageLocationBuilder

	for _, runningBsl := range bslList.Items {
		copiedBsl := runningBsl
		bslBuilder := &BackupStorageLocationBuilder{
			apiClient:  apiClient.Client,
			Object:     &copiedBsl,
			Definition: &copiedBsl,
		}

		bslObjects = append(bslObjects, bslBuilder)
	}

	return bslObjects, nil
}
