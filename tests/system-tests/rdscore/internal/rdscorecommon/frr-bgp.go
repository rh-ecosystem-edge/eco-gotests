package rdscorecommon

import (
	"fmt"
	"slices"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/klog/v2"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"
)

// ReachURLviaFRRroute test URL via route learned by MetalLB FRR.
func ReachURLviaFRRroute(ctx SpecContext) {
	By("Asserting if test URL is provided")

	if RDSCoreConfig.MetalLBFRRTestURLIPv4 == "" && RDSCoreConfig.MetalLBFRRTestURLIPv6 == "" {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Test URLs for MetalLB FRR testing not specified or are empty. Skipping...")
		Skip("Test URL for MetalLB FRR testing not specified or are empty")
	}

	By("Finding MetalLB-FRR pods")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Searching for pods in %q namespace with %q label",
		RDSCoreConfig.MetalLBFRRNamespace, rdscoreparams.MetalLBFRRPodSelector)

	frrPodList := findPodWithSelector(RDSCoreConfig.MetalLBFRRNamespace,
		rdscoreparams.MetalLBFRRPodSelector)

	Expect(len(frrPodList)).ToNot(Equal(0), "No MetalLB FRR pods found")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Found %d 'frr' pods", len(frrPodList))

	for _, _pod := range frrPodList {
		for _, testURL := range []string{RDSCoreConfig.MetalLBFRRTestURLIPv4, RDSCoreConfig.MetalLBFRRTestURLIPv6} {
			if testURL == "" {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Empty URL continue")

				continue
			}

			if len(RDSCoreConfig.FRRExpectedNodes) != 0 {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("User specified list of FRR nodes present: %q",
					RDSCoreConfig.FRRExpectedNodes)

				if !slices.Contains(RDSCoreConfig.FRRExpectedNodes, _pod.Definition.Spec.NodeName) {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod %q(%q) runs on not user expected node %q. Skipping",
						_pod.Definition.Name, _pod.Definition.Namespace, _pod.Definition.Spec.NodeName)

					continue
				}
			}

			cmd := fmt.Sprintf("curl -Ls --max-time 5 -o /dev/null -w '%%{http_code}' %s", testURL)

			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Running command %q from within pod %s",
				cmd, _pod.Definition.Name)

			Eventually(func() bool {
				output, err := _pod.ExecCommand([]string{"/bin/sh", "-c", cmd},
					rdscoreparams.MetalLBFRRContainerName)
				if err != nil {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to run command due to: %v", err)

					return false
				}

				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Command's Output:\n%v\n", output.String())

				codesPattern := "200 404"

				return strings.Contains(codesPattern, strings.Trim(output.String(), "'"))
			}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(1*time.Minute).Should(BeTrue(),
				"Error fetching outside URL from within pod")
		}
	}
}
