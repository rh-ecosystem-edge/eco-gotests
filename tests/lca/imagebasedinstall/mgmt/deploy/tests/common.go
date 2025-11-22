package deploy_test

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmh"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/hive"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ibi"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/api/v1beta1"
	hivev1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/hive/api/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/hive/api/v1/none"
	ibiv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/imagebasedinstall/api/hiveextensions/v1alpha1"
	siteconfigv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/siteconfig/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/siteconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedinstall/mgmt/deploy/internal/networkconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedinstall/mgmt/internal/mgmtconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedinstall/mgmt/internal/mgmtparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/internal/brutil"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sScheme "k8s.io/client-go/kubernetes/scheme"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedinstall/mgmt/internal/mgmtinittools"
)

const (
	interfaceName = "enp1s0"

	extraManifestNamespace = "extranamespace"
	extraManifestConfigmap = "extra-configmap"

	extraManifestNamespaceConfigmapName = "extra-manifests-cm0"
	extraManifestConfigmapConfigmapName = "extra-manifests-cm1"

	caBundleConfigMapName = "ca-bundle-configmap"

	ibiClusterTemplateName = "ibi-cluster-templates-v1"
	ibiNodeTemplateName    = "ibi-node-templates-v1"

	ipv4AddrFamily               = "ipv4"
	ipv6AddrFamily               = "ipv6"
	dualstackPrimaryv4AddrFamily = "dualstackv4"
	dualstackPrimaryv6AddrFamily = "dualstackv6"
	reporterNamespaceToDump      = "spoke namespace"

	trueStatus  = "True"
	falseStatus = "False"
)

var (
	ibiImageSetName string

	spokeClient *clients.Settings
)

func createSharedResources() {
	By("Create namespace for IBI installation")

	_, err := namespace.NewBuilder(APIClient, MGMTConfig.Cluster.Info.ClusterName).Create()
	Expect(err).NotTo(HaveOccurred(), "error creating namespace")

	By("Get pull secret from hub cluster")

	spokePullSecret, err := secret.Pull(APIClient, "pull-secret", "openshift-config")
	Expect(err).NotTo(HaveOccurred(), "error getting pull-secret from hub cluster")

	By("Create pull secret for spoke cluster")

	spokePullSecret.Definition.Name = MGMTConfig.Cluster.Info.ClusterName
	spokePullSecret.Definition.Namespace = MGMTConfig.Cluster.Info.ClusterName
	spokePullSecret.Definition.ResourceVersion = ""
	_, err = spokePullSecret.Create()
	Expect(err).NotTo(HaveOccurred(), "error creating spoke pull-secret")

	if MGMTConfig.ExtraManifests {
		By("Create namespace builder for extramanifests")

		extraNamespace := namespace.NewBuilder(APIClient, extraManifestNamespace)

		By("Create configmap for extra manifests namespace")

		extraNamespaceString, err := brutil.NewBackupRestoreObject(
			extraNamespace.Definition, k8sScheme.Scheme, corev1.SchemeGroupVersion).String()
		Expect(err).NotTo(HaveOccurred(), "error creating configmap data for extramanifest namespace")
		_, err = configmap.NewBuilder(
			APIClient, extraManifestNamespaceConfigmapName, MGMTConfig.Cluster.Info.ClusterName).WithData(map[string]string{
			"00-namespace.yaml": extraNamespaceString,
		}).Create()
		Expect(err).NotTo(HaveOccurred(), "error creating configmap for extra manifests namespace")

		By("Create configmap builder for extramanifests")

		extraConfigmap := configmap.NewBuilder(
			APIClient, extraManifestConfigmap, extraManifestNamespace).WithData(map[string]string{
			"hello": "world",
		})

		By("Create configmap for extramanifests configmap")

		extraConfigmapString, err := brutil.NewBackupRestoreObject(
			extraConfigmap.Definition, k8sScheme.Scheme, corev1.SchemeGroupVersion).String()
		Expect(err).NotTo(HaveOccurred(), "error creating configmap data for extramanifest configmap")
		_, err = configmap.NewBuilder(
			APIClient, extraManifestConfigmapConfigmapName, MGMTConfig.Cluster.Info.ClusterName).WithData(map[string]string{
			"01-configmap.yaml": extraConfigmapString,
		}).Create()
		Expect(err).NotTo(HaveOccurred(), "error creating configmap for extra manifests configmap")
	}

	if MGMTConfig.CABundle {
		By("Create configmap for CA bundle")

		_, err = configmap.NewBuilder(
			APIClient, caBundleConfigMapName, MGMTConfig.Cluster.Info.ClusterName).WithData(map[string]string{
			"tls-ca-bundle.pem": mgmtparams.CaBundleString,
		}).Create()
		Expect(err).NotTo(HaveOccurred(), "error creating configmap with CA bundle")
	}

	for host, info := range MGMTConfig.Cluster.Info.Hosts {
		By("Create baremetalhost secret for " + host)

		_, err = secret.NewBuilder(
			APIClient, host, MGMTConfig.Cluster.Info.ClusterName, corev1.SecretTypeOpaque).WithData(map[string][]byte{
			"username": []byte(info.BMC.User),
			"password": []byte(info.BMC.Password),
		}).Create()
		Expect(err).NotTo(HaveOccurred(), "error creating bmh secret")
	}
}

//nolint:funlen
func createIBIOResouces(addressFamily string) {
	createSharedResources()

	var err error

	for host, info := range MGMTConfig.Cluster.Info.Hosts {
		By("Create baremetalhost for " + host)

		var bmcAddress string

		if addressFamily == ipv4AddrFamily || addressFamily == dualstackPrimaryv4AddrFamily {
			bmcAddress = info.BMC.URLv4
		} else {
			bmcAddress = info.BMC.URLv6
		}

		hostBMH := bmh.NewBuilder(
			APIClient, host, MGMTConfig.Cluster.Info.ClusterName, bmcAddress, host, info.BMC.MACAddress, "UEFI")
		hostBMH.Definition.Spec.AutomatedCleaningMode = "disabled"
		hostBMH.Definition.Spec.ExternallyProvisioned = true

		if MGMTConfig.StaticNetworking {
			nodeNetworkingConfig := createNetworkConfig(*MGMTConfig.Cluster, addressFamily)

			networkSecretContent, err := yaml.Marshal(&nodeNetworkingConfig)
			Expect(err).NotTo(HaveOccurred(), "error marshaling network configuration")

			_, err = secret.NewBuilder(APIClient, fmt.Sprintf("%s-nmstate-config", host),
				MGMTConfig.Cluster.Info.ClusterName, corev1.SecretTypeOpaque).WithData(map[string][]byte{
				"nmstate": networkSecretContent,
			}).Create()
			Expect(err).NotTo(HaveOccurred(), "error creating network configuration secret")

			hostBMH.Definition.Spec.PreprovisioningNetworkDataName = fmt.Sprintf("%s-nmstate-config", host)
		}

		_, err = hostBMH.Create()
		Expect(err).NotTo(HaveOccurred(), "error creating baremetalhost")
	}

	var snoNodeName string

	for hostname := range MGMTConfig.Cluster.Info.Hosts {
		snoNodeName = hostname

		break
	}

	imageClusterInstall := ibi.NewImageClusterInstallBuilder(
		APIClient, MGMTConfig.Cluster.Info.ClusterName, MGMTConfig.Cluster.Info.ClusterName, ibiImageSetName).
		WithClusterDeployment(MGMTConfig.Cluster.Info.ClusterName).WithHostname(snoNodeName)

	switch addressFamily {
	case ipv4AddrFamily:
		imageClusterInstall.WithMachineNetwork(MGMTConfig.Cluster.Info.MachineCIDR.IPv4)

	case ipv6AddrFamily:
		imageClusterInstall.WithMachineNetwork(MGMTConfig.Cluster.Info.MachineCIDR.IPv6)

	case dualstackPrimaryv4AddrFamily:
		imageClusterInstall.Definition.Spec.MachineNetworks =
			append(imageClusterInstall.Definition.Spec.MachineNetworks, ibiv1alpha1.MachineNetworkEntry{
				CIDR: MGMTConfig.Cluster.Info.MachineCIDR.IPv4,
			})

		imageClusterInstall.Definition.Spec.MachineNetworks =
			append(imageClusterInstall.Definition.Spec.MachineNetworks, ibiv1alpha1.MachineNetworkEntry{
				CIDR: MGMTConfig.Cluster.Info.MachineCIDR.IPv6,
			})

	case dualstackPrimaryv6AddrFamily:
		imageClusterInstall.Definition.Spec.MachineNetworks =
			append(imageClusterInstall.Definition.Spec.MachineNetworks, ibiv1alpha1.MachineNetworkEntry{
				CIDR: MGMTConfig.Cluster.Info.MachineCIDR.IPv6,
			})

		imageClusterInstall.Definition.Spec.MachineNetworks =
			append(imageClusterInstall.Definition.Spec.MachineNetworks, ibiv1alpha1.MachineNetworkEntry{
				CIDR: MGMTConfig.Cluster.Info.MachineCIDR.IPv4,
			})

	default:
		Fail("Invalid address family: " + addressFamily)
	}

	if MGMTConfig.ExtraManifests {
		imageClusterInstall.WithExtraManifests(extraManifestNamespaceConfigmapName).
			WithExtraManifests(extraManifestConfigmapConfigmapName)
	}

	if MGMTConfig.CABundle {
		imageClusterInstall.WithCABundle(caBundleConfigMapName)
	}

	if MGMTConfig.PublicSSHKey != "" {
		imageClusterInstall.WithSSHKey(MGMTConfig.PublicSSHKey)
	}

	if MGMTConfig.SeedClusterInfo.MirrorRegistryConfigured {
		imageClusterInstall.Definition.Spec.ImageDigestSources =
			MGMTConfig.SeedClusterInfo.MirrorConfig.Spec.ImageDigestMirrors
	}

	imageClusterInstall.Definition.Spec.BareMetalHostRef = &ibiv1alpha1.BareMetalHostReference{
		Name:      snoNodeName,
		Namespace: MGMTConfig.Cluster.Info.ClusterName,
	}

	if MGMTConfig.SeedClusterInfo.HasProxy {
		imageClusterInstall.Definition.Spec.Proxy = &ibiv1alpha1.Proxy{
			HTTPProxy:  MGMTConfig.SeedClusterInfo.Proxy.HTTPProxy,
			HTTPSProxy: MGMTConfig.SeedClusterInfo.Proxy.HTTPSProxy,
			NoProxy:    MGMTConfig.SeedClusterInfo.Proxy.NOProxy,
		}
	}

	By("Create imageclusterinstall for IBI installation")

	_, err = imageClusterInstall.Create()
	Expect(err).NotTo(HaveOccurred(), "error creating imageclusterinstall")

	By("Create cluster deployment for IBI installation")

	_, err = hive.NewClusterDeploymentByInstallRefBuilder(
		APIClient, MGMTConfig.Cluster.Info.ClusterName,
		MGMTConfig.Cluster.Info.ClusterName, MGMTConfig.Cluster.Info.ClusterName,
		MGMTConfig.Cluster.Info.BaseDomain, hivev1.ClusterInstallLocalReference{
			Group:   ibiv1alpha1.Group,
			Version: ibiv1alpha1.Version,
			Kind:    "ImageClusterInstall",
			Name:    MGMTConfig.Cluster.Info.ClusterName,
		}, hivev1.Platform{
			None: &none.Platform{},
		}).
		WithPullSecret(MGMTConfig.Cluster.Info.ClusterName).Create()
	Expect(err).NotTo(HaveOccurred(), "error creating cluster deployment")

	By("Create managedcluster for IBI cluster")

	_, err = ocm.NewManagedClusterBuilder(APIClient, MGMTConfig.Cluster.Info.ClusterName).
		WithHubAcceptsClient(true).Create()
	Expect(err).NotTo(HaveOccurred(), "error creating managedcluster resource")

	Eventually(func() (bool, error) {
		imageClusterInstall.Object, err = imageClusterInstall.Get()
		if err != nil {
			return false, err
		}

		condition, err := imageClusterInstall.GetCompletedCondition()
		if err != nil {
			return false, err
		}

		return condition.Status == trueStatus && condition.Reason == ibiv1alpha1.InstallSucceededReason, nil
	}).WithTimeout(time.Minute*20).WithPolling(time.Second*5).Should(
		BeTrue(), "error waiting for imageclusterinstall to complete")
}

//nolint:funlen
func createSiteConfigResouces(addressFamily string) {
	createSharedResources()

	By("Find cluster template configmap")

	clusterTemplateConfigmap, err := configmap.ListInAllNamespaces(APIClient, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", ibiClusterTemplateName),
	})
	Expect(err).To(BeNil(), "error encountered when listing configmaps mactching cluster template name")
	Expect(len(clusterTemplateConfigmap)).To(Equal(1),
		"error: received unexpected configmap count: %d", len(clusterTemplateConfigmap))

	By("Find node template configmap")

	nodeTemplateConfigmap, err := configmap.ListInAllNamespaces(APIClient, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", ibiNodeTemplateName),
	})
	Expect(err).To(BeNil(), "error encountered when listing configmaps mactching node template name")
	Expect(len(nodeTemplateConfigmap)).To(Equal(1),
		"error: received unexpected configmap count: %d", len(nodeTemplateConfigmap))

	clusterInstanceBuilder := siteconfig.NewCIBuilder(
		APIClient, MGMTConfig.Cluster.Info.ClusterName, MGMTConfig.Cluster.Info.ClusterName).
		WithPullSecretRef(MGMTConfig.Cluster.Info.ClusterName).
		WithClusterTemplateRef(ibiClusterTemplateName, clusterTemplateConfigmap[0].Object.Namespace).
		WithBaseDomain(MGMTConfig.Cluster.Info.BaseDomain).
		WithClusterImageSetRef(ibiImageSetName).
		WithClusterName(MGMTConfig.Cluster.Info.ClusterName)

	switch addressFamily {
	case ipv4AddrFamily:
		clusterInstanceBuilder.WithMachineNetwork(MGMTConfig.Cluster.Info.MachineCIDR.IPv4)

	case ipv6AddrFamily:
		clusterInstanceBuilder.WithMachineNetwork(MGMTConfig.Cluster.Info.MachineCIDR.IPv6)

	case dualstackPrimaryv4AddrFamily:
		clusterInstanceBuilder.WithMachineNetwork(MGMTConfig.Cluster.Info.MachineCIDR.IPv4)
		clusterInstanceBuilder.WithMachineNetwork(MGMTConfig.Cluster.Info.MachineCIDR.IPv6)

	case dualstackPrimaryv6AddrFamily:
		clusterInstanceBuilder.WithMachineNetwork(MGMTConfig.Cluster.Info.MachineCIDR.IPv6)
		clusterInstanceBuilder.WithMachineNetwork(MGMTConfig.Cluster.Info.MachineCIDR.IPv4)

	default:
		Fail("Invalid address family: " + addressFamily)
	}

	if MGMTConfig.PublicSSHKey != "" {
		clusterInstanceBuilder.WithSSHPubKey(MGMTConfig.PublicSSHKey)
	}

	if MGMTConfig.ExtraManifests {
		clusterInstanceBuilder.WithExtraManifests(extraManifestNamespaceConfigmapName).
			WithExtraManifests(extraManifestConfigmapConfigmapName)
	}

	if MGMTConfig.CABundle {
		clusterInstanceBuilder.WithCABundle(caBundleConfigMapName)
	}

	if MGMTConfig.SeedClusterInfo.Proxy.HTTPProxy != "" || MGMTConfig.SeedClusterInfo.Proxy.HTTPSProxy != "" {
		clusterInstanceBuilder.WithProxy(&v1beta1.Proxy{
			HTTPProxy:  MGMTConfig.SeedClusterInfo.Proxy.HTTPProxy,
			HTTPSProxy: MGMTConfig.SeedClusterInfo.Proxy.HTTPSProxy,
			NoProxy:    MGMTConfig.SeedClusterInfo.Proxy.NOProxy,
		})
	}

	Expect(len(MGMTConfig.Cluster.Info.Hosts)).To(Equal(1), "error: can only support SNO deployments")

	for host, info := range MGMTConfig.Cluster.Info.Hosts {
		var bmcAddress string

		if addressFamily == ipv4AddrFamily || addressFamily == dualstackPrimaryv4AddrFamily {
			bmcAddress = info.BMC.URLv4
		} else {
			bmcAddress = info.BMC.URLv6
		}

		By("Add node entry for " + host)

		siteconfigNode := siteconfig.NewNodeBuilder(host, bmcAddress, info.BMC.MACAddress, host, ibiNodeTemplateName,
			nodeTemplateConfigmap[0].Object.Namespace).WithAutomatedCleaningMode("disabled")

		if MGMTConfig.StaticNetworking {
			if len(info.Network.Interfaces) != 1 {
				Skip("Cannot support nodes with more than one network interface")
			}

			nodeNetwork := &v1beta1.NMStateConfigSpec{}

			nodeNetworkingConfig := createNetworkConfig(*MGMTConfig.Cluster, addressFamily)

			for _, iface := range nodeNetworkingConfig.Interfaces {
				nodeNetwork.Interfaces = append(nodeNetwork.Interfaces, &v1beta1.Interface{
					Name:       iface.Name,
					MacAddress: iface.MACAddress,
				})
			}

			rawNetwork, err := yaml.Marshal(&nodeNetworkingConfig)
			Expect(err).NotTo(HaveOccurred(), "error marshaling network configuration")

			nodeNetwork.NetConfig = v1beta1.NetConfig{
				Raw: rawNetwork,
			}

			siteconfigNode.WithNodeNetwork(nodeNetwork)
		}

		nodeSpec, err := siteconfigNode.Generate()
		Expect(err).NotTo(HaveOccurred(), "error generating node spec for clusterinstance")

		clusterInstanceBuilder.WithNode(nodeSpec)
	}

	_, err = clusterInstanceBuilder.Create()
	Expect(err).NotTo(HaveOccurred(), "error creating clusterinstance")

	Eventually(func() (bool, error) {
		clusterInstanceBuilder.Object, err = clusterInstanceBuilder.Get()
		if err != nil {
			return false, err
		}

		for _, condition := range clusterInstanceBuilder.Object.Status.Conditions {
			if condition.Type == string(siteconfigv1alpha1.ClusterProvisioned) {
				return condition.Status == trueStatus && condition.Reason == string(siteconfigv1alpha1.Completed), nil
			}
		}

		return false, nil
	}).WithTimeout(time.Minute*30).WithPolling(time.Second*10).Should(
		BeTrue(), "error waiting for clusterinstance to finish provisioning")
}

func createNetworkConfig(config mgmtconfig.Cluster, addressFamily string) networkconfig.NetworkConfig {
	nodeNetworkingConfig := networkconfig.NetworkConfig{}

	Expect(len(config.Info.Hosts)).To(Equal(1), "error: can only support SNO deployments")

	for _, info := range MGMTConfig.Cluster.Info.Hosts {
		Expect(len(info.Network.Interfaces)).To(Equal(1), "error: can only support nodes with single network interface")

		for _, iface := range info.Network.Interfaces {
			var address, gateway, dns, destination string
			if addressFamily == ipv4AddrFamily {
				address = info.Network.Address.IPv4
				gateway = info.Network.Gateway.IPv4
				dns = info.Network.DNS.IPv4
				destination = "0.0.0.0/0"
			} else {
				address = info.Network.Address.IPv6
				gateway = info.Network.Gateway.IPv6
				dns = info.Network.DNS.IPv6
				destination = "::/0"
			}

			nodeIPAddr, nodeIPNetwork, err := net.ParseCIDR(address)
			Expect(err).NotTo(HaveOccurred(), "error gathering network info from provided address")

			cidr, _ := nodeIPNetwork.Mask.Size()

			nodeNetworkingConfig = networkconfig.NetworkConfig{
				Interfaces: []networkconfig.Interface{
					{
						Name:       interfaceName,
						Type:       "ethernet",
						State:      "up",
						Identifier: "mac-address",
						MACAddress: iface.MACAddress,
					},
				},
				Routes: networkconfig.Routes{
					Config: []networkconfig.RouteConfig{
						{
							Destination:      destination,
							NextHopAddress:   gateway,
							NextHopInterface: interfaceName,
						},
					},
				},
				DNSResolver: networkconfig.DNSResolver{
					Config: networkconfig.DNSResolverConfig{
						Server: []string{
							dns,
						},
					},
				},
			}

			if addressFamily == ipv4AddrFamily {
				nodeNetworkingConfig.Interfaces[0].IPv4 = networkconfig.IPConfig{
					DHCP: false,
					Address: []networkconfig.IPAddress{
						{
							IP:           nodeIPAddr.String(),
							PrefixLength: strconv.Itoa(cidr),
						},
					},
					Enabled: true,
				}
				nodeNetworkingConfig.Interfaces[0].IPv6 = networkconfig.IPConfig{
					Enabled: false,
				}
			} else {
				nodeNetworkingConfig.Interfaces[0].IPv6 = networkconfig.IPConfig{
					DHCP: false,
					Address: []networkconfig.IPAddress{
						{
							IP:           nodeIPAddr.String(),
							PrefixLength: strconv.Itoa(cidr),
						},
					},
					Enabled: true,
				}
				nodeNetworkingConfig.Interfaces[0].IPv4 = networkconfig.IPConfig{
					Enabled: false,
				}
			}
		}
	}

	return nodeNetworkingConfig
}

func getSpokeClient() *clients.Settings {
	if spokeClient == nil {
		By("Get spoke admin kubeconfig")

		adminKubeconfigSecret, err := secret.Pull(APIClient,
			fmt.Sprintf("%s-admin-kubeconfig", MGMTConfig.Cluster.Info.ClusterName), MGMTConfig.Cluster.Info.ClusterName)
		Expect(err).NotTo(HaveOccurred(), "error pulling spoke kubeconfig secret")

		adminKubeconfigContent, ok := adminKubeconfigSecret.Object.Data["kubeconfig"]
		Expect(ok).To(BeTrue(), "error checking for kubeconfig key from admin kubeconfig secret")

		By("Writing spoke admin kubeconfig to file")

		err = os.WriteFile("/tmp/spoke-kubeconfig", adminKubeconfigContent, 0755)
		Expect(err).NotTo(HaveOccurred(), "error writing spoke kubeconfig to file")

		spokeClient = clients.New("/tmp/spoke-kubeconfig")
		Expect(spokeClient).NotTo(BeNil(), "error creating client from spoke kubeconfig file")
	}

	return spokeClient
}
