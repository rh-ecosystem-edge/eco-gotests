package rdscorecommon

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	goclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nmstate"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"
)

// VerifyNMStateNamespaceExists asserts namespace for NMState operator exists.
func VerifyNMStateNamespaceExists(ctx SpecContext) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Verify namespace %q exists",
		RDSCoreConfig.NMStateOperatorNamespace)

	err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 1*time.Minute, true,
		func(ctx context.Context) (bool, error) {
			_, pullErr := namespace.Pull(APIClient, RDSCoreConfig.NMStateOperatorNamespace)
			if pullErr != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
					"Failed to pull in namespace %q - %v",
					RDSCoreConfig.NMStateOperatorNamespace, pullErr)

				return false, pullErr
			}

			return true, nil
		})

	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to pull %q namespace", RDSCoreConfig.NMStateOperatorNamespace))
}

// VerifyNMStateInstanceExists assert that NMState instance exists.
func VerifyNMStateInstanceExists(ctx SpecContext) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Verify NMState instance exists")

	err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 1*time.Minute, true,
		func(ctx context.Context) (bool, error) {
			_, pullErr := nmstate.PullNMstate(APIClient, rdscoreparams.NMStateInstanceName)
			if pullErr != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
					"Failed to pull in NMState instance %q - %v",
					rdscoreparams.NMStateInstanceName, pullErr)

				return false, pullErr
			}

			return true, nil
		})

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to pull in NMState instance %q", rdscoreparams.NMStateInstanceName))
}

// VerifyAllNNCPsAreOK assert all available NNCPs are Available, not progressing and not degraded.
func VerifyAllNNCPsAreOK(ctx SpecContext) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Verify NodeNetworkConfigurationPolicies are Available")

	const ConditionTypeTrue = "True"

	nncps, err := nmstate.ListPolicy(APIClient, goclient.ListOptions{})
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to list NodeNetworkConfigurationPolicies: %v", err))
	Expect(len(nncps)).ToNot(Equal(0), "0 NodeNetworkConfigurationPolicies found")

	nonAvailableNNCP := make(map[string]string)
	progressingNNCP := make(map[string]string)
	degradedNNCP := make(map[string]string)

	for _, nncp := range nncps {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(
			fmt.Sprintf("\t Processing %s NodeNetworkConfigurationPolicy", nncp.Definition.Name))

		for _, condition := range nncp.Object.Status.Conditions {
			//nolint:nolintlint
			switch condition.Type { //nolint:exhaustive
			//nolint:goconst
			case "Available":
				if condition.Status != ConditionTypeTrue {
					nonAvailableNNCP[nncp.Definition.Name] = condition.Message
					klog.V(rdscoreparams.RDSCoreLogLevel).Info(
						fmt.Sprintf("\t%s NNCP is not Available: %s\n", nncp.Definition.Name, condition.Message))
				} else {
					klog.V(rdscoreparams.RDSCoreLogLevel).Info(
						fmt.Sprintf("\t%s NNCP is Available: %s\n", nncp.Definition.Name, condition.Message))
				}
			case "Degraded":
				if condition.Status == ConditionTypeTrue {
					degradedNNCP[nncp.Definition.Name] = condition.Message
					klog.V(rdscoreparams.RDSCoreLogLevel).Info(
						fmt.Sprintf("\t%s NNCP is Degraded: %s\n", nncp.Definition.Name, condition.Message))
				} else {
					klog.V(rdscoreparams.RDSCoreLogLevel).Info(
						fmt.Sprintf("\t%s NNCP is Not-Degraded\n", nncp.Definition.Name))
				}
			case "Progressing":
				if condition.Status == ConditionTypeTrue {
					progressingNNCP[nncp.Definition.Name] = condition.Message
					klog.V(rdscoreparams.RDSCoreLogLevel).Info(
						fmt.Sprintf("\t%s NNCP is Progressing: %s\n", nncp.Definition.Name, condition.Message))
				} else {
					klog.V(rdscoreparams.RDSCoreLogLevel).Info(
						fmt.Sprintf("\t%s NNCP is Not-Progressing\n", nncp.Definition.Name))
				}
			}
		}
	}

	Expect(len(nonAvailableNNCP)).To(Equal(0), "There are NonAvailable NodeNetworkConfigurationPolicies")
	Expect(len(degradedNNCP)).To(Equal(0), "There are Degraded NodeNetworkConfigurationPolicies")
	Expect(len(nonAvailableNNCP)).To(Equal(0), "There are Progressing NodeNetworkConfigurationPolicies")
} // func VerifyNNCP (ctx SpecContext)

// VerifyNMStateSuite container that contains tests for NMState verification.
func VerifyNMStateSuite() {
	Describe(
		"NMState validation",
		Label(rdscoreparams.LabelValidateNMState), func() {
			It(fmt.Sprintf("Verifies %s namespace exists", RDSCoreConfig.NMStateOperatorNamespace),
				Label("nmstate-ns"), VerifyNMStateNamespaceExists)

			It("Verifies NMState instance exists",
				Label("nmstate-instance"), reportxml.ID("67027"), VerifyNMStateInstanceExists)

			It("Verifies all NodeNetworkConfigurationPolicies are Available",
				Label("nmstate-nncp"), reportxml.ID("71846"), VerifyAllNNCPsAreOK)
		})
}
