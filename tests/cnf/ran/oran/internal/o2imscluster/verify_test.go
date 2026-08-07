//go:build unit_test

package o2imscluster

import (
	"strconv"
	"testing"

	"github.com/google/uuid"
	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
	agentInstallV1Beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/api/v1beta1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestVerifyNodeClusterTypeMatchesKey(t *testing.T) {
	t.Parallel()

	key := NodeClusterTypeKey{Model: tsparams.ClusterModelHubCluster, Vendor: "redhat", Version: "4.16"}
	matchingExtensions := map[string]any{
		tsparams.ClusterVendorLabel:      "redhat",
		tsparams.ClusterVersionExtension: "4.16",
		tsparams.ClusterModelExtension:   tsparams.ClusterModelHubCluster,
	}

	tests := []struct {
		name    string
		apiType oranapi.NodeClusterType
		wantErr bool
	}{
		{
			name: "matching type",
			apiType: oranapi.NodeClusterType{
				Name:        key.Name(),
				Description: key.Name(),
				Extensions:  &matchingExtensions,
			},
		},
		{
			name: "nil extensions",
			apiType: oranapi.NodeClusterType{
				Name:        key.Name(),
				Description: key.Name(),
			},
			wantErr: true,
		},
		{
			name: "wrong name",
			apiType: oranapi.NodeClusterType{
				Name:        "wrong",
				Description: key.Name(),
				Extensions:  &matchingExtensions,
			},
			wantErr: true,
		},
		{
			name: "wrong vendor",
			apiType: oranapi.NodeClusterType{
				Name:        key.Name(),
				Description: key.Name(),
				Extensions: &map[string]any{
					tsparams.ClusterVendorLabel:      "wrong",
					tsparams.ClusterVersionExtension: "4.16",
					tsparams.ClusterModelExtension:   tsparams.ClusterModelHubCluster,
				},
			},
			wantErr: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := VerifyNodeClusterTypeMatchesKey(testCase.apiType, key)
			if testCase.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestVerifyClusterResourceTypeMatchesKey(t *testing.T) {
	t.Parallel()

	key := ClusterResourceTypeKey{Architecture: "x86_64", Cores: 96}

	tests := []struct {
		name    string
		apiType oranapi.ClusterResourceType
		wantErr bool
	}{
		{
			name:    "matching type",
			apiType: oranapi.ClusterResourceType{Name: key.Name(), Description: key.Name()},
		},
		{
			name:    "wrong name",
			apiType: oranapi.ClusterResourceType{Name: "wrong", Description: key.Name()},
			wantErr: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := VerifyClusterResourceTypeMatchesKey(testCase.apiType, key)
			if testCase.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestVerifyClusterResourceIDsExist(t *testing.T) {
	t.Parallel()

	resourceID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	resources := []oranapi.ClusterResource{{ClusterResourceId: resourceID}}

	tests := []struct {
		name      string
		ids       []uuid.UUID
		resources []oranapi.ClusterResource
		wantErr   bool
	}{
		{
			name:      "all present",
			ids:       []uuid.UUID{resourceID},
			resources: resources,
		},
		{
			name: "missing id",
			ids: []uuid.UUID{
				resourceID,
				uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			},
			resources: resources,
			wantErr:   true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := VerifyClusterResourceIDsExist(testCase.ids, testCase.resources)
			if testCase.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestVerifyClusterResourceMatchesAgent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		setup            func() (oranapi.ClusterResource, *agentInstallV1Beta1.Agent)
		wantError        string
		wantErrorContain string
	}{
		{
			name: "nil agent",
			setup: func() (oranapi.ClusterResource, *agentInstallV1Beta1.Agent) {
				return oranapi.ClusterResource{}, nil
			},
			wantError: "agent is nil",
		},
		{
			name: "matching resource",
			setup: func() (oranapi.ClusterResource, *agentInstallV1Beta1.Agent) {
				agent := verificationAgent("node-2")

				return testClusterResourceForAgent(agent), agent
			},
		},
		{
			name: "nil cluster resource type ID",
			setup: func() (oranapi.ClusterResource, *agentInstallV1Beta1.Agent) {
				agent := verificationAgent("node-3")
				resource := testClusterResourceForAgent(agent)
				resource.ClusterResourceTypeId = uuid.Nil

				return resource, agent
			},
			wantError: "clusterResourceTypeId: want non-nil UUID, got 00000000-0000-0000-0000-000000000000",
		},
		{
			name: "resource mismatch",
			setup: func() (oranapi.ClusterResource, *agentInstallV1Beta1.Agent) {
				agent := verificationAgent("node-4")
				resource := testClusterResourceForAgent(agent)
				resource.Name = "unexpected-name"

				return resource, agent
			},
			wantErrorContain: "cluster resource mismatch",
		},
		{
			name: "missing resource ID label",
			setup: func() (oranapi.ClusterResource, *agentInstallV1Beta1.Agent) {
				agent := verificationAgent("node-5")
				agent.Labels = nil

				return oranapi.ClusterResource{}, agent
			},
			wantError: "agent test-namespace/test-agent is missing " + tsparams.HardwareManagerNodeIDLabel + " label",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			resource, agent := testCase.setup()
			err := VerifyClusterResourceMatchesAgent(resource, agent)

			switch {
			case testCase.wantError != "":
				assert.EqualError(t, err, testCase.wantError)
			case testCase.wantErrorContain != "":
				assert.ErrorContains(t, err, testCase.wantErrorContain)
			default:
				assert.NoError(t, err)
			}
		})
	}
}

// verificationAgent returns an Agent fixture for cluster resource verification tests.
func verificationAgent(hostname string) *agentInstallV1Beta1.Agent {
	return &agentInstallV1Beta1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-namespace",
			Name:      "test-agent",
			Labels: map[string]string{
				tsparams.HardwareManagerNodeIDLabel: uuid.New().String(),
			},
		},
		Spec: agentInstallV1Beta1.AgentSpec{Hostname: hostname},
		Status: agentInstallV1Beta1.AgentStatus{
			Role: "worker",
			Inventory: agentInstallV1Beta1.HostInventory{
				Cpu: agentInstallV1Beta1.HostCPU{
					Architecture: "x86_64",
					Count:        96,
					ModelName:    "test-cpu",
				},
				Memory: agentInstallV1Beta1.HostMemory{
					PhysicalBytes: 8 * 1024 * 1024 * 1024,
				},
			},
		},
	}
}

// testClusterResourceForAgent returns a ClusterResource matching the agent's projected fields.
func testClusterResourceForAgent(agent *agentInstallV1Beta1.Agent) oranapi.ClusterResource {
	physicalMemoryGiB := agent.Status.Inventory.Memory.PhysicalBytes / (1024 * 1024 * 1024)
	extensions := map[string]any{
		"role": string(agent.Status.Role),
		"cpu": map[string]any{
			"architecture": agent.Status.Inventory.Cpu.Architecture,
			"cores":        agent.Status.Inventory.Cpu.Count,
			"model":        agent.Status.Inventory.Cpu.ModelName,
		},
		"memory": map[string]any{
			"GiB": strconv.FormatInt(physicalMemoryGiB, 10),
		},
	}

	return oranapi.ClusterResource{
		ClusterResourceTypeId: uuid.New(),
		Description:           agent.Spec.Hostname,
		Extensions:            &extensions,
		Name:                  agent.Spec.Hostname,
		ResourceId:            uuid.MustParse(agent.Labels[tsparams.HardwareManagerNodeIDLabel]),
	}
}
