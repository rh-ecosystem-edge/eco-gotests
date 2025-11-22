package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/argocd"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/gitopsztp/internal/gitdetails"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/gitopsztp/internal/helper"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/gitopsztp/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
	"k8s.io/klog/v2"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

var _ = Describe("ZTP Argo CD ACM CR Tests", Label(tsparams.LabelArgoCdAcmCrsTestCases), func() {
	var (
		acmPolicyGeneratorImage string
		policiesApp             *argocd.ApplicationBuilder
		originalPoliciesGitPath string
	)

	BeforeEach(func() {
		By("verifying that ZTP meets the minimum version")
		versionInRange, err := version.IsVersionStringInRange(RANConfig.ZTPVersion, "4.12", "")
		Expect(err).ToNot(HaveOccurred(), "Failed to compare ZTP version string")

		if !versionInRange {
			Skip("ZTP Argo CD ACM CRs tests require ZTP 4.12 or later")
		}

		By("saving the original policies app source")
		policiesApp, err = argocd.PullApplication(
			HubAPIClient, tsparams.ArgoCdPoliciesAppName, ranparam.OpenshiftGitOpsNamespace)
		Expect(err).ToNot(HaveOccurred(), "Failed to get the original policies app")

		originalPoliciesGitPath, err = gitdetails.GetGitPath(policiesApp)
		Expect(err).ToNot(HaveOccurred(), "Failed to get the original policies app git path")

		By("determining the container image for ACM CR integration")
		multiClusterDeployment, err := deployment.Pull(
			HubAPIClient, tsparams.MultiClusterHubOperator, ranparam.AcmOperatorNamespace)
		Expect(err).ToNot(HaveOccurred(), "Failed to get multi cluster operator deployment")

		acmPolicyGeneratorImage = getContainerImageFromDeploymentEnvironment(
			multiClusterDeployment, tsparams.MultiClusterHubOperator, "OPERAND_IMAGE_MULTICLUSTER_OPERATORS_SUBSCRIPTION")
		Expect(acmPolicyGeneratorImage).ToNot(BeEmpty(), "Failed to find ACM policy generator container image")

		klog.V(tsparams.LogLevel).Infof("Found ACM policy generator container image: '%s'", acmPolicyGeneratorImage)
	})

	AfterEach(func() {
		if CurrentSpecReport().State.Is(types.SpecStateSkipped) {
			return
		}

		By("resetting the policies app back to the original settings")
		policiesApp.Definition.Spec.Source.Path = originalPoliciesGitPath
		policiesApp, err := policiesApp.Update(true)
		Expect(err).ToNot(HaveOccurred(), "Failed to update the policies app back to the original settings")

		By("waiting for the policies app to sync")
		err = policiesApp.WaitForSourceUpdate(true, tsparams.ArgoCdChangeTimeout)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for the policies app to sync")
	})

	// 54236 - Evaluating use of ACM's version of PolicyGenTemplates with our ZTP flow. This enables user created
	// content that does not depend on our ZTP container but works "seamlessly" with it.
	It("should use ACM CRs to template a policy, deploy it, and validate it succeeded", reportxml.ID("54236"), func() {
		By("checking if the ztp test path exists")
		if !policiesApp.DoesGitPathExist(tsparams.ZtpTestPathAcmCrs) {
			Skip(fmt.Sprintf("git path '%s' could not be found", tsparams.ZtpTestPathAcmCrs))
		}

		By("updating the policies app git path")
		err := gitdetails.UpdateAndWaitForSync(policiesApp, true, tsparams.ZtpTestPathAcmCrs)
		Expect(err).ToNot(HaveOccurred(), "Failed to update the policies app git path")

		By("waiting for policies to be created")
		policy, err := helper.WaitForPolicyToExist(
			HubAPIClient, tsparams.AcmCrsPolicyName, tsparams.TestNamespace, tsparams.ArgoCdChangeTimeout)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for the ACM CRs policy to be created")

		By("validating the policy was created and wait for it to finish")
		err = policy.WaitUntilComplianceState(policiesv1.NonCompliant, 1*time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for ACM CRs policy to be non-compliant")
	})
})

// getContainerImageFromDeploymentEnvironment gets the value of an environment variable from a specific container in a
// deployment.
func getContainerImageFromDeploymentEnvironment(
	deploymentBuilder *deployment.Builder, containerName, envName string) string {
	for _, container := range deploymentBuilder.Definition.Spec.Template.Spec.Containers {
		if container.Name == containerName {
			for _, envVar := range container.Env {
				if envVar.Name == envName {
					return envVar.Value
				}
			}
		}
	}

	return ""
}
