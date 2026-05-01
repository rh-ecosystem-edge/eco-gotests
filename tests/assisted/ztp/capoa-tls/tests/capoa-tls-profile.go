package capoa_tls_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/capoa-tls/internal/inittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/capoa-tls/internal/tsparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	capoaNS                    = "multicluster-engine"
	bootstrapDeploy            = "capoa-bootstrap-controller-manager"
	ctrlplaneDeploy            = "capoa-controlplane-controller-manager"
	bootstrapWebhookSvc        = "capoa-bootstrap-webhook-service"
	ctrlplaneWebhookSvc        = "capoa-controlplane-webhook-service"
	webhookPort                = 9443
	managerContainer           = "manager"
	honoringLogPattern         = "honoring cluster-wide TLS profile"
	capoapodReadyTimeout       = 5 * time.Minute
	capoapodAutoRestartTimeout = 10 * time.Minute
)

var _ = Describe(
	"CAPOATLSProfile",
	Ordered, ContinueOnFailure,
	Label(tsparams.LabelCAPOATLSProfile), func() {
		BeforeAll(func() {
			By("Verifying hub API client is available")

			if HubAPIClient == nil {
				Skip("Hub API client is nil")
			}

			By("Verifying TLS adherence is enabled on the cluster")

			apiserverU := &unstructured.Unstructured{}
			apiserverU.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   "config.openshift.io",
				Version: "v1",
				Kind:    "APIServer",
			})

			err := HubAPIClient.Get(context.TODO(), runtimeclient.ObjectKey{Name: "cluster"}, apiserverU)
			Expect(err).ToNot(HaveOccurred(), "failed to get apiserver/cluster")

			tlsAdherence, _, _ := unstructured.NestedString(apiserverU.Object, "spec", "tlsAdherence")
			if tlsAdherence != "StrictAllComponents" {
				Skip("TLS adherence is not set to StrictAllComponents on apiserver/cluster (got: " + tlsAdherence + ")")
			}

			By("Verifying CAPOA pods are running")

			pods, err := pod.ListByNamePattern(HubAPIClient, "capoa", capoaNS)
			Expect(err).ToNot(HaveOccurred(), "failed to list CAPOA pods")

			if len(pods) == 0 {
				Skip("No CAPOA pods found in namespace " + capoaNS + " — CAPOA not deployed")
			}

			waitCAPOAPodsReady()
		})

		AfterAll(func() {
			By("Restoring default Intermediate TLS profile")
			removeAPIServerTLSProfile()
			restartCAPOAPods()
		})

		It("Verifies default Intermediate TLS profile on CAPOA webhooks",
			reportxml.ID("88843"), func() {
				By("Confirming no tlsSecurityProfile is set on apiserver")
				removeAPIServerTLSProfile()
				restartCAPOAPods()

				By("Verifying controller logs show honoring message")
				assertControllerLogsContain(bootstrapDeploy, honoringLogPattern)
				assertControllerLogsContain(ctrlplaneDeploy, honoringLogPattern)

				By("Port-forwarding to bootstrap webhook and probing TLS 1.2")
				assertTLSConnects(bootstrapWebhookSvc, tls.VersionTLS12, tls.VersionTLS12, nil)

				By("Probing TLS 1.3 on bootstrap webhook")
				assertTLSConnects(bootstrapWebhookSvc, tls.VersionTLS13, tls.VersionTLS13, nil)

				By("Port-forwarding to controlplane webhook and probing TLS 1.2")
				assertTLSConnects(ctrlplaneWebhookSvc, tls.VersionTLS12, tls.VersionTLS12, nil)

				By("Probing TLS 1.3 on controlplane webhook")
				assertTLSConnects(ctrlplaneWebhookSvc, tls.VersionTLS13, tls.VersionTLS13, nil)

				By("Verifying TLS 1.1 is rejected on bootstrap webhook")
				assertTLSRejectedVersion(bootstrapWebhookSvc, tls.VersionTLS11)

				By("Verifying TLS 1.0 is rejected on bootstrap webhook")
				assertTLSRejectedVersion(bootstrapWebhookSvc, tls.VersionTLS10)
			})

		It("Verifies Old TLS profile enables broader cipher set",
			reportxml.ID("88844"), func() {
				By("Applying Old TLS profile")
				patchAPIServerTLSProfile(configv1.TLSSecurityProfile{
					Type: configv1.TLSProfileOldType,
					Old:  &configv1.OldTLSProfile{},
				})

				By("Restarting CAPOA pods to pick up Old profile")
				restartCAPOAPods()

				By("Verifying controller logs show VersionTLS10")
				assertControllerLogsContain(bootstrapDeploy, "VersionTLS10")
				assertControllerLogsContain(ctrlplaneDeploy, "VersionTLS10")

				By("Verifying Old-specific cipher connects on bootstrap webhook")
				assertTLSConnects(bootstrapWebhookSvc, tls.VersionTLS12, tls.VersionTLS12,
					[]uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256})

				By("Verifying Old-specific cipher connects on controlplane webhook")
				assertTLSConnects(ctrlplaneWebhookSvc, tls.VersionTLS12, tls.VersionTLS12,
					[]uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256})
			})

		It("Verifies Modern TLS profile restricts to TLS 1.3 only",
			reportxml.ID("88845"), func() {
				By("Applying Modern TLS profile")
				patchAPIServerTLSProfile(configv1.TLSSecurityProfile{
					Type:   configv1.TLSProfileModernType,
					Modern: &configv1.ModernTLSProfile{},
				})

				By("Restarting CAPOA pods to pick up Modern profile")
				restartCAPOAPods()

				By("Verifying controller logs show VersionTLS13")
				assertControllerLogsContain(bootstrapDeploy, "VersionTLS13")
				assertControllerLogsContain(ctrlplaneDeploy, "VersionTLS13")

				By("Verifying TLS 1.3 connects on bootstrap webhook")
				assertTLSConnects(bootstrapWebhookSvc, tls.VersionTLS13, tls.VersionTLS13, nil)

				By("Verifying TLS 1.2 is rejected on bootstrap webhook")
				assertTLSRejected(bootstrapWebhookSvc, nil)

				By("Verifying TLS 1.3 connects on controlplane webhook")
				assertTLSConnects(ctrlplaneWebhookSvc, tls.VersionTLS13, tls.VersionTLS13, nil)

				By("Verifying TLS 1.2 is rejected on controlplane webhook")
				assertTLSRejected(ctrlplaneWebhookSvc, nil)
			})

		It("Verifies Custom TLS profile restricts to specified ciphers",
			reportxml.ID("88846"), func() {
				By("Applying Custom TLS profile with 2 ciphers")
				patchAPIServerTLSProfile(configv1.TLSSecurityProfile{
					Type: configv1.TLSProfileCustomType,
					Custom: &configv1.CustomTLSProfile{
						TLSProfileSpec: configv1.TLSProfileSpec{
							Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256", "ECDHE-RSA-AES256-GCM-SHA384"},
							MinTLSVersion: configv1.VersionTLS12,
						},
					},
				})

				By("Restarting CAPOA pods to pick up Custom profile")
				restartCAPOAPods()

				By("Verifying allowed cipher connects on bootstrap webhook")
				assertTLSConnects(bootstrapWebhookSvc, tls.VersionTLS12, tls.VersionTLS12,
					[]uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256})

				By("Verifying allowed cipher connects on controlplane webhook")
				assertTLSConnects(ctrlplaneWebhookSvc, tls.VersionTLS12, tls.VersionTLS12,
					[]uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256})

				By("Verifying disallowed cipher is rejected on bootstrap webhook")
				assertTLSRejected(bootstrapWebhookSvc,
					[]uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256})

				By("Verifying disallowed cipher is rejected on controlplane webhook")
				assertTLSRejected(ctrlplaneWebhookSvc,
					[]uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256})
			})

		It("Verifies profile change triggers automatic reconciliation",
			reportxml.ID("88847"), func() {
				By("Restoring Intermediate baseline")
				removeAPIServerTLSProfile()
				restartCAPOAPods()

				By("Recording baseline cipher connectivity")
				assertTLSConnects(bootstrapWebhookSvc, tls.VersionTLS12, tls.VersionTLS12,
					[]uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256})

				By("Switching to Custom single-cipher profile")
				patchAPIServerTLSProfile(configv1.TLSSecurityProfile{
					Type: configv1.TLSProfileCustomType,
					Custom: &configv1.CustomTLSProfile{
						TLSProfileSpec: configv1.TLSProfileSpec{
							Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256"},
							MinTLSVersion: configv1.VersionTLS12,
						},
					},
				})

				By("Waiting for automatic reconciliation (no manual pod restart)")
				waitCAPOAPodsRestarted()
				assertControllerLogsContain(bootstrapDeploy, honoringLogPattern)

				By("Verifying AES256 is now rejected under single-cipher Custom profile")
				assertTLSRejected(bootstrapWebhookSvc,
					[]uint16{tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384})

				By("Switching back to Intermediate")
				removeAPIServerTLSProfile()
				waitCAPOAPodsRestarted()

				By("Verifying AES256 is restored under Intermediate")
				assertTLSConnects(bootstrapWebhookSvc, tls.VersionTLS12, tls.VersionTLS12,
					[]uint16{tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384})
			})

		It("Verifies webhook validation works after TLS profile change",
			reportxml.ID("88848"), func() {
				By("Applying Custom TLS profile")
				patchAPIServerTLSProfile(configv1.TLSSecurityProfile{
					Type: configv1.TLSProfileCustomType,
					Custom: &configv1.CustomTLSProfile{
						TLSProfileSpec: configv1.TLSProfileSpec{
							Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256", "ECDHE-RSA-AES256-GCM-SHA384"},
							MinTLSVersion: configv1.VersionTLS12,
						},
					},
				})
				restartCAPOAPods()

				testNS := "ocp-88848-test"

				By("Ensuring test namespace does not exist")

				nsBuilder := namespace.NewBuilder(HubAPIClient, testNS)

				err := nsBuilder.DeleteAndWait(2 * time.Minute)
				Expect(err).ToNot(HaveOccurred(), "failed to clean up namespace %s", testNS)

				By("Creating test namespace")

				nsBuilder, err = namespace.NewBuilder(HubAPIClient, testNS).Create()
				Expect(err).ToNot(HaveOccurred(), "failed to create test namespace")

				DeferCleanup(func() {
					_ = nsBuilder.Delete()
				})

				By("Creating valid OpenshiftAssistedConfig")

				oac := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha2",
						"kind":       "OpenshiftAssistedConfig",
						"metadata": map[string]interface{}{
							"name":      "test-webhook-validation",
							"namespace": testNS,
						},
						"spec": map[string]interface{}{
							"cpuArchitecture": "x86_64",
						},
					},
				}

				err = HubAPIClient.Create(context.TODO(), oac)
				Expect(err).ToNot(HaveOccurred(), "valid OpenshiftAssistedConfig should be accepted")

				By("Attempting to update spec (should be rejected — immutable)")

				patchData := []byte(`{"spec":{"cpuArchitecture":"aarch64"}}`)

				err = HubAPIClient.Patch(context.TODO(), oac, runtimeclient.RawPatch(types.MergePatchType, patchData))
				Expect(err).To(HaveOccurred(), "spec update should be rejected by webhook")
				Expect(err.Error()).To(ContainSubstring("immutable"),
					"rejection reason should mention immutability")

				By("Updating metadata only (should succeed)")

				labelPatch := []byte(`{"metadata":{"labels":{"test-label":"ocp-88848"}}}`)

				err = HubAPIClient.Patch(context.TODO(), oac, runtimeclient.RawPatch(types.MergePatchType, labelPatch))
				Expect(err).ToNot(HaveOccurred(), "metadata update should be accepted")

				By("Deleting the resource")

				err = HubAPIClient.Delete(context.TODO(), oac)
				Expect(err).ToNot(HaveOccurred(), "deletion should succeed")
			})

		It("Verifies restore to default profile leaves no residual configuration",
			reportxml.ID("88850"), func() {
				By("Applying Custom TLS profile")
				patchAPIServerTLSProfile(configv1.TLSSecurityProfile{
					Type: configv1.TLSProfileCustomType,
					Custom: &configv1.CustomTLSProfile{
						TLSProfileSpec: configv1.TLSProfileSpec{
							Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256", "ECDHE-RSA-AES256-GCM-SHA384"},
							MinTLSVersion: configv1.VersionTLS12,
						},
					},
				})
				restartCAPOAPods()

				By("Removing Custom profile to restore default")
				removeAPIServerTLSProfile()
				restartCAPOAPods()

				By("Verifying no tlsSecurityProfile remains on apiserver")

				apiserver := &configv1.APIServer{}

				err := HubAPIClient.Get(context.TODO(), runtimeclient.ObjectKey{Name: "cluster"}, apiserver)
				Expect(err).ToNot(HaveOccurred())
				Expect(apiserver.Spec.TLSSecurityProfile).To(BeNil(),
					"tlsSecurityProfile should be nil after restore")

				By("Verifying controller logs show Intermediate profile")
				assertControllerLogsContain(bootstrapDeploy, "VersionTLS12")
				assertControllerLogsContain(ctrlplaneDeploy, "VersionTLS12")

				By("Verifying Intermediate ciphers are available")
				assertTLSConnects(bootstrapWebhookSvc, tls.VersionTLS12, tls.VersionTLS12,
					[]uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256})
				assertTLSConnects(bootstrapWebhookSvc, tls.VersionTLS13, tls.VersionTLS13, nil)
			})

		It("Verifies SecurityProfileWatcher triggers manager restart on profile change",
			reportxml.ID("88858"), func() {
				By("Ensuring Intermediate baseline and pods are ready")
				removeAPIServerTLSProfile()
				waitCAPOAPodsReady()

				By("Recording baseline restart counts")

				bootstrapRestarts := getCAPOAPodRestartCount(bootstrapDeploy)
				ctrlplaneRestarts := getCAPOAPodRestartCount(ctrlplaneDeploy)

				By("Changing TLS profile to Custom")
				patchAPIServerTLSProfile(configv1.TLSSecurityProfile{
					Type: configv1.TLSProfileCustomType,
					Custom: &configv1.CustomTLSProfile{
						TLSProfileSpec: configv1.TLSProfileSpec{
							Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256", "ECDHE-RSA-AES256-GCM-SHA384"},
							MinTLSVersion: configv1.VersionTLS12,
						},
					},
				})

				By("Waiting for pods to restart (restart count increases)")
				Eventually(func() int32 {
					return getCAPOAPodRestartCount(bootstrapDeploy)
				}).WithTimeout(capoapodAutoRestartTimeout).WithPolling(5*time.Second).
					Should(BeNumerically(">", bootstrapRestarts),
						"bootstrap pod should have restarted")

				Eventually(func() int32 {
					return getCAPOAPodRestartCount(ctrlplaneDeploy)
				}).WithTimeout(capoapodAutoRestartTimeout).WithPolling(5*time.Second).
					Should(BeNumerically(">", ctrlplaneRestarts),
						"controlplane pod should have restarted")

				By("Waiting for pods to become ready again")
				waitCAPOAPodsReady()

				By("Verifying controllers honour the new profile")
				assertControllerLogsContain(bootstrapDeploy, honoringLogPattern)
				assertControllerLogsContain(ctrlplaneDeploy, honoringLogPattern)
			})
	})

func patchAPIServerTLSProfile(profile configv1.TLSSecurityProfile) {
	patchMap := map[string]interface{}{
		"spec": map[string]interface{}{
			"tlsSecurityProfile": profile,
		},
	}

	patchBytes, err := json.Marshal(patchMap)
	Expect(err).ToNot(HaveOccurred(), "failed to marshal TLS profile patch")

	apiserver := &configv1.APIServer{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}}
	err = HubAPIClient.Patch(context.TODO(), apiserver, runtimeclient.RawPatch(types.MergePatchType, patchBytes))
	Expect(err).ToNot(HaveOccurred(), "failed to patch apiserver TLS profile")
}

func removeAPIServerTLSProfile() {
	patchBytes := []byte(`[{"op":"remove","path":"/spec/tlsSecurityProfile"}]`)
	apiserver := &configv1.APIServer{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}}

	err := HubAPIClient.Patch(context.TODO(), apiserver,
		runtimeclient.RawPatch(types.JSONPatchType, patchBytes))
	if err != nil && !strings.Contains(err.Error(), "doesn't exist") {
		Expect(err).ToNot(HaveOccurred(), "failed to remove apiserver TLS profile")
	}
}

func listCAPOAPods() []*pod.Builder {
	pods, err := pod.ListByNamePattern(HubAPIClient, "capoa", capoaNS)
	Expect(err).ToNot(HaveOccurred(), "failed to list CAPOA pods")

	return pods
}

func findCAPOAPod(deployName string) *pod.Builder {
	for _, p := range listCAPOAPods() {
		if strings.Contains(p.Object.Name, deployName) {
			return p
		}
	}

	Fail(fmt.Sprintf("no CAPOA pod found matching %s", deployName))

	return nil
}

func waitCAPOAPodsReady() {
	Eventually(func() int {
		pods, err := pod.ListByNamePattern(HubAPIClient, "capoa", capoaNS)
		if err != nil {
			return 0
		}

		ready := 0

		for _, p := range pods {
			if p.IsHealthy() {
				ready++
			}
		}

		return ready
	}).WithTimeout(capoapodReadyTimeout).WithPolling(10*time.Second).
		Should(BeNumerically(">=", 2), "both CAPOA pods should be ready")
}

// waitCAPOAPodsRestarted waits for the operator to automatically restart CAPOA containers.
// The SecurityProfileWatcher cancels the manager context causing the container to exit and
// be restarted by kubelet. The pod UID stays the same — only the restart count increases.
func waitCAPOAPodsRestarted() {
	bootstrapRestarts := getCAPOAPodRestartCount(bootstrapDeploy)
	ctrlplaneRestarts := getCAPOAPodRestartCount(ctrlplaneDeploy)

	Eventually(func() bool {
		return getCAPOAPodRestartCount(bootstrapDeploy) > bootstrapRestarts
	}).WithTimeout(capoapodAutoRestartTimeout).WithPolling(5*time.Second).
		Should(BeTrue(), "bootstrap pod should have been restarted by the operator")

	Eventually(func() bool {
		return getCAPOAPodRestartCount(ctrlplaneDeploy) > ctrlplaneRestarts
	}).WithTimeout(capoapodAutoRestartTimeout).WithPolling(5*time.Second).
		Should(BeTrue(), "controlplane pod should have been restarted by the operator")

	waitCAPOAPodsReady()
}

func restartCAPOAPods() {
	pods := listCAPOAPods()

	for _, p := range pods {
		podName := p.Object.Name
		_, err := p.DeleteAndWait(capoapodReadyTimeout)
		Expect(err).ToNot(HaveOccurred(), "failed to delete pod %s", podName)
	}

	waitCAPOAPodsReady()
}

func getCAPOAPodRestartCount(deployName string) int32 {
	p := findCAPOAPod(deployName)
	for _, cs := range p.Object.Status.ContainerStatuses {
		if cs.Name == managerContainer {
			return cs.RestartCount
		}
	}

	return -1
}

func assertControllerLogsContain(deployName, pattern string) {
	Eventually(func() string {
		p := findCAPOAPod(deployName)

		logs, err := p.GetFullLog(managerContainer)
		if err != nil {
			return ""
		}

		return logs
	}).WithTimeout(30*time.Second).WithPolling(5*time.Second).
		Should(ContainSubstring(pattern),
			fmt.Sprintf("%s logs should contain %q", deployName, pattern))
}

func assertTLSConnects(svcName string, minVersion, maxVersion uint16, cipherSuites []uint16) {
	addr := startPortForward(svcName)

	defer stopPortForward(svcName)

	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec
		MinVersion:         minVersion,
		MaxVersion:         maxVersion,
	}

	if len(cipherSuites) > 0 {
		tlsConfig.CipherSuites = cipherSuites
	}

	Eventually(func() error {
		dialer := &net.Dialer{Timeout: 10 * time.Second}

		conn, dialErr := tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
		if dialErr != nil {
			return dialErr
		}

		defer conn.Close()

		state := conn.ConnectionState()
		if !state.HandshakeComplete {
			return fmt.Errorf("TLS handshake not complete")
		}

		return nil
	}).WithTimeout(30*time.Second).WithPolling(2*time.Second).
		Should(Succeed(), "TLS connection to %s should succeed", svcName)
}

func assertTLSRejectedVersion(svcName string, version uint16) {
	assertTLSRejectedWith(svcName, version, version, nil)
}

func assertTLSRejected(svcName string, cipherSuites []uint16) {
	assertTLSRejectedWith(svcName, tls.VersionTLS12, tls.VersionTLS12, cipherSuites)
}

func assertTLSRejectedWith(svcName string, minVersion, maxVersion uint16, cipherSuites []uint16) {
	addr := startPortForward(svcName)

	defer stopPortForward(svcName)

	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec
		MinVersion:         minVersion,
		MaxVersion:         maxVersion,
	}

	if len(cipherSuites) > 0 {
		tlsConfig.CipherSuites = cipherSuites
	}

	Eventually(func() string {
		dialer := &net.Dialer{Timeout: 10 * time.Second}
		conn, dialErr := tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)

		if conn != nil {
			conn.Close()
		}

		if dialErr == nil {
			return "connected"
		}

		errMsg := dialErr.Error()
		if strings.Contains(errMsg, "connection refused") || errMsg == "EOF" {
			return "not-ready"
		}

		return errMsg
	}).WithTimeout(30*time.Second).WithPolling(2*time.Second).
		Should(ContainSubstring("tls:"), "TLS connection to %s should be rejected with TLS error", svcName)
}

var portForwardStopChans = map[string]chan struct{}{}

var portForwardPorts = map[string]int{
	bootstrapWebhookSvc: 19443,
	ctrlplaneWebhookSvc: 19444,
}

func startPortForward(svcName string) string {
	stopPortForward(svcName)

	localPort := portForwardPorts[svcName]

	svcPrefix := strings.TrimSuffix(svcName, "-webhook-service")
	targetPod := findCAPOAPod(svcPrefix)

	restConfig := HubAPIClient.Config
	apiURL, err := url.Parse(restConfig.Host)
	Expect(err).ToNot(HaveOccurred(), "failed to parse API server URL")

	apiURL.Path = fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", capoaNS, targetPod.Object.Name)

	transport, upgrader, err := spdy.RoundTripperFor(restConfig)
	Expect(err).ToNot(HaveOccurred(), "failed to create SPDY round-tripper")

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, apiURL)

	stopChan := make(chan struct{})
	readyChan := make(chan struct{})

	forwarder, err := portforward.New(dialer,
		[]string{fmt.Sprintf("%d:%d", localPort, webhookPort)},
		stopChan, readyChan, nil, nil)
	Expect(err).ToNot(HaveOccurred(), "failed to create port-forwarder for %s", svcName)

	go func() {
		_ = forwarder.ForwardPorts()
	}()

	select {
	case <-readyChan:
	case <-time.After(15 * time.Second):
		close(stopChan)
		Fail(fmt.Sprintf("port-forward to %s did not become ready in 15s", svcName))
	}

	portForwardStopChans[svcName] = stopChan

	return fmt.Sprintf("localhost:%d", localPort)
}

func stopPortForward(svcName string) {
	if stopChan, ok := portForwardStopChans[svcName]; ok {
		close(stopChan)
		delete(portForwardStopChans, svcName)
	}
}
