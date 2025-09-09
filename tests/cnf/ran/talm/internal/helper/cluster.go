package helper

import (
	configv1 "github.com/openshift/api/config/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clusterversion"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetClusterVersionDefinition returns a ClusterVersion definition based on the client.
//
// Use "Image" to include only DesiredUpdate.Image retrieved from the provided client. Use "Version" to include only
// DesiredUpdate.Version retrieved from the provided client. Use "Both" to include both DesiredUpdate.Image and
// DesiredUpdate.Version retrieved from the provided client.
func GetClusterVersionDefinition(client *clients.Settings, config string) (*configv1.ClusterVersion, error) {
	clusterVersion := &configv1.ClusterVersion{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterVersion",
			APIVersion: "config.openshift.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "version",
		},
	}

	currentVersion, err := clusterversion.Pull(client)
	if err != nil {
		return nil, err
	}

	var desiredUpdate configv1.Update

	// channel and upstream specs are required when desiredUpdate.version is used
	if config != "Image" {
		version, err := version.GetOCPVersion(client)
		if err != nil {
			return nil, err
		}

		desiredUpdate.Version = version
		clusterVersion.Spec.Upstream = configv1.URL(RANConfig.OcpUpgradeUpstreamURL)
		clusterVersion.Spec.Channel = currentVersion.Definition.Spec.Channel
	}

	// no other specs are needed when desiredUpdate.image is used, but usually force upgrade is used in combination
	// when upgrade path is unavailable in upgrade graph.
	if config != "Version" {
		desiredUpdate.Image = currentVersion.Object.Status.Desired.Image
		desiredUpdate.Force = true
	}

	// Add composed desiredUpdate to cluster version spec.
	clusterVersion.Spec.DesiredUpdate = &desiredUpdate

	return clusterVersion, nil
}

// DeleteClusterLabel deletes a label from and updates the specified cluster.
func DeleteClusterLabel(client *clients.Settings, clusterName string, labelToBeDeleted string) error {
	managedCluster, err := ocm.PullManagedCluster(client, clusterName)
	if err != nil {
		return err
	}

	delete(managedCluster.Object.Labels, labelToBeDeleted)

	managedCluster.Definition = managedCluster.Object
	_, err = managedCluster.Update()

	return err
}

// DoesClusterLabelExist looks for a label on a managed cluster and returns true if it exists.
func DoesClusterLabelExist(client *clients.Settings, clusterName string, label string) (bool, error) {
	managedCluster, err := ocm.PullManagedCluster(client, clusterName)
	if err != nil {
		return false, err
	}

	_, exists := managedCluster.Object.Labels[label]

	return exists, nil
}
