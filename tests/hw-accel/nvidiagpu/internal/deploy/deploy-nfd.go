package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nfd"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/olm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/olm/operators/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/internal/hwaccelparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nvidiagpu/internal/get"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nvidiagpu/internal/gpuparams"
	nvidiagpuwait "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nvidiagpu/internal/wait"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const (
	nfdOperatorGroupName                        = "nfd-og"
	nfdSubscriptionName                         = "nfd-subscription"
	nfdSubscriptionNamespace                    = "openshift-nfd"
	nfdCatalogSource                            = "redhat-operators"
	nfdCatalogSourceNamespace                   = "openshift-marketplace"
	nfdOperatorDeploymentName                   = "nfd-controller-manager"
	nfdPackage                                  = "nfd"
	nfdChannel                                  = "stable"
	nfdInstallPlanApproval    v1alpha1.Approval = "Automatic"
	nfdCRDeploymentName                         = "nfd-master"
)

// CreateNFDNamespace creates and labels NFD namespace.
func CreateNFDNamespace(apiClient *clients.Settings) error {
	klog.V(gpuparams.GpuLogLevel).Infof("Check if NFD Operator namespace exists, otherwise created it")

	nfdNsBuilder := namespace.NewBuilder(apiClient, hwaccelparams.NFDNamespace)

	klog.V(gpuparams.GpuLogLevel).Infof("Creating the namespace:  %v", hwaccelparams.NFDNamespace)

	createdNfdNsBuilder, err := nfdNsBuilder.Create()
	if err != nil {
		klog.V(gpuparams.GpuLogLevel).Infof("error creating NFD namespace '%s' :  %v ",
			createdNfdNsBuilder.Definition.Name, err)

		return err
	}

	klog.V(gpuparams.GpuLogLevel).Infof("Successfully created NFD namespace '%s'",
		createdNfdNsBuilder.Object.Name)

	klog.V(gpuparams.GpuLogLevel).Infof("Labeling the newly created NFD namespace '%s'",
		nfdNsBuilder.Object.Name)

	labeledNfdNsBuilder := createdNfdNsBuilder.WithMultipleLabels(map[string]string{
		"openshift.io/cluster-monitoring":    "true",
		"pod-security.kubernetes.io/enforce": "privileged",
	})

	newLabeledNfdNsBuilder, err := labeledNfdNsBuilder.Update()
	if err != nil {
		klog.V(gpuparams.GpuLogLevel).Infof("error labeling NFD namespace %s :  %v ",
			newLabeledNfdNsBuilder.Definition.Name, err)

		return err
	}

	klog.V(gpuparams.GpuLogLevel).Infof("The NFD labeled namespace has "+
		"labels:  %v", newLabeledNfdNsBuilder.Object.Labels)

	return nil
}

// CreateNFDOperatorGroup creates NFD OperatorGroup in NFD namespace.
func CreateNFDOperatorGroup(apiClient *clients.Settings) error {
	klog.V(gpuparams.GpuLogLevel).Infof("Create the NFD operatorgroup")

	nfdOgBuilder := olm.NewOperatorGroupBuilder(apiClient, nfdOperatorGroupName, hwaccelparams.NFDNamespace)

	if nfdOgBuilder.Exists() {
		klog.V(gpuparams.GpuLogLevel).Infof("The nfdOgBuilder that exists has name:  %v",
			nfdOgBuilder.Object.Name)
	} else {
		klog.V(gpuparams.GpuLogLevel).Infof("Create a new NFD OperatorGroup with name:  %s",
			nfdOperatorGroupName)

		nfdOgBuilderCreated, err := nfdOgBuilder.Create()
		if err != nil {
			klog.V(gpuparams.GpuLogLevel).Infof("error creating NFD operatorgroup %v :  %v ",
				nfdOgBuilderCreated.Definition.Name, err)

			return err
		}
	}

	return nil
}

// CreateNFDSubscription creates NFD Subscription in NFD namespace.
func CreateNFDSubscription(apiClient *clients.Settings) error {
	klog.V(gpuparams.GpuLogLevel).Info("Create Subscription in NFD Operator Namespace")

	nfdSubBuilder := olm.NewSubscriptionBuilder(apiClient, nfdSubscriptionName, nfdSubscriptionNamespace,
		nfdCatalogSource, nfdCatalogSourceNamespace, nfdPackage)

	nfdSubBuilder.WithChannel(nfdChannel)
	nfdSubBuilder.WithInstallPlanApproval(nfdInstallPlanApproval)

	klog.V(gpuparams.GpuLogLevel).Infof("Creating the NFD subscription, i.e Deploy the NFD operator")

	createdNfdSub, err := nfdSubBuilder.Create()
	if err != nil {
		klog.V(gpuparams.GpuLogLevel).Infof("error creating NFD subscription %v :  %v ",
			createdNfdSub.Definition.Name, err)

		return err
	}

	if createdNfdSub.Exists() {
		klog.V(gpuparams.GpuLogLevel).Infof("Newly created NFD subscription: %s was successfully created",
			createdNfdSub.Object.Name)
		klog.V(gpuparams.GpuLogLevel).Infof("The newly created subscription: %s in namespace: %v "+
			"has current CSV:  %v", createdNfdSub.Object.Name, createdNfdSub.Object.Namespace,
			createdNfdSub.Object.Status.CurrentCSV)
	} else {
		return fmt.Errorf("could not determine the current CSV from newly created subscription: %s in"+
			" namespace %s", createdNfdSub.Object.Name, createdNfdSub.Object.Namespace)
	}

	return nil
}

// CheckNFDOperatorDeployed checks that NFD Operator is successfully deployed in NFD namespace.
func CheckNFDOperatorDeployed(apiClient *clients.Settings, waitTime time.Duration) (bool, error) {
	klog.V(gpuparams.GpuLogLevel).Infof("Check if the NFD operator deployment is ready")

	nfdOperatorDeployment, err := deployment.Pull(apiClient, nfdOperatorDeploymentName, hwaccelparams.NFDNamespace)
	if err != nil {
		klog.V(gpuparams.GpuLogLevel).Infof("Error trying to pull NFD operator "+
			"deployment is: %v", err)

		return false, err
	}

	klog.V(gpuparams.GpuLogLevel).Infof("Pulled NFD operator deployment is:  %v ",
		nfdOperatorDeployment.Definition.Name)

	if nfdOperatorDeployment.IsReady(waitTime) {
		klog.V(gpuparams.GpuLogLevel).Infof("Pulled NFD operator deployment is:  %v is Ready ",
			nfdOperatorDeployment.Definition.Name)
	} else {
		return false, fmt.Errorf("NFD operator deployment:  %v is still not Ready "+
			"after waiting %v time duration", nfdOperatorDeployment.Definition.Name, waitTime)
	}

	klog.V(gpuparams.GpuLogLevel).Infof("Get currentCSV from NFD subscription")

	nfdCurrentCSVFromSub, err := get.CurrentCSVFromSubscription(apiClient, nfdSubscriptionName,
		nfdSubscriptionNamespace)
	if err != nil {
		klog.V(gpuparams.GpuLogLevel).Infof("error pulling NFD currentCSV from cluster:  %v", err)

		return false, err
	}

	if nfdCurrentCSVFromSub == "" {
		klog.V(gpuparams.GpuLogLevel).Infof("NFD currentCSV from subscription is null:  %s",
			nfdCurrentCSVFromSub)

		return false, err
	}

	klog.V(gpuparams.GpuLogLevel).Infof("currentCSV %s extracted from NFD Subscription %s",
		nfdCurrentCSVFromSub, nfdSubscriptionName)

	klog.V(gpuparams.GpuLogLevel).Infof("Wait for NFD ClusterServiceVersion to be in " +
		"Succeeded phase")
	klog.V(gpuparams.GpuLogLevel).Infof("Waiting for NFD ClusterServiceVersion to be Succeeded phase")

	err = nvidiagpuwait.CSVSucceeded(
		apiClient, nfdCurrentCSVFromSub, hwaccelparams.NFDNamespace, 60*time.Second, 5*time.Minute)

	klog.V(gpuparams.GpuLogLevel).Infof("error waiting for NFD ClusterServiceVersion to be "+
		"in Succeeded phase:  %v ", err)

	if err != nil {
		klog.V(gpuparams.GpuLogLevel).Infof("error waiting for NFD ClusterServiceVersion"+
			" to be in Succeeded phase:  %v ", err)

		return false, err
	}

	klog.V(gpuparams.GpuLogLevel).Infof("Pull existing CSV in NFD Operator Namespace")

	clusterNfdCSV, err := olm.PullClusterServiceVersion(apiClient, nfdCurrentCSVFromSub, hwaccelparams.NFDNamespace)
	if err != nil {
		klog.V(gpuparams.GpuLogLevel).Infof("error pulling CSV %v from cluster:  %v",
			nfdCurrentCSVFromSub, err)

		return false, err
	}

	klog.V(gpuparams.GpuLogLevel).Infof("NFD clusterCSV from cluster lastUpdatedTime is : %v ",
		clusterNfdCSV.Definition.Status.LastUpdateTime)

	klog.V(gpuparams.GpuLogLevel).Infof("clusterCSV from cluster Phase is : \"%v\"",
		clusterNfdCSV.Definition.Status.Phase)

	succeeded := v1alpha1.ClusterServiceVersionPhase("Succeeded")

	if clusterNfdCSV.Definition.Status.Phase != succeeded {
		klog.V(gpuparams.GpuLogLevel).Infof("CSV Phase is not succeeded")

		return false, fmt.Errorf("CSV Phase is not 'succeeded'")
	}

	return true, nil
}

// DeployCRInstance deploys NodeFeatureDiscovery instance from current CSV almExamples.
func DeployCRInstance(apiClient *clients.Settings) error {
	klog.V(gpuparams.GpuLogLevel).Infof("Get ALM examples block form NFD CSV")
	klog.V(gpuparams.GpuLogLevel).Infof("Get currentCSV from NFD subscription")

	nfdCurrentCSVFromSub, err := get.CurrentCSVFromSubscription(apiClient, nfdSubscriptionName,
		nfdSubscriptionNamespace)
	if err != nil {
		klog.V(gpuparams.GpuLogLevel).Infof("Error from getting CurrentCSVFromSubscription:  %v ", err)

		return err
	}

	klog.V(gpuparams.GpuLogLevel).Infof("Pull existing CSV in NFD Operator Namespace")

	clusterNfdCSV, err := olm.PullClusterServiceVersion(apiClient, nfdCurrentCSVFromSub, hwaccelparams.NFDNamespace)
	if err != nil {
		klog.V(gpuparams.GpuLogLevel).Infof("Error from PullClusterServiceVersion:  %v ", err)

		return err
	}

	almExamples, err := clusterNfdCSV.GetAlmExamples()
	if err != nil {
		klog.V(gpuparams.GpuLogLevel).Infof("Error from pulling almExamples from NFD CSV:  %v ", err)

		return err
	}

	klog.V(gpuparams.GpuLogLevel).Infof("almExamples block from cluster NFD CSV is : %v ", almExamples)

	klog.V(gpuparams.GpuLogLevel).Infof("Creating NodeFeatureDiscovery instance from CSV almExamples")

	nodeFeatureDiscoveryBuilder := nfd.NewBuilderFromObjectString(apiClient, almExamples)

	_, err = nodeFeatureDiscoveryBuilder.Create()
	if err != nil {
		klog.V(gpuparams.GpuLogLevel).Infof("Error Creating NodeFeatureDiscovery instance from CSV "+
			"almExamples  %v ", err)

		return err
	}

	klog.V(gpuparams.GpuLogLevel).Infof("Waiting for NFD CR deployment '%s' to be created", nfdCRDeploymentName)

	nfdCRDeploymentCreated := nvidiagpuwait.DeploymentCreated(apiClient, nfdCRDeploymentName, hwaccelparams.NFDNamespace,
		30*time.Second, 4*time.Minute)

	if !nfdCRDeploymentCreated {
		klog.V(gpuparams.GpuLogLevel).Infof("timed out waiting to deploy NFD CR deployment")

		return fmt.Errorf("timed out waiting to deploy NFD CR deployment")
	}

	klog.V(gpuparams.GpuLogLevel).Infof("Check if the NFD CR deployment is ready")

	nfdCRDeployment, err := deployment.Pull(apiClient, nfdCRDeploymentName, hwaccelparams.NFDNamespace)
	if err != nil {
		klog.V(gpuparams.GpuLogLevel).Infof("Error pulling NFD CR deployment  %v ", err)

		return err
	}

	klog.V(gpuparams.GpuLogLevel).Infof("Pulled NFD CR deployment is:  %v ",
		nfdCRDeployment.Definition.Name)

	if nfdCRDeployment.IsReady(180 * time.Second) {
		klog.V(gpuparams.GpuLogLevel).Infof("Pulled NFD operator deployment is:  %v is Ready ",
			nfdCRDeployment.Definition.Name)
	} else {
		return fmt.Errorf("NFD CR deployment is not ready after wait period")
	}

	return nil
}

// GetNFDCRJson outputs the NFD CR instance json file.
func GetNFDCRJson(apiClient *clients.Settings, nfdCRName string, nfdNamespace string) error {
	klog.V(gpuparams.GpuLogLevel).Infof("Pull the NodeFeatureDiscovery just created from cluster, " +
		"with updated fields")

	pulledNodeFeatureDiscovery, err := nfd.Pull(apiClient, nfdCRName, nfdNamespace)
	if err != nil {
		klog.V(gpuparams.GpuLogLevel).Infof("error pulling NodeFeatureDiscovery %s from "+
			"cluster: %v ", nfdCRName, err)

		return err
	}

	nfdCRJson, err := json.MarshalIndent(pulledNodeFeatureDiscovery, "", " ")
	if err == nil {
		klog.V(gpuparams.GpuLogLevel).Infof("The NodeFeatureDiscovery just created has name:  %v",
			pulledNodeFeatureDiscovery.Definition.Name)
		klog.V(gpuparams.GpuLogLevel).Infof("The NodeFeatureDiscovery just created marshalled "+
			"in json: %v", string(nfdCRJson))
	} else {
		klog.V(gpuparams.GpuLogLevel).Infof("Error Marshalling NodeFeatureDiscovery into json:  %v",
			err)
	}

	return nil
}

// NFDCRDeleteAndWait deletes NodeFeatureDiscovery instance and waits until it is deleted.
func NFDCRDeleteAndWait(apiClient *clients.Settings, nfdCRName string, nfdCRNamespace string, pollInterval,
	timeout time.Duration) error {
	// return wait.PollImmediate(pollInterval, timeout, func() (bool, error) {
	return wait.PollUntilContextTimeout(
		context.TODO(), pollInterval, timeout, false, func(ctx context.Context) (bool, error) {
			nfdCR, err := nfd.Pull(apiClient, nfdCRName, nfdCRNamespace)
			if err != nil {
				klog.V(gpuparams.GpuLogLevel).Infof("NodeFeatureDiscovery pull from cluster error: %s\n", err)

				return false, err
			}

			_, err = nfdCR.Delete()
			if err != nil {
				return false, err
			}

			if !nfdCR.Exists() {
				klog.V(gpuparams.GpuLogLevel).Infof("NodeFeatureDiscovery instance '%s' in namespace '%s' does "+
					"not exist", nfdCRName, nfdCRNamespace)

				// this exists out of the wait.PollImmediate()
				return true, nil
			}

			klog.V(gpuparams.GpuLogLevel).Infof("NodeFeatureDiscovery instance %s in namespace %s still exists",
				nfdCR.Object.Name, nfdCR.Object.Namespace)

			return false, err
		})
}

// DeleteNFDNamespace creates and labels NFD namespace.
func DeleteNFDNamespace(apiClient *clients.Settings) error {
	klog.V(gpuparams.GpuLogLevel).Infof("Deleting NFD namespace '%s'", hwaccelparams.NFDNamespace)

	pulledNFDNsBuilder, err := namespace.Pull(apiClient, hwaccelparams.NFDNamespace)
	if err != nil {
		klog.V(gpuparams.GpuLogLevel).Infof("error pulling NFD namespace '%s' :  %v ",
			hwaccelparams.NFDNamespace, err)

		return err
	}

	err = pulledNFDNsBuilder.Delete()

	return err
}

// DeleteNFDOperatorGroup creates NFD OperatorGroup in NFD namespace.
func DeleteNFDOperatorGroup(apiClient *clients.Settings) error {
	klog.V(gpuparams.GpuLogLevel).Infof("Deleting NFD OperatorGroup '%s' in namespace '%s'",
		nfdOperatorGroupName, hwaccelparams.NFDNamespace)

	pulledNFDOg, err := olm.PullOperatorGroup(apiClient, nfdOperatorGroupName, hwaccelparams.NFDNamespace)

	if !pulledNFDOg.Exists() {
		klog.V(gpuparams.GpuLogLevel).Infof("The NFD OperatorGroup %s does not exist", nfdOperatorGroupName)

		return err
	}

	err = pulledNFDOg.Delete()

	return err
}

// DeleteNFDSubscription Deletes NFD Subscription in NFD namespace.
func DeleteNFDSubscription(apiClient *clients.Settings) error {
	klog.V(gpuparams.GpuLogLevel).Infof("Deleting NFD Subscription '%s' in namespace '%s'",
		nfdSubscriptionName, hwaccelparams.NFDNamespace)

	pulledNFDSub, err := olm.PullSubscription(apiClient, nfdSubscriptionName, nfdSubscriptionNamespace)

	if !pulledNFDSub.Exists() {
		klog.V(gpuparams.GpuLogLevel).Infof("The NFD Subscription %s does not exist", nfdOperatorGroupName)

		return err
	}

	err = pulledNFDSub.Delete()

	return err
}

// DeleteNFDCSV Deletes NFD CSV in NFD namespace.
func DeleteNFDCSV(apiClient *clients.Settings) error {
	klog.V(gpuparams.GpuLogLevel).Infof("Deleting currently installed NFD CSV")

	nfdCurrentCSVFromSub, err := get.CurrentCSVFromSubscription(apiClient, nfdSubscriptionName,
		nfdSubscriptionNamespace)
	if err != nil {
		return fmt.Errorf("error trying to get current NFD CSV from subscription '%w'", err)
	}

	if nfdCurrentCSVFromSub == "" {
		return fmt.Errorf("current NFD CSV name is empty string '%s'", nfdCurrentCSVFromSub)
	}

	clusterNfdCSV, err := olm.PullClusterServiceVersion(apiClient, nfdCurrentCSVFromSub, hwaccelparams.NFDNamespace)
	if err != nil {
		return fmt.Errorf("error pulling CSV %v from cluster:  %w", nfdCurrentCSVFromSub, err)
	}

	err = clusterNfdCSV.Delete()

	return err
}
