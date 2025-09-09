package tests

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	performancev2 "github.com/openshift/cluster-node-tuning-operator/pkg/apis/performanceprofile/v2"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nto"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/powermanagement/internal/helper"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/powermanagement/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/cpuset"
)

var _ = Describe("CPU frequency tuning tests change the core frequencies of isolated and reserved cores",
	Label(tsparams.LabelCPUFrequency), func() {
		var (
			perfProfile             *nto.Builder
			desiredReservedCoreFreq = performancev2.CPUfrequency(2500002)
			desiredIsolatedCoreFreq = performancev2.CPUfrequency(2200002)
			originalIsolatedCPUFreq performancev2.CPUfrequency
			originalReservedCPUFreq performancev2.CPUfrequency
			err                     error
		)

		BeforeEach(func() {
			perfProfile, err = helper.GetPerformanceProfileWithCPUSet()
			Expect(err).ToNot(HaveOccurred(), "Failed to get performance profile")

			By("getting isolated core ID")
			isolatedCPUSet, err := cpuset.Parse(string(*perfProfile.Object.Spec.CPU.Isolated))
			Expect(err).ToNot(HaveOccurred(), "Failed to get isolated cpu set")
			isolatedCPUsList := isolatedCPUSet.List()
			isolatedCPUNumber := isolatedCPUsList[0]

			By("getting reserved core ID")
			reservedCPUSet, err := cpuset.Parse(string(*perfProfile.Object.Spec.CPU.Reserved))
			Expect(err).ToNot(HaveOccurred(), "Failed to get reserved cpu set")
			reservedCPUsList := reservedCPUSet.List()
			reservedCPUNumber := reservedCPUsList[0]

			By("getting original isolated core frequency")
			originalIsolatedCPUFreq = getCPUFreq(isolatedCPUNumber)

			By("getting original reserved core frequency")
			originalReservedCPUFreq = getCPUFreq(reservedCPUNumber)
		})

		AfterEach(func() {
			By("reverting the CPU frequencies to the original setting")
			err := helper.SetCPUFreq(perfProfile, &originalIsolatedCPUFreq, &originalReservedCPUFreq)
			Expect(err).ToNot(HaveOccurred(), "Failed to set CPU Freq")
		})

		When("reserved and isolated core frequency is configured via PerformanceProfile", func() {
			It("sets the reserved and isolated core frequency correctly on the DUT", func() {

				versionInRange, err := version.IsVersionStringInRange(RANConfig.Spoke1OCPVersion, "4.16", "")
				Expect(err).ToNot(HaveOccurred(), "Failed to compare OCP version string")

				if !versionInRange {
					Skip("OCP 4.16 or higher required for this test")
				}

				err = helper.SetCPUFreq(perfProfile, &desiredIsolatedCoreFreq, &desiredReservedCoreFreq)
				Expect(err).ToNot(HaveOccurred(), "Failed to set CPU Freq")

			})
		})
	})

// getCPUFreq gets the current frequency of a given CPU core.
func getCPUFreq(coreID int) performancev2.CPUfrequency {
	spokeCommand := fmt.Sprintf("cat /sys/devices/system/cpu/cpufreq/policy%v/scaling_max_freq",
		coreID)

	var frequency int

	// Retry if we cannot convert the frequency to an int, since this usually means malformed output. Errors in
	// command execution are actual errors though and are returned immediately.
	err := wait.PollUntilContextTimeout(
		context.TODO(), time.Second, 10*time.Second, true, func(ctx context.Context) (bool, error) {
			cmdOut, err := cluster.ExecCommandOnSNOWithRetries(Spoke1APIClient, ranparam.RetryCount,
				ranparam.RetryInterval, spokeCommand)
			if err != nil {
				return false, err
			}

			frequency, err = strconv.Atoi(strings.TrimSpace(cmdOut))
			if err != nil {
				return false, nil
			}

			return true, nil
		})

	Expect(err).ToNot(HaveOccurred(), "Failed to check cpu %d frequency", coreID)

	return performancev2.CPUfrequency(frequency)
}
