package ocloudcommon

import (
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/o-cloud/internal/ocloudinittools"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/o-cloud/internal/ocloudparams"

	"sync"

	"github.com/golang/glog"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/olm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/olm/version"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/csv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/shell"
)

// VerifySuccessfulOperatorUpgrade verifies the test case of the successful upgrade of the operators in all
// the SNOs.
//
//nolint:funlen
func VerifySuccessfulOperatorUpgrade(ctx SpecContext) {
	downgradeOperatorImages()

	provisioningRequest1 := VerifyProvisionSnoCluster(
		OCloudConfig.TemplateName,
		OCloudConfig.TemplateVersionDay2,
		OCloudConfig.NodeClusterName1,
		OCloudConfig.OCloudSiteID,
		ocloudparams.PolicyTemplateParameters,
		ocloudparams.ClusterInstanceParameters1)

	provisioningRequest2 := VerifyProvisionSnoCluster(
		OCloudConfig.TemplateName,
		OCloudConfig.TemplateVersionDay2,
		OCloudConfig.NodeClusterName2,
		OCloudConfig.OCloudSiteID,
		ocloudparams.PolicyTemplateParameters,
		ocloudparams.ClusterInstanceParameters2)

	node1, nodePool1, namespace1, clusterInstance1 := VerifyAndRetrieveAssociatedCRsForAI(
		provisioningRequest1.Object.Name, OCloudConfig.ClusterName1, ctx)
	node2, nodePool2, namespace2, clusterInstance2 := VerifyAndRetrieveAssociatedCRsForAI(
		provisioningRequest2.Object.Name, OCloudConfig.ClusterName2, ctx)

	VerifyAllPoliciesInNamespaceAreCompliant(namespace1.Object.Name, ctx, nil, nil)
	VerifyAllPoliciesInNamespaceAreCompliant(namespace2.Object.Name, ctx, nil, nil)

	provisioningRequest1, err := oran.PullPR(HubAPIClient, provisioningRequest1.Object.Name)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to retrieve PR %s", provisioningRequest1.Object.Name))

	VerifyProvisioningRequestIsFulfilled(provisioningRequest1)
	glog.V(ocloudparams.OCloudLogLevel).Infof("Provisioning request %s is fulfilled", provisioningRequest1.Object.Name)

	provisioningRequest2, err = oran.PullPR(HubAPIClient, provisioningRequest2.Object.Name)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to retrieve PR %s", provisioningRequest2.Object.Name))

	VerifyProvisioningRequestIsFulfilled(provisioningRequest2)
	glog.V(ocloudparams.OCloudLogLevel).Infof("Provisioning request %s is fulfilled", provisioningRequest2.Object.Name)

	sno1ApiClient := CreateSnoAPIClient(OCloudConfig.ClusterName1)
	sno2ApiClient := CreateSnoAPIClient(OCloudConfig.ClusterName2)

	oldPTPVersionSno1 := getPtpOperatorVersionInSno(sno1ApiClient)
	oldPTPVersionSno2 := getPtpOperatorVersionInSno(sno2ApiClient)

	upgradeOperatorImages()

	var wg1 sync.WaitGroup

	var mu1 sync.Mutex

	wg1.Add(2)

	go VerifyPoliciesAreNotCompliant(OCloudConfig.ClusterName1, ctx, &wg1, &mu1)
	go VerifyPoliciesAreNotCompliant(OCloudConfig.ClusterName2, ctx, &wg1, &mu1)

	wg1.Wait()

	VerifyAllPoliciesInNamespaceAreCompliant(namespace1.Object.Name, ctx, nil, nil)
	VerifyAllPoliciesInNamespaceAreCompliant(namespace2.Object.Name, ctx, nil, nil)

	provisioningRequest1, err = oran.PullPR(HubAPIClient, provisioningRequest1.Object.Name)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to retrieve PR %s", provisioningRequest1.Object.Name))

	VerifyProvisioningRequestIsFulfilled(provisioningRequest1)
	glog.V(ocloudparams.OCloudLogLevel).Infof("Provisioning request %s is fulfilled", provisioningRequest1.Object.Name)

	provisioningRequest2, err = oran.PullPR(HubAPIClient, provisioningRequest2.Object.Name)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to retrieve PR %s", provisioningRequest2.Object.Name))

	VerifyProvisioningRequestIsFulfilled(provisioningRequest2)
	glog.V(ocloudparams.OCloudLogLevel).Infof("Provisioning request %s is fulfilled", provisioningRequest2.Object.Name)

	newPTPVersionSno1 := getPtpOperatorVersionInSno(sno1ApiClient)
	newPTPVersionSno2 := getPtpOperatorVersionInSno(sno2ApiClient)

	Expect(oldPTPVersionSno1).NotTo(Equal(newPTPVersionSno1),
		fmt.Sprintf("PTP operator has not being upgraded in SNO1: old version (%s) == new version (%s)",
			oldPTPVersionSno1, newPTPVersionSno1))

	Expect(oldPTPVersionSno2).NotTo(Equal(newPTPVersionSno2),
		fmt.Sprintf("PTP operator has not being upgraded in SNO2: old version (%s) == new version (%s)",
			oldPTPVersionSno2, newPTPVersionSno2))

	err = os.RemoveAll("tmp/")
	Expect(err).NotTo(HaveOccurred(), "Error removing directory tmp/")

	var wg2 sync.WaitGroup

	wg2.Add(2)

	go DeprovisionAiSnoCluster(provisioningRequest1, namespace1, clusterInstance1, node1, nodePool1, ctx, &wg2)
	go DeprovisionAiSnoCluster(provisioningRequest2, namespace2, clusterInstance2, node2, nodePool2, ctx, &wg2)

	wg2.Wait()
}

// VerifyFailedOperatorUpgradeAllSnos verifies the test case where the upgrade of the operators fails in all
// the SNOs.
//
//nolint:funlen
func VerifyFailedOperatorUpgradeAllSnos(ctx SpecContext) {
	downgradeOperatorImages()

	provisioningRequest1 := VerifyProvisionSnoCluster(
		OCloudConfig.TemplateName,
		OCloudConfig.TemplateVersionDay2,
		OCloudConfig.NodeClusterName1,
		OCloudConfig.OCloudSiteID,
		ocloudparams.PolicyTemplateParameters,
		ocloudparams.ClusterInstanceParameters1)

	provisioningRequest2 := VerifyProvisionSnoCluster(
		OCloudConfig.TemplateName,
		OCloudConfig.TemplateVersionDay2,
		OCloudConfig.NodeClusterName2,
		OCloudConfig.OCloudSiteID,
		ocloudparams.PolicyTemplateParameters,
		ocloudparams.ClusterInstanceParameters2)

	node1, nodePool1, namespace1, clusterInstance1 := VerifyAndRetrieveAssociatedCRsForAI(
		provisioningRequest1.Object.Name, OCloudConfig.ClusterName1, ctx)
	node2, nodePool2, namespace2, clusterInstance2 := VerifyAndRetrieveAssociatedCRsForAI(
		provisioningRequest2.Object.Name, OCloudConfig.ClusterName2, ctx)

	var wg1 sync.WaitGroup

	var mu1 sync.Mutex

	wg1.Add(2)

	go VerifyAllPoliciesInNamespaceAreCompliant(namespace1.Object.Name, ctx, &wg1, &mu1)
	go VerifyAllPoliciesInNamespaceAreCompliant(namespace2.Object.Name, ctx, &wg1, &mu1)

	wg1.Wait()

	provisioningRequest1, err := oran.PullPR(HubAPIClient, provisioningRequest1.Object.Name)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to retrieve PR %s", provisioningRequest1.Object.Name))

	VerifyProvisioningRequestIsFulfilled(provisioningRequest1)
	glog.V(ocloudparams.OCloudLogLevel).Infof("Provisioning request %s is fulfilled", provisioningRequest1.Object.Name)

	provisioningRequest2, err = oran.PullPR(HubAPIClient, provisioningRequest2.Object.Name)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to retrieve PR %s", provisioningRequest2.Object.Name))

	VerifyProvisioningRequestIsFulfilled(provisioningRequest2)
	glog.V(ocloudparams.OCloudLogLevel).Infof("Provisioning request %s is fulfilled", provisioningRequest2.Object.Name)

	sno1ApiClient := CreateSnoAPIClient(OCloudConfig.ClusterName1)
	sno2ApiClient := CreateSnoAPIClient(OCloudConfig.ClusterName2)

	oldPTPVersionSno1 := getPtpOperatorVersionInSno(sno1ApiClient)
	oldPTPVersionSno2 := getPtpOperatorVersionInSno(sno2ApiClient)

	VerifyAllPodsRunningInNamespace(sno1ApiClient, ocloudparams.PtpNamespace)
	VerifyAllPodsRunningInNamespace(sno2ApiClient, ocloudparams.PtpNamespace)

	upgradeOperatorImages()

	modifyDeploymentResources(
		sno1ApiClient,
		ocloudparams.PtpOperatorSubscriptionName,
		ocloudparams.PtpNamespace,
		ocloudparams.PtpDeploymentName,
		ocloudparams.PtpContainerName,
		ocloudparams.PtpCPURequest,
		ocloudparams.PtpMemoryRequest,
		ocloudparams.PtpCPULimit,
		ocloudparams.PtpMemoryLimit)

	modifyDeploymentResources(
		sno2ApiClient,
		ocloudparams.PtpOperatorSubscriptionName,
		ocloudparams.PtpNamespace,
		ocloudparams.PtpDeploymentName,
		ocloudparams.PtpContainerName,
		ocloudparams.PtpCPURequest,
		ocloudparams.PtpMemoryRequest,
		ocloudparams.PtpCPULimit,
		ocloudparams.PtpMemoryLimit)

	var wg2 sync.WaitGroup

	var mu2 sync.Mutex

	wg2.Add(2)

	go VerifyPoliciesAreNotCompliant(OCloudConfig.ClusterName1, ctx, &wg2, &mu2)
	go VerifyPoliciesAreNotCompliant(OCloudConfig.ClusterName2, ctx, &wg2, &mu2)

	wg2.Wait()

	provisioningRequest1, err = oran.PullPR(HubAPIClient, provisioningRequest1.Object.Name)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to retrieve PR %s", provisioningRequest1.Object.Name))

	VerifyProvisioningRequestTimeout(provisioningRequest1)
	glog.V(ocloudparams.OCloudLogLevel).Infof("Provisioning request %s is timeout", provisioningRequest1.Object.Name)

	provisioningRequest2, err = oran.PullPR(HubAPIClient, provisioningRequest2.Object.Name)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to retrieve PR %s", provisioningRequest2.Object.Name))

	VerifyProvisioningRequestTimeout(provisioningRequest2)
	glog.V(ocloudparams.OCloudLogLevel).Infof("Provisioning request %s is timeout", provisioningRequest2.Object.Name)

	newPTPVersionSno1 := getPtpOperatorVersionInSno(sno1ApiClient)
	newPTPVersionSno2 := getPtpOperatorVersionInSno(sno2ApiClient)

	Expect(oldPTPVersionSno1).To(Equal(newPTPVersionSno1),
		fmt.Sprintf("PTP operator version has changed in SNO1: old version (%s) != new version (%s)",
			oldPTPVersionSno1, newPTPVersionSno1))

	Expect(oldPTPVersionSno2).To(Equal(newPTPVersionSno2),
		fmt.Sprintf("PTP operator version has changed in SNO2: old version (%s) != new version (%s)",
			oldPTPVersionSno2, newPTPVersionSno2))

	err = os.RemoveAll("tmp/")
	Expect(err).NotTo(HaveOccurred(), "Error removing directory /tmp")

	var wg3 sync.WaitGroup

	wg3.Add(2)

	go DeprovisionAiSnoCluster(provisioningRequest1, namespace1, clusterInstance1, node1, nodePool1, ctx, &wg3)
	go DeprovisionAiSnoCluster(provisioningRequest2, namespace2, clusterInstance2, node2, nodePool2, ctx, &wg3)

	wg3.Wait()
}

// VerifyFailedOperatorUpgradeSubsetSnos verifies the test case where the upgrade of the operators fails in a
// subset of the SNOs.
//
//nolint:funlen
func VerifyFailedOperatorUpgradeSubsetSnos(ctx SpecContext) {
	downgradeOperatorImages()

	provisioningRequest1 := VerifyProvisionSnoCluster(
		OCloudConfig.TemplateName,
		OCloudConfig.TemplateVersionDay2,
		OCloudConfig.NodeClusterName1,
		OCloudConfig.OCloudSiteID,
		ocloudparams.PolicyTemplateParameters,
		ocloudparams.ClusterInstanceParameters1)

	provisioningRequest2 := VerifyProvisionSnoCluster(
		OCloudConfig.TemplateName,
		OCloudConfig.TemplateVersionDay2,
		OCloudConfig.NodeClusterName2,
		OCloudConfig.OCloudSiteID,
		ocloudparams.PolicyTemplateParameters,
		ocloudparams.ClusterInstanceParameters2)

	node1, nodePool1, namespace1, clusterInstance1 := VerifyAndRetrieveAssociatedCRsForAI(
		provisioningRequest1.Object.Name, OCloudConfig.ClusterName1, ctx)
	node2, nodePool2, namespace2, clusterInstance2 := VerifyAndRetrieveAssociatedCRsForAI(
		provisioningRequest2.Object.Name, OCloudConfig.ClusterName2, ctx)

	var wg1 sync.WaitGroup

	var mu1 sync.Mutex

	wg1.Add(2)

	go VerifyAllPoliciesInNamespaceAreCompliant(namespace1.Object.Name, ctx, &wg1, &mu1)
	go VerifyAllPoliciesInNamespaceAreCompliant(namespace2.Object.Name, ctx, &wg1, &mu1)

	wg1.Wait()

	provisioningRequest1, err := oran.PullPR(HubAPIClient, provisioningRequest1.Object.Name)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to retrieve PR %s", provisioningRequest1.Object.Name))

	VerifyProvisioningRequestIsFulfilled(provisioningRequest1)
	glog.V(ocloudparams.OCloudLogLevel).Infof("Provisioning request %s is fulfilled", provisioningRequest1.Object.Name)

	provisioningRequest2, err = oran.PullPR(HubAPIClient, provisioningRequest2.Object.Name)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to retrieve PR %s", provisioningRequest2.Object.Name))

	VerifyProvisioningRequestIsFulfilled(provisioningRequest2)
	glog.V(ocloudparams.OCloudLogLevel).Infof("Provisioning request %s is fulfilled", provisioningRequest2.Object.Name)

	sno1ApiClient := CreateSnoAPIClient(OCloudConfig.ClusterName1)
	sno2ApiClient := CreateSnoAPIClient(OCloudConfig.ClusterName2)

	oldPTPVersionSno1 := getPtpOperatorVersionInSno(sno1ApiClient)
	oldPTPVersionSno2 := getPtpOperatorVersionInSno(sno2ApiClient)

	VerifyAllPodsRunningInNamespace(sno1ApiClient, ocloudparams.PtpNamespace)
	VerifyAllPodsRunningInNamespace(sno2ApiClient, ocloudparams.PtpNamespace)

	upgradeOperatorImages()

	modifyDeploymentResources(
		sno1ApiClient,
		ocloudparams.PtpOperatorSubscriptionName,
		ocloudparams.PtpNamespace,
		ocloudparams.PtpDeploymentName,
		ocloudparams.PtpContainerName,
		ocloudparams.PtpCPURequest,
		ocloudparams.PtpMemoryRequest,
		ocloudparams.PtpCPULimit,
		ocloudparams.PtpMemoryLimit)

	var wg2 sync.WaitGroup

	var mu2 sync.Mutex

	wg2.Add(2)

	go VerifyPoliciesAreNotCompliant(OCloudConfig.ClusterName1, ctx, &wg2, &mu2)
	go VerifyPoliciesAreNotCompliant(OCloudConfig.ClusterName2, ctx, &wg2, &mu2)

	wg2.Wait()

	VerifyAllPoliciesInNamespaceAreCompliant(namespace2.Object.Name, ctx, nil, nil)

	provisioningRequest1, err = oran.PullPR(HubAPIClient, provisioningRequest1.Object.Name)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to retrieve PR %s", provisioningRequest1.Object.Name))

	VerifyProvisioningRequestTimeout(provisioningRequest1)
	glog.V(ocloudparams.OCloudLogLevel).Infof("Provisioning request %s timedout", provisioningRequest1.Object.Name)

	provisioningRequest2, err = oran.PullPR(HubAPIClient, provisioningRequest2.Object.Name)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to retrieve PR %s", provisioningRequest2.Object.Name))

	VerifyProvisioningRequestIsFulfilled(provisioningRequest2)
	glog.V(ocloudparams.OCloudLogLevel).Infof("Provisioning request %s is fulfilled", provisioningRequest2.Object.Name)

	newPTPVersionSno1 := getPtpOperatorVersionInSno(sno1ApiClient)
	newPTPVersionSno2 := getPtpOperatorVersionInSno(sno2ApiClient)

	Expect(oldPTPVersionSno1).To(Equal(newPTPVersionSno1),
		fmt.Sprintf("PTP operator version has changed in SNO1: old version (%s) != new version (%s)",
			oldPTPVersionSno1, newPTPVersionSno1))

	Expect(oldPTPVersionSno2).NotTo(Equal(newPTPVersionSno2),
		fmt.Sprintf("PTP operator has not being upgraded in SNO2: old version (%s) == new version (%s)",
			oldPTPVersionSno2, newPTPVersionSno2))

	err = os.RemoveAll("tmp/")
	Expect(err).NotTo(HaveOccurred(), "Error removing directory /tmp")

	var wg4 sync.WaitGroup

	wg4.Add(2)

	go DeprovisionAiSnoCluster(provisioningRequest1, namespace1, clusterInstance1, node1, nodePool1, ctx, &wg4)
	go DeprovisionAiSnoCluster(provisioningRequest2, namespace2, clusterInstance2, node2, nodePool2, ctx, &wg4)

	wg4.Wait()
}

// getPtpOperatorVersionInSno returns the PTP operator version.
func getPtpOperatorVersionInSno(apiClient *clients.Settings) version.OperatorVersion {
	By("Retrieving the PTP Operator version")

	csvName, err := csv.GetCurrentCSVNameFromSubscription(apiClient,
		ocloudparams.PtpOperatorSubscriptionName, ocloudparams.PtpNamespace)
	Expect(err).NotTo(HaveOccurred(),
		fmt.Sprintf("csv %s not found in namespace %s", csvName, ocloudparams.PtpNamespace))

	csvObj, err := olm.PullClusterServiceVersion(apiClient, csvName, ocloudparams.PtpNamespace)
	Expect(err).NotTo(HaveOccurred(),
		fmt.Sprintf("failed to pull %q csv from the %s namespace", csvName, ocloudparams.PtpNamespace))

	return csvObj.Object.Spec.Version
}

// modifyDeploymentResources modifies the cpu and memory resources available for a given container, in a given
// deployment in a given subscription.
func modifyDeploymentResources(
	apiClient *clients.Settings,
	subscriptionName string,
	nsname string,
	deploymentName string,
	containerName string,
	cpuRequest string,
	memoryRequest string,
	cpuLimit string,
	memoryLimit string) {
	csvName, err := csv.GetCurrentCSVNameFromSubscription(apiClient, subscriptionName, nsname)
	if err != nil {
		Skip(fmt.Sprintf("csv %s not found in namespace %s", csvName, nsname))
	}

	csvObj, err := olm.PullClusterServiceVersion(apiClient, csvName, nsname)
	if err != nil {
		Skip(fmt.Sprintf("failed to pull %q csv from the %s namespace", csvName, nsname))
	}

	for i, deployment := range csvObj.Object.Spec.InstallStrategy.StrategySpec.DeploymentSpecs {
		if deployment.Name == deploymentName {
			for j, container := range deployment.Spec.Template.Spec.Containers {
				if container.Name == containerName {
					csvObj.Object.Spec.InstallStrategy.
						StrategySpec.DeploymentSpecs[i].Spec.Template.
						Spec.Containers[j].Resources = corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							"cpu":    resource.MustParse(cpuLimit),
							"memory": resource.MustParse(memoryLimit),
						},
						Requests: corev1.ResourceList{
							"cpu":    resource.MustParse(cpuRequest),
							"memory": resource.MustParse(memoryRequest),
						},
					}
					_, err = csvObj.Update()
					Expect(err).ToNot(HaveOccurred(), "failed to update deployment resources %s - %s: %v",
						subscriptionName, deploymentName, err)
				}
			}
		}
	}
}

// upgradeOperatorImages upgrades the operator images.
func upgradeOperatorImages() {
	cmd := fmt.Sprintf(ocloudparams.SkopeoRedhatOperatorsUpgrade,
		OCloudConfig.AuthfilePath, OCloudConfig.Registry5000, OCloudConfig.Registry5000)
	_, err := shell.ExecuteCmd(cmd)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error tagging redhat-operators image for upgrade: %v", err))
}

// downgradeOperatorImages downgrades the operator images.
func downgradeOperatorImages() {
	cmd := fmt.Sprintf(ocloudparams.SkopeoRedhatOperatorsDowngrade,
		OCloudConfig.AuthfilePath, OCloudConfig.Registry5000, OCloudConfig.Registry5000)
	_, err := shell.ExecuteCmd(cmd)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error tagging redhat-operators image for downgrade: %v", err))
}
