package tests

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/gitopsztp/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	"k8s.io/klog/v2"
)

var _ = Describe("ZTP Spoke Checker Tests", Label(tsparams.LabelSpokeCheckerTests), func() {
	When("a TunedPerformancePatch.yaml PGT disables the chronyd service", func() {
		// 54237 - Moving chronyd disable to tuned patch
		It("should disable and inactivate the chronyd service on spoke", reportxml.ID("54237"), func() {
			By("verifying chronyd is inactive")
			// Use `| cat -` to explicitly ignore the return code since it will be nonzero when the service
			// is not active, which is expected here.
			statuses, err := cluster.ExecCmdWithStdoutWithRetries(Spoke1APIClient,
				ranparam.RetryCount, ranparam.RetryInterval,
				"systemctl is-active chronyd.service | cat -")
			Expect(err).ToNot(HaveOccurred(), "Failed to check if chronyd service is active")
			Expect(statuses).ToNot(BeEmpty(), "Failed to find statuses for chronyd service")

			for nodeName, status := range statuses {
				klog.V(tsparams.LogLevel).Infof("%s active status: %s", nodeName, status)

				status = strings.TrimSpace(status)
				Expect(status).To(Equal("inactive"), "chronyd service was not inactive")
			}

			By("verifying chronyd is disabled")
			// Use `| cat -` to explicitly ignore the return code since it will be nonzero when the service
			// is disabled, which is expected here.
			statuses, err = cluster.ExecCmdWithStdoutWithRetries(Spoke1APIClient,
				ranparam.RetryCount, ranparam.RetryInterval,
				"systemctl is-enabled chronyd.service | cat -")
			Expect(err).ToNot(HaveOccurred(), "Failed to check if chronyd service is enabled")
			Expect(statuses).ToNot(BeEmpty(), "Failed to find statuses for chronyd service")

			for nodeName, status := range statuses {
				klog.V(tsparams.LogLevel).Infof("%s enabled status: %s", nodeName, status)

				status = strings.TrimSpace(status)
				Expect(status).To(Equal("disabled"), "chronyd service was not disabled")
			}

		})

		// 60904 - Verifies list of pods in openshift-network-diagnostics namespace on spoke
		It("should not have pods in the network diagnostics namespace", reportxml.ID("60904"), func() {
			pods, err := pod.List(Spoke1APIClient, tsparams.NetworkDiagnosticsNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to list pods in the network diagnostics namespace")
			Expect(pods).To(BeEmpty(), "Found pods in the network diagnostics namespace")
		})

		// 60905 - Verifies list of pods in openshift-console namespace on spoke
		It("should not have pods in the console namespace", reportxml.ID("60905"), func() {
			pods, err := pod.List(Spoke1APIClient, tsparams.ConsoleNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to list pods in the console namespace")
			Expect(pods).To(BeEmpty(), "Found pods in the console namespace")
		})
	})
})
