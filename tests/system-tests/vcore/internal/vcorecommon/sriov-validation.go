package vcorecommon

import (
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clusteroperator"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/await"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/apiobjectshelper"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreparams"
)

// VerifySRIOVSuite container that contains tests for SR-IOV verification.
func VerifySRIOVSuite() {
	Describe(
		"SR-IOV Operator deployment and configuration validation",
		Label(vcoreparams.LabelVCoreOperators), func() {
			It(fmt.Sprintf("Verifies %s namespace exists", vcoreparams.SRIOVNamespace),
				Label("sriov"), VerifySRIOVNamespaceExists)

			It("Verifies SR-IOV Operator deployment succeeded",
				Label("sriov"), reportxml.ID("60041"), VerifySRIOVDeployment)

			It("Verifies SR-IOV configuration procedure succeeded",
				Label("sriov"), reportxml.ID("60088"), VerifySRIOVConfig)
		})
}

// VerifySRIOVNamespaceExists asserts Distributed Tracing Platform Operator namespace exists.
func VerifySRIOVNamespaceExists(ctx SpecContext) {
	err := apiobjectshelper.VerifyNamespaceExists(APIClient, vcoreparams.SRIOVNamespace, time.Second)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to pull %q namespace", vcoreparams.SRIOVNamespace))
} // func VerifySRIOVNamespaceExists (ctx SpecContext)

// VerifySRIOVDeployment assert SR-IOV Operator deployment succeeded.
func VerifySRIOVDeployment(ctx SpecContext) {
	klog.V(vcoreparams.VCoreLogLevel).Infof("Verify SR-IOV Operator deployment")

	err := apiobjectshelper.VerifyOperatorDeployment(APIClient,
		vcoreparams.SRIOVSubscriptionName,
		vcoreparams.SRIOVDeploymentName,
		vcoreparams.SRIOVNamespace,
		time.Minute)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("operator deployment %s failure in the namespace %s; %v",
			vcoreparams.SRIOVSubscriptionName, vcoreparams.SRIOVNamespace, err))
} // func VerifySRIOVDeployment (ctx SpecContext)

// VerifySRIOVConfig assert SR-IOV Operator configuration procedure.
//
//nolint:funlen
func VerifySRIOVConfig(ctx SpecContext) {
	klog.V(vcoreparams.VCoreLogLevel).Infof("Verify SR-IOV Operator configuration procedure")

	sriovNet1Name := "upf-sriov-dpdk-ext0"
	sriovNet2Name := "upf-sriov-dpdk-ext1"
	net1ResourceName := "sriov_dpdk_ext0"
	net2ResourceName := "sriov_dpdk_ext1"
	targetNetworkNamespace := "upf1"
	snnpPfname1 := []string{"ens2f0#0-7"}
	snnpPfname2 := []string{"ens2f1#0-7"}

	klog.V(vcoreparams.VCoreLogLevel).Info("Pre-configuration: change SR-IOV config")

	sriovOperatorConfigObj, err := sriov.PullOperatorConfig(APIClient, vcoreparams.SRIOVNamespace)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("failed to load sriovoperatorconfig object from namespace %s due to %v",
			vcoreparams.SRIOVNamespace, err))

	klog.V(vcoreparams.VCoreLogLevel).Infof("Disable Injector and OperatorWebhook and set a node selector value %v "+
		"for the sriovoperatorconfig object in namespace %s",
		VCoreConfig.VCorePpLabelMap, vcoreparams.SRIOVNamespace)

	_, err = sriovOperatorConfigObj.WithInjector(false).
		WithOperatorWebhook(false).
		WithConfigDaemonNodeSelector(VCoreConfig.VCorePpLabelMap).Update()
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("failed to change the sriovoperatorconfig object in namespace %s configuration due to %v",
			vcoreparams.SRIOVNamespace, err))

	klog.V(vcoreparams.VCoreLogLevel).Info("Wait until Webhook and Injector pods are un-deployed")

	err = await.WaitUntilDaemonSetDeleted(APIClient,
		vcoreparams.SRIOVInjectorDaemonsetName,
		vcoreparams.SRIOVNamespace,
		3*time.Minute,
	)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("failed to delete daemonset %s in namespace %s due to %v",
			vcoreparams.SRIOVInjectorDaemonsetName, vcoreparams.SRIOVNamespace, err))

	err = await.WaitUntilDaemonSetDeleted(APIClient,
		vcoreparams.SRIOVWebhookDaemonsetName,
		vcoreparams.SRIOVNamespace,
		3*time.Minute,
	)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("failed to delete daemonset %s in namespace %s due to %v",
			vcoreparams.SRIOVWebhookDaemonsetName, vcoreparams.SRIOVNamespace, err))

	klog.V(vcoreparams.VCoreLogLevel).Infof("Create SR-IOV network %s in namespace %s",
		sriovNet1Name, vcoreparams.SRIOVNamespace)

	sriovNetwork1 := sriov.NewNetworkBuilder(APIClient,
		sriovNet1Name,
		vcoreparams.SRIOVNamespace,
		targetNetworkNamespace,
		net1ResourceName)
	if !sriovNetwork1.Exists() {
		sriovNetwork1, err = sriovNetwork1.Create()
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("failed to create SR-IOV network %s in namespace %s due to %v",
				sriovNet1Name, vcoreparams.SRIOVNamespace, err))
		Expect(sriovNetwork1.Exists()).To(Equal(true),
			fmt.Sprintf("SR-IOV network %s in namespace %s not found",
				sriovNet1Name, vcoreparams.SRIOVNamespace))
	}

	klog.V(vcoreparams.VCoreLogLevel).Infof("Create SR-IOV network %s in namespace %s",
		sriovNet2Name, vcoreparams.SRIOVNamespace)

	sriovNetwork2 := sriov.NewNetworkBuilder(APIClient,
		sriovNet2Name,
		vcoreparams.SRIOVNamespace,
		targetNetworkNamespace,
		net2ResourceName)
	if !sriovNetwork2.Exists() {
		sriovNetwork2, err = sriovNetwork2.Create()
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("failed to create SR-IOV network %s in namespace %s due to %v",
				sriovNet2Name, vcoreparams.SRIOVNamespace, err))
		Expect(sriovNetwork2.Exists()).To(Equal(true),
			fmt.Sprintf("SR-IOV network %s in namespace %s not found",
				sriovNet2Name, vcoreparams.SRIOVNamespace))
	}

	klog.V(vcoreparams.VCoreLogLevel).Infof("Create SR-IOV network node policy %s in namespace %s",
		sriovNet1Name, vcoreparams.SRIOVNamespace)

	sriovNetworkNodePolicy1 := sriov.NewPolicyBuilder(APIClient,
		sriovNet1Name,
		vcoreparams.SRIOVNamespace,
		net1ResourceName,
		6,
		snnpPfname1,
		VCoreConfig.VCorePpLabelMap)
	if !sriovNetworkNodePolicy1.Exists() {
		sriovNetworkNodePolicy1, err = sriovNetworkNodePolicy1.Create()
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("failed to create SR-IOV network node policy %s in namespace %s due to %v",
				sriovNet1Name, vcoreparams.SRIOVNamespace, err))
		Expect(sriovNetworkNodePolicy1.Exists()).To(Equal(true),
			fmt.Sprintf("SR-IOV network node policy %s in namespace %s not found",
				sriovNet1Name, vcoreparams.SRIOVNamespace))
	}

	klog.V(vcoreparams.VCoreLogLevel).Infof("Create SR-IOV network node policy %s in namespace %s",
		sriovNet2Name, vcoreparams.SRIOVNamespace)

	sriovNetworkNodePolicy2 := sriov.NewPolicyBuilder(APIClient,
		sriovNet2Name,
		vcoreparams.SRIOVNamespace,
		net1ResourceName,
		6,
		snnpPfname2,
		VCoreConfig.VCorePpLabelMap)
	if !sriovNetworkNodePolicy2.Exists() {
		sriovNetworkNodePolicy2, err = sriovNetworkNodePolicy2.Create()
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("failed to create SR-IOV network node policy %s in namespace %s due to %v",
				sriovNet2Name, vcoreparams.SRIOVNamespace, err))
		Expect(sriovNetworkNodePolicy2.Exists()).To(Equal(true),
			fmt.Sprintf("SR-IOV network node policy %s in namespace %s not found",
				sriovNet2Name, vcoreparams.SRIOVNamespace))
	}

	klog.V(vcoreparams.VCoreLogLevel).Info("Wait for all node reach the Ready state")

	_, err = nodes.WaitForAllNodesAreReady(APIClient, 10*time.Minute)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error waiting for all nodes reach the Ready state: %v", err))

	klog.V(vcoreparams.VCoreLogLevel).Info("Wait for all clusteroperators availability")

	_, err = clusteroperator.WaitForAllClusteroperatorsAvailable(APIClient, 60*time.Second)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error waiting for all available clusteroperators: %v", err))
} // func VerifySRIOVConfig (ctx SpecContext)
