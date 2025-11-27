package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/olm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/webhook"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/sriovocpenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/tsparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

var _ = Describe("SRIOV Operator re-installation", Ordered, Label(tsparams.LabelOcpSriovReinstallation),
	ContinueOnFailure, func() {

		var (
			sriovInterfacesUnderTest []string
			workerNodeList           []*nodes.Builder
			sriovTestResourceName    = "sriovtestresource"
			sriovNamespace           *namespace.Builder
			sriovOperatorgroup       *olm.OperatorGroupBuilder
			sriovSubscription        *olm.SubscriptionBuilder
		)

		BeforeAll(func() {
			By("Verifying if SR-IOV tests can be executed on given cluster")
			err := sriovocpenv.DoesClusterHaveEnoughNodes(1, 1)
			if err != nil {
				Skip(fmt.Sprintf(
					"given cluster is not suitable for SR-IOV tests because it doesn't have enough nodes: %s", err.Error()))
			}

			By("Validating SR-IOV interfaces")
			workerNodeList, err = nodes.List(APIClient,
				metav1.ListOptions{LabelSelector: labels.Set(SriovOcpConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "Failed to discover worker nodes")

			Expect(sriovocpenv.ValidateSriovInterfaces(workerNodeList, 2)).ToNot(HaveOccurred(),
				"Failed to get required SR-IOV interfaces")
			sriovInterfacesUnderTest, err = SriovOcpConfig.GetSriovInterfaces(2)
			Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

			By("Collecting info about installed SR-IOV operator")
			sriovNamespace, sriovOperatorgroup, sriovSubscription = collectingInfoSriovOperator()
		})

		AfterAll(func() {
			By("Removing SR-IOV configuration")
			err := sriovoperator.RemoveSriovConfigurationAndWaitForSriovAndMCPStable(
				APIClient,
				SriovOcpConfig.WorkerLabelEnvVar,
				SriovOcpConfig.SriovOperatorNamespace,
				tsparams.MCOWaitTimeout,
				tsparams.DefaultTimeout)
			Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV configuration")
		})

		It("Verify SR-IOV operator control plane is operational before removal", reportxml.ID("46528"), func() {
			By("Applying SR-IOV NetworkNodePolicy")
			sriovPolicy := sriov.NewPolicyBuilder(
				APIClient,
				sriovTestResourceName,
				SriovOcpConfig.SriovOperatorNamespace,
				sriovTestResourceName,
				5,
				[]string{sriovInterfacesUnderTest[0] + "#0-1"}, SriovOcpConfig.WorkerLabelMap)
			err := sriovoperator.CreateSriovPolicyAndWaitUntilItsApplied(
				APIClient,
				SriovOcpConfig.WorkerLabelEnvVar,
				SriovOcpConfig.SriovOperatorNamespace,
				sriovPolicy,
				tsparams.MCOWaitTimeout,
				tsparams.DefaultStableDuration)
			Expect(err).ToNot(HaveOccurred(), "Failed to configure SR-IOV policy")

			By("Creating SR-IOV network")
			_, err = sriov.NewNetworkBuilder(APIClient, sriovTestResourceName, SriovOcpConfig.SriovOperatorNamespace,
				tsparams.TestNamespaceName, sriovTestResourceName).WithStaticIpam().WithMacAddressSupport().WithIPAddressSupport().
				WithLogLevel("debug").Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network")
		})

		It("Operator re-installation. Verify SR-IOV operator data plane is operational before removal",
			reportxml.ID("46529"), func() {
				By("Creating test pods and checking connectivity")
				err := sriovocpenv.CreatePodsAndRunTraffic(workerNodeList[0].Object.Name, workerNodeList[0].Object.Name,
					sriovTestResourceName, sriovTestResourceName, "", "",
					[]string{tsparams.ClientIPv4IPAddress}, []string{tsparams.ServerIPv4IPAddress})
				Expect(err).ToNot(HaveOccurred(), "Failed to test connectivity between test pods")
			})

		It("Operator re-installation. Verify all SR-IOV components are deleted when operator is removed",
			reportxml.ID("46530"), func() {
				removeSriovOperator(sriovNamespace)
				Expect(sriovoperator.IsSriovDeployed(APIClient, SriovOcpConfig.OcpSriovOperatorNamespace)).
					To(HaveOccurred(), "SR-IOV operator is not removed")
			})

		It("Operator re-installation. Validate that SR-IOV resources can not be deployed without SR-IOV operator",
			reportxml.ID("46531"),
			func() {
				By("Validate that SR-IOV operator namespace was removed")
				_, err := namespace.Pull(APIClient, SriovOcpConfig.OcpSriovOperatorNamespace)
				Expect(err).To(HaveOccurred(), "Failed to remove SR-IOV operator namespace")

				By("Validate that SR-IOV api doesn't work")
				_, err = sriov.NewPolicyBuilder(
					APIClient,
					sriovTestResourceName,
					SriovOcpConfig.SriovOperatorNamespace,
					sriovTestResourceName,
					5,
					[]string{sriovInterfacesUnderTest[0] + "#0-1"}, SriovOcpConfig.WorkerLabelMap).Create()
				Expect(err).To(HaveOccurred(), "SriovNetworkNodePolicy is created unexpectedly")

				_, err = sriov.NewNetworkBuilder(APIClient, sriovTestResourceName, SriovOcpConfig.SriovOperatorNamespace,
					tsparams.TestNamespaceName, sriovTestResourceName).WithStaticIpam().WithMacAddressSupport().WithIPAddressSupport().
					WithLogLevel("debug").Create()
				Expect(err).To(HaveOccurred(), "SriovNetwork is created unexpectedly")
			})

		It("Operator re-installation. Validate that re-installed SR-IOV operator’s control plane is up and running.",
			reportxml.ID("46532"),
			func() {
				By("Deploy SR-IOV operator")
				installSriovOperator(sriovNamespace, sriovOperatorgroup, sriovSubscription)

				Eventually(sriovoperator.IsSriovDeployed,
					time.Minute, tsparams.RetryInterval).
					WithArguments(APIClient, SriovOcpConfig.OcpSriovOperatorNamespace).
					ShouldNot(HaveOccurred(), "SR-IOV operator is not installed")

				By("Applying SR-IOV NetworkNodePolicy")
				sriovPolicy := sriov.NewPolicyBuilder(
					APIClient,
					sriovTestResourceName,
					SriovOcpConfig.SriovOperatorNamespace,
					sriovTestResourceName,
					5,
					[]string{sriovInterfacesUnderTest[0] + "#0-1"}, SriovOcpConfig.WorkerLabelMap)
				err := sriovoperator.CreateSriovPolicyAndWaitUntilItsApplied(
					APIClient,
					SriovOcpConfig.WorkerLabelEnvVar,
					SriovOcpConfig.SriovOperatorNamespace,
					sriovPolicy,
					tsparams.MCOWaitTimeout,
					tsparams.DefaultStableDuration)
				Expect(err).ToNot(HaveOccurred(), "Failed to configure SR-IOV policy")

				By("Creating SR-IOV network")
				_, err = sriov.NewNetworkBuilder(APIClient, sriovTestResourceName, SriovOcpConfig.SriovOperatorNamespace,
					tsparams.TestNamespaceName, sriovTestResourceName).WithStaticIpam().WithMacAddressSupport().WithIPAddressSupport().
					WithLogLevel("debug").Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network")
			})

		It("Operator re-installation. Validate that re-installed SR-IOV operator’s data plane is up and running",
			reportxml.ID("46533"),
			func() {
				By("Creating test pods and checking connectivity")
				err := sriovocpenv.CreatePodsAndRunTraffic(workerNodeList[0].Object.Name, workerNodeList[0].Object.Name,
					sriovTestResourceName, sriovTestResourceName, "", "",
					[]string{tsparams.ClientIPv4IPAddress}, []string{tsparams.ServerIPv4IPAddress})
				Expect(err).ToNot(HaveOccurred(), "Failed to test connectivity between test pods")
			})
	})

func collectingInfoSriovOperator() (
	sriovNamespace *namespace.Builder,
	sriovOperatorGroup *olm.OperatorGroupBuilder,
	sriovSubscription *olm.SubscriptionBuilder) {
	sriovNs, err := namespace.Pull(APIClient, SriovOcpConfig.SriovOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull SR-IOV operator namespace")
	sriovOg, err := olm.PullOperatorGroup(APIClient, "sriov-network-operators", SriovOcpConfig.SriovOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull SR-IOV OperatorGroup")
	sriovSub, err := olm.PullSubscription(
		APIClient,
		"sriov-network-operator-subscription",
		SriovOcpConfig.SriovOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull sriov-network-operator-subscription")

	return sriovNs, sriovOg, sriovSub
}

func removeSriovOperator(sriovNamespace *namespace.Builder) {
	By("Clean all SR-IOV policies and networks")

	err := sriovoperator.RemoveSriovConfigurationAndWaitForSriovAndMCPStable(
		APIClient,
		SriovOcpConfig.WorkerLabelEnvVar,
		SriovOcpConfig.SriovOperatorNamespace,
		tsparams.MCOWaitTimeout,
		tsparams.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV configuration")

	By("Remove SR-IOV operator config")

	sriovOperatorConfig, err := sriov.PullOperatorConfig(APIClient, SriovOcpConfig.SriovOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull SriovOperatorConfig")

	_, err = sriovOperatorConfig.Delete()
	Expect(err).ToNot(HaveOccurred(), "Failed to remove default SR-IOV operator config")

	By("Validation that SR-IOV webhooks are not available")

	for _, webhookname := range []string{"network-resources-injector-config", "sriov-operator-webhook-config"} {
		Eventually(func() error {
			_, err := webhook.PullMutatingConfiguration(APIClient, webhookname)

			return err
		}, time.Minute, tsparams.RetryInterval).Should(HaveOccurred(),
			fmt.Sprintf("MutatingWebhook %s was not removed", webhookname))
	}

	Eventually(func() error {
		_, err := webhook.PullValidatingConfiguration(APIClient, "sriov-operator-webhook-config")

		return err
	}, time.Minute, tsparams.RetryInterval).Should(HaveOccurred(),
		"ValidatingWebhook sriov-operator-webhook-config was not removed")

	By("Removing SR-IOV namespace")

	err = sriovNamespace.DeleteAndWait(tsparams.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf(
		"Failed to delete SR-IOV namespace %s", SriovOcpConfig.SriovOperatorNamespace))
}

func installSriovOperator(sriovNamespace *namespace.Builder,
	sriovOperatorGroup *olm.OperatorGroupBuilder,
	sriovSubscription *olm.SubscriptionBuilder) {
	By("Creating SR-IOV operator namespace")

	_, err := namespace.NewBuilder(APIClient, sriovNamespace.Definition.Name).Create()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create SR-IOV namespace %s", sriovNamespace.Definition.Name))

	By("Creating SR-IOV OperatorGroup")

	_, err = olm.NewOperatorGroupBuilder(
		APIClient,
		sriovOperatorGroup.Definition.Name,
		sriovOperatorGroup.Definition.Namespace).Create()
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create SR-IOV OperatorGroup %s", sriovOperatorGroup.Definition.Name))

	By("Creating SR-IOV operator Subscription")

	_, err = olm.NewSubscriptionBuilder(
		APIClient, sriovSubscription.Definition.Name,
		sriovSubscription.Definition.Namespace,
		sriovSubscription.Definition.Spec.CatalogSource,
		sriovSubscription.Definition.Spec.CatalogSourceNamespace,
		sriovSubscription.Definition.Spec.Package).Create()
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create SR-IOV Subscription %s", sriovSubscription.Definition.Name))

	By("Creating SR-IOV operator default configuration")

	_, err = sriov.NewOperatorConfigBuilder(APIClient, sriovNamespace.Definition.Name).
		WithOperatorWebhook(true).
		WithInjector(true).
		Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV operator config")
}
