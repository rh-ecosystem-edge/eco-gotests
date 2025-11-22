package vcorecommon

import (
	"context"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/apiobjectshelper"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nmstate"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreparams"
)

// VerifyNMStateSuite container that contains tests for NMState verification.
func VerifyNMStateSuite() {
	Describe(
		"NMState validation",
		Label(vcoreparams.LabelVCoreOperators), func() {
			It(fmt.Sprintf("Verifies %s namespace exists", VCoreConfig.NMStateOperatorNamespace),
				Label("nmstate"), VerifyNMStateNamespaceExists)

			It("Verifies NMState operator deployment succeeded",
				Label("nmstate"), reportxml.ID("67027"), VerifyNMStateCSVConditionSucceeded)

			It("Verifies NMState instance exists",
				Label("nmstate"), reportxml.ID("67027"), VerifyNMStateInstanceExists)
		})
}

// VerifyNMStateNamespaceExists asserts namespace for NMState operator exists.
func VerifyNMStateNamespaceExists(ctx SpecContext) {
	err := apiobjectshelper.VerifyNamespaceExists(APIClient, VCoreConfig.NMStateOperatorNamespace, time.Second)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to pull namespace %q; %v", VCoreConfig.NMStateOperatorNamespace, err))
} // func VerifyNMStateNamespaceExists (ctx SpecContext)

// VerifyNMStateCSVConditionSucceeded assert that NMState operator deployment succeeded.
func VerifyNMStateCSVConditionSucceeded(ctx SpecContext) {
	err := apiobjectshelper.VerifyOperatorDeployment(APIClient,
		vcoreparams.NMStateOperatorName,
		vcoreparams.NMStateDeploymentName,
		VCoreConfig.NMStateOperatorNamespace,
		time.Minute)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("NMState operator deployment %s failure in the namespace %s; %v",
			vcoreparams.NMStateOperatorName, VCoreConfig.NMStateOperatorNamespace, err))
} // func VerifyNMStateCSVConditionSucceeded (ctx SpecContext)

// VerifyNMStateInstanceExists assert that NMState instance exists.
func VerifyNMStateInstanceExists(ctx SpecContext) {
	klog.V(vcoreparams.VCoreLogLevel).Infof("Verify NMState instance exists")

	err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 1*time.Minute, true,
		func(ctx context.Context) (bool, error) {
			_, pullErr := nmstate.PullNMstate(APIClient, vcoreparams.NMStateInstanceName)
			if pullErr != nil {
				klog.V(vcoreparams.VCoreLogLevel).Infof("Failed to pull in NMState instance %q due to %v",
					vcoreparams.NMStateInstanceName, pullErr)

				return false, pullErr
			}

			return true, nil
		})

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to pull in NMState instance %q; %v", vcoreparams.NMStateInstanceName, err))
} // func VerifyNMStateInstanceExists (ctx SpecContext)
