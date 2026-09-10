package nicinfo

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// resetClusterNodesNICInfo clears the package-global state so tests are independent.
func resetClusterNodesNICInfo() {
	clusterNodesNICInfo = &sync.Map{}
}

func TestTestedInterfaceNames(t *testing.T) {
	testCases := []struct {
		name     string
		marks    map[string][]string
		expected string
	}{
		{
			name:     "no interfaces marked",
			marks:    nil,
			expected: "",
		},
		{
			name:     "single node single interface",
			marks:    map[string][]string{"worker-0": {"ens1f0"}},
			expected: "worker-0=ens1f0",
		},
		{
			name: "interfaces sorted within node",
			marks: map[string][]string{
				"worker-0": {"ens1f1", "ens1f0"},
			},
			expected: "worker-0=ens1f0,ens1f1",
		},
		{
			name: "nodes sorted and multiple interfaces",
			marks: map[string][]string{
				"worker-1": {"ens1f1", "ens1f0"},
				"worker-0": {"ens1f0", "ens1f1"},
			},
			expected: "worker-0=ens1f0,ens1f1;worker-1=ens1f0,ens1f1",
		},
		{
			name: "duplicate marks deduplicated",
			marks: map[string][]string{
				"worker-0": {"ens1f0", "ens1f0", "ens1f1"},
			},
			expected: "worker-0=ens1f0,ens1f1",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			resetClusterNodesNICInfo()

			for nodeName, interfaces := range testCase.marks {
				Node(nodeName).MarkTested(interfaces...)
			}

			assert.Equal(t, testCase.expected, TestedInterfaceNames())
		})
	}
}
