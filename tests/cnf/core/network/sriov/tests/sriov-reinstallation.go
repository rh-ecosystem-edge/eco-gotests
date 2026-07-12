package tests

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netenv"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/sriovenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	sriovReinstallResourceName = "sriovreinstall"
	sriovReinstallNumVFs       = 5
)

var _ = Describe("SRIOV Operator re-installation", Ordered, Label(tsparams.LabelSriovReinstallation),
	ContinueOnFailure, func() {
		var (
			sriovInterfacesUnderTest []string
			workerNodeList           []*nodes.Builder
			installedOperator        *sriovenv.InstalledSriovOperator
		)

		BeforeAll(func() {
			By("Verifying if SR-IOV tests can be executed on given cluster")

			err := netenv.DoesClusterHasEnoughNodes(APIClient, NetConfig, 1, 1)
			if err != nil {
				Skip(fmt.Sprintf(
					"given cluster is not suitable for SR-IOV tests because it doesn't have enough nodes: %s", err.Error()))
			}

			By("Validating SR-IOV interfaces")

			workerNodeList, err = nodes.List(APIClient,
				metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "Failed to discover worker nodes")

			Expect(sriovenv.ValidateSriovInterfaces(workerNodeList, 1)).ToNot(HaveOccurred(),
				"Failed to get required SR-IOV interfaces")

			sriovInterfacesUnderTest, err = NetConfig.GetSriovInterfaces(1)
			Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

			By("Collecting info about installed SR-IOV operator")

			installedOperator, err = sriovenv.CollectInstalledSriovOperatorInfo()
			Expect(err).ToNot(HaveOccurred(), "Failed to collect SR-IOV operator installation info")
		})

		AfterAll(func() {
			if err := sriovoperator.IsSriovDeployed(APIClient, NetConfig.SriovOperatorNamespace); err != nil {
				By("Restoring SR-IOV operator before cleanup")

				installErr := sriovenv.InstallSriovOperator(installedOperator)
				if installErr != nil {
					AddReportEntry(
						"SR-IOV operator restore failure",
						fmt.Sprintf("Failed to restore SR-IOV operator after test suite: %v", installErr),
						ReportEntryVisibilityFailureOrVerbose,
					)
				} else {
					By("Waiting for restored SR-IOV operator to become ready")

					waitErr := sriovenv.WaitForSriovOperatorDeployed(
						tsparams.OperatorInstallWaitTimeout, tsparams.RetryInterval)
					if waitErr != nil {
						AddReportEntry(
							"SR-IOV operator restore failure",
							fmt.Sprintf("SR-IOV operator is not restored: %v", waitErr),
							ReportEntryVisibilityFailureOrVerbose,
						)
					}
				}
			}

			By("Removing SR-IOV configuration")

			err := sriovoperator.RemoveSriovConfigurationAndWaitForSriovAndMCPStable(
				APIClient,
				NetConfig.WorkerLabelEnvVar,
				NetConfig.SriovOperatorNamespace,
				tsparams.MCOWaitTimeout,
				tsparams.DefaultTimeout)
			Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV configuration")
		})

		It("Operator re-installation. Verify SR-IOV operator control plane is operational before removal",
			reportxml.ID("46528"), func() {
				createReinstallSriovConfiguration(sriovInterfacesUnderTest[0])
			})

		It("Operator re-installation. Verify SR-IOV operator data plane is operational before removal",
			reportxml.ID("46529"), func() {
				By("Creating test pods and checking connectivity")

				err := sriovenv.CreatePodsAndRunTraffic(workerNodeList[0].Object.Name, workerNodeList[0].Object.Name,
					sriovReinstallResourceName, sriovReinstallResourceName, "", "",
					[]string{tsparams.ClientIPv4IPAddress}, []string{tsparams.ServerIPv4IPAddress})
				Expect(err).ToNot(HaveOccurred(), "Failed to test connectivity between test pods")
			})

		It("Operator re-installation. Verify all SR-IOV components are deleted when operator is removed",
			reportxml.ID("46530"), func() {
				By("Removing test pods before operator removal")

				err := namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
					netparam.DefaultTimeout, pod.GetGVR())
				Expect(err).ToNot(HaveOccurred(), "Failed to clean test pods")

				By("Removing SR-IOV operator")

				err = sriovenv.RemoveSriovOperator(installedOperator.Namespace)
				Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV operator")

				Expect(sriovoperator.IsSriovDeployed(APIClient, NetConfig.SriovOperatorNamespace)).
					To(HaveOccurred(), "SR-IOV operator is not removed")
			})

		It("Operator re-installation. Validate that SR-IOV resources can not be deployed without SR-IOV operator",
			reportxml.ID("46531"),
			func() {
				By("Validate that SR-IOV operator namespace was removed")

				_, err := namespace.Pull(APIClient, NetConfig.SriovOperatorNamespace)
				Expect(err).To(HaveOccurred(), "SR-IOV operator namespace still exists")

				By("Validate that SR-IOV api doesn't work")

				_, err = sriov.NewPolicyBuilder(
					APIClient,
					sriovReinstallResourceName,
					NetConfig.SriovOperatorNamespace,
					sriovReinstallResourceName,
					sriovReinstallNumVFs,
					[]string{sriovInterfacesUnderTest[0] + "#0-1"}, NetConfig.WorkerLabelMap).Create()
				Expect(err).To(HaveOccurred(), "SriovNetworkNodePolicy is created unexpectedly")

				_, err = sriov.NewNetworkBuilder(
					APIClient,
					sriovReinstallResourceName,
					NetConfig.SriovOperatorNamespace,
					tsparams.TestNamespaceName,
					sriovReinstallResourceName).
					WithStaticIpam().
					WithMacAddressSupport().
					WithIPAddressSupport().
					WithLogLevel(netparam.LogLevelDebug).
					Create()
				Expect(err).To(HaveOccurred(), "SriovNetwork is created unexpectedly")
			})

		It("Operator re-installation. Validate that re-installed SR-IOV operator’s control plane is up and running.",
			reportxml.ID("46532"),
			func() {
				By("Deploy SR-IOV operator")

				err := sriovenv.InstallSriovOperator(installedOperator)
				Expect(err).ToNot(HaveOccurred(), "Failed to install SR-IOV operator")

				By("Waiting for SR-IOV operator to become ready")

				err = sriovenv.WaitForSriovOperatorDeployed(
					tsparams.OperatorInstallWaitTimeout, tsparams.RetryInterval)
				Expect(err).ToNot(HaveOccurred(), "SR-IOV operator is not installed")

				By("Validate that SR-IOV webhooks have expected failure policies")

				Eventually(sriovenv.ValidateReinstalledSriovWebhookFailurePolicies,
					tsparams.OperatorInstallWaitTimeout, tsparams.RetryInterval).
					Should(Succeed(), "SR-IOV webhook failure policies are not configured as expected")

				createReinstallSriovConfiguration(sriovInterfacesUnderTest[0])
			})

		It("Operator re-installation. Validate that re-installed SR-IOV operator’s data plane is up and running",
			reportxml.ID("46533"),
			func() {
				By("Removing test pods before data plane retest")

				err := namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
					netparam.DefaultTimeout, pod.GetGVR())
				Expect(err).ToNot(HaveOccurred(), "Failed to clean test pods")

				By("Creating test pods and checking connectivity")

				err = sriovenv.CreatePodsAndRunTraffic(workerNodeList[0].Object.Name, workerNodeList[0].Object.Name,
					sriovReinstallResourceName, sriovReinstallResourceName, "", "",
					[]string{tsparams.ClientIPv4IPAddress}, []string{tsparams.ServerIPv4IPAddress})
				Expect(err).ToNot(HaveOccurred(), "Failed to test connectivity between test pods")
			})
	})

func createReinstallSriovConfiguration(pfName string) {
	By("Applying SR-IOV NetworkNodePolicy")

	sriovPolicy := sriov.NewPolicyBuilder(
		APIClient,
		sriovReinstallResourceName,
		NetConfig.SriovOperatorNamespace,
		sriovReinstallResourceName,
		sriovReinstallNumVFs,
		[]string{pfName + "#0-1"},
		NetConfig.WorkerLabelMap)
	err := sriovoperator.CreateSriovPolicyAndWaitUntilItsApplied(
		APIClient,
		NetConfig.WorkerLabelEnvVar,
		NetConfig.SriovOperatorNamespace,
		sriovPolicy,
		tsparams.MCOWaitTimeout,
		tsparams.DefaultStableDuration)
	Expect(err).ToNot(HaveOccurred(), "Failed to configure SR-IOV policy")

	By("Creating SR-IOV network")

	sriovNetworkBuilder := sriov.NewNetworkBuilder(
		APIClient,
		sriovReinstallResourceName,
		NetConfig.SriovOperatorNamespace,
		tsparams.TestNamespaceName,
		sriovReinstallResourceName).
		WithStaticIpam().
		WithMacAddressSupport().
		WithIPAddressSupport().
		WithLogLevel(netparam.LogLevelDebug)
	err = sriovenv.CreateSriovNetworkAndWaitForNADCreation(sriovNetworkBuilder, tsparams.NADWaitTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network")
}
