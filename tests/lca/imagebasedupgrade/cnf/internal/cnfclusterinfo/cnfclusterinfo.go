package cnfclusterinfo

import (
	"errors"
	"strings"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/olm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/statefulset"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfparams"
	"k8s.io/klog/v2"
	"k8s.io/utils/strings/slices"
)

var (
	// PreUpgradeClusterInfo holds the cluster info pre upgrade.
	PreUpgradeClusterInfo = ClusterStruct{}

	// PostUpgradeClusterInfo holds the cluster info post upgrade.
	PostUpgradeClusterInfo = ClusterStruct{}
)

// WorkloadObjects is a struct that holds the workload objects.
type WorkloadObjects struct {
	Deployment  []string
	StatefulSet []string
}

// WorkloadStruct is a struct that holds the workload info.
type WorkloadStruct struct {
	Namespace string
	Objects   WorkloadObjects
}

// WorkloadPV struct holds the information to test that persistent volume content is not lost during upgrade.
type WorkloadPV struct {
	Namespace string
	PodName   string
	FilePath  string
	Digest    string
}

// ClusterStruct is a struct that holds the cluster info pre and post upgrade.
type ClusterStruct struct {
	Version                  string
	ID                       string
	Name                     string
	Operators                []string
	NodeName                 string
	SriovNetworks            []string
	SriovNetworkNodePolicies []string
	WorkloadResources        []WorkloadStruct
	WorkloadPVs              WorkloadPV
}

// SaveClusterInfo is a dedicated func to save cluster info.
//
//nolint:funlen
func (upgradeVar *ClusterStruct) SaveClusterInfo() error {
	clusterVersion, err := cluster.GetOCPClusterVersion(cnfinittools.TargetSNOAPIClient)
	if err != nil {
		klog.V(cnfparams.CNFLogLevel).Infof("Could not retrieve cluster version")

		return err
	}

	targetSnoClusterName, err := cluster.GetOCPClusterName(cnfinittools.TargetSNOAPIClient)
	if err != nil {
		klog.V(cnfparams.CNFLogLevel).Infof("Could not retrieve target sno cluster name")

		return err
	}

	csvList, err := olm.ListClusterServiceVersionInAllNamespaces(cnfinittools.TargetSNOAPIClient)
	if err != nil {
		klog.V(cnfparams.CNFLogLevel).Infof("Could not retrieve csv list")

		return err
	}

	var installedCSV []string

	for _, csv := range csvList {
		if !slices.Contains(installedCSV, csv.Object.Name) {
			if !strings.Contains(csv.Object.Name, "oadp-operator") &&
				!strings.Contains(csv.Object.Name, "packageserver") &&
				!strings.Contains(csv.Object.Name, "sriov-fec") {
				installedCSV = append(installedCSV, csv.Object.Name)
			}
		}
	}

	node, err := nodes.List(cnfinittools.TargetSNOAPIClient)
	if err != nil {
		klog.V(cnfparams.CNFLogLevel).Infof("Could not retrieve node list")

		return err
	}

	if len(node) == 0 {
		return errors.New("node list is empty")
	}

	sriovNet, err := sriov.List(cnfinittools.TargetSNOAPIClient, cnfinittools.CNFConfig.SriovOperatorNamespace)
	if err != nil {
		return err
	}

	for _, net := range sriovNet {
		upgradeVar.SriovNetworks = append(upgradeVar.SriovNetworks, net.Object.Name)
	}

	sriovPolicy, err := sriov.ListPolicy(cnfinittools.TargetSNOAPIClient, cnfinittools.CNFConfig.SriovOperatorNamespace)

	for _, policy := range sriovPolicy {
		if !strings.Contains(policy.Object.Name, "default") {
			upgradeVar.SriovNetworkNodePolicies = append(upgradeVar.SriovNetworkNodePolicies, policy.Object.Name)
		}
	}

	upgradeVar.Version = clusterVersion.Object.Status.Desired.Version
	upgradeVar.ID = string(clusterVersion.Object.Spec.ClusterID)
	upgradeVar.Name = targetSnoClusterName
	upgradeVar.Operators = installedCSV
	upgradeVar.NodeName = node[0].Object.Name

	_ = upgradeVar.getWorkloadInfo()

	if err != nil {
		return err
	}

	return nil
}

func (upgradeVar *ClusterStruct) getWorkloadInfo() error {
	for _, workloadNS := range strings.Split(cnfinittools.CNFConfig.IbuWorkloadNS, ",") {
		workloadMap := WorkloadStruct{
			Namespace: workloadNS,
			Objects: WorkloadObjects{
				Deployment:  []string{},
				StatefulSet: []string{},
			},
		}

		deployments, err := deployment.List(cnfinittools.TargetSNOAPIClient, workloadNS)
		if err != nil {
			return err
		}

		for _, deployment := range deployments {
			workloadMap.Objects.Deployment = append(workloadMap.Objects.Deployment, deployment.Definition.Name)
		}

		statefulsets, err := statefulset.List(cnfinittools.TargetSNOAPIClient, workloadNS)
		if err != nil {
			return err
		}

		for _, statefulset := range statefulsets {
			workloadMap.Objects.StatefulSet = append(workloadMap.Objects.StatefulSet, statefulset.Definition.Name)
		}

		upgradeVar.WorkloadResources = append(upgradeVar.WorkloadResources, workloadMap)
	}

	workloadPod, err := pod.Pull(
		cnfinittools.TargetSNOAPIClient,
		cnfinittools.CNFConfig.IbuWorkloadPVPod,
		cnfinittools.CNFConfig.IbuWorkloadPVNS,
	)
	if err != nil {
		return err
	}

	cmd := []string{"bash", "-c", "md5sum " + cnfinittools.CNFConfig.IbuWorkloadPVFilePath + " ||true"}

	getDigest, err := workloadPod.ExecCommand(cmd)
	if err != nil {
		return err
	}

	upgradeVar.WorkloadPVs.Namespace = cnfinittools.CNFConfig.IbuWorkloadPVNS
	upgradeVar.WorkloadPVs.PodName = cnfinittools.CNFConfig.IbuWorkloadPVPod
	upgradeVar.WorkloadPVs.FilePath = cnfinittools.CNFConfig.IbuWorkloadPVFilePath
	upgradeVar.WorkloadPVs.Digest = getDigest.String()

	return nil
}
