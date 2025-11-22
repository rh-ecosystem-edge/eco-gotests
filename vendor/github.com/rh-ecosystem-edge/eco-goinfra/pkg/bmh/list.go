package bmh

import (
	"context"
	"fmt"
	"time"

	goclient "sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	bmhv1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
)

const (
	fiveScds time.Duration = 5 * time.Second
)

// List returns bareMetalHosts inventory in the given namespace.
func List(apiClient *clients.Settings, nsname string, options ...goclient.ListOptions) ([]*BmhBuilder, error) {
	if apiClient == nil || apiClient.Client == nil {
		klog.V(100).Info("BareMetalHosts 'apiClient' parameter can not be empty")

		return nil, fmt.Errorf("failed to list bareMetalHosts, 'apiClient' parameter is empty")
	}

	if nsname == "" {
		klog.V(100).Info("bareMetalHost 'nsname' parameter can not be empty")

		return nil, fmt.Errorf("failed to list bareMetalHosts, 'nsname' parameter is empty")
	}

	logMessage := fmt.Sprintf("Listing bareMetalHosts in the namespace %s", nsname)
	passedOptions := goclient.ListOptions{}

	if len(options) > 1 {
		klog.V(100).Info("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	passedOptions.Namespace = nsname

	klog.V(100).Infof("%v", logMessage)

	return list(apiClient, passedOptions)
}

// ListInAllNamespaces lists the BareMetalHosts across all namespaces on the provided cluster.
func ListInAllNamespaces(apiClient *clients.Settings, options ...goclient.ListOptions) ([]*BmhBuilder, error) {
	if apiClient == nil || apiClient.Client == nil {
		klog.V(100).Info("BareMetalHost's 'apiClient' parameter cannot be empty")

		return nil, fmt.Errorf("failed to list bareMetalHosts, 'apiClient' parameter is empty")
	}

	logMessage := "Listing bareMetalHosts in all namespaces"
	passedOptions := goclient.ListOptions{}

	if len(options) > 1 {
		klog.V(100).Info("'options' parameter must be empty or single-valued")

		return nil, fmt.Errorf("error: more than one ListOptions was passed")
	}

	if len(options) == 1 {
		passedOptions = options[0]
		logMessage += fmt.Sprintf(" with the options %v", passedOptions)
	}

	klog.V(100).Info(logMessage)

	return list(apiClient, passedOptions)
}

// WaitForAllBareMetalHostsInGoodOperationalState waits for all baremetalhosts to be in good Operational State
// for a time duration up to the timeout.
func WaitForAllBareMetalHostsInGoodOperationalState(apiClient *clients.Settings,
	nsname string,
	timeout time.Duration,
	options ...goclient.ListOptions) (bool, error) {
	klog.V(100).Infof("Waiting for all bareMetalHosts in %s namespace to have OK operationalStatus",
		nsname)

	bmhList, err := List(apiClient, nsname, options...)
	if err != nil {
		klog.V(100).Infof("Failed to list all bareMetalHosts in the %s namespace due to %s",
			nsname, err.Error())

		return false, err
	}

	// Wait 5 secs in each iteration before condition function () returns true or errors or times out
	// after availableDuration
	err = wait.PollUntilContextTimeout(
		context.TODO(), fiveScds, timeout, true, func(ctx context.Context) (bool, error) {
			for _, baremetalhost := range bmhList {
				status := baremetalhost.GetBmhOperationalState()

				if status != bmhv1alpha1.OperationalStatusOK {
					klog.V(100).Infof("The %s bareMetalHost in namespace %s has an unexpected operational status: %s",
						baremetalhost.Object.Name, baremetalhost.Object.Namespace, status)

					return false, nil
				}
			}

			return true, nil
		})
	if err == nil {
		klog.V(100).Infof("All baremetalhosts were found in the good Operational State "+
			"during defined timeout: %v", timeout)

		return true, nil
	}

	// Here err is "timed out waiting for the condition"
	klog.V(100).Infof("Not all baremetalhosts were found in the good Operational State "+
		"during defined timeout: %v", timeout)

	return false, err
}

// list lists the BareMetalHosts according to the provided options.
func list(apiClient *clients.Settings, options goclient.ListOptions) ([]*BmhBuilder, error) {
	err := apiClient.AttachScheme(bmhv1alpha1.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add bmhv1alpha1 scheme to client schemes")

		return nil, err
	}

	var bmhList bmhv1alpha1.BareMetalHostList

	err = apiClient.List(logging.DiscardContext(), &bmhList, &options)
	if err != nil {
		klog.V(100).Infof("Failed to list bareMetalHosts due to %s", err.Error())

		return nil, err
	}

	var bmhObjects []*BmhBuilder

	for _, baremetalhost := range bmhList.Items {
		copiedBmh := baremetalhost
		bmhBuilder := &BmhBuilder{
			apiClient:  apiClient.Client,
			Object:     &copiedBmh,
			Definition: &copiedBmh,
		}

		bmhObjects = append(bmhObjects, bmhBuilder)
	}

	return bmhObjects, nil
}
