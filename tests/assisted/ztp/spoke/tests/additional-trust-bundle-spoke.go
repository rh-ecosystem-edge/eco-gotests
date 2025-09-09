package spoke_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/ztpinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/spoke/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
)

var (
	trustBundle string
)
var _ = Describe(
	"AdditionalTrustBundle",
	Ordered,
	ContinueOnFailure,
	Label(tsparams.LabelAdditionalTrustBundle), func() {
		When("on MCE 2.4 and above", func() {
			BeforeAll(func() {
				if len(ZTPConfig.SpokeInfraEnv.Object.Spec.AdditionalTrustBundle) == 0 {
					Skip("spoke cluster was not installed with additional trust bundle")
				}
				trustBundle = ZTPConfig.SpokeInfraEnv.Object.Spec.AdditionalTrustBundle
			})
			It("Assure trust bundle exists on all nodes", reportxml.ID("67492"), func() {
				shellCmd := "cat /etc/pki/ca-trust/source/anchors/openshift-config-user-ca-bundle.crt"
				cmdResult, err := cluster.ExecCmdWithStdout(SpokeAPIClient, shellCmd)
				Expect(err).
					ToNot(HaveOccurred(), "error getting openshift-config-user-ca-bundle.crt content")
				for _, stdout := range cmdResult {
					Expect(stdout).To(
						ContainSubstring(trustBundle),
						"crt file does not contain additional certificate")
				}
			})
		})
	})
