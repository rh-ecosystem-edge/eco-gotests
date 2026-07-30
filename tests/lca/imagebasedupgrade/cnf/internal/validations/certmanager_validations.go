package cnfibuvalidations

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	corev1 "k8s.io/api/core/v1"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfclusterinfo"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfinittools"
)

// Cert-manager namespace constants duplicated from tests/system-tests/internal/certmanager/constants.go
// because Go internal package visibility prevents import from tests/lca/.
const (
	certManagerOperatorNS = "cert-manager-operator"
	certManagerCoreNS     = "cert-manager"
)

// ValidateCertManagerPersistence checks that cert-manager workload certificates,
// secrets, and CRs survive the IBU process.
func ValidateCertManagerPersistence() {
	// 89049 - Validate cert-manager workload certificate persistence after IBU
	It("Validate cert-manager workload certificate persistence after IBU",
		reportxml.ID("89049"),
		Label("ValidateCertManagerPersistence"), func() {
			preInfo := cnfclusterinfo.PreUpgradeClusterInfo.CertManager
			postInfo := cnfclusterinfo.PostUpgradeClusterInfo.CertManager

			if preInfo.TLSCertChecksum == "" {
				Skip("cert-manager workload certificate not found pre-upgrade, skipping")
			}

			By("Verifying cert-manager operator pods are running after upgrade")

			operatorPods, err := pod.ListByNamePattern(
				TargetSNOAPIClient, "cert-manager-operator-controller-manager", certManagerOperatorNS)
			Expect(err).ToNot(HaveOccurred(), "Failed to list cert-manager-operator pods")
			Expect(operatorPods).ToNot(BeEmpty(),
				"No cert-manager-operator controller-manager pod found after upgrade")

			for _, p := range operatorPods {
				Expect(p.Object.Status.Phase).To(Equal(corev1.PodRunning),
					"cert-manager-operator pod %s is not Running", p.Object.Name)
			}

			By("Verifying cert-manager core pods are running after upgrade")

			corePrefixes := []string{"cert-manager-", "cert-manager-cainjector-", "cert-manager-webhook-"}
			for _, prefix := range corePrefixes {
				pods, podErr := pod.ListByNamePattern(TargetSNOAPIClient, prefix, certManagerCoreNS)
				Expect(podErr).ToNot(HaveOccurred(),
					"Failed to list cert-manager pods with prefix %s", prefix)
				Expect(pods).ToNot(BeEmpty(),
					"No cert-manager pod with prefix %s found after upgrade", prefix)

				for _, p := range pods {
					Expect(p.Object.Status.Phase).To(Equal(corev1.PodRunning),
						"cert-manager pod %s is not Running", p.Object.Name)
				}
			}

			By("Verifying ClusterIssuer is still Ready after upgrade")

			Expect(postInfo.IssuerReady).To(BeTrue(),
				"ClusterIssuer is not Ready after upgrade")

			By("Verifying workload certificate is still Ready after upgrade")

			Expect(postInfo.CertReady).To(BeTrue(),
				"Workload certificate is not Ready after upgrade")

			By(fmt.Sprintf("Comparing TLS secret checksums: pre=%s post=%s",
				preInfo.TLSCertChecksum, postInfo.TLSCertChecksum))

			Expect(postInfo.TLSCertChecksum).To(Equal(preInfo.TLSCertChecksum),
				"TLS certificate checksum changed during IBU — certificate was not preserved")

			By(fmt.Sprintf("Comparing TLS certificate serial numbers: pre=%s post=%s",
				preInfo.TLSCertSerial, postInfo.TLSCertSerial))

			Expect(postInfo.TLSCertSerial).To(Equal(preInfo.TLSCertSerial),
				"TLS certificate serial number changed during IBU — certificate was re-issued")
		})
}
