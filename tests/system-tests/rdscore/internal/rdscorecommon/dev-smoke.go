package rdscorecommon

import (
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
)

// VerifyDevSmokeSanity prints visible output and checks that the Kubernetes API answers.
// Use label dev-smoke to run only this test while setting up the environment.
func VerifyDevSmokeSanity(ctx SpecContext) {
	By("Confirm API client is initialized")
	Expect(APIClient).NotTo(BeNil(), "APIClient must be set; check KUBECONFIG")

	_, err := fmt.Fprintf(GinkgoWriter, "\n[rdscore dev-smoke] eco-gotests harness OK — contacting cluster API...\n")
	Expect(err).NotTo(HaveOccurred())

	By("List namespaces (limit 1) to verify connectivity")
	_, err = APIClient.Namespaces().List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		msg := fmt.Sprintf("[rdscore dev-smoke] List namespaces failed: %v", err)
		_, _ = fmt.Fprintln(GinkgoWriter, msg)
		_, _ = fmt.Fprintln(os.Stderr, msg)
	}

	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf(
		"cluster API unreachable from this machine: %v — check VPN, kubeconfig server URL, or run tests from a host that can reach the API (e.g. jumphost)", err))

	klog.Infof("[rdscore dev-smoke] Kubernetes API responded successfully")
	_, err = fmt.Fprintf(GinkgoWriter, "[rdscore dev-smoke] cluster API responded — smoke test passed\n\n")
	Expect(err).NotTo(HaveOccurred())
}
