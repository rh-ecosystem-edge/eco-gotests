//go:build unit_test

package o2imscluster

import (
	"testing"

	"github.com/google/uuid"
	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
	agentInstallV1Beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/api/v1beta1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestExpectedInventoryResourceIDRequiresHardwareManagerNodeID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		labels map[string]string
	}{
		{
			name: "nil labels",
		},
		{
			name:   "other labels",
			labels: map[string]string{"some-label": "some-value"},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			agent := &agentInstallV1Beta1.Agent{
				Namespace: "test-namespace",
				Name:      "test-agent",
				Labels:    testCase.labels,
			}

			_, err := ExpectedInventoryResourceID(agent)

			assert.EqualError(t, err,
				"agent test-namespace/test-agent is missing "+tsparams.HardwareManagerNodeIDLabel+" label")
		})
	}
}

func TestClusterResourceTypeKeyFromAgent(t *testing.T) {
	t.Parallel()

	agent := testAgent("node-1", uuid.New().String(), "x86_64", 96)

	assert.Equal(t, ClusterResourceTypeKey{Architecture: "x86_64", Cores: 96},
		ClusterResourceTypeKeyFromAgent(agent))
}

func TestFindClusterResourceForAgent(t *testing.T) {
	t.Parallel()

	agentResourceID := uuid.New()
	agent := testAgent("node-1", agentResourceID.String(), "x86_64", 96)
	wrongResourceID := uuid.New()

	tests := []struct {
		name          string
		resources     []oranapi.ClusterResource
		agent         *agentInstallV1Beta1.Agent
		wantResource  oranapi.ClusterResource
		wantErrorText string
	}{
		{
			name: "match",
			resources: []oranapi.ClusterResource{
				{
					Name:       "different-name",
					ResourceId: agentResourceID,
					Extensions: clusterResourceCPU("arm64", 32),
				},
			},
			wantResource: oranapi.ClusterResource{
				Name:       "different-name",
				ResourceId: agentResourceID,
				Extensions: clusterResourceCPU("arm64", 32),
			},
		},
		{
			name: "missing label",
			agent: &agentInstallV1Beta1.Agent{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-namespace", Name: "test-agent"},
			},
			wantErrorText: "missing " + tsparams.HardwareManagerNodeIDLabel + " label",
		},
		{
			name: "resource ID miss",
			resources: []oranapi.ClusterResource{
				{
					Name:       "node-1",
					ResourceId: wrongResourceID,
					Extensions: clusterResourceCPU("x86_64", 96),
				},
			},
			wantErrorText: "no ClusterResource matched",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			testAgent := agent
			if testCase.agent != nil {
				testAgent = testCase.agent
			}

			resource, err := FindClusterResourceForAgent(testCase.resources, testAgent)

			if testCase.wantErrorText != "" {
				assert.ErrorContains(t, err, testCase.wantErrorText)

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, testCase.wantResource, resource)
		})
	}
}

func TestNodeClusterTypeKeyName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		key      NodeClusterTypeKey
		expected string
	}{
		{
			name:     "managed cluster",
			key:      NodeClusterTypeKey{Model: tsparams.ClusterModelManagedCluster, Vendor: "redhat", Version: "4.16"},
			expected: "managed-cluster-redhat-4.16",
		},
		{
			name:     "hub cluster",
			key:      NodeClusterTypeKey{Model: tsparams.ClusterModelHubCluster, Vendor: "redhat", Version: "4.17"},
			expected: "hub-cluster-redhat-4.17",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, testCase.expected, testCase.key.Name())
		})
	}
}

func TestClusterResourceTypeKeyName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		key      ClusterResourceTypeKey
		expected string
	}{
		{
			name:     "uppercase architecture",
			key:      ClusterResourceTypeKey{Architecture: "x86_64", Cores: 96},
			expected: "X86_64 CPU with 96 Cores",
		},
		{
			name:     "mixed case architecture",
			key:      ClusterResourceTypeKey{Architecture: "Arm64", Cores: 32},
			expected: "ARM64 CPU with 32 Cores",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, testCase.expected, testCase.key.Name())
		})
	}
}

// clusterResourceCPU returns the extension shape produced for a ClusterResource CPU.
func clusterResourceCPU(architecture string, cores int64) *map[string]any {
	extensions := map[string]any{
		"cpu": map[string]any{
			"architecture": architecture,
			"cores":        cores,
		},
	}

	return &extensions
}

// testAgent returns an Agent with the inventory and identity fields used by discovery.
func testAgent(hostname, hardwareManagerNodeID, architecture string, cores int64) *agentInstallV1Beta1.Agent {
	return &agentInstallV1Beta1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-namespace",
			Name:      "test-agent",
			Labels: map[string]string{
				tsparams.HardwareManagerNodeIDLabel: hardwareManagerNodeID,
			},
		},
		Spec: agentInstallV1Beta1.AgentSpec{Hostname: hostname},
		Status: agentInstallV1Beta1.AgentStatus{
			Inventory: agentInstallV1Beta1.HostInventory{
				Cpu: agentInstallV1Beta1.HostCPU{Architecture: architecture, Count: cores},
			},
		},
	}
}
