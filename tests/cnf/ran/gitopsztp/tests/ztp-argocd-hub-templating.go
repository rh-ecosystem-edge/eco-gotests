package tests

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/argocd"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/cgu"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/gitopsztp/internal/gitdetails"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/gitopsztp/internal/helper"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/gitopsztp/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

var _ = Describe("ZTP Argo CD Hub Templating Tests", Label(tsparams.LabelArgoCdHubTemplatingTestCases), func() {
	var (
		policiesApp             *argocd.ApplicationBuilder
		originalPoliciesGitPath string
	)

	BeforeEach(func() {
		By("checking the ZTP version")
		versionInRange, err := version.IsVersionStringInRange(RANConfig.ZTPVersion, "4.12", "")
		Expect(err).ToNot(HaveOccurred(), "Failed to check if ZTP version is in range")

		if !versionInRange {
			Skip("This test requires a ZTP version of at least 4.12")
		}

		By("saving the original policies app source")
		policiesApp, err = argocd.PullApplication(
			HubAPIClient, tsparams.ArgoCdPoliciesAppName, ranparam.OpenshiftGitOpsNamespace)
		Expect(err).ToNot(HaveOccurred(), "Failed to get the original policies app")

		originalPoliciesGitPath, err = gitdetails.GetGitPath(policiesApp)
		Expect(err).ToNot(HaveOccurred(), "Failed to get the original policies app git path")

		By("ensuring the test namespace exists")
		_, err = namespace.NewBuilder(HubAPIClient, tsparams.TestNamespace).Create()
		Expect(err).ToNot(HaveOccurred(), "Failed to create the test namespace")
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

		By("removing the hub templating leftovers if any exist")
		network, err := sriov.PullNetwork(Spoke1APIClient, tsparams.TestNamespace, RANConfig.SriovOperatorNamespace)
		if err == nil {
			err = network.DeleteAndWait(tsparams.ArgoCdChangeTimeout)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete SR-IOV network")
		}

		By("removing the CGU if it exists")
		cguBuilder, err := cgu.Pull(
			HubAPIClient, tsparams.HubTemplatingCguName, tsparams.HubTemplatingCguNamespace)
		if err == nil {
			_, err = cguBuilder.DeleteAndWait(5 * time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete and wait for CGU to be deleted")
		}
	})

	// 54240 - Hub-side ACM templating with TALM
	It("should report an error for using autoindent function where not allowed", reportxml.ID("54240"), func() {
		setupHubTemplateTest(policiesApp, tsparams.ZtpTestPathTemplatingAutoIndent)

		By("validating TALM reported a policy error")
		assertTalmPodLog(HubAPIClient, "policy has hub template error")

		By("validating the specific error using the policy message")
		policy, err := ocm.PullPolicy(
			HubAPIClient, tsparams.TestNamespace+"."+tsparams.HubTemplatingPolicyName, RANConfig.Spoke1Name)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull hub side templating policy")

		_, err = policy.WaitForStatusMessageToContain(
			"wrong type for value; expected string; got int", tsparams.ArgoCdChangeTimeout)
		Expect(err).ToNot(HaveOccurred(), "Failed to validate error using policy message")
	})

	When("supported ACM hub side templating is used", func() {
		// 54240 - Hub-side ACM templating with TALM
		It("should create the policy successfully with a valid template", reportxml.ID("54240"), func() {
			By("checking the ZTP version")
			versionInRange, err := version.IsVersionStringInRange(RANConfig.ZTPVersion, "4.16", "")
			Expect(err).ToNot(HaveOccurred(), "Failed to check if ZTP version is in range")

			validTestPath := tsparams.ZtpTestPathTemplatingValid
			if versionInRange {
				validTestPath = tsparams.ZtpTestPathTemplatingValid416
			}

			// We must create the secret before the test since creating it by ZTP is not allowed.
			validSecret, err := secret.NewBuilder(
				HubAPIClient, tsparams.HubTemplatingSecretName, tsparams.TestNamespace, corev1.SecretTypeOpaque).
				WithData(map[string][]byte{"vlanQoS": []byte("MAo=")}).
				Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create hub templating secret")

			DeferCleanup(func() {
				err := validSecret.Delete()
				Expect(err).ToNot(HaveOccurred(), "Failed to clean up hub templating secret")
			})

			setupHubTemplateTest(policiesApp, validTestPath)

			By("validating the policy reaches compliant status")
			policy, err := ocm.PullPolicy(HubAPIClient, tsparams.HubTemplatingPolicyName, tsparams.TestNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to get policy from hub cluster")

			err = policy.WaitUntilComplianceState(policiesv1.Compliant, tsparams.ArgoCdChangeTimeout)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for policy to become compliant")
		})
	})
})

// setupHubTemplateTest extracts the core setup logic for the hub templating test cases.
func setupHubTemplateTest(app *argocd.ApplicationBuilder, ztpTestPath string) {
	By("checking if the ztp test path exists")

	if !app.DoesGitPathExist(ztpTestPath) {
		Skip(fmt.Sprintf("git path '%s' could not be found", ztpTestPath))
	}

	By("updating the policies app git path")

	err := gitdetails.UpdateAndWaitForSync(app, true, ztpTestPath)
	Expect(err).ToNot(HaveOccurred(), "Failed to update the policies app git path")

	By("waiting for the policy to exist")

	_, err = helper.WaitForPolicyToExist(
		HubAPIClient, tsparams.HubTemplatingPolicyName, tsparams.TestNamespace, tsparams.ArgoCdChangeTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to wait for hub templating policy to be created")

	By("creating the CGU")

	cguBuilder := cgu.NewCguBuilder(
		HubAPIClient, tsparams.HubTemplatingCguName, tsparams.HubTemplatingCguNamespace, 1).
		WithCluster(RANConfig.Spoke1Name).
		WithManagedPolicy(tsparams.HubTemplatingPolicyName)
	cguBuilder.Definition.Spec.RemediationStrategy.Timeout = 10

	_, err = cguBuilder.Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create hub templating CGU")
}

// assertTalmPodLog asserts that the TALM pod log contains the expected substring.
func assertTalmPodLog(client *clients.Settings, expectedSubstring string) {
	klog.V(tsparams.LogLevel).Infof("Waiting for TALM log to report: '%s'", expectedSubstring)

	Eventually(func() string {
		podList, err := pod.List(client, ranparam.OpenshiftOperatorNamespace)
		Expect(err).ToNot(HaveOccurred(), "Failed to list pods is openshift operator namespace")
		Expect(podList).ToNot(BeEmpty(), "Failed to find any pods in the openshift operator namespace")

		var podLog string

		for _, podBuilder := range podList {
			if strings.HasPrefix(podBuilder.Object.Name, tsparams.TalmHubPodName) {
				klog.V(tsparams.LogLevel).Infof("Checking logs for pod %s", podBuilder.Object.Name)

				podLog, err = podBuilder.GetLog(1*time.Minute, ranparam.TalmContainerName)
				Expect(err).ToNot(HaveOccurred(), "Failed to get TALM pod log")

				break
			}
		}

		return podLog
	}, tsparams.ArgoCdChangeTimeout, tsparams.ArgoCdChangeInterval).
		Should(ContainSubstring(expectedSubstring), "Failed to assert TALM pod log contains %s", expectedSubstring)
}
