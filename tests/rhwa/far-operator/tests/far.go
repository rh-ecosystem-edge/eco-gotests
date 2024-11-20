package tests

import (
	"fmt"
	"time"

	"github.com/golang/glog"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/rbac"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/serviceaccount"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/far-operator/internal/farparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/internal/rhwainittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/internal/rhwaparams"

	v1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe(
	"FAR Post Deployment tests",
	Ordered,
	ContinueOnFailure,
	Label(farparams.Label), func() {
		BeforeAll(func() {
			By("Get FAR deployment object")
			farDeployment, err := deployment.Pull(
				APIClient, farparams.OperatorDeploymentName, rhwaparams.RhwaOperatorNs)
			Expect(err).ToNot(HaveOccurred(), "Failed to get FAR deployment")

			By("Verify FAR deployment is Ready")
			Expect(farDeployment.IsReady(rhwaparams.DefaultTimeout)).To(BeTrue(), "FAR deployment is not Ready")
		})
		It("Verify Fence Agents Remediation Operator pod is running", reportxml.ID("66026"), func() {

			listOptions := metav1.ListOptions{
				LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", farparams.OperatorControllerPodLabel),
			}
			_, err := pod.WaitForAllPodsInNamespaceRunning(
				APIClient,
				rhwaparams.RhwaOperatorNs,
				rhwaparams.DefaultTimeout,
				listOptions,
			)
			Expect(err).ToNot(HaveOccurred(), "Pod is not ready")
		})

		It("Verify FAR Operator passes trivy scan without vulnerabilities", reportxml.ID("76877"), func() {

			By("Retrieve list of nodes")
			nodes, err := nodes.List(APIClient)
			Expect(err).ToNot(HaveOccurred(), "Error getting nodes list")

			By("Create service account")
			_, err = serviceaccount.NewBuilder(APIClient, "trivy-service-account", rhwaparams.TestNamespaceName).Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create Service Account")

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
			Expect(err).ToNot(HaveOccurred(), "Failed to create Cluster Role")

			_, err = rbac.NewClusterRoleBindingBuilder(APIClient, "trivy-clusterrole-binding", "trivy-clusterrole", v1.Subject{
				Kind:      "ServiceAccount",
				Name:      "trivy-service-account",
				Namespace: rhwaparams.TestNamespaceName,
			}).Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create Cluster Role Binding")

			dastTestPod := pod.NewBuilder(
				APIClient, "rapidastclientpod", rhwaparams.TestNamespaceName, rhwaparams.TestContainerDast).
				DefineOnNode(nodes[0].Object.Name).
				WithTolerationToMaster().
				WithPrivilegedFlag()

			dastTestPod.Definition.Spec.ServiceAccountName = "trivy-service-account"

			By("Creating client test pod")
			dastTestPod, err = pod.NewBuilder(
				APIClient, "rapidastclientpod", rhwaparams.TestNamespaceName, rhwaparams.TestContainerDast).
				DefineOnNode(nodes[0].Object.Name).
				WithTolerationToMaster().
				WithPrivilegedFlag().CreateAndWaitUntilRunning(time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to create client test pod")

			//TODO: check that the command can be actually executed by the pod.
			command := []string{"bash", "-c", "export NAMESPACE=openshift-workload-availability rapidast.py --config ./config/rapidastConfig.yaml"}
			output, err := dastTestPod.ExecCommand(command)
			Expect(err).ToNot(HaveOccurred(), "Command failed")

			//TODO: The output of the rapidast command is a JSON that can be handled
			glog.V(90).Infof("TRIVY command output: %s/n:", output.String())

		})
	})
