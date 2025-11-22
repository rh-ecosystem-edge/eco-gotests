package tests

import (
	"fmt"

	"regexp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/diskencryption/internal/diskencryptioninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/diskencryption/internal/helper"
	stdinmatcher "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/diskencryption/internal/stdin-matcher"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/diskencryption/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/file"
	"k8s.io/klog/v2"
)

var _ = Describe("TPM2", func() {
	var (
		err error
	)

	// BeforeEach is preparing the cluster before the tests can be run.
	// In case the cluster is in a bad state from previously running tests,
	// recovery is attempted as follows:
	// - If Secure Boot is enabled, the disk should be able to be decrypted.
	// - If not, enable it and power cycle. Otherwise the cluster will not be able to come back online
	// - If the cluster does not come back online, maybe the boot order was changed. Change it back
	// and power cycle
	// - Finally, clean the "/etc/host-hw-Updating.flag" file and wait for the cluster to come back
	// Once up check that the cluster is SNO, etc
	BeforeEach(func() {
		if BMCClient == nil {
			Skip("collecting power usage metrics requires the BMC configuration be set.")
		}

		By("checking if Secure Boot is enabled")
		var isSecureBootEnabled bool

		Eventually(func() error {
			isSecureBootEnabled, err = BMCClient.IsSecureBootEnabled()

			return err
		}).WithTimeout(tsparams.TimeoutWaitingOnBMC).
			WithPolling(tsparams.PollingIntervalBMC).ShouldNot(HaveOccurred(),
			" IsSecureBootEnabled should not return an error")

		if !isSecureBootEnabled {

			By("enabling SecureBoot")
			Eventually(BMCClient.SecureBootEnable).WithTimeout(tsparams.TimeoutWaitingOnBMC).
				WithPolling(tsparams.PollingIntervalBMC).
				Should(Or(Succeed(), MatchError("secure boot is already enabled")),
					"enabling Secure Boot should succeed even if it is already enabled")

			By("restarting node")

			Eventually(BMCClient.SystemPowerCycle).WithTimeout(tsparams.TimeoutWaitingOnBMC).
				WithPolling(tsparams.PollingIntervalBMC).ShouldNot(HaveOccurred(),
				"power cycling node should succeed")
		}

		By("waiting until all spoke 1 pods are ready")
		err = cluster.WaitForRecover(APIClient, []string{
			DiskEncryptionTestConfig.GeneralConfig.MCONamespace}, tsparams.TimeoutClusterRecovery)

		if err != nil {

			By("swapping back boot order, the server did not come up, maybe the boot order was swapped before")
			swapFirstSecondBootItems()

			By("restarting node")
			Eventually(BMCClient.SystemPowerCycle).WithTimeout(tsparams.TimeoutWaitingOnBMC).
				WithPolling(tsparams.PollingIntervalBMC).ShouldNot(HaveOccurred(),
				"power cycling node should succeed")

			By("waiting cluster recovery")
			Eventually(cluster.WaitForRecover).WithTimeout(tsparams.TimeoutClusterRecovery).
				WithPolling(tsparams.RetryInterval).
				WithArguments(APIClient, []string{
					DiskEncryptionTestConfig.GeneralConfig.MCONamespace}, tsparams.TimeoutClusterRecovery).
				ShouldNot(HaveOccurred(),
					"the cluster should be up before continuing")
		}

		// After this point the cluster should be back up.
		var nodeList []*nodes.Builder
		Eventually(func() int {
			nodeList, err = nodes.List(APIClient)
			Expect(err).ToNot(HaveOccurred(), "error listing nodes")

			return len(nodeList)
		}).WithTimeout(tsparams.TimeoutWaitingOnK8S).
			WithPolling(tsparams.PollingIntervalK8S).
			Should(BeNumerically("==", 1), "Currently only SNO clusters are supported")

		var isTTYConsole bool
		Eventually(func() error {
			isTTYConsole, err = helper.IsTTYConsole()
			Expect(err).ToNot(HaveOccurred(), "error checking kernel command line for tty console")

			return err
		}).WithTimeout(tsparams.TimeoutWaitingOnK8S).
			WithPolling(tsparams.PollingIntervalK8S).
			ShouldNot(HaveOccurred(), "Currently only SNO clusters are supported")

		Expect(isTTYConsole).To(BeTrue(), "the TTY options should be configured on the kernel"+
			" boot line (nomodeset console=tty0 console=ttyS0,115200n8)")

		Eventually(file.DeleteFile).WithArguments("/etc/host-hw-Updating.flag").
			WithTimeout(tsparams.TimeoutWaitingOnK8S).
			WithPolling(tsparams.PollingIntervalK8S).ShouldNot(HaveOccurred(),
			"error deleting /etc/host-hw-Updating.flag file")

		By("backing up TPM max failed retries ")
		Eventually(func() error {
			tsparams.OriginalTPMMaxRetries, err = helper.GetTPMMaxRetries()

			return err
		}).WithTimeout(tsparams.TimeoutWaitingOnK8S).
			WithPolling(tsparams.PollingIntervalK8S).ShouldNot(HaveOccurred(),
			"error getting TPM max retries")

		By(fmt.Sprintf("listing original TPM Max Failed Retries: %d", tsparams.OriginalTPMMaxRetries))

		By("setting max retries in test environment")
		Eventually(helper.SetTPMMaxRetries).WithArguments(tsparams.TestTPMMaxRetries).
			WithTimeout(tsparams.TimeoutWaitingOnK8S).
			WithPolling(tsparams.PollingIntervalK8S).ShouldNot(HaveOccurred(),
			"error setting TPM max retries")

		By("resetting lockout counter in test environment")
		Eventually(helper.SetTPMLockoutCounterZero).
			WithTimeout(tsparams.TimeoutWaitingOnK8S).
			WithPolling(tsparams.PollingIntervalK8S).ShouldNot(HaveOccurred(),
			"error resetting TPM lockout counter to zero")

		By("verifying max retries in test environment")
		var maxRetriesCheck int64
		Eventually(func() error {
			maxRetriesCheck, err = helper.GetTPMMaxRetries()

			return err
		}).WithTimeout(tsparams.TimeoutWaitingOnK8S).
			WithPolling(tsparams.PollingIntervalK8S).ShouldNot(HaveOccurred(),
			"error getting TPM max retries")

		By(fmt.Sprintf("verifying TPM Max Failed Retries: %d", maxRetriesCheck))

		By("verifying lockout counter in test environment")
		var lockoutCounterCheck int64
		Eventually(func() error {
			lockoutCounterCheck, err = helper.GetTPMLockoutCounter()

			return err
		}).WithTimeout(tsparams.TimeoutWaitingOnK8S).
			WithPolling(tsparams.PollingIntervalK8S).ShouldNot(HaveOccurred(),
			"error getting lockout counter")

		By(fmt.Sprintf("verifying Test lockout counter: %d", lockoutCounterCheck))

	})

	AfterEach(func() {
		By("waiting for the cluster to become unreachable")
		err = cluster.WaitForUnreachable(APIClient, tsparams.TimeoutClusterUnreachable)
		Expect(err).ToNot(HaveOccurred(), "error waiting for cluster to be unreachable")

		By("waiting for cluster recovery after test")
		Eventually(cluster.WaitForRecover).WithTimeout(tsparams.TimeoutClusterRecovery).
			WithPolling(tsparams.RetryInterval).
			WithArguments(APIClient, []string{
				DiskEncryptionTestConfig.GeneralConfig.MCONamespace}, tsparams.TimeoutClusterRecovery).
			ShouldNot(HaveOccurred(),
				"the cluster should be up before continuing")

		By(fmt.Sprintf("restoring max retries to initial value: %d", tsparams.OriginalTPMMaxRetries))
		Eventually(helper.SetTPMMaxRetries).WithTimeout(tsparams.TimeoutWaitingOnK8S).
			WithPolling(tsparams.RetryInterval).
			WithArguments(tsparams.OriginalTPMMaxRetries).
			ShouldNot(HaveOccurred(),
				"setting original TPM max retries should succeed")
	})

	It("Verifies that disabling Secure Boot prevents"+
		" Disk decryption (TPM PCR 7)", func() {

		By("checking that Root disk is encrypted with tpm2 with PCR 1 and 7")
		var luksListOutput string

		Eventually(func() error {
			luksListOutput, err = helper.GetClevisLuksListOutput()

			return err
		}).WithTimeout(tsparams.TimeoutWaitingOnK8S).
			WithPolling(tsparams.PollingIntervalK8S).ShouldNot(HaveOccurred(),
			"error getting of clevis luks list command")

		isRootDiskTPM2PCR1AND7 := helper.LuksListContainsPCR1And7(luksListOutput)
		Expect(err).ToNot(HaveOccurred(), "error when checking if the root disk is configured with TPM 1 and 7")

		if !isRootDiskTPM2PCR1AND7 {
			Skip("Root disk is not encrypted with PCR 1 and 7, skip TPM1 tests")
		}

		By("checking if Secure Boot is enabled")
		var isSecureBootEnabled bool

		Eventually(func() error {
			isSecureBootEnabled, err = BMCClient.IsSecureBootEnabled()

			return err
		}).WithTimeout(tsparams.TimeoutWaitingOnBMC).
			WithPolling(tsparams.PollingIntervalBMC).ShouldNot(HaveOccurred(),
			"IsSecureBootEnabled should not return an error")

		if !isSecureBootEnabled {
			Skip("Secure boot is not enabled test cannot proceed")
		}

		By("checking that the reserved slot is not present")
		Eventually(func() error {
			luksListOutput, err = helper.GetClevisLuksListOutput()

			return err
		}).WithTimeout(tsparams.TimeoutWaitingOnK8S).
			WithPolling(tsparams.PollingIntervalK8S).ShouldNot(HaveOccurred(),
			"error getting of clevis luks list command")

		isRootDiskReservedSlotPresent := helper.LuksListContainsReservedSlot(luksListOutput)
		Expect(isRootDiskReservedSlotPresent).To(BeFalse(), "there should be no reserved slot present at this point")

		By("disabling Secure Boot")
		Eventually(BMCClient.SecureBootDisable).WithTimeout(tsparams.TimeoutWaitingOnBMC).
			WithPolling(tsparams.PollingIntervalBMC).ShouldNot(HaveOccurred(),
			"disabling Secure Boot should not return an error")

		By("restarting node gracefully")
		Eventually(cluster.SoftRebootSNO).WithArguments(APIClient, tsparams.RetryCount, tsparams.RetryInterval).
			WithTimeout(tsparams.TimeoutWaitingOnK8S).
			WithPolling(tsparams.PollingIntervalK8S).ShouldNot(HaveOccurred(),
			"error rebooting node")

		By("waiting for Disk decryption Failure log to appear")
		matches := []stdinmatcher.Matcher{
			{
				Regex: regexp.MustCompile("TPM failed"),
				Times: 10,
			},
			{
				Regex: regexp.MustCompile("pcr-rebind-boot"),
				Times: 1,
			},
		}

		matchIndex, err := stdinmatcher.WaitForRegex(BMCClient, matches, tsparams.TimeoutWaitRegex)

		Expect(err).ToNot(HaveOccurred(), "WaitForRegex should not fail")
		Expect(matchIndex).To(Equal(0), "WaitForRegex should match TPM failed (0)")

		By("enabling SecureBoot")
		Eventually(BMCClient.SecureBootEnable).WithTimeout(tsparams.TimeoutWaitingOnBMC).
			WithPolling(tsparams.PollingIntervalBMC).
			Should(Or(Succeed(), MatchError("secure boot is already enabled")),
				"enabling Secure Boot should not return an error")

		By("powercycling node")
		Eventually(BMCClient.SystemPowerCycle).WithTimeout(tsparams.TimeoutWaitingOnBMC).
			WithPolling(tsparams.PollingIntervalBMC).ShouldNot(HaveOccurred(),
			"power cycling the node should succeed")

	})

	It("Verifies update indication with file (/etc/host-hw-Updating.flag)", func() {

		By("checking that Root disk is encrypted with tpm2 with PCR 1 and 7")
		var luksListOutput string
		Eventually(func() error {
			luksListOutput, err = helper.GetClevisLuksListOutput()

			return err
		}).WithTimeout(tsparams.TimeoutWaitingOnK8S).
			WithPolling(tsparams.PollingIntervalK8S).ShouldNot(HaveOccurred(),
			"error getting of clevis luks list command")

		isRootDiskTPM2PCR1AND7 := helper.LuksListContainsPCR1And7(luksListOutput)
		if !isRootDiskTPM2PCR1AND7 {
			Skip("Root disk is not encrypted with PCR 1 and 7, skip TPM1 tests")
		}

		By("checking if Secure Boot is enabled")
		var isSecureBootEnabled bool

		Eventually(func() error {
			isSecureBootEnabled, err = BMCClient.IsSecureBootEnabled()

			return err
		}).WithTimeout(tsparams.TimeoutWaitingOnBMC).
			WithPolling(tsparams.PollingIntervalBMC).ShouldNot(HaveOccurred(),
			"IsSecureBootEnabled should not return an error")

		if !isSecureBootEnabled {
			Skip("Secure boot is not enabled test cannot proceed")
		}

		By("checking that the reserved slot is not present")
		Eventually(func() error {
			luksListOutput, err = helper.GetClevisLuksListOutput()

			return err
		}).WithTimeout(tsparams.TimeoutWaitingOnK8S).
			WithPolling(tsparams.PollingIntervalK8S).ShouldNot(HaveOccurred(),
			"error getting of clevis luks list command")

		isRootDiskReservedSlotPresent := helper.LuksListContainsReservedSlot(luksListOutput)
		Expect(isRootDiskReservedSlotPresent).To(BeFalse(), "there should be no reserved slot present at this point")

		By("touching upgrade indication file")
		Eventually(file.TouchFile).WithArguments("/etc/host-hw-Updating.flag").
			WithTimeout(tsparams.TimeoutWaitingOnK8S).
			WithPolling(tsparams.PollingIntervalK8S).ShouldNot(HaveOccurred(),
			"the /etc/host-hw-Updating.flag file should be created without error")

		By("disabling Secure Boot")
		Eventually(BMCClient.SecureBootDisable).WithTimeout(tsparams.TimeoutWaitingOnBMC).
			WithPolling(tsparams.PollingIntervalBMC).ShouldNot(HaveOccurred(),
			"SecureBootDisable should not return an error")

		By("restarting node gracefully")
		Eventually(cluster.SoftRebootSNO).WithArguments(APIClient, tsparams.RetryCount, tsparams.RetryInterval).
			WithTimeout(tsparams.TimeoutWaitingOnK8S).
			WithPolling(tsparams.PollingIntervalK8S).ShouldNot(HaveOccurred(),
			"error rebooting node")

		By("waiting for pcr-rebind-boot log to appear (disk decryption succeeded)")

		matches := []stdinmatcher.Matcher{
			{
				Regex: regexp.MustCompile("TPM failed"),
				Times: 10,
			},
			{
				Regex: regexp.MustCompile("pcr-rebind-boot"),
				Times: 1,
			},
		}

		matchIndex, err := stdinmatcher.WaitForRegex(BMCClient, matches, tsparams.TimeoutWaitRegex)

		Expect(err).ToNot(HaveOccurred(), "WaitForRegex should not fail")
		Expect(matchIndex).To(Equal(1), "WaitForRegex should match pcr-rebind-boot (1)")

		By("waiting for cluster to recover")
		err = cluster.WaitForRecover(APIClient, []string{DiskEncryptionTestConfig.GeneralConfig.MCONamespace,
			DiskEncryptionTestConfig.GeneralConfig.MCONamespace}, tsparams.TimeoutClusterRecovery)
		Expect(err).ToNot(HaveOccurred(), "cluster should recover without error")

		By("enabling SecureBoot")
		Eventually(BMCClient.SecureBootEnable).WithTimeout(tsparams.TimeoutWaitingOnBMC).
			WithPolling(tsparams.PollingIntervalBMC).
			Should(Or(Succeed(), MatchError("secure boot is already enabled")),
				"enabling Secure Boot should succeed, even if it is already enabled ")

		By("restarting node gracefully")
		Eventually(cluster.SoftRebootSNO).WithArguments(APIClient, tsparams.RetryCount, tsparams.RetryInterval).
			WithTimeout(tsparams.TimeoutWaitingOnK8S).
			WithPolling(tsparams.PollingIntervalK8S).ShouldNot(HaveOccurred(),
			"error rebooting node")
	})

	It("Verifies that changing Host boot order prevents Disk decryption (TPM PCR 1)", func() {

		By("checking that Root disk is encrypted with tpm2 with PCR 1 and 7")
		var luksListOutput string
		Eventually(func() error {
			luksListOutput, err = helper.GetClevisLuksListOutput()

			return err
		}).WithTimeout(tsparams.TimeoutWaitingOnK8S).
			WithPolling(tsparams.PollingIntervalK8S).ShouldNot(HaveOccurred(),
			"error getting of clevis luks list command")

		isRootDiskTPM2PCR1AND7 := helper.LuksListContainsPCR1And7(luksListOutput)
		if !isRootDiskTPM2PCR1AND7 {
			Skip("Root disk is not encrypted with PCR 1 and 7, skip TPM1 tests")
		}

		By("checking if Secure Boot is enabled")
		var isSecureBootEnabled bool

		Eventually(func() error {
			isSecureBootEnabled, err = BMCClient.IsSecureBootEnabled()

			return err
		}).WithTimeout(tsparams.TimeoutWaitingOnBMC).
			WithPolling(tsparams.PollingIntervalBMC).ShouldNot(HaveOccurred(),
			"IsSecureBootEnabled should not return an error")

		if !isSecureBootEnabled {
			Skip("Secure boot is not enabled test cannot proceed")
		}

		By("checking that the reserved slot is not present")
		Eventually(func() error {
			luksListOutput, err = helper.GetClevisLuksListOutput()

			return err
		}).WithTimeout(tsparams.TimeoutWaitingOnK8S).
			WithPolling(tsparams.PollingIntervalK8S).ShouldNot(HaveOccurred(),
			"error getting of clevis luks list command")

		isRootDiskReservedSlotPresent := helper.LuksListContainsReservedSlot(luksListOutput)
		Expect(isRootDiskReservedSlotPresent).To(BeFalse(), "there should be no reserved slot present at this point")

		By("saving the original server boot order")
		var originalBootOrder []string
		Eventually(func() error {
			originalBootOrder, err = BMCClient.SystemBootOrderReferences()

			if len(originalBootOrder) < 2 {
				return fmt.Errorf("need at least 2 boot entries")
			}

			return err
		}).WithTimeout(tsparams.TimeoutWaitingOnBMC).
			WithPolling(tsparams.PollingIntervalBMC).
			ShouldNot(HaveOccurred(), "getting boot order should not return an error")

		klog.V(tsparams.LogLevel).Infof("listing original boot Order: %s", originalBootOrder)

		By("changing the server boot order")
		swapFirstSecondBootItems()

		By("restarting node gracefully")
		Eventually(cluster.SoftRebootSNO).WithArguments(APIClient, tsparams.RetryCount, tsparams.RetryInterval).
			WithTimeout(tsparams.TimeoutWaitingOnK8S).
			WithPolling(tsparams.PollingIntervalK8S).ShouldNot(HaveOccurred(),
			"error rebooting node")

		By("waiting for TPM Failed log to appear (disk decryption failed)")
		matches := []stdinmatcher.Matcher{
			{
				Regex: regexp.MustCompile("TPM failed"),
				Times: 10,
			},
			{
				Regex: regexp.MustCompile("pcr-rebind-boot"),
				Times: 1,
			},
		}

		matchIndex, err := stdinmatcher.WaitForRegex(BMCClient, matches, tsparams.TimeoutWaitRegex)

		Expect(err).ToNot(HaveOccurred(), "WaitForRegex should not fail")
		Expect(matchIndex).To(Equal(0), "WaitForRegex should match TPM failed (0)")

		By("changing the server boot order back to original settings")
		Eventually(BMCClient.SetSystemBootOrderReferences).WithTimeout(tsparams.TimeoutWaitingOnBMC).
			WithPolling(tsparams.PollingIntervalBMC).WithArguments(originalBootOrder).ShouldNot(HaveOccurred(),
			"changing boot order should not return an error")

		By("power cycling node")
		Eventually(BMCClient.SystemPowerCycle).WithTimeout(tsparams.TimeoutWaitingOnBMC).
			WithPolling(tsparams.PollingIntervalBMC).ShouldNot(HaveOccurred(),
			"power cycling the node should not return an error")

	})
})

func swapFirstSecondBootItems() {
	var bootRefs []string

	var err error

	Eventually(func() error {
		bootRefs, err = BMCClient.SystemBootOrderReferences()

		if len(bootRefs) < 2 {
			return fmt.Errorf("need at least 2 boot entries")
		}

		return err
	}).WithTimeout(tsparams.TimeoutWaitingOnBMC).
		WithPolling(tsparams.PollingIntervalBMC).
		ShouldNot(HaveOccurred(), "getting boot order should not return an error")

	By(fmt.Sprintf("listing current boot Order: %s", bootRefs))
	// Creates a boot override swapping first and second boot references
	var bootOverride []string

	bootOverride, err = helper.SwapFirstAndSecondSliceItems(bootRefs)
	Expect(err).ToNot(HaveOccurred(), "SwapFirstAndSecondSliceItems should not fail")
	By(fmt.Sprintf("setting new boot Order: %s", bootOverride))
	Eventually(BMCClient.SetSystemBootOrderReferences).WithTimeout(tsparams.TimeoutWaitingOnBMC).
		WithPolling(tsparams.PollingIntervalBMC).WithArguments(bootOverride).ShouldNot(HaveOccurred(),
		"changing boot order should not return an error")
}
