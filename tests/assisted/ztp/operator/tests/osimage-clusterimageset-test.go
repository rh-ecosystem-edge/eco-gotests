package operator_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/assisted"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/hive"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/api/hiveextension/v1beta1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/ztpinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/operator/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	osImageClusterImageSetName = "osimage-clusterimageset-test"
)

var (
	osImageClusterImageSetNS       *namespace.Builder
	osImageClusterImageSetSecret   *secret.Builder
	osImageClusterImageSetCD       *hive.ClusterDeploymentBuilder
	osImageClusterImageSetACI      *assisted.AgentClusterInstallBuilder
	osImageClusterImageSetInfraEnv *assisted.InfraEnvBuilder
)

var _ = Describe(
	"Hub cluster with corresponding OS image version and clusterimageset",
	Ordered,
	ContinueOnFailure,
	Label(tsparams.LabelClusterImageSetMatchingOSImage), func() {
		When("on MCE 2.0 and above", func() {
			BeforeAll(func() {
				By("Checking that clusterimageset is available on hub cluster")
				_, err := hive.PullClusterImageSet(HubAPIClient, ZTPConfig.HubOCPXYVersion)
				if err != nil {
					Skip("Clusterimageset not created on hub")
				}

				osImageFound := false

				for _, image := range ZTPConfig.HubAgentServiceConfig.Object.Spec.OSImages {
					if image.OpenshiftVersion == ZTPConfig.HubOCPXYVersion {
						osImageFound = true
					}
				}

				if !osImageFound {
					Skip("Corresponding OS Image not found in AgentServiceConfig")
				}

				tsparams.ReporterNamespacesToDump[osImageClusterImageSetName] = "osimage-clusterimageset-test namespace"
			})

			It("result in successfully created infraenv", reportxml.ID("41937"), func() {
				By("Creating osimage-clusterimageset-test namespace")
				osImageClusterImageSetNS, err = namespace.NewBuilder(HubAPIClient, osImageClusterImageSetName).Create()
				Expect(err).ToNot(HaveOccurred(), "error occurred when creating namespace")

				By("Creating osimage-clusterimageset-test pull-secret")
				osImageClusterImageSetSecret, err = secret.NewBuilder(
					HubAPIClient,
					fmt.Sprintf("%s-pull-secret", osImageClusterImageSetName),
					osImageClusterImageSetName,
					corev1.SecretTypeDockerConfigJson).WithData(ZTPConfig.HubPullSecret.Object.Data).Create()
				Expect(err).ToNot(HaveOccurred(), "error occurred when creating pull-secret")

				By("Creating osimage-clusterimageset-test clusterdeployment")
				osImageClusterImageSetCD, err = hive.NewABMClusterDeploymentBuilder(
					HubAPIClient,
					osImageClusterImageSetName,
					osImageClusterImageSetName,
					osImageClusterImageSetName,
					"assisted.test.com",
					osImageClusterImageSetName,
					metav1.LabelSelector{
						MatchLabels: map[string]string{
							"dummy": "label",
						},
					}).WithPullSecret(osImageClusterImageSetSecret.Definition.Name).Create()
				Expect(err).ToNot(HaveOccurred(), "error occurred when creating clusterdeployment")

				By("Creating osimage-clusterimageset-test agentclusterinstall")
				osImageClusterImageSetACI, err = assisted.NewAgentClusterInstallBuilder(
					HubAPIClient,
					osImageClusterImageSetName,
					osImageClusterImageSetName,
					osImageClusterImageSetName,
					3,
					2,
					v1beta1.Networking{
						ClusterNetwork: []v1beta1.ClusterNetworkEntry{{
							CIDR:       "10.128.0.0/14",
							HostPrefix: 23,
						}},
						MachineNetwork: []v1beta1.MachineNetworkEntry{{
							CIDR: "192.168.254.0/24",
						}},
						ServiceNetwork: []string{"172.30.0.0/16"},
					}).WithImageSet(ZTPConfig.HubOCPXYVersion).Create()

				By("Creating osimage-clusterimageset-test infraenv")
				osImageClusterImageSetInfraEnv, err = assisted.NewInfraEnvBuilder(
					HubAPIClient,
					osImageClusterImageSetName,
					osImageClusterImageSetName,
					osImageClusterImageSetSecret.Definition.Name).
					WithClusterRef(osImageClusterImageSetName, osImageClusterImageSetName).Create()
				Expect(err).ToNot(HaveOccurred(), "error creating infraenv")

				Eventually(func() (string, error) {
					osImageClusterImageSetInfraEnv.Object, err = osImageClusterImageSetInfraEnv.Get()
					if err != nil {
						return "", err
					}

					return osImageClusterImageSetInfraEnv.Object.Status.ISODownloadURL, nil
				}).WithTimeout(time.Minute*3).ProbeEvery(time.Second*3).
					Should(Not(BeEmpty()), "error waiting for download url to be created")
			})

			AfterAll(func() {
				By("Deleting osimage-clusterimageset-test infraenv")
				err = osImageClusterImageSetInfraEnv.Delete()
				Expect(err).ToNot(HaveOccurred(), "error deleting infraenv")

				By("Deleting osimage-clusterimageset-test agentclusterinstall")
				err = osImageClusterImageSetACI.Delete()
				Expect(err).ToNot(HaveOccurred(), "error deleting agentclusterinstall")

				By("Deleting osimage-clusterimageset-test clusterdeployment")
				err = osImageClusterImageSetCD.Delete()
				Expect(err).ToNot(HaveOccurred(), "error deleting clusterdeployment")

				By("Deleting osimage-clusterimageset-test secret")
				err = osImageClusterImageSetSecret.Delete()
				Expect(err).ToNot(HaveOccurred(), "error deleting secret")

				By("Deleting osimage-clusterimageset-test namespace")
				err = osImageClusterImageSetNS.DeleteAndWait(time.Second * 120)
				Expect(err).ToNot(HaveOccurred(), "error deleting namespace")
			})
		})
	})
