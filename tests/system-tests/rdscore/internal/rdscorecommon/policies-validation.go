package rdscorecommon

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"
)

// ValidateAllPoliciesCompliant asserts all policies are compliant.
func ValidateAllPoliciesCompliant() {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Check all policies are compliant")

	var (
		allPolicies []*ocm.PolicyBuilder
		err         error
		ctx         SpecContext
	)

	Eventually(func() bool {
		allPolicies, err = ocm.ListPoliciesInAllNamespaces(APIClient,
			runtimeclient.ListOptions{Namespace: RDSCoreConfig.PolicyNS})
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Error listing policies: %v", err)

			return false
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Found %d policies", len(allPolicies))

		return true
	}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(1*time.Minute).Should(BeTrue(),
		"Failed to list policies")

	Expect(len(allPolicies)).NotTo(Equal(0), "Found 0 policies")

	NonCompliant := []string{}

	for _, policy := range allPolicies {
		pName := policy.Definition.Name
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Processing policy is %q", pName)

		pState := string(policy.Object.Status.ComplianceState)

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Policy %q is in %q state", pName, pState)

		if pState != "Compliant" {
			NonCompliant = append(NonCompliant, pName)
		}
	}

	Expect(len(NonCompliant)).To(BeZero(),
		fmt.Sprintf("There are %d NonCompliant policies", len(NonCompliant)))
}
