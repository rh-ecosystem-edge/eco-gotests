package tests

import (
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
<<<<<<< HEAD
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/rbac"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/serviceaccount"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/far-operator/internal/farparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/internal/rhwainittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/internal/rhwaparams"
=======

	"github.com/openshift-kni/eco-goinfra/pkg/deployment"
	"github.com/openshift-kni/eco-goinfra/pkg/pod"
	"github.com/openshift-kni/eco-goinfra/pkg/reportxml"

	"github.com/openshift-kni/eco-gotests/tests/rhwa/far-operator/internal/farparams"
	. "github.com/openshift-kni/eco-gotests/tests/rhwa/internal/rapidast"
	. "github.com/openshift-kni/eco-gotests/tests/rhwa/internal/rhwainittools"
	"github.com/openshift-kni/eco-gotests/tests/rhwa/internal/rhwaparams"
>>>>>>> e8f14f6a (extract preparation of rapidast pod)

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DASTReport struct {
	ClusterName string
	Resources   []struct {
		Name      string
		Namespace string
		Results   []struct {
			Target         string
			Class          string
			Type           string
			MisconfSummary struct {
				Success    int
				Failures   int
				Exceptions int
			}
			Misconfigurations []struct {
				Type        string
				ID          string
				AVDID       string
				Description string
				Message     string
				Namespace   string
				Query       string
				Resolution  string
				Severity    string
				PrimaryURL  string
				References  []string
				Status      string
			}
		}
	}
}

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

			dastTestPod := PrepareRapidastPod(APIClient)

			By("Running vulnerability scan")
			command := []string{"bash", "-c",
				fmt.Sprintf("NAMESPACE=%s rapidast.py --config ./config/rapidastConfig.yaml 2> /dev/null", rhwaparams.RhwaOperatorNs)}
			output, err := dastTestPod.ExecCommand(command)
			Expect(err).ToNot(HaveOccurred(), "Command failed")

			By("Checking vulnerability scan results")
			var parsableStruct DASTReport
			err = json.Unmarshal(output.Bytes(), &parsableStruct)
			Expect(err).ToNot(HaveOccurred())

			var vulnerability_found bool = false
			for _, resource := range parsableStruct.Resources {
				for _, result := range resource.Results {
					if result.MisconfSummary.Failures > 0 {
						fmt.Printf("%d vulnerability(s) found in %s\n", result.MisconfSummary.Failures, resource.Name)
						vulnerability_found = true
					}
				}
			}
			Expect(vulnerability_found).NotTo(BeTrue(), "Found vulnerability(s)")
		})
	})
