package tests

import (
	"context" // For context.TODO() in API calls
	"fmt"     // For formatted string output

	. "github.com/onsi/ginkgo/v2" // Ginkgo BDD-style testing framework
	. "github.com/onsi/gomega"   // Gomega matcher library for assertions

	"github.com/openshift-kni/eco-goinfra/pkg/nodes"      // eco-goinfra package for Kubernetes Node operations
	"github.com/openshift-kni/eco-goinfra/pkg/reportxml" // eco-goinfra package for JUnit XML reporting IDs
	"github.com/openshift-kni/eco-gotests/tests/cnf/ran/healthcheck/internal/tsparams" // Our test suite parameters (for labels)
	. "github.com/openshift-kni/eco-gotests/tests/cnf/ran/internal/raninittools"       // Provides Spoke1APIClient for cluster access
	corev1 "k8s.io/api/core/v1" // Kubernetes core API types (e.g., Node, NodeCondition)
)

// Define the test suite for "Cluster Health Check".
// The Label() function associates this suite with the "healthcheck" label,
// allowing it to be selectively run via test filters.
var _ = Describe("Cluster Health Check", Label(tsparams.LabelHealthCheckTestCases), func() {

	// Define a single test case within the "Cluster Health Check" suite.
	// reportxml.ID() links this test case to a specific ID in a test management system.
	It("should have all cluster nodes in Ready state", reportxml.ID("NEW-HEALTH-CHECK-1"), func() {
		By("Getting all nodes in the cluster")
		// Use Spoke1APIClient (provided by raninittools) to list all nodes.
		// context.TODO() is used here for simplicity; in more complex scenarios,
		// a context with timeout or cancellation might be preferred.
		nodeList, err := nodes.List(Spoke1APIClient, context.TODO())
		Expect(err).ToNot(HaveOccurred(), "Failed to list cluster nodes: %v", err)
		// Assert that at least one node was found, otherwise the cluster is likely empty or inaccessible.
		Expect(nodeList).ToNot(BeEmpty(), "No nodes found in the cluster. This indicates a critical issue.")

		By("Verifying all nodes are in Ready state")
		// Iterate through each node retrieved from the cluster.
		for _, node := range nodeList {
			// Initialize a flag to track if the node is Ready.
			isReady := false
			// Iterate through the conditions reported by the node.
			for _, condition := range node.Status.Conditions {
				// Check for the "Ready" condition type and ensure its status is "True".
				if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
					isReady = true
					break // Found the Ready condition, no need to check further conditions for this node.
				}
			}
			// Assert that the node is ready. If not, the test will fail and report the node's name.
			Expect(isReady).To(BeTrue(), fmt.Sprintf("Node %s is not in Ready state. Current conditions: %+v", node.Name, node.Status.Conditions))
			// Print a message to the Ginkgo output (Jenkins console) for successful nodes.
			GinkgoWriter.Printf("Node %s is Ready.\n", node.Name)
		}
		// Final confirmation message if all nodes are ready.
		GinkgoWriter.Printf("All %d nodes are in Ready state.\n", len(nodeList))
	})
})
