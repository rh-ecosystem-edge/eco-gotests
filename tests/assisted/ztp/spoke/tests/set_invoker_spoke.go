package spoke_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/ztpinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/spoke/internal/tsparams"
)

var _ = Describe(
	"DataInvoker",
	Label(tsparams.LabelInvokerVerificationTestCases), func() {
		When("on MCE 2.0 and above", func() {

			It("Assert the invoker is set with the proper value",
				reportxml.ID("43613"), func() {
					configMapName := "openshift-install-manifests"
					nameSpaceName := "openshift-config"
					expectedInvoker := "assisted-installer-operator"
					invokerKeyName := "invoker"

					By("Assure the configmap can be pulled")
					configMapBuilder, err := configmap.Pull(SpokeAPIClient, configMapName, nameSpaceName)
					Expect(err).ToNot(HaveOccurred(),
						"error pulling configmap %s in ns %s", configMapName, nameSpaceName)

					By("Assure the configmap has the proper key with a value")
					invoker, keyExists := configMapBuilder.Object.Data[invokerKeyName]
					Expect(keyExists).To(BeTrue(), "invoker key does not exist in configmap")
					Expect(invoker).To(Equal(expectedInvoker), "error matching the invoker's value to the expected one")
				})

		})
	})
