package operator_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/olm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/meets"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/ztpinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/operator/internal/tsparams"
)

const (
	hiveNamespace = "hive"
	acmNamespace  = "rhacm"
	acmCSVPattern = "advanced-cluster-management"
)

var _ = Describe(
	"Deploying",
	Ordered,
	ContinueOnFailure,
	Label(tsparams.LabelAssistedHiveOperatorDeploy), func() {
		When("on MCE 2.0 and above", func() {
			DescribeTable("infrastructure operator", func(requirement func() (bool, string)) {
				if reqMet, msg := requirement(); !reqMet {
					Skip(msg)
				}

				By("Checking that the operand was successfully deployed")
				ok, msg := meets.HubInfrastructureOperandRunningRequirement()
				Expect(ok).To(BeTrue(), msg)

				By("Check that pods exist in hive namespace")
				hivePods, err := pod.List(HubAPIClient, hiveNamespace)
				Expect(err).NotTo(HaveOccurred(), "error occurred while listing pods in hive namespace")
				Expect(len(hivePods) > 0).To(BeTrue(), "error: did not find any pods in the hive namespace")

				By("Check that hive pods are running correctly")
				running, err := pod.WaitForAllPodsInNamespaceRunning(HubAPIClient, hiveNamespace, time.Minute)
				Expect(err).NotTo(HaveOccurred(), "error occurred while waiting for hive pods to be in Running state")
				Expect(running).To(BeTrue(), "some hive pods are not in Running state")
			},
				Entry("in an IPv4 environment is successful", meets.HubSingleStackIPv4Requirement, reportxml.ID("41634")),
				Entry("in an IPv6 environment is successful", meets.HubSingleStackIPv6Requirement, reportxml.ID("41640")),
			)

			DescribeTable("by advanced cluster management operator", func(requirement func() (bool, string)) {
				if reqMet, msg := requirement(); !reqMet {
					Skip(msg)
				}

				By("Checking that rhacm namespace exists")
				_, err := namespace.Pull(HubAPIClient, acmNamespace)
				if err != nil {
					Skip("Advanced Cluster Management is not installed")
				}

				By("Getting clusterserviceversion")
				clusterServiceVersions, err := olm.ListClusterServiceVersionWithNamePattern(
					HubAPIClient, acmCSVPattern, acmNamespace)
				Expect(err).NotTo(HaveOccurred(), "error listing clusterserviceversions by name pattern")
				Expect(len(clusterServiceVersions)).To(Equal(1), "error did not receieve expected list of clusterserviceversions")

				success, err := clusterServiceVersions[0].IsSuccessful()
				Expect(err).NotTo(HaveOccurred(), "error checking clusterserviceversions phase")
				Expect(success).To(BeTrue(), "error advanced-cluster-management clusterserviceversion is not Succeeded")

				By("Check that pods exist in rhacm namespace")
				rhacmPods, err := pod.List(HubAPIClient, acmNamespace)
				Expect(err).NotTo(HaveOccurred(), "error occurred while listing pods in rhacm namespace")
				Expect(len(rhacmPods) > 0).To(BeTrue(), "error: did not find any pods in the hive namespace")

				By("Check that rhacm pods are running correctly")
				running, err := pod.WaitForAllPodsInNamespaceRunning(HubAPIClient, acmNamespace, time.Minute)
				Expect(err).NotTo(HaveOccurred(), "error occurred while waiting for rhacm pods to be in Running state")
				Expect(running).To(BeTrue(), "some rhacm pods are not in Running state")
			},
				Entry("in an IPv4 environment is successful", meets.HubSingleStackIPv4Requirement, reportxml.ID("42042")),
				Entry("in an IPv6 environment is successful", meets.HubSingleStackIPv6Requirement, reportxml.ID("42043")),
			)
		})
	})
