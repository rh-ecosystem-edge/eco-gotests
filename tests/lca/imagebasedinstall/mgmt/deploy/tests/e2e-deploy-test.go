package deploy_test

import (
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedinstall/mgmt/deploy/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedinstall/mgmt/internal/mgmtinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/internal/installconfig"
)

var _ = Describe(
	"Performing image based installation",
	Ordered,
	Label(tsparams.LabelEndToEndDeployment), func() {
		BeforeAll(func() {
			if MGMTConfig.Cluster == nil {
				Skip("Failed to collect cluster info")
			}

			if MGMTConfig.SeedClusterInfo == nil || MGMTConfig.SeedClusterInfo.SeedClusterOCPVersion == "" {
				Skip("Seed clusterinfo not supplied")
			}

			splitOCPVersion := strings.Split(MGMTConfig.SeedClusterInfo.SeedClusterOCPVersion, ".")

			if len(splitOCPVersion) < 2 {
				Skip("Could not determine short OCP version from seed cluster ocp version")
			}

			ibiImageSetName = strings.Join(splitOCPVersion[:2], ".")
		})

		It("through IBI operator is successful in a connected environment with static networking",
			reportxml.ID("76641"), func() {
				if !MGMTConfig.StaticNetworking {
					Skip("Cluster is deployed without static networking")
				}

				if MGMTConfig.SiteConfig {
					Skip("Cluster is deployed with siteconfig operator")
				}

				tsparams.ReporterNamespacesToDump[MGMTConfig.Cluster.Info.ClusterName] = reporterNamespaceToDump

				createIBIOResouces(ipv4AddrFamily)
			})

		It("through siteconfig operator is successful in a primary IPv4 dual-stack "+
			"proxy-enabled environment with DHCP networking",
			reportxml.ID("76642"), func() {
				if MGMTConfig.StaticNetworking {
					Skip("Cluster is deployed with static networking")
				}

				if !MGMTConfig.SiteConfig {
					Skip("Cluster is deployed without siteconfig operator")
				}

				if MGMTConfig.SeedClusterInfo.Proxy.HTTPProxy == "" && MGMTConfig.SeedClusterInfo.Proxy.HTTPSProxy == "" {
					Skip("Cluster not installed with proxy")
				}

				if MGMTConfig.Cluster.Info.PrimaryIPFamily != ipv4AddrFamily {
					Skip("Cluster is not deployed with dual-stack primary IPv4")
				}

				tsparams.ReporterNamespacesToDump[MGMTConfig.Cluster.Info.ClusterName] = reporterNamespaceToDump

				createSiteConfigResouces(dualstackPrimaryv4AddrFamily)
			})

		It("through siteconfig operator is successful in a primary IPv6 dual-stack "+
			"proxy-enabled environment with DHCP networking",
			reportxml.ID("no-testcase"), func() {
				if MGMTConfig.StaticNetworking {
					Skip("Cluster is deployed with static networking")
				}

				if !MGMTConfig.SiteConfig {
					Skip("Cluster is deployed without siteconfig operator")
				}

				if MGMTConfig.SeedClusterInfo.Proxy.HTTPProxy == "" && MGMTConfig.SeedClusterInfo.Proxy.HTTPSProxy == "" {
					Skip("Cluster not installed with proxy")
				}

				if MGMTConfig.Cluster.Info.PrimaryIPFamily != ipv6AddrFamily {
					Skip("Cluster is not deployed with dual-stack primary IPv6")
				}

				tsparams.ReporterNamespacesToDump[MGMTConfig.Cluster.Info.ClusterName] = reporterNamespaceToDump

				createSiteConfigResouces(dualstackPrimaryv6AddrFamily)
			})

		It("through siteconfig operator is successful in an IPv4 environment with DHCP networking",
			reportxml.ID("no-testcase"), func() {
				if MGMTConfig.StaticNetworking {
					Skip("Cluster is deployed with static networking")
				}

				if !MGMTConfig.SiteConfig {
					Skip("Cluster is deployed without siteconfig operator")
				}

				if MGMTConfig.SeedClusterInfo.Proxy.HTTPProxy != "" || MGMTConfig.SeedClusterInfo.Proxy.HTTPSProxy != "" {
					Skip("Cluster installed with proxy")
				}

				tsparams.ReporterNamespacesToDump[MGMTConfig.Cluster.Info.ClusterName] = reporterNamespaceToDump

				createSiteConfigResouces(ipv4AddrFamily)
			})

		It("successfully creates extramanifests", reportxml.ID("76643"), func() {
			if !MGMTConfig.ExtraManifests {
				Skip("Cluster not configured with extra manifests")
			}

			By("Get spoke client")
			spokeClient = getSpokeClient()

			By("Pull namespace created by extra manifests")
			extraNamespace, err := namespace.Pull(spokeClient, extraManifestNamespace)
			Expect(err).NotTo(HaveOccurred(), "error pulling namespace created by extra manifests")

			By("Pull configmap created by extra manifests")
			extraConfigmap, err := configmap.Pull(spokeClient, extraManifestConfigmap, extraNamespace.Object.Name)
			Expect(err).NotTo(HaveOccurred(), "error pulling configmap created by extra manifests")
			Expect(len(extraConfigmap.Object.Data)).To(Equal(1), "error: got unexpected data in configmap")
			Expect(extraConfigmap.Object.Data["hello"]).To(Equal("world"),
				"error: extra manifest configmap has incorrect content")
		})

		It("successfully creates extra partition", reportxml.ID("79049"), func() {
			if MGMTConfig.ExtraPartName == "" {
				Skip("Cluster not configured with extra partition")
			}

			By("Get spoke client")
			spokeClient = getSpokeClient()

			By("Validate the value of ExtraPartSizeMib variable")
			extraPartSizeMibInt, err := strconv.ParseInt(MGMTConfig.ExtraPartSizeMib, 10, 0)
			Expect(err).ToNot(HaveOccurred(), "failed to convert the extra partition size to int64: %s", err)

			By("Validate size of extra partition")
			Eventually(func() (string, error) {
				execCmd := "lsblk -o size -b -n " + MGMTConfig.ExtraPartName
				cmdOutput, err := cluster.ExecCmdWithStdout(spokeClient, execCmd)
				if err != nil {
					return "", nil
				}

				for _, stdout := range cmdOutput {
					return strings.ReplaceAll(stdout, "\n", ""), nil
				}

				return "", nil

			}).WithTimeout(time.Minute*20).WithPolling(time.Second*5).Should(
				Equal(strconv.Itoa(int(extraPartSizeMibInt)*1024*1024)), "error waiting for imageclusterinstall to complete")
		})

		It("successfully adds CA bundle", reportxml.ID("77795"), func() {
			if !MGMTConfig.CABundle {
				Skip("Cluster not configured with CA bundle")
			}

			By("Get spoke client")
			spokeClient = getSpokeClient()

			By("Validate adding a certificate by referencing a CA bundle", func() {
				execCmd := "grep -q qebox.redhat.com /etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem"
				_, err := cluster.ExecCmdWithStdout(spokeClient, execCmd)
				Expect(err).ToNot(HaveOccurred(), "failed checking the ca bundle for expected entry: %s", err)
			})
		})

		It("successfully configured using FIPs", reportxml.ID("76644"), func() {
			if !MGMTConfig.SeedClusterInfo.HasFIPS {
				Skip("Cluster not using FIPS enabled seed image")
			}

			By("Get spoke client")
			spokeClient = getSpokeClient()

			By("Get spoke cluster-config configmap")
			clusterConifgMap, err := configmap.Pull(spokeClient, "cluster-config-v1", "kube-system")
			Expect(err).NotTo(HaveOccurred(), "error pulling cluster-config configmap from spoke cluster")

			installConfigData, ok := clusterConifgMap.Object.Data["install-config"]
			Expect(ok).To(BeTrue(), "error: cluster-config does not contain appropriate install-config key")

			spokeInstallConfig, err := installconfig.NewInstallConfigFromString(installConfigData)
			Expect(err).NotTo(HaveOccurred(), "error creating InstallConfig struct from configmap data")
			Expect(spokeInstallConfig.FIPS).To(BeTrue(),
				"error: installed spoke does not have expected FIPS value set in install-config")
		})
	})
