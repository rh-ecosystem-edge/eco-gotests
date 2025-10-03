package rapidast

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/rbac"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/serviceaccount"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/internal/rhwainittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/internal/rhwaparams"

	v1 "k8s.io/api/rbac/v1"
)

const (
	logLevel = rhwaparams.LogLevel
)

// PrepareRapidastPod initializes the pod responsible for running rapidast scanner.
func PrepareRapidastPod(apiClient *clients.Settings) (*pod.Builder, error) {
	nodes, err := nodes.List(apiClient)
	if err != nil {
		glog.V(logLevel).Infof("Error in node list retrieval %s", err.Error())

		return nil, err
	}

	_, err = serviceaccount.NewBuilder(APIClient, "trivy-service-account", rhwaparams.TestNamespaceName).Create()
	if err != nil {
		glog.V(logLevel).Infof("Error in service account creation %s", err.Error())

		return nil, err
	}

	_, err = rbac.NewClusterRoleBuilder(APIClient, "trivy-clusterrole", v1.PolicyRule{
		APIGroups: []string{
			"",
		},
		Resources: []string{
			"pods",
		},
		Verbs: []string{
			"get",
			"list",
			"watch",
		},
	}).Create()
	if err != nil {
		glog.V(logLevel).Infof("Error in ClusterRoleBuilder creation %s", err.Error())

		return nil, err
	}

	_, err = rbac.NewClusterRoleBindingBuilder(APIClient, "trivy-clusterrole-binding", "trivy-clusterrole", v1.Subject{
		Kind:      "ServiceAccount",
		Name:      "trivy-service-account",
		Namespace: rhwaparams.TestNamespaceName,
	}).Create()
	if err != nil {
		glog.V(logLevel).Infof("Error in ClusterRoleBindingBuilder creation %s", err.Error())

		return nil, err
	}

	dastTestPod := pod.NewBuilder(
		APIClient, "rapidastclientpod", rhwaparams.TestNamespaceName, rhwaparams.TestContainerDast).
		DefineOnNode(nodes[0].Object.Name).
		WithTolerationToMaster().
		WithPrivilegedFlag()
	dastTestPod.Definition.Spec.ServiceAccountName = "trivy-service-account"

	_, err = dastTestPod.CreateAndWaitUntilRunning(time.Minute)
	if err != nil {
		glog.V(logLevel).Infof("Error in rapidast client pod creation %s", err.Error())

		return nil, err
	}

	return dastTestPod, nil
}

// RunRapidastScan executes the rapidast scan configured in the container with a timeout.
// Returns the command output (for JSON parsing) and any error that occurred (including timeout).
func RunRapidastScan(dastTestPod pod.Builder, namespace string) (bytes.Buffer, error) {
	command := []string{"bash", "-c",
		fmt.Sprintf("NAMESPACE=%s rapidast.py --config ./config/rapidastConfig.yaml 2> /dev/null", namespace)}

	// Create a channel to receive the result
	type result struct {
		output bytes.Buffer
		err    error
	}

	resultCh := make(chan result, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)

	defer cancel()

	// Execute the command in a goroutine
	go func() {
		output, err := dastTestPod.ExecCommand(command)
		resultCh <- result{output: output, err: err}
	}()

	// Wait for either completion or timeout
	select {
	case res := <-resultCh:
		return res.output, res.err
	case <-ctx.Done():
		return bytes.Buffer{}, fmt.Errorf("rapidast scan exceeded timeout of 10 minutes")
	}
}
