package deploy_test

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmh"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clusterversion"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/hive"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ibi"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	siteconfigv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/siteconfig/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/siteconfig"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedinstall/mgmt/deploy/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedinstall/mgmt/internal/mgmtinittools"
)

var _ = Describe(
	"Performing re-install with image based installation",
	Ordered,
	Label(tsparams.LabelReinstall), func() {
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
			reportxml.ID("83059"), func() {
				if !MGMTConfig.StaticNetworking {
					Skip("Cluster is deployed without static networking")
				}

				if MGMTConfig.SiteConfig {
					Skip("Cluster is deployed with siteconfig operator")
				}

				tsparams.ReporterNamespacesToDump[MGMTConfig.Cluster.Info.ClusterName] = reporterNamespaceToDump

				By("Load original spoke api client")
				spokeClient := getSpokeClient()

				By("Pulling existing clusterdeployment")
				clusterDeployment, err := hive.PullClusterDeployment(APIClient, MGMTConfig.Cluster.Info.ClusterName,
					MGMTConfig.Cluster.Info.ClusterName)
				Expect(err).NotTo(HaveOccurred(), "error pulling clusterdeployment")

				originalClusterID := clusterDeployment.Object.Spec.ClusterMetadata.ClusterID

				By("Pulling existing admin-kubeconfig secret")
				ibiAdminKubeconfigSecret, err := secret.Pull(APIClient, MGMTConfig.Cluster.Info.ClusterName+"-admin-kubeconfig",
					MGMTConfig.Cluster.Info.ClusterName)
				Expect(err).NotTo(HaveOccurred(), "error pulling admin-kubeconfig secret")
				ibiAdminKubeconfigSecret.Definition.ObjectMeta.CreationTimestamp = metav1.Time{}
				ibiAdminKubeconfigSecret.Definition.ObjectMeta.OwnerReferences = nil
				ibiAdminKubeconfigSecret.Definition.ObjectMeta.UID = ""
				ibiAdminKubeconfigSecret.Definition.ObjectMeta.ResourceVersion = ""

				By("Pulling existing admin-password secret")
				ibiAdminPasswordSecret, err := secret.Pull(APIClient, MGMTConfig.Cluster.Info.ClusterName+"-admin-password",
					MGMTConfig.Cluster.Info.ClusterName)
				Expect(err).NotTo(HaveOccurred(), "error pulling admin-password secret")
				ibiAdminPasswordSecret.Definition.ObjectMeta.CreationTimestamp = metav1.Time{}
				ibiAdminPasswordSecret.Definition.ObjectMeta.OwnerReferences = nil
				ibiAdminPasswordSecret.Definition.ObjectMeta.UID = ""
				ibiAdminPasswordSecret.Definition.ObjectMeta.ResourceVersion = ""

				By("Pulling existing seed-reconfiguration secret")
				ibiSeedRecoonfigurationSecret, err := secret.Pull(
					APIClient, MGMTConfig.Cluster.Info.ClusterName+"-seed-reconfiguration",
					MGMTConfig.Cluster.Info.ClusterName)
				Expect(err).NotTo(HaveOccurred(), "error pulling seed-reconfiguration secret")
				ibiSeedRecoonfigurationSecret.Definition.ObjectMeta.CreationTimestamp = metav1.Time{}
				ibiSeedRecoonfigurationSecret.Definition.ObjectMeta.OwnerReferences = nil
				ibiSeedRecoonfigurationSecret.Definition.ObjectMeta.UID = ""
				ibiSeedRecoonfigurationSecret.Definition.ObjectMeta.ResourceVersion = ""

				By("Listing baremetalhosts")
				ibiBmhList, err := bmh.List(APIClient, MGMTConfig.Cluster.Info.ClusterName)
				Expect(err).NotTo(HaveOccurred(), "error listing BMH resources")

				By("Deleting baremetalhosts")
				for _, bmhBulider := range ibiBmhList {
					_, err = bmhBulider.Delete()
					Expect(err).NotTo(HaveOccurred(), "error deleting BMH resource %s", bmhBulider.Definition.Name)
					waitForResourceToDelete("baremetalhost", bmhBulider.Exists)
				}

				By("Pulling imageclusterinstall")
				ibiICI, err := ibi.PullImageClusterInstall(APIClient,
					MGMTConfig.Cluster.Info.ClusterName, MGMTConfig.Cluster.Info.ClusterName)
				Expect(err).NotTo(HaveOccurred(), "error pulling imageclusterinstall")

				By("Deleting imageclusterinstall")
				err = ibiICI.Delete()
				Expect(err).NotTo(HaveOccurred(), "error deleting imageclusterinstall resource")
				waitForResourceToDelete("imageclusterinstall", ibiICI.Exists)

				By("Pulling clusterdeployment")
				ibiCD, err := hive.PullClusterDeployment(APIClient,
					MGMTConfig.Cluster.Info.ClusterName, MGMTConfig.Cluster.Info.ClusterName)
				Expect(err).NotTo(HaveOccurred(), "error pulling clusterdeployment")

				By("Deleting clusterdeployment")
				err = ibiCD.Delete()
				Expect(err).NotTo(HaveOccurred(), "error deleting clusterdeployment resource")
				waitForResourceToDelete("clusterdeployment", ibiCD.Exists)

				By("Pulling managedcluster")
				ibiManagedCluster, err := ocm.PullManagedCluster(APIClient,
					MGMTConfig.Cluster.Info.ClusterName)
				Expect(err).NotTo(HaveOccurred(), "error pulling managedcluster")

				By("Deleting managedcluster")
				err = ibiManagedCluster.Delete()
				Expect(err).NotTo(HaveOccurred(), "error deleting managedcluster resource")
				waitForResourceToDelete("managedcluster", ibiManagedCluster.Exists)

				By("Pulling namespace")
				ibiNamespace, err := namespace.Pull(APIClient, MGMTConfig.Cluster.Info.ClusterName)
				if err != nil {
					Expect(err.Error()).To(ContainSubstring("does not exist"), "error pulling namespace")
				} else {
					Expect(err).NotTo(HaveOccurred(), "error pulling namespace")
					waitForResourceToDelete("namespace", ibiNamespace.Exists)
				}

				By("Create namespace for IBI reinstallation")

				_, err = namespace.NewBuilder(APIClient, MGMTConfig.Cluster.Info.ClusterName).Create()
				Expect(err).NotTo(HaveOccurred(), "error creating namespace")

				By("Re-create admin-kubeconfig secret")
				_, err = ibiAdminKubeconfigSecret.Create()
				Expect(err).NotTo(HaveOccurred(), "error re-creating admin-kubeconfig secret")

				By("Re-create admin-password secret")
				_, err = ibiAdminPasswordSecret.Create()
				Expect(err).NotTo(HaveOccurred(), "error re-creating admin-password secret")

				By("Re-create seed-reconfiguration secret")
				_, err = ibiSeedRecoonfigurationSecret.Create()
				Expect(err).NotTo(HaveOccurred(), "error re-creating seed-reconfiguration secret")

				createIBIOResouces(ipv4AddrFamily)

				By("Pull spoke cluster version using original client")
				targetClusterVersion, err := clusterversion.Pull(spokeClient)
				Expect(err).NotTo(HaveOccurred(), "error pulling target cluster OCP version")
				Expect(targetClusterVersion.Object.Status.Desired.Version).To(
					Equal(MGMTConfig.SeedClusterInfo.SeedClusterOCPVersion),
					"error: target cluster version does not match seedimage cluster version")
				Expect(originalClusterID).To(Equal(string(targetClusterVersion.Object.Spec.ClusterID)),
					"error: reinstalled cluster has different cluster identity than original cluster")

			})

		It("through siteconfig operator is successful in an IPv4 environment with DHCP networking",
			reportxml.ID("83060"), func() {
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

				reinstallWithClusterInstance(ipv4AddrFamily)
			})

		It("through siteconfig operator is successful in a primary IPv4 dual-stack "+
			"proxy-enabled environment with DHCP networking",
			reportxml.ID("83061"), func() {
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

				reinstallWithClusterInstance(dualstackPrimaryv4AddrFamily)

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

				reinstallWithClusterInstance(dualstackPrimaryv4AddrFamily)

			})
	})

func waitForResourceToDelete(resourceType string, exists func() bool) {
	Eventually(func() bool {
		return !exists()
	}).WithTimeout(time.Minute*30).WithPolling(time.Second*10).Should(
		BeTrue(), "error waiting for resource %s to be deleted", resourceType)
}

//nolint:gocognit,funlen
func reinstallWithClusterInstance(addressFamily string) {
	By("Load original spoke api client")

	spokeClient := getSpokeClient()

	By("Enable cluster reinstallation")

	scoConfig, err := configmap.Pull(APIClient, "siteconfig-operator-configuration", tsparams.RHACMNamespace)
	Expect(err).NotTo(HaveOccurred(), "error pulling siteconfig-operator-configuration configmap")

	scoConfig.Definition.Data["allowReinstalls"] = "true"
	_, err = scoConfig.Update()
	Expect(err).NotTo(HaveOccurred(), "error updating siteconfig-operator-configuration configmap to allow reinstalls")

	By("Pulling existing clusterdeployment")

	clusterDeployment, err := hive.PullClusterDeployment(APIClient, MGMTConfig.Cluster.Info.ClusterName,
		MGMTConfig.Cluster.Info.ClusterName)
	Expect(err).NotTo(HaveOccurred(), "error pulling clusterdeployment")

	originalClusterID := clusterDeployment.Object.Spec.ClusterMetadata.ClusterID

	By("Pulling existing clusterinstance")

	clusterInstace, err := siteconfig.PullClusterInstance(APIClient, MGMTConfig.Cluster.Info.ClusterName,
		MGMTConfig.Cluster.Info.ClusterName)
	Expect(err).NotTo(HaveOccurred(), "error pulling clusterinstance")

	for idx := range clusterInstace.Definition.Spec.Nodes {
		for _, host := range MGMTConfig.Cluster.Info.Hosts {
			if addressFamily == ipv4AddrFamily || addressFamily == dualstackPrimaryv4AddrFamily {
				clusterInstace.Definition.Spec.Nodes[idx].BmcAddress = host.BMC.URLv4
			} else {
				clusterInstace.Definition.Spec.Nodes[idx].BmcAddress = host.BMC.URLv6
			}

			clusterInstace.Definition.Spec.Nodes[idx].BootMACAddress = host.BMC.MACAddress
		}
	}

	if clusterInstace.Definition.Spec.Reinstall == nil {
		clusterInstace.Definition.Spec.Reinstall = new(siteconfigv1alpha1.ReinstallSpec)
	}

	clusterInstace.Definition.Spec.Reinstall.Generation = MGMTConfig.ReinstallGenerationLabel
	clusterInstace.Definition.Spec.Reinstall.PreservationMode = "ClusterIdentity"

	By("Updating clusterinstance for reinstallation")

	_, err = clusterInstace.Update(false)
	Expect(err).NotTo(HaveOccurred(), "error updating clusterinstance")

	By("Waiting for clusterinstance re-installation to trigger")

	Eventually(func() (bool, error) {
		clusterInstace.Object, err = clusterInstace.Get()
		if err != nil {
			return false, err
		}

		if clusterInstace.Object.Status.Reinstall == nil || clusterInstace.Object.Status.Reinstall.Conditions == nil {
			return false, nil
		}

		for _, condition := range clusterInstace.Object.Status.Reinstall.Conditions {
			if condition.Type == string(siteconfigv1alpha1.ReinstallRequestProcessed) {
				return condition.Status == "True" && condition.Reason == string(siteconfigv1alpha1.Completed), nil
			}
		}

		return false, nil
	}).WithTimeout(time.Minute*40).WithPolling(time.Second*10).Should(
		BeTrue(), "error waiting for clusterinstance to begin re-install")

	By("Waiting for clusterinstance to start provisioning")

	Eventually(func() (bool, error) {
		clusterInstace.Object, err = clusterInstace.Get()
		if err != nil {
			return false, err
		}

		for _, condition := range clusterInstace.Object.Status.Conditions {
			if condition.Type == string(siteconfigv1alpha1.ClusterProvisioned) {
				return condition.Status == falseStatus && condition.Reason == string(siteconfigv1alpha1.InProgress), nil
			}
		}

		return false, nil
	}).WithTimeout(time.Minute*5).WithPolling(time.Second*10).Should(
		BeTrue(), "error waiting for clusterinstance to start provisioning")

	By("Waiting for clusterinstance to finish provisioning")

	Eventually(func() (bool, error) {
		clusterInstace.Object, err = clusterInstace.Get()
		if err != nil {
			return false, err
		}

		for _, condition := range clusterInstace.Object.Status.Conditions {
			if condition.Type == string(siteconfigv1alpha1.ClusterProvisioned) {
				return condition.Status == trueStatus && condition.Reason == string(siteconfigv1alpha1.Completed), nil
			}
		}

		return false, nil
	}).WithTimeout(time.Minute*30).WithPolling(time.Second*10).Should(
		BeTrue(), "error waiting for clusterinstance to finish provisioning")

	By("Pull spoke cluster version using original client")

	targetClusterVersion, err := clusterversion.Pull(spokeClient)
	Expect(err).NotTo(HaveOccurred(), "error pulling target cluster OCP version")
	Expect(targetClusterVersion.Object.Status.Desired.Version).To(
		Equal(MGMTConfig.SeedClusterInfo.SeedClusterOCPVersion),
		"error: target cluster version does not match seedimage cluster version")
	Expect(originalClusterID).To(Equal(string(targetClusterVersion.Object.Spec.ClusterID)),
		"error: reinstalled cluster has different cluster identity than original cluster")
}
