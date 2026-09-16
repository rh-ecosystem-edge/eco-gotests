package o2imscluster

import (
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
	agentInstallV1Beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/api/v1beta1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/o2imstest"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
)

// VerifyNodeClusterTypeMatchesKey checks that an API NodeClusterType matches an expected type key.
func VerifyNodeClusterTypeMatchesKey(apiType oranapi.NodeClusterType, key NodeClusterTypeKey) error {
	type view struct {
		Name        string
		Description string
		Vendor      string
		Version     string
		Model       string
	}

	extensions := map[string]any{}
	if apiType.Extensions != nil {
		extensions = *apiType.Extensions
	}

	if diff := cmp.Diff(
		view{Name: key.Name(), Description: key.Name(), Vendor: key.Vendor, Version: key.Version, Model: key.Model},
		view{Name: apiType.Name, Description: apiType.Description,
			Vendor:  o2imstest.ExtensionString(extensions, tsparams.ClusterVendorLabel),
			Version: o2imstest.ExtensionString(extensions, tsparams.ClusterVersionExtension),
			Model:   o2imstest.ExtensionString(extensions, tsparams.ClusterModelExtension)},
	); diff != "" {
		return fmt.Errorf("node cluster type mismatch (-want +got):\n%s", diff)
	}

	return nil
}

// VerifyNodeClusterMatchesManagedCluster checks that an API NodeCluster matches a ManagedCluster.
func VerifyNodeClusterMatchesManagedCluster(
	apiCluster oranapi.NodeCluster,
	cluster *ocm.ManagedClusterBuilder,
	nodeClusterTypes []oranapi.NodeClusterType,
) error {
	var errs []error

	type view struct {
		NodeClusterID       uuid.UUID
		Name                string
		Description         string
		NodeClusterTypeName string
		Vendor              string
		OpenShiftVersion    string
		Model               string
	}

	expectedID, idErr := ManagedClusterID(cluster)
	errs = append(errs, idErr)

	key, keyErr := NodeClusterTypeKeyFromManagedCluster(cluster)
	errs = append(errs, keyErr)

	if cluster == nil || cluster.Definition == nil {
		return errors.Join(errs...)
	}

	expected := view{NodeClusterID: expectedID, Name: cluster.Definition.Name, Description: cluster.Definition.Name}
	got := view{NodeClusterID: apiCluster.NodeClusterId, Name: apiCluster.Name, Description: apiCluster.Description}

	typeIndex := slices.IndexFunc(nodeClusterTypes, func(nodeType oranapi.NodeClusterType) bool {
		return nodeType.NodeClusterTypeId == apiCluster.NodeClusterTypeId
	})
	if typeIndex == -1 {
		errs = append(errs, fmt.Errorf("nodeClusterTypeId %s not found in NodeClusterType list",
			apiCluster.NodeClusterTypeId))
	} else {
		expected.NodeClusterTypeName = key.Name()
		got.NodeClusterTypeName = nodeClusterTypes[typeIndex].Name
	}

	extensions := map[string]any{}
	if apiCluster.Extensions != nil {
		extensions = *apiCluster.Extensions
	}

	if cluster.Definition.Labels != nil {
		expected.Vendor = cluster.Definition.Labels[tsparams.ClusterVendorLabel]
		expected.OpenShiftVersion = cluster.Definition.Labels[tsparams.OpenshiftVersionLabel]
	}

	expected.Model = key.Model
	got.Vendor = o2imstest.ExtensionString(extensions, tsparams.ClusterVendorLabel)
	got.OpenShiftVersion = o2imstest.ExtensionString(extensions, tsparams.OpenshiftVersionLabel)
	got.Model = o2imstest.ExtensionString(extensions, tsparams.ClusterModelExtension)

	if diff := cmp.Diff(expected, got); diff != "" {
		errs = append(errs, fmt.Errorf("node cluster mismatch (-want +got):\n%s", diff))
	}

	return errors.Join(errs...)
}

// VerifyClusterResourceIDsExist reports an error when any clusterResourceId is missing from the
// listed ClusterResources.
func VerifyClusterResourceIDsExist(ids []uuid.UUID, resources []oranapi.ClusterResource) error {
	resourceIDs := make([]string, 0, len(resources))
	for _, resource := range resources {
		resourceIDs = append(resourceIDs, resource.ClusterResourceId.String())
	}

	var errs []error

	for _, id := range ids {
		if !slices.Contains(resourceIDs, id.String()) {
			errs = append(errs, fmt.Errorf("clusterResourceId %s not found in ClusterResource list", id))
		}
	}

	return errors.Join(errs...)
}

// VerifyClusterResourceTypeMatchesKey checks that an API ClusterResourceType matches an expected type key.
func VerifyClusterResourceTypeMatchesKey(apiType oranapi.ClusterResourceType, key ClusterResourceTypeKey) error {
	type view struct {
		Name        string
		Description string
	}

	if diff := cmp.Diff(view{Name: key.Name(), Description: key.Name()},
		view{Name: apiType.Name, Description: apiType.Description}); diff != "" {
		return fmt.Errorf("cluster resource type mismatch (-want +got):\n%s", diff)
	}

	return nil
}

// VerifyClusterResourceMatchesAgent checks that an API ClusterResource matches the corresponding Agent.
func VerifyClusterResourceMatchesAgent(
	apiResource oranapi.ClusterResource,
	agent *agentInstallV1Beta1.Agent,
	clusterResourceTypes []oranapi.ClusterResourceType,
) error {
	var errs []error

	type view struct {
		Name        string
		Description string
		ResourceID  uuid.UUID
		Role        string
		CPU         map[string]string
		MemoryGiB   string
	}

	expectedResourceID, idErr := ExpectedInventoryResourceID(agent)
	errs = append(errs, idErr)

	if idErr != nil {
		return errors.Join(errs...)
	}

	if apiResource.ClusterResourceTypeId == uuid.Nil {
		errs = append(errs, fmt.Errorf("clusterResourceTypeId: want non-nil UUID, got %s", apiResource.ClusterResourceTypeId))

		return errors.Join(errs...)
	}

	typeIndex := slices.IndexFunc(clusterResourceTypes, func(resourceType oranapi.ClusterResourceType) bool {
		return resourceType.ClusterResourceTypeId == apiResource.ClusterResourceTypeId
	})
	if typeIndex == -1 {
		errs = append(errs, fmt.Errorf("clusterResourceTypeId %s not found in ClusterResourceType list",
			apiResource.ClusterResourceTypeId))
	} else if typeErr := VerifyClusterResourceTypeMatchesKey(
		clusterResourceTypes[typeIndex], ClusterResourceTypeKeyFromAgent(agent)); typeErr != nil {
		errs = append(errs, fmt.Errorf("cluster resource type association mismatch: %w", typeErr))
	}

	extensions := map[string]any{}
	if apiResource.Extensions != nil {
		extensions = *apiResource.Extensions
	}

	expected := view{Name: agent.Spec.Hostname, Description: agent.Spec.Hostname,
		ResourceID: expectedResourceID, Role: string(agent.Status.Role),
		CPU: map[string]string{
			"architecture": agent.Status.Inventory.Cpu.Architecture,
			"cores":        strconv.FormatInt(agent.Status.Inventory.Cpu.Count, 10),
			"model":        agent.Status.Inventory.Cpu.ModelName,
		}, MemoryGiB: strconv.FormatInt(agent.Status.Inventory.Memory.PhysicalBytes/(1024*1024*1024), 10)}

	got := view{Name: apiResource.Name, Description: apiResource.Description, ResourceID: apiResource.ResourceId,
		Role: o2imstest.ExtensionString(extensions, "role")}

	if cpu, converted := o2imstest.AsStringKeyedMap(extensions["cpu"]); converted {
		got.CPU = map[string]string{
			"architecture": cpu["architecture"],
			"cores":        cpu["cores"],
			"model":        cpu["model"],
		}
	}

	if memory, converted := o2imstest.AsStringKeyedMap(extensions["memory"]); converted {
		got.MemoryGiB = memory["GiB"]
	}

	if diff := cmp.Diff(expected, got); diff != "" {
		errs = append(errs, fmt.Errorf("cluster resource mismatch (-want +got):\n%s", diff))
	}

	return errors.Join(errs...)
}
