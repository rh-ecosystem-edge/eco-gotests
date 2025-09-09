package operator_test

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/assisted"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/hive"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/api/hiveextension/v1beta1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/meets"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/ztpinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/operator/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	platformtypeSpoke = "platform-test"
)

var (
	testNS                  *namespace.Builder
	testSecret              *secret.Builder
	testClusterDeployment   *hive.ClusterDeploymentBuilder
	testAgentClusterInstall *assisted.AgentClusterInstallBuilder
	err                     error
)

var _ = Describe(
	"PlatformSelection",
	Ordered,
	ContinueOnFailure,
	Label(tsparams.LabelPlatformSelectionTestCases), func() {
		When("on MCE 2.1", func() {
			BeforeAll(func() {
				By("Check clusterimageset ocp version meets requirement")
				reqMet, msg := meets.SpokeClusterImageSetVersionRequirement("4.8")
				if !reqMet {
					Skip(msg)
				}

				tsparams.ReporterNamespacesToDump[platformtypeSpoke] = "platform-selection namespace"

				By("Create platform-test namespace")
				testNS, err = namespace.NewBuilder(HubAPIClient, platformtypeSpoke).Create()
				Expect(err).ToNot(HaveOccurred(), "error occurred when creating namespace")

				By("Create platform-test pull-secret")
				testSecret, err = secret.NewBuilder(
					HubAPIClient,
					fmt.Sprintf("%s-pull-secret", platformtypeSpoke),
					platformtypeSpoke,
					corev1.SecretTypeDockerConfigJson).WithData(ZTPConfig.HubPullSecret.Object.Data).Create()
				Expect(err).ToNot(HaveOccurred(), "error occurred when creating pull-secret")

				By("Create platform-test clusterdeployment")
				testClusterDeployment, err = hive.NewABMClusterDeploymentBuilder(
					HubAPIClient,
					platformtypeSpoke,
					testNS.Definition.Name,
					platformtypeSpoke,
					"assisted.test.com",
					platformtypeSpoke,
					metav1.LabelSelector{
						MatchLabels: map[string]string{
							"dummy": "label",
						},
					}).WithPullSecret(testSecret.Definition.Name).Create()
				Expect(err).ToNot(HaveOccurred(), "error occurred when creating clusterdeployment")
			})

			DescribeTable("defining agentclusterinstall",
				func(
					platformType v1beta1.PlatformType,
					userManagedNetworking bool,
					masterCount int,
					workerCount int,
					message string) {

					By("Create agentclusterinstall")
					testAgentClusterInstall, err = assisted.NewAgentClusterInstallBuilder(
						HubAPIClient,
						platformtypeSpoke,
						testNS.Definition.Name,
						testClusterDeployment.Definition.Name,
						masterCount,
						workerCount,
						v1beta1.Networking{
							ClusterNetwork: []v1beta1.ClusterNetworkEntry{{
								CIDR:       "10.128.0.0/14",
								HostPrefix: 23,
							}},
							MachineNetwork: []v1beta1.MachineNetworkEntry{{
								CIDR: "192.168.254.0/24",
							}},
							ServiceNetwork: []string{"172.30.0.0/16"},
						}).WithImageSet(ZTPConfig.SpokeClusterImageSet).
						WithPlatformType(platformType).
						WithUserManagedNetworking(userManagedNetworking).Create()
					if masterCount == 3 {
						Expect(err).To(HaveOccurred(), "error: created agentclusterinstall with invalid data")
						Expect(strings.Contains(err.Error(), message)).To(BeTrue(), "error: received incorrect error message")
					} else if masterCount == 1 && !userManagedNetworking {
						Expect(err).To(HaveOccurred(), "error: created agentclusterinstall with invalid data")
						Expect(strings.Contains(err.Error(), message)).To(BeTrue(), "error: received incorrect error message")
					} else {
						Expect(err).ToNot(HaveOccurred(), "error creating agentclusterinstall")
						By("Waiting for condition to report expected failure message")
						Eventually(func() (string, error) {
							testAgentClusterInstall.Object, err = testAgentClusterInstall.Get()
							if err != nil {
								return "", err
							}

							for _, condition := range testAgentClusterInstall.Object.Status.Conditions {
								if condition.Type == v1beta1.ClusterSpecSyncedCondition {
									return condition.Message, nil
								}
							}

							return "", nil
						}).WithTimeout(time.Minute*2).Should(Equal("The Spec could not be synced due to an input error: "+message),
							"got unexpected message from SpecSynced condition")
					}
				},
				Entry("that is SNO with VSphere platform", v1beta1.VSpherePlatformType, true, 1, 0,
					"Single node cluster is not supported alongside vsphere platform", reportxml.ID("56198")),
				Entry("that is SNO with BareMetal platform", v1beta1.BareMetalPlatformType, false, 1, 0,
					"UserManagedNetworking must be set to true with SNO", reportxml.ID("56418")),
				Entry("that is BareMetal platform with user-managed-networking", v1beta1.BareMetalPlatformType, true, 3, 2,
					"Can't set baremetal platform with user-managed-networking enabled", reportxml.ID("56419")),
				Entry("that is None platform without user-managed-networking", v1beta1.NonePlatformType, false, 3, 2,
					"Can't set none platform with user-managed-networking disabled", reportxml.ID("56420")),
			)

			AfterEach(func() {
				if testAgentClusterInstall.Exists() {
					By("Delete agentclusterinstall")
					err := testAgentClusterInstall.DeleteAndWait(time.Second * 10)
					Expect(err).ToNot(HaveOccurred(), "error deleting agentclusterinstall")
				}
			})

			AfterAll(func() {
				By("Delete platform-test clusterdeployment")
				err := testClusterDeployment.Delete()
				Expect(err).ToNot(HaveOccurred(), "error occurred when deleting clusterdeployment")

				By("Delete platform-test pull-secret")
				Expect(testSecret.Delete()).ToNot(HaveOccurred(), "error occurred when deleting pull-secret")

				By("Delete platform-test namespace")
				Expect(testNS.Delete()).ToNot(HaveOccurred(), "error occurred when deleting namespace")
			})
		})
	})
