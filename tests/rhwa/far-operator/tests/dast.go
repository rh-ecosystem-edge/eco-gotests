package tests

import (
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/far-operator/internal/farparams"
	rapidast "github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/internal/rapidast"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/internal/rhwainittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/rhwa/internal/rhwaparams"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe(
	"FAR Post Deployment tests",
	Ordered,
	ContinueOnFailure,
	Label(farparams.Label), Label("dast"), func() {
		BeforeAll(func() {
			By("Verify fence-agents-remediation is the only deployment in namespace")
			deploymentList, err := deployment.List(APIClient, rhwaparams.RhwaOperatorNs, metav1.ListOptions{})
			Expect(err).ToNot(HaveOccurred(), "Failed to list deployments")

			if len(deploymentList) != 1 {
				Skip(fmt.Sprintf("Expected only fence-agents-remediation deployment, found %d deployment(s)", len(deploymentList)))
			}

			By("Get FAR deployment object")
			farDeployment, err := deployment.Pull(
				APIClient, farparams.OperatorDeploymentName, rhwaparams.RhwaOperatorNs)
			Expect(err).ToNot(HaveOccurred(), "Failed to get FAR deployment")

			By("Verify FAR deployment is Ready")
			Expect(farDeployment.IsReady(rhwaparams.DefaultTimeout)).To(BeTrue(), "FAR deployment is not Ready")
		})

		It("Verify FAR Operator passes trivy scan without vulnerabilities", reportxml.ID("76877"), func() {

			By("Creating rapidast pod")
			dastTestPod, err := rapidast.PrepareRapidastPod(APIClient)
			Expect(err).ToNot(HaveOccurred())

			output, err := rapidast.RunRapidastScan(*dastTestPod, rhwaparams.RhwaOperatorNs)
			Expect(err).ToNot(HaveOccurred())

			By("Checking vulnerability scan results")
			var parsableStruct rapidast.DASTReport
			err = json.Unmarshal(output.Bytes(), &parsableStruct)
			Expect(err).ToNot(HaveOccurred())

			var vulnerabilityFound = false
			for _, resource := range parsableStruct.Resources {
				for _, result := range resource.Results {
					if result.MisconfSummary.Failures > 0 {
						fmt.Printf("%d vulnerability(s) found in %s\n", result.MisconfSummary.Failures, resource.Name)
						for _, misconfiguration := range result.Misconfigurations {
							fmt.Printf("- %+v\n", misconfiguration)
						}
						vulnerabilityFound = true
					}
				}
			}
			Expect(vulnerabilityFound).NotTo(BeTrue(), "Found vulnerability(s)")
		})
	})
