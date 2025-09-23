package olm

import (
	"context"
	"fmt"

	oplmV1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/olm/operators/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
)

// ListInstallPlan returns a list of installplans found for specific namespace.
func ListInstallPlan(
	apiClient *clients.Settings, nsname string, options ...client.ListOptions) ([]*InstallPlanBuilder, error) {
	if nsname == "" {
		glog.V(100).Info("The nsname of the installplan is empty")

		return nil, fmt.Errorf("the nsname of the installplan is empty")
	}

	if apiClient == nil {
		glog.V(100).Infof("The apiClient cannot be nil")

		return nil, fmt.Errorf("failed to list installPlan, 'apiClient' parameter is empty")
	}

	err := apiClient.AttachScheme(oplmV1alpha1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add oplmV1alpha1 scheme to client schemes")

		return nil, err
	}

	passedOptions := client.ListOptions{}
	logMessage := fmt.Sprintf("Listing InstallPlans in namespace %s", nsname)

	if len(options) > 1 {
		glog.V(100).Infof("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	glog.V(100).Infof(logMessage)

	installPlanList := new(oplmV1alpha1.InstallPlanList)

	err = apiClient.List(context.TODO(), installPlanList, &passedOptions)
	if err != nil {
		glog.V(100).Infof("Failed to list all installplan in namespace %s due to %s",
			nsname, err.Error())

		return nil, err
	}

	var installPlanObjects []*InstallPlanBuilder

	for _, foundCsv := range installPlanList.Items {
		copiedCsv := foundCsv
		csvBuilder := &InstallPlanBuilder{
			apiClient:  apiClient.Client,
			Object:     &copiedCsv,
			Definition: &copiedCsv,
		}

		installPlanObjects = append(installPlanObjects, csvBuilder)
	}

	if len(installPlanObjects) == 0 {
		return nil, fmt.Errorf("installplan not found in namespace %s", nsname)
	}

	return installPlanObjects, nil
}
