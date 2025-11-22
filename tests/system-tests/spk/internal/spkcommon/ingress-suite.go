package spkcommon

import (
	"fmt"
	"math/rand"
	"net"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/url"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/spk/internal/spkinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/spk/internal/spkparams"
)

//nolint:unparam
func reachURL(targetURL, pollInterval, pollDuration string, expectedCode int) {
	klog.V(spkparams.SPKLogLevel).Infof("Accessing %q via SPK Ingress", targetURL)

	var ctx SpecContext

	pollFrequency, err := time.ParseDuration(pollInterval)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to parse polling interval %q", pollInterval))

	pollTimeout, err := time.ParseDuration(pollDuration)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to parse polling interval %q", pollDuration))

	Eventually(func() bool {
		data, httpCode, err := url.Fetch(targetURL, "GET", true)
		if err != nil {
			klog.V(spkparams.SPKLogLevel).Infof("Failed to reach %q: %v", targetURL, err)

			return false
		}

		klog.V(spkparams.SPKLogLevel).Infof("Reached URL %q", targetURL)
		klog.V(spkparams.SPKLogLevel).Infof("HTTP Code: %d", httpCode)
		klog.V(spkparams.SPKLogLevel).Infof("Reached data\n%v", data)

		return httpCode == expectedCode
	}).WithContext(ctx).WithPolling(pollFrequency).WithTimeout(pollTimeout).Should(BeTrue(),
		fmt.Sprintf("Failed to reach %q URL", targetURL))
}

// AssertIPv4WorkloadURL access workload via IPv4 address.
func AssertIPv4WorkloadURL(ctx SpecContext) {
	klog.V(spkparams.SPKLogLevel).Infof("*** AssertIPv4WorkloadURL ***")

	if SPKConfig.IngressTCPIPv4URL == "" {
		Skip("IPv4 URL for SPK backed workload not defined")
	}

	reachURL(SPKConfig.IngressTCPIPv4URL, "10s", "3m", int(200))
}

// AssertIPv4WorkloadURLAfterAppRecreated access workload via IPv4 address,
// after target workload was re-created.
func AssertIPv4WorkloadURLAfterAppRecreated(ctx SpecContext) {
	klog.V(spkparams.SPKLogLevel).Infof("*** AssertIPv4WorkloadURL ***")

	if SPKConfig.IngressTCPIPv4URL == "" {
		Skip("IPv4 URL for SPK backed workload not defined")
	}

	SetupSPKBackendWorkload()

	reachURL(SPKConfig.IngressTCPIPv4URL, "15s", "5m", int(200))
}

// AssertIPv6WorkloadURL access workload via IPv6 address.
func AssertIPv6WorkloadURL(ctx SpecContext) {
	klog.V(spkparams.SPKLogLevel).Infof("*** AssertIPv6WorkloadURL ***")

	if SPKConfig.IngressTCPIPv6URL == "" {
		Skip("IPv6 URL for SPK backed workload not defined")
	}

	reachURL(SPKConfig.IngressTCPIPv6URL, "5s", "3m", int(200))
}

// AssertIPv6WorkloadURLAfterAppRecreated access workload via IPv6 address,
// after target workload was re-created.
func AssertIPv6WorkloadURLAfterAppRecreated(ctx SpecContext) {
	klog.V(spkparams.SPKLogLevel).Infof("*** AssertIPv6WorkloadURL ***")

	if SPKConfig.IngressTCPIPv6URL == "" {
		Skip("IPv6 URL for SPK backed workload not defined")
	}

	SetupSPKBackendWorkload()
	reachURL(SPKConfig.IngressTCPIPv6URL, "15s", "5m", int(200))
}

// AssertIPv4WorkloadURLAfterIngressPodDeleted assert workoads are reachable over IPv4 SPK Ingress,
// after SPK Ingress pods are deleted.
func AssertIPv4WorkloadURLAfterIngressPodDeleted(ctx SpecContext) {
	if SPKConfig.IngressTCPIPv4URL == "" {
		Skip("IPv4 URL for SPK backed workload not defined. Skipping")
	}

	deletePodMatchingLabel(SPKConfig.SPKDataNS, ingressDataLabel, "3m")
	deletePodMatchingLabel(SPKConfig.SPKDnsNS, ingressDNSLabel, "3m")

	dataPods := findPodWithSelector(SPKConfig.SPKDataNS, ingressDataLabel, "3s", "60s")

	for _, dPod := range dataPods {
		err := dPod.WaitUntilReady(5 * time.Minute)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Pod %q is not Ready", dPod.Definition.Name))
	}

	dnsPods := findPodWithSelector(SPKConfig.SPKDnsNS, ingressDNSLabel, "3s", "60s")

	for _, dPod := range dnsPods {
		err := dPod.WaitUntilReady(5 * time.Minute)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Pod %q is not Ready", dPod.Definition.Name))
	}

	// klog.V(spkparams.SPKLogLevel).Infof("Sleeping for 4 minutes")
	// time.Sleep(4 * time.Minute)

	AssertIPv4WorkloadURL(ctx)
}

// AssertIPv6WorkloadURLAfterIngressPodDeleted assert workoads are reachable over IPv6 SPK Ingress,
// after SPK Ingress pods are deleted.
func AssertIPv6WorkloadURLAfterIngressPodDeleted(ctx SpecContext) {
	if SPKConfig.IngressTCPIPv6URL == "" {
		Skip("IPv6 URL for SPK backed workload not defined. Skipping")
	}

	deletePodMatchingLabel(SPKConfig.SPKDataNS, ingressDataLabel, "3m")
	deletePodMatchingLabel(SPKConfig.SPKDnsNS, ingressDNSLabel, "3m")

	dataPods := findPodWithSelector(SPKConfig.SPKDataNS, ingressDataLabel, "3s", "60s")

	for _, dPod := range dataPods {
		err := dPod.WaitUntilReady(5 * time.Minute)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Pod %q is not Ready", dPod.Definition.Name))
	}

	dnsPods := findPodWithSelector(SPKConfig.SPKDnsNS, ingressDNSLabel, "3s", "60s")

	for _, dPod := range dnsPods {
		err := dPod.WaitUntilReady(5 * time.Minute)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Pod %q is not Ready", dPod.Definition.Name))
	}

	klog.V(spkparams.SPKLogLevel).Infof("Sleeping for 4 minutes")
	time.Sleep(4 * time.Minute)

	AssertIPv6WorkloadURL(ctx)
}

// AssertIPv4UDPWorkloadURLAfterIngressPodDeleted assert workoads are reachable over IPv4 SPK Ingress,
// after SPK Ingress pods are deleted.
func AssertIPv4UDPWorkloadURLAfterIngressPodDeleted(ctx SpecContext) {
	if SPKConfig.IngressUDPIPv4URL == "" {
		klog.V(spkparams.SPKLogLevel).Infof("IPv4 URL for SPK UDP backed workload not defined")
		Skip("IPv4 URL for SPK UDP backed workload not defined. Skipping")
	}

	deletePodMatchingLabel(SPKConfig.SPKDataNS, ingressDataLabel, "3m")
	deletePodMatchingLabel(SPKConfig.SPKDnsNS, ingressDNSLabel, "3m")

	dataPods := findPodWithSelector(SPKConfig.SPKDataNS, ingressDataLabel, "5s", "60s")

	for _, dPod := range dataPods {
		err := dPod.WaitUntilReady(5 * time.Minute)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Pod %q is not Ready", dPod.Definition.Name))
	}

	dnsPods := findPodWithSelector(SPKConfig.SPKDnsNS, ingressDNSLabel, "5s", "60s")

	for _, dPod := range dnsPods {
		err := dPod.WaitUntilReady(5 * time.Minute)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Pod %q is not Ready", dPod.Definition.Name))
	}

	VerifySPKIngressUDPviaIPv4()
}

// AssertIPv6UDPWorkloadURLAfterIngressPodDeleted assert workoads are reachable over IPv6 SPK Ingress,
// after SPK Ingress pods are deleted.
func AssertIPv6UDPWorkloadURLAfterIngressPodDeleted(ctx SpecContext) {
	if SPKConfig.IngressUDPIPv6URL == "" {
		Skip("IPv6 UDP URL for SPK backed workload not defined. Skipping")
	}

	deletePodMatchingLabel(SPKConfig.SPKDataNS, ingressDataLabel, "3m")
	deletePodMatchingLabel(SPKConfig.SPKDnsNS, ingressDNSLabel, "3m")

	dataPods := findPodWithSelector(SPKConfig.SPKDataNS, ingressDataLabel, "5s", "60s")

	for _, dPod := range dataPods {
		err := dPod.WaitUntilReady(5 * time.Minute)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Pod %q is not Ready", dPod.Definition.Name))
	}

	deletePodMatchingLabel(SPKConfig.SPKDnsNS, tmmLabel, "5m")

	dnsPods := findPodWithSelector(SPKConfig.SPKDnsNS, ingressDNSLabel, "5s", "60s")

	for _, dPod := range dnsPods {
		err := dPod.WaitUntilReady(5 * time.Minute)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Pod %q is not Ready", dPod.Definition.Name))
	}

	VerifySPKIngressUDPviaIPv6()
}

func getPodLogs(podObj *pod.Builder, cName string, timeSpan time.Time) string {
	klog.V(spkparams.SPKLogLevel).Infof("Parsing duration %q", timeSpan)

	var (
		podLog string
		err    error
		ctx    SpecContext
	)

	Eventually(func() bool {
		logStartTimestamp := time.Since(timeSpan)
		klog.V(spkparams.SPKLogLevel).Infof("\tTime duration is %s", logStartTimestamp)

		if logStartTimestamp.Abs().Seconds() < 1 {
			logStartTimestamp, err = time.ParseDuration("1s")
			Expect(err).ToNot(HaveOccurred(), "Failed to parse time duration")
		}

		podLog, err = podObj.GetLog(logStartTimestamp, cName)
		if err != nil {
			klog.V(spkparams.SPKLogLevel).Infof("Failed to get logs from pod %q: %v", podObj.Definition.Name, err)

			return false
		}

		klog.V(spkparams.SPKLogLevel).Infof("Logs from pod %s:\n%s", podObj.Definition.Name, podLog)

		return true
	}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(1*time.Minute).Should(BeTrue(),
		fmt.Sprintf("Failed to get logs from pod %q", podObj.Definition.Name))

	return podLog
}

func findPodWithSelector(fNamespace, podLabel, pollInterval, pollDuration string) []*pod.Builder {
	By(fmt.Sprintf("Getting pod(s) matching selector %q", podLabel))

	var (
		podMatchingSelector []*pod.Builder
		err                 error
		ctx                 SpecContext
	)

	pollFrequencey, err := time.ParseDuration(pollInterval)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to parse polling interval %q", pollInterval))

	pollTimeout, err := time.ParseDuration(pollDuration)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to parse polling interval %q", pollDuration))

	podOneSelector := metav1.ListOptions{
		LabelSelector: podLabel,
	}

	Eventually(func() bool {
		podMatchingSelector, err = pod.List(APIClient, fNamespace, podOneSelector)
		if err != nil {
			klog.V(spkparams.SPKLogLevel).Infof("Failed to list pods in %q namespace: %v",
				fNamespace, err)

			return false
		}

		if len(podMatchingSelector) == 0 {
			klog.V(spkparams.SPKLogLevel).Infof("Found 0 pods matching label %q in namespace %q",
				podLabel, fNamespace)

			return false
		}

		return true
	}).WithContext(ctx).WithPolling(pollFrequencey).WithTimeout(pollTimeout).Should(BeTrue(),
		fmt.Sprintf("Failed to find pod matching label %q in %q namespace", podLabel, fNamespace))

	return podMatchingSelector
}

// VerifySPKIngressUDPviaIPv4 verifies SPK UDP Ingress.
func VerifySPKIngressUDPviaIPv4() {
	klog.V(spkparams.SPKLogLevel).Infof("*** Verify SPK UPD Ingress over IPv4 ***")

	if SPKConfig.IngressUDPIPv4URL == "" {
		klog.V(spkparams.SPKLogLevel).Infof("IPv4 URL for SPK UDP backed workload not defined")
		Skip("IPv4 URL for SPK UDP backed workload not defined")
	}

	verifyUDPIngress(SPKConfig.IngressUDPIPv4URL, "15s", "11m")
}

// VerifySPKIngressUDPviaIPv6 verifies SPK UDP Ingress.
func VerifySPKIngressUDPviaIPv6() {
	klog.V(spkparams.SPKLogLevel).Infof("*** Verify SPK UPD Ingress over IPv6 ***")

	if SPKConfig.IngressUDPIPv6URL == "" {
		klog.V(spkparams.SPKLogLevel).Infof("IPv6 URL for SPK UDP backed workload not defined")
		Skip("IPv6 URL for SPK UDP backed workload not defined")
	}

	verifyUDPIngress(SPKConfig.IngressUDPIPv6URL, "15s", "11m")
}

func verifyUDPIngress(udpAddr, pollInterval, pollDuration string) {
	var (
		err      error
		ctx      SpecContext
		bWritten int
		rNum     int
	)

	pollFrequency, err := time.ParseDuration(pollInterval)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to parse polling interval %q", pollInterval))

	pollTimeout, err := time.ParseDuration(pollDuration)

	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to parse polling interval %q", pollDuration))

	// depending on the time when workload is started and data is sent
	// there's possible race condition, hence random sleep.
	rSrc := rand.NewSource(time.Now().Unix())
	rSrc.Seed(time.Now().Unix())

	Eventually(func() bool {
		rNum = rand.Intn(3)

		return rNum != 0
	}).WithContext(ctx).WithPolling(5*time.Millisecond).WithTimeout(1*time.Second).Should(BeTrue(),
		"Failed to generate pseudo number")

	rSleep, err := time.ParseDuration(fmt.Sprintf("%ds", rNum))

	Expect(err).ToNot(HaveOccurred(), "Failed to parse time duration")

	klog.V(spkparams.SPKLogLevel).Infof("Sleeping for %s", rSleep.String())
	time.Sleep(rSleep)

	Eventually(func() bool {
		klog.V(spkparams.SPKLogLevel).Infof("Dialing to UDP endpoint %q", udpAddr)

		udpConnection, err := net.Dial("udp", udpAddr)

		Expect(err).ToNot(HaveOccurred(), "Failed to Dial to UDP endpoint")

		defer udpConnection.Close()

		By("Looking for UDP server pods")

		udpPods := findPodWithSelector(SPKConfig.Namespace, SPKBackendUDPSelector, "5s", "1m")

		timeStart := time.Now()
		udpMSG := fmt.Sprintf("UDP message sent at %d", time.Now().Unix())

		klog.V(spkparams.SPKLogLevel).Infof("Sending message: %q(%d bytes)", udpMSG, len([]byte(udpMSG)))

		bWritten, err = udpConnection.Write([]byte(udpMSG))
		if err != nil {
			klog.V(spkparams.SPKLogLevel).Infof("Failed to send message: %v", err)

			return false
		}

		if bWritten != len([]byte(udpMSG)) {
			klog.V(spkparams.SPKLogLevel).Infof("Sent %d bytes via UDP, expected %d",
				bWritten, len([]byte(udpMSG)))

			return false
		}

		klog.V(spkparams.SPKLogLevel).Infof("Successfully sent %d bytes via UDP", bWritten)

		for _, udpPod := range udpPods {
			klog.V(spkparams.SPKLogLevel).Infof("Checking logs in %q", udpPod.Definition.Name)
			logMsg := getPodLogs(udpPod, udpPod.Definition.Spec.Containers[0].Name, timeStart)

			return strings.Contains(logMsg, udpMSG)
		}

		return false
	}).WithContext(ctx).WithPolling(pollFrequency).WithTimeout(pollTimeout).Should(BeTrue(),
		"Failed to send message via UDP")
}

// AssertIPv4WorkloadURLAfterTMMPodDeleted assert connectivity to workload running on OCP cluster,
// after TMM pods are deleted.
func AssertIPv4WorkloadURLAfterTMMPodDeleted(ctx SpecContext) {
	if SPKConfig.IngressTCPIPv4URL == "" {
		Skip("IPv4 URL for SPK backed workload not defined. Skipping")
	}

	deletePodMatchingLabel(SPKConfig.SPKDataNS, tmmLabel, "5m")

	dataPods := findPodWithSelector(SPKConfig.SPKDataNS, tmmLabel, "3s", "60s")

	for _, dPod := range dataPods {
		err := dPod.WaitUntilReady(5 * time.Minute)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Pod %q is not Ready", dPod.Definition.Name))
	}

	deletePodMatchingLabel(SPKConfig.SPKDnsNS, tmmLabel, "5m")

	dnsPods := findPodWithSelector(SPKConfig.SPKDnsNS, tmmLabel, "3s", "60s")

	for _, dPod := range dnsPods {
		err := dPod.WaitUntilReady(5 * time.Minute)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Pod %q is not Ready", dPod.Definition.Name))
	}

	klog.V(spkparams.SPKLogLevel).Infof("Sleeping for 4 minutes")
	time.Sleep(4 * time.Minute)

	AssertIPv4WorkloadURL(ctx)
}

// AssertIPv6WorkloadURLAfterTMMPodDeleted assert workoads are reachable over IPv6 SPK Ingress,
// after SPK TMM pods are deleted.
func AssertIPv6WorkloadURLAfterTMMPodDeleted(ctx SpecContext) {
	if SPKConfig.IngressTCPIPv6URL == "" {
		Skip("IPv6 URL for SPK backed workload not defined. Skipping")
	}

	deletePodMatchingLabel(SPKConfig.SPKDataNS, tmmLabel, "5m")

	dataPods := findPodWithSelector(SPKConfig.SPKDataNS, tmmLabel, "3s", "60s")

	for _, dPod := range dataPods {
		err := dPod.WaitUntilReady(5 * time.Minute)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Pod %q is not Ready", dPod.Definition.Name))
	}

	deletePodMatchingLabel(SPKConfig.SPKDnsNS, tmmLabel, "5m")

	dnsPods := findPodWithSelector(SPKConfig.SPKDnsNS, tmmLabel, "3s", "60s")

	for _, dPod := range dnsPods {
		err := dPod.WaitUntilReady(5 * time.Minute)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Pod %q is not Ready", dPod.Definition.Name))
	}

	klog.V(spkparams.SPKLogLevel).Infof("Sleeping for 4 minutes")
	time.Sleep(4 * time.Minute)

	AssertIPv6WorkloadURL(ctx)
}

// AssertIPv4UDPWorkloadURLAfterTMMPodDeleted assert workoads are reachable over IPv4 SPK Ingress,
// after SPK TMM pods are deleted.
func AssertIPv4UDPWorkloadURLAfterTMMPodDeleted(ctx SpecContext) {
	if SPKConfig.IngressUDPIPv4URL == "" {
		klog.V(spkparams.SPKLogLevel).Infof("IPv4 URL for SPK UDP backed workload not defined")
		Skip("IPv4 URL for SPK UDP backed workload not defined. Skipping")
	}

	deletePodMatchingLabel(SPKConfig.SPKDataNS, tmmLabel, "5m")

	dataPods := findPodWithSelector(SPKConfig.SPKDataNS, tmmLabel, "5s", "60s")

	for _, dPod := range dataPods {
		err := dPod.WaitUntilReady(5 * time.Minute)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Pod %q is not Ready", dPod.Definition.Name))
	}

	deletePodMatchingLabel(SPKConfig.SPKDnsNS, tmmLabel, "5m")

	dnsPods := findPodWithSelector(SPKConfig.SPKDnsNS, tmmLabel, "5s", "60s")

	for _, dPod := range dnsPods {
		err := dPod.WaitUntilReady(5 * time.Minute)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Pod %q is not Ready", dPod.Definition.Name))
	}

	VerifySPKIngressUDPviaIPv4()
}

// AssertIPv6UDPWorkloadURLAfterTMMPodDeleted assert workoads are reachable over IPv6 SPK Ingress,
// after SPK TMM pods are deleted.
func AssertIPv6UDPWorkloadURLAfterTMMPodDeleted(ctx SpecContext) {
	if SPKConfig.IngressUDPIPv6URL == "" {
		Skip("IPv6 UDP URL for SPK backed workload not defined. Skipping")
	}

	deletePodMatchingLabel(SPKConfig.SPKDataNS, tmmLabel, "5m")

	dataPods := findPodWithSelector(SPKConfig.SPKDataNS, tmmLabel, "5s", "60s")

	for _, dPod := range dataPods {
		err := dPod.WaitUntilReady(5 * time.Minute)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Pod %q is not Ready", dPod.Definition.Name))
	}

	deletePodMatchingLabel(SPKConfig.SPKDnsNS, tmmLabel, "5m")

	dnsPods := findPodWithSelector(SPKConfig.SPKDnsNS, tmmLabel, "5s", "60s")

	for _, dPod := range dnsPods {
		err := dPod.WaitUntilReady(5 * time.Minute)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Pod %q is not Ready", dPod.Definition.Name))
	}

	VerifySPKIngressUDPviaIPv6()
}

// RestartSPKIngressPods delete SPK Ingress pods.
func RestartSPKIngressPods() {
	deletePodMatchingLabel(SPKConfig.SPKDataNS, ingressDataLabel, "3m")
	deletePodMatchingLabel(SPKConfig.SPKDnsNS, ingressDNSLabel, "3m")

	dataPods := findPodWithSelector(SPKConfig.SPKDataNS, ingressDataLabel, "3s", "60s")

	for _, dPod := range dataPods {
		err := dPod.WaitUntilReady(5 * time.Minute)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Pod %q is not Ready", dPod.Definition.Name))
	}

	dnsPods := findPodWithSelector(SPKConfig.SPKDnsNS, ingressDNSLabel, "3s", "60s")

	for _, dPod := range dnsPods {
		err := dPod.WaitUntilReady(5 * time.Minute)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Pod %q is not Ready", dPod.Definition.Name))
	}
}
