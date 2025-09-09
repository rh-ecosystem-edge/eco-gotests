package operator_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/assisted"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/hive"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/api/hiveextension/v1beta1"
	agentinstallv1beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/api/v1beta1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/ztpinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/operator/internal/tsparams"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	timeout                    = 300 * time.Second
	trustBundleTestNS          = "trustbundle-test"
	additionalTrustCertificate = `-----BEGIN CERTIFICATE-----
MIIFPjCCAyagAwIBAgIUV3ZmDsSwF6/E2CPhFChz3w14OLMwDQYJKoZIhvcNAQEL
BQAwFjEUMBIGA1UEAwwLZXhhbXBsZS5jb20wHhcNMjIxMTI3MjM0MjMwWhcNMzIx
MTI0MjM0MjMwWjAWMRQwEgYDVQQDDAtleGFtcGxlLmNvbTCCAiIwDQYJKoZIhvcN
AQEBBQADggIPADCCAgoCggIBALxURtV3Wd8NEFIplXSZpIdx5I0jFU8thmb2vZON
oNxr31OsHYqkA07RpGSmyn+hv03OI9g4AzuMGs48XoPxZGtWUr0wany1LDDW8t/J
PytYeZyXAJM0zl6/AlzSzYRPk22LykdzVBQosUeRP42a2xWEdDRkJqxxBHQ0eLiC
9g32w57YomhbgCR2OnUxzVmMuQmk987WG7u3/ssSBPEuIebOoX+6G3uLaw/Ka6zQ
XGzRgFq3mskPVfw3exQ46WZfgu6PtG5zxKmty75fNPPwdyw+lwm3u8pH5jpJYvOZ
RHbk7+nxWxLxe5r3FzaNeWskb24J9x53nQzwfcF0MtuRvMycO1i/3e5Y4TanEmmu
GbUOKlJxyaFQaVa2udWAxZ8w1W5u4aKrBprXEAXXDghXbxrgRry2zPO1vqZ/aLH8
YKnHLifjdsNMxrA3nsKAViY0erwYmTF+c551gxkW7vZCtJStzDcMVM16U76jato7
fNb64VUtviVCWeHvh7aTpxENPCh6T8eGh3K4HUESTNpBggs3TXhF1yEcS+aKVJ3z
6CZcke1ph/vpMt/684xx8tICp2KMWbwk3nIBaMw84hrVZyKFgpW/gZOE+ktV91zw
LF1oFn+2F8PwGSphBwhBE0uoyFRNmUXiPsHUyEh7kF7EU5gb1sxTzM5sWCNm6nIS
QRlXAgMBAAGjgYMwgYAwHQYDVR0OBBYEFHuAjvmIDJX76uWtnfirReeBU+f2MB8G
A1UdIwQYMBaAFHuAjvmIDJX76uWtnfirReeBU+f2MA8GA1UdEwEB/wQFMAMBAf8w
LQYDVR0RBCYwJIILZXhhbXBsZS5jb22CD3d3dy5leGFtcGxlLm5ldIcECgAAATAN
BgkqhkiG9w0BAQsFAAOCAgEACn2BTzH89jDBHAy1rREJY8nYhH8GQxsPQn3MZAjA
OiAQRSqqaduYdM+Q6X3V/A8n2vtS1vjs2msQwg6uNN/yNNgdo+Nobj74FmF+kwaf
hodvMJ7z+MyeuxONYL/rbolc8N031nPWim8HTQsS/hxiiwqMHzgz6hQou1OFPwTJ
QdhsfXgqbNRiMkF/UxLfIDEP8J5VAEzVJlyrGUrUOuaMU6TZ+tx1VbNQm3Xum5GW
UgtmE36wWp/M1VeNSsm3GOQRlyWFGmE0sgA95IxLRMgL1mpd8IS3iU6TVZLx0+sA
Bly38R1z8Vcwr1vOurQ8g76Epdet2ZkQNQBwvgeVvnCsoy4CQf2AvDzKgEeTdXMM
WdO6UnG2+PgJ6YQHyfCB34mjPqrJul/0YwWo/p+PxSHRKdJZJTKzZPi1sPuxA2iO
YiJIS94ZRlkPxrD4pYdGiXPigC+0motT6cYxQ8SKTVOs7aEax/xQngrcQPLNXTgn
LtoT4hLCJpP7PTLgL91Dvu/dUMR4SEUNojUBul67D5fIjD0sZvJFZGd78apl/gdf
PxkCHm4A07Zwl/x+89Ia73mk+y8O2u+CGh7oDrO565ADxKj6/UhxhVKmV9DG1ono
AjGUGkvXVVvurf5CwGxpwT/G5UXpSK+314eMVxz5s3yDb2J2J2rvIk6ROPxBK0ws
Sj8=
-----END CERTIFICATE-----`

	additionalTrustCertificateEmpty = `-----BEGIN CERTIFICATE----
-----END CERTIFICATE-----`
)

var _ = Describe(
	"AdditionalTrustBundle",
	Ordered,
	ContinueOnFailure,
	Label(tsparams.LabelAdditionalTrustBundle), func() {
		When("on MCE 2.4 and above", func() {
			BeforeAll(func() {

				tsparams.ReporterNamespacesToDump[trustBundleTestNS] = "trustbundle-test namespace"

				By("Create trustbundle-test namespace")
				testNS, err = namespace.NewBuilder(HubAPIClient, trustBundleTestNS).Create()
				Expect(err).ToNot(HaveOccurred(), "error occurred when creating namespace")

				By("Create trustbundle-test pull-secret")
				testSecret, err = secret.NewBuilder(
					HubAPIClient,
					trustBundleTestNS+"-pull-secret",
					trustBundleTestNS,
					corev1.SecretTypeDockerConfigJson).WithData(ZTPConfig.HubPullSecret.Object.Data).Create()
				Expect(err).ToNot(HaveOccurred(), "error occurred when creating pull-secret")

				By("Create trustbundle-test clusterdeployment")
				testClusterDeployment, err = hive.NewABMClusterDeploymentBuilder(
					HubAPIClient,
					trustBundleTestNS+"clusterdeployment",
					testNS.Definition.Name,
					trustBundleTestNS,
					"assisted.test.com",
					trustBundleTestNS,
					metav1.LabelSelector{
						MatchLabels: map[string]string{
							"dummy": "label",
						},
					}).WithPullSecret(testSecret.Definition.Name).Create()
				Expect(err).ToNot(HaveOccurred(), "error occurred when creating clusterdeployment")

				By("Create agentclusterinstall")

				testAgentClusterInstall, err = assisted.NewAgentClusterInstallBuilder(
					HubAPIClient,
					trustBundleTestNS+"agentclusterinstall",
					testNS.Definition.Name,
					testClusterDeployment.Definition.Name,
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
					}).Create()
				Expect(err).ToNot(HaveOccurred(), "error creating agentclusterinstall")
			})

			It("Validates that InfraEnv can be updated with additionalTrustedBundle", reportxml.ID("65936"), func() {
				By("Creating Infraenv")
				infraenv := assisted.NewInfraEnvBuilder(
					HubAPIClient,
					"testinfraenv",
					trustBundleTestNS,
					testSecret.Definition.Name)
				infraenv.Definition.Spec.AdditionalTrustBundle = additionalTrustCertificate
				_, err = infraenv.Create()
				Eventually(func() (string, error) {
					infraenv.Object, err = infraenv.Get()
					if err != nil {
						return "", err
					}

					return infraenv.Object.Status.ISODownloadURL, nil
				}).WithTimeout(time.Minute*3).ProbeEvery(time.Second*3).
					Should(Not(BeEmpty()), "error waiting for download url to be created")
				By("Checking additionalTrustBundle equal to additionalTrustCertificate")
				Expect(infraenv.Object.Spec.AdditionalTrustBundle).
					To(Equal(additionalTrustCertificate), "infraenv was created with wrong certificate")
				By("Checking image was created with additionalTrustCertificate")
				By("Getting Infraenv")
				infraenv, err = assisted.PullInfraEnvInstall(HubAPIClient, "testinfraenv", trustBundleTestNS)
				Expect(err).ToNot(HaveOccurred(), "error retrieving infraenv")
				for _, condition := range infraenv.Object.Status.Conditions {
					if agentinstallv1beta1.ImageCreatedCondition == condition.Type {
						Expect(condition.Status).To(Equal(corev1.ConditionTrue), "error creating image")
					}
				}

			})

			It("Validates invalid certificate throws proper status", reportxml.ID("67490"), func() {
				By("Creating Infraenv")
				infraenv := assisted.NewInfraEnvBuilder(
					HubAPIClient,
					"testinfraenv",
					trustBundleTestNS,
					testSecret.Definition.Name)
				infraenv.Definition.Spec.AdditionalTrustBundle = additionalTrustCertificateEmpty
				_, err = infraenv.Create()
				Expect(err).ToNot(HaveOccurred(), "error creating infraenv")
				Eventually(func() (string, error) {
					infraenv.Object, err = infraenv.Get()
					if err != nil {
						return "", err
					}

					return infraenv.Object.Status.ISODownloadURL, nil
				}).WithTimeout(time.Minute*3).ProbeEvery(time.Second*3).
					Should(BeEmpty(), "error waiting for download url to be created")
				By("Getting Infraenv")
				infraenv, err = assisted.PullInfraEnvInstall(HubAPIClient, "testinfraenv", trustBundleTestNS)
				Expect(err).ToNot(HaveOccurred(), "error in retrieving infraenv")
				By("Checking additionalTrustBundle equal to additionalTrustCertificateEmpty")
				Expect(infraenv.Object.Spec.AdditionalTrustBundle).
					To(Equal(additionalTrustCertificateEmpty), "certificate should be empty")
				By("Checking image was not created due to invalid certificate")
				for _, condition := range infraenv.Object.Status.Conditions {
					if agentinstallv1beta1.ImageCreatedCondition == condition.Type {
						Expect(condition.Status).ToNot(Equal(corev1.ConditionTrue), "image was created with invalid certificate")
					}
				}

			})
			AfterEach(func() {
				By("Getting Infraenv")
				infraenv, err := assisted.PullInfraEnvInstall(HubAPIClient, "testinfraenv", trustBundleTestNS)
				Expect(err).ToNot(HaveOccurred(), "error retrieving infraenv")
				By("Deleting infraenv")
				err = infraenv.DeleteAndWait(time.Second * 20)
				Expect(err).ToNot(HaveOccurred(), "error deleting infraenv")
			})

			AfterAll(func() {

				By("Deleting agentCLusterInstall")
				err = testAgentClusterInstall.Delete()
				Expect(err).ToNot(HaveOccurred(), "error deleting aci")

				By("Deleting clusterdeployment")
				err = testClusterDeployment.Delete()
				Expect(err).ToNot(HaveOccurred(), "error deleting clusterdeployment")

				By("Deleting pull secret")
				err = testSecret.Delete()
				Expect(err).ToNot(HaveOccurred(), "error deleting pull secret")

				By("Deleting test namespace")
				err = testNS.DeleteAndWait(timeout)
				Expect(err).ToNot(HaveOccurred(), "error deleting test namespace")
			})

		})
	})
