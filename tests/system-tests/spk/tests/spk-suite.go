package spk_system_test

import (
	. "github.com/onsi/ginkgo/v2"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/spk/internal/spkcommon"
)

var _ = Describe(
	"SPK DNS/NAT46",
	Ordered,
	ContinueOnFailure,
	Label("spk-all-suite"), func() {
		Describe("On clean deployment", Label("spk-vanila-deployment"), func() {
			BeforeAll(func() {
				spkcommon.SetupSPKBackendUDPWorkload()
				spkcommon.SetupSPKBackendWorkload()
			})

			AfterEach(func(ctx SpecContext) {
				spkcommon.ResetTMMReplicas(ctx)
			})

			It("Asserts workload reachable via IPv4 address", reportxml.ID("64119"),
				Label("spkingresstcp"), spkcommon.AssertIPv4WorkloadURL)

			It("Asserts workload reachable via IPv6 address", reportxml.ID("65886"),
				Label("spkingresstcp"), spkcommon.AssertIPv6WorkloadURL)

			It("Asserts workload reachable via IPv4 UDP", reportxml.ID("72777"), FlakeAttempts(2),
				Label("spkingress-udp", "spkingress-udp-ipv4"),
				spkcommon.VerifySPKIngressUDPviaIPv4)

			It("Asserts workload reachable via IPv6 UDP", reportxml.ID("72778"), FlakeAttempts(2),
				Label("spkingress-udp", "spkingress-udp-ipv6"),
				spkcommon.VerifySPKIngressUDPviaIPv6)

			It("Asserts workload reachable via IPv4 address after application recreation", reportxml.ID("72437"),
				Label("spkingresstcp-app-recreate"), MustPassRepeatedly(3),
				spkcommon.AssertIPv4WorkloadURLAfterAppRecreated)

			It("Asserts workload reachable via IPv6 address after application recreation", reportxml.ID("72439"),
				Label("spkingresstcp-app-recreate"), MustPassRepeatedly(3),
				spkcommon.AssertIPv6WorkloadURLAfterAppRecreated)

			It("Asserts DNS resoulution from new deployment", reportxml.ID("63171"),
				Label("spkdns46new"), spkcommon.VerifyDNSResolutionFromNewDeploy)

			It("Asserts DNS resolution from existing deployment", reportxml.ID("66091"),
				Label("spkdns46existing"), spkcommon.VerifyDNSResolutionFromExistingDeploy)

			It("Asserts DNS Resolution after SPK scale-down and scale-up from existing deployment",
				reportxml.ID("72139"), Label("spkscaledown"), spkcommon.VerifyDNSResolutionAfterTMMScaleUpDownExisting)

			It("Asserts DNS Resolution after SPK scale-down and scale-up from new deployment",
				reportxml.ID("72140"), Label("spkscaledown"), spkcommon.VerifyDNSResolutionFromNewDeploy)

			It("Asserts DNS Resolution after SPK TMM pod(s) are deleted from existing deployment",
				reportxml.ID("72644"), Label("spk-tmm-delete"), spkcommon.VerifyDNSResolutionAfterTMMPodIsDeletedExistingDeploy)

			It("Asserts DNS Resolution after SPK TMM pod(s) are deleted from new deployment",
				reportxml.ID("72645"), Label("spk-tmm-delete"), spkcommon.VerifyDNSResolutionAfterTMMPodIsDeletedNewDeploy)

			It("Asserts DNS Resolution with multiple TMM controllers from existing deployment",
				reportxml.ID("72141"), Label("spk-multi-tmm-new"), spkcommon.VerifyDNSResolutionWithMultipleTMMsExisting)

			It("Asserts DNS Resolution with multiple TMM controllers from new deployment",
				reportxml.ID("72142"), Label("spk-multi-tmm-existing"), spkcommon.VerifyDNSResolutionFromNewDeploy)

			It("Asserts DNS Resolution after Ingress scale-down and scale-up", Label("spk-ingress-scale-down-up"),
				reportxml.ID("72143"), spkcommon.VerifyIngressScaleDownUp)

			It("Asserts DNS Resolution after Ingress scale-down and scale-up from new deployment",
				reportxml.ID("72144"), Label("spk-multi-tmm-existing"), spkcommon.VerifyDNSResolutionFromNewDeploy)

			It("Assert DNS resolution after Ingress pods were deleted from existing deployment", reportxml.ID("72280"),
				Label("spk-ingress-delete-existing"), spkcommon.VerifyDNSResolutionAfterIngressPodIsDeleteExistinDeploy)

			It("Assert DNS resolution after Ingress pods were deleted from new deployment", reportxml.ID("72283"),
				Label("spk-ingress-delete-existing"), spkcommon.VerifyDNSResolutionAfterIngressPodIsDeleteNewDeploy)

			It("Assert workload is reachable over IPv4 SPK ingress after pod was deleted", reportxml.ID("72278"),
				Label("spk-ingress-delete", "spk-ingress-tcp-delete-ipv4"),
				spkcommon.AssertIPv4WorkloadURLAfterIngressPodDeleted)

			It("Assert workload is reachable over IPv6 SPK ingress after pod was deleted", reportxml.ID("72279"),
				Label("spk-ingress-delete", "spk-ingress-tcp-delete-ipv6"),
				spkcommon.AssertIPv6WorkloadURLAfterIngressPodDeleted)

			It("Assert workload is reachable over IPv4 SPK Ingress after TMM pod is deleted", reportxml.ID("72825"),
				Label("spk-ingress-tmm-delete", "spk-ingress-tcp-tmm-delete-ipv4"),
				spkcommon.AssertIPv4WorkloadURLAfterTMMPodDeleted)

			It("Assert workload is reachable over IPv6 SPK ingress after TMM pod was deleted", reportxml.ID("72826"),
				Label("spk-ingress-tmm-delete", "spk-ingress-tcp-tmm-delete-ipv6"),
				spkcommon.AssertIPv6WorkloadURLAfterTMMPodDeleted)

			It("Assert workload is reachable over IPv4 SPK UDP ingress after pod was deleted", reportxml.ID("72782"),
				Label("spk-ingress-delete", "spk-ingress-udp-delete-ipv4"),
				spkcommon.AssertIPv4UDPWorkloadURLAfterIngressPodDeleted)

			It("Assert workload is reachable over IPv6 SPK UDP ingress after pod was deleted", reportxml.ID("72783"),
				Label("spk-ingress-delete", "spk-ingress-udp-delete-ipv6"),
				spkcommon.AssertIPv6UDPWorkloadURLAfterIngressPodDeleted)

			It("Assert workload is reachable over IPv4 SPK UDP ingress after TMM pod was deleted", reportxml.ID("72827"),
				Label("spk-ingress-delete", "spk-ingress-udp-delete-tmm-ipv4"),
				spkcommon.AssertIPv4UDPWorkloadURLAfterTMMPodDeleted)

			It("Assert workload is reachable over IPv6 SPK UDP ingress after TMM pod was deleted", reportxml.ID("72828"),
				Label("spk-ingress-delete", "spk-ingress-udp-delete-tmm-ipv6"),
				spkcommon.AssertIPv6UDPWorkloadURLAfterTMMPodDeleted)
		})

		Describe("Hard reboot", Label("spk-hard-reboot"), func() {
			BeforeAll(func() {
				spkcommon.SetupSPKBackendUDPWorkload()
				spkcommon.SetupSPKBackendWorkload()
			})

			AfterEach(func(ctx SpecContext) {
				spkcommon.ResetTMMReplicas(ctx)
			})

			spkcommon.VerifyHardRebootSuite()

			It("Removes stuck SPK pods",
				Label("spk-post-hard-reboot", "spk-cleanup-stuck-pods"), spkcommon.CleanupStuckContainerPods)

			// NOTE: looks like an issue with pods using cached data,
			// which requires pods restart to reload it.
			It("Restart SPK Ingress pods after hard reboot",
				Label("spk-post-hard-reboot", "spk-restart-ingress-pods"), spkcommon.RestartSPKIngressPods)

			It("Asserts workload reachable via IPv4 address", reportxml.ID("72193"),
				Label("spk-post-hard-reboot", "spkingresstcp"), spkcommon.AssertIPv4WorkloadURL)

			It("Asserts workload reachable via IPv6 address", reportxml.ID("72194"),
				Label("spk-post-hard-reboot", "spkingresstcp"), spkcommon.AssertIPv6WorkloadURL)

			It("Asserts workload reachable via IPv4 UDP after hard reboot", reportxml.ID("72785"),
				Label("spkingress-udp", "spkingress-udp-ipv4"),
				spkcommon.VerifySPKIngressUDPviaIPv4)

			It("Asserts workload reachable via IPv6 UDP after hard reboot", reportxml.ID("72786"),
				Label("spkingress-udp", "spkingress-udp-ipv6"),
				spkcommon.VerifySPKIngressUDPviaIPv6)

			It("Asserts DNS resoulution from new deployment", reportxml.ID("72196"),
				Label("spk-post-hard-reboot", "spkdns46new"), spkcommon.VerifyDNSResolutionFromNewDeploy)

			It("Asserts DNS resolution from existing deployment", reportxml.ID("72197"),
				Label("spk-post-hard-reboot", "spkdns46existing"), spkcommon.VerifyDNSResolutionFromExistingDeploy)

			It("Asserts workload reachable via IPv4 address after hard reboot and application recreation",
				Label("spkingresstcp-app-recreate"), MustPassRepeatedly(3), reportxml.ID("72438"),
				spkcommon.AssertIPv4WorkloadURLAfterAppRecreated)

			It("Asserts workload reachable via IPv6 address after hard reboot and application recreation",
				Label("spkingresstcp-app-recreate"), MustPassRepeatedly(3), reportxml.ID("72440"),
				spkcommon.AssertIPv6WorkloadURLAfterAppRecreated)

			It("Asserts DNS Resolution after soft reboot and SPK TMM pod(s) are deleted from existing deployment",
				reportxml.ID("72648"), Label("spk-tmm-delete"), spkcommon.VerifyDNSResolutionAfterTMMPodIsDeletedExistingDeploy)

			It("Asserts DNS Resolution after soft reboot and SPK TMM pod(s) are deleted from new deployment",
				reportxml.ID("72649"), Label("spk-tmm-delete"), spkcommon.VerifyDNSResolutionAfterTMMPodIsDeletedNewDeploy)
		})

		Describe("Soft Reboot", Label("spk-soft-reboot"), func() {
			BeforeAll(func() {
				spkcommon.SetupSPKBackendUDPWorkload()
				spkcommon.SetupSPKBackendWorkload()
			})

			AfterEach(func(ctx SpecContext) {
				spkcommon.ResetTMMReplicas(ctx)
			})

			spkcommon.VerifyGracefulRebootSuite()

			// NOTE: looks like an issue with pods using cached data,
			// which requires pods restart to reload it.
			It("Restart SPK Ingress pods after soft reboot",
				Label("spk-post-soft-reboot", "spk-restart-ingress-pods"), spkcommon.RestartSPKIngressPods)

			It("Asserts workload reachable via IPv4 address", reportxml.ID("72198"),
				Label("spk-post-soft-reboot", "spkingresstcp"), spkcommon.AssertIPv4WorkloadURL)

			It("Asserts workload reachable via IPv6 address", reportxml.ID("72199"),
				Label("spk-post-soft-reboot", "spkingresstcp"), spkcommon.AssertIPv6WorkloadURL)

			It("Asserts workload reachable via IPv4 UDP after soft reboot", reportxml.ID("72787"),
				Label("spkingress-udp", "spkingress-udp-ipv4"),
				spkcommon.VerifySPKIngressUDPviaIPv4)

			It("Asserts workload reachable via IPv6 UDP after soft reboot", reportxml.ID("72788"),
				Label("spkingress-udp", "spkingress-udp-ipv6"),
				spkcommon.VerifySPKIngressUDPviaIPv6)

			It("Asserts DNS resoulution from new deployment", reportxml.ID("72200"),
				Label("spk-post-soft-reboot", "spkdns46new"), spkcommon.VerifyDNSResolutionFromNewDeploy)

			It("Asserts DNS resolution from existing deployment", reportxml.ID("72201"),
				Label("spk-post-soft-reboot", "spkdns46existing"), spkcommon.VerifyDNSResolutionFromExistingDeploy)

			It("Asserts workload reachable via IPv4 address after soft reboot and application recreation",
				Label("spkingresstcp-app-recreate"), MustPassRepeatedly(3), reportxml.ID("72441"),
				spkcommon.AssertIPv4WorkloadURLAfterAppRecreated)

			It("Asserts workload reachable via IPv6 address after soft reboot and application recreation",
				Label("spkingresstcp-app-recreate"), MustPassRepeatedly(3), reportxml.ID("72442"),
				spkcommon.AssertIPv6WorkloadURLAfterAppRecreated)

			It("Asserts DNS Resolution after soft reboot and SPK TMM pod(s) are deleted from existing deployment",
				reportxml.ID("72646"), Label("spk-tmm-delete"), spkcommon.VerifyDNSResolutionAfterTMMPodIsDeletedExistingDeploy)

			It("Asserts DNS Resolution after soft reboot and SPK TMM pod(s) are deleted from new deployment",
				reportxml.ID("72647"), Label("spk-tmm-delete"), spkcommon.VerifyDNSResolutionAfterTMMPodIsDeletedNewDeploy)
		})
	})
