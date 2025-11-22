package ocloudcommon

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/o-cloud/internal/ocloudinittools"
	"k8s.io/klog/v2"

	"fmt"
	"sync"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/o-cloud/internal/ocloudparams"
)

// VerifySuccessfulSnoProvisioning verifies the successful provisioning of a SNO cluster using
// Assisted Installer.
func VerifySuccessfulSnoProvisioning(ctx SpecContext) {
	// Verify that both BMHs are available before provisioning the SNO cluster because either of them
	// can be used to provision the SNO cluster.
	VerifyBmhIsAvailable(OCloudConfig.BmhSpoke1, OCloudConfig.InventoryPoolNamespace)
	VerifyBmhIsAvailable(OCloudConfig.BmhSpoke2, OCloudConfig.InventoryPoolNamespace)

	provisioningRequest := VerifyProvisionSnoCluster(
		OCloudConfig.TemplateName,
		OCloudConfig.TemplateVersionAISuccess,
		OCloudConfig.NodeClusterName1,
		OCloudConfig.OCloudSiteID,
		ocloudparams.PolicyTemplateParameters,
		ocloudparams.ClusterInstanceParameters1)

	VerifyOcloudCRsExist(provisioningRequest)

	clusterInstance := VerifyClusterInstanceCompleted(provisioningRequest, ctx)
	nsname := provisioningRequest.Object.Status.Extensions.ClusterDetails.Name

	VerifyAllPoliciesInNamespaceAreCompliant(nsname, ctx, nil, nil)
	klog.V(ocloudparams.OCloudLogLevel).Infof("all the policies in namespace %s are compliant", nsname)

	VerifyProvisioningRequestIsFulfilled(provisioningRequest)
	klog.V(ocloudparams.OCloudLogLevel).Infof("provisioning request %s is fulfilled", provisioningRequest.Object.Name)

	DeprovisionAiSnoCluster(provisioningRequest, clusterInstance, ctx, nil)
}

// VerifyFailedSnoProvisioning verifies that the provisioning of a SNO cluster using
// Assisted Installer fails.
func VerifyFailedSnoProvisioning(ctx SpecContext) {
	// Verify that both BMHs are available before provisioning the SNO cluster because either of them
	// can be used to provision the SNO cluster.
	VerifyBmhIsAvailable(OCloudConfig.BmhSpoke1, OCloudConfig.InventoryPoolNamespace)
	VerifyBmhIsAvailable(OCloudConfig.BmhSpoke2, OCloudConfig.InventoryPoolNamespace)

	provisioningRequest := VerifyProvisionSnoCluster(
		OCloudConfig.TemplateName,
		OCloudConfig.TemplateVersionAIFailure,
		OCloudConfig.NodeClusterName1,
		OCloudConfig.OCloudSiteID,
		ocloudparams.PolicyTemplateParameters,
		ocloudparams.ClusterInstanceParameters1)

	VerifyOcloudCRsExist(provisioningRequest)

	clusterInstance := VerifyClusterInstanceCompleted(provisioningRequest, ctx)

	VerifyProvisioningRequestTimeout(provisioningRequest)

	DeprovisionAiSnoCluster(provisioningRequest, clusterInstance, ctx, nil)
}

// VerifySimultaneousSnoProvisioningSameClusterTemplate verifies the successful provisioning of two SNO clusters
// simultaneously with the same cluster templates.
func VerifySimultaneousSnoProvisioningSameClusterTemplate(ctx SpecContext) {
	// Verify that both BMHs are available before provisioning the SNO cluster because either of them
	// can be used to provision the SNO cluster.
	VerifyBmhIsAvailable(OCloudConfig.BmhSpoke1, OCloudConfig.InventoryPoolNamespace)
	VerifyBmhIsAvailable(OCloudConfig.BmhSpoke2, OCloudConfig.InventoryPoolNamespace)

	provisioningRequest1 := VerifyProvisionSnoCluster(
		OCloudConfig.TemplateName,
		OCloudConfig.TemplateVersionAISuccess,
		OCloudConfig.NodeClusterName1,
		OCloudConfig.OCloudSiteID,
		ocloudparams.PolicyTemplateParameters,
		ocloudparams.ClusterInstanceParameters1)
	provisioningRequest2 := VerifyProvisionSnoCluster(
		OCloudConfig.TemplateName,
		OCloudConfig.TemplateVersionAISuccess,
		OCloudConfig.NodeClusterName2,
		OCloudConfig.OCloudSiteID,
		ocloudparams.PolicyTemplateParameters,
		ocloudparams.ClusterInstanceParameters2)

	VerifyOcloudCRsExist(provisioningRequest1)
	nsname1 := provisioningRequest1.Object.Status.Extensions.ClusterDetails.Name

	VerifyOcloudCRsExist(provisioningRequest2)
	nsname2 := provisioningRequest2.Object.Status.Extensions.ClusterDetails.Name

	var waitGroup sync.WaitGroup

	var mutex sync.Mutex

	waitGroup.Add(2)

	go VerifyAllPoliciesInNamespaceAreCompliant(nsname1, ctx, &waitGroup, &mutex)
	go VerifyAllPoliciesInNamespaceAreCompliant(nsname2, ctx, &waitGroup, &mutex)

	waitGroup.Wait()

	VerifyProvisioningRequestIsFulfilled(provisioningRequest1)
	VerifyProvisioningRequestIsFulfilled(provisioningRequest2)
}

// VerifySimultaneousSnoDeprovisioningSameClusterTemplate verifies the successful deletion of
// two SNO clusters with the same cluster template.
func VerifySimultaneousSnoDeprovisioningSameClusterTemplate(ctx SpecContext) {
	prName1 := GetProvisioningRequestName(OCloudConfig.ClusterName1)
	prName2 := GetProvisioningRequestName(OCloudConfig.ClusterName2)

	provisioningRequest1, err := oran.PullPR(HubAPIClient, prName1)
	Expect(err).ToNot(HaveOccurred(), "Failed to get PR %s", prName1)
	VerifyProvisioningRequestIsFulfilled(provisioningRequest1)

	provisioningRequest2, err := oran.PullPR(HubAPIClient, prName2)
	Expect(err).ToNot(HaveOccurred(), "Failed to get PR %s", prName2)
	VerifyProvisioningRequestIsFulfilled(provisioningRequest2)

	By(fmt.Sprintf("Verify that %s PR and %s PR are using the same template version", prName1, prName2))

	pr1TemplateVersion := provisioningRequest1.Object.Spec.TemplateName + provisioningRequest1.Object.Spec.TemplateVersion
	pr2TemplateVersion := provisioningRequest2.Object.Spec.TemplateName + provisioningRequest2.Object.Spec.TemplateVersion
	Expect(pr1TemplateVersion).To(Equal(pr2TemplateVersion),
		fmt.Sprintf("PR %s and %s are not using the same cluster template", prName1, prName2))

	nodeAllocationRequest1, allocatedNodes1, namespace1 := VerifyOcloudCRsExist(provisioningRequest1)
	clusterInstance1 := VerifyClusterInstanceCompleted(provisioningRequest1, ctx)
	bmhs1 := GetBMHsFromAllocatedNodes(allocatedNodes1)

	nodeAllocationRequest2, allocatedNodes2, namespace2 := VerifyOcloudCRsExist(provisioningRequest2)
	clusterInstance2 := VerifyClusterInstanceCompleted(provisioningRequest2, ctx)
	bmhs2 := GetBMHsFromAllocatedNodes(allocatedNodes2)

	var waitGroup sync.WaitGroup

	waitGroup.Add(10)

	go VerifyProvisioningRequestIsDeleted(provisioningRequest1, &waitGroup, ctx)
	go VerifyProvisioningRequestIsDeleted(provisioningRequest2, &waitGroup, ctx)
	go VerifyNamespaceDoesNotExist(namespace1, &waitGroup, ctx)
	go VerifyNamespaceDoesNotExist(namespace2, &waitGroup, ctx)
	go VerifyClusterInstanceDoesNotExist(clusterInstance1, &waitGroup, ctx)
	go VerifyClusterInstanceDoesNotExist(clusterInstance2, &waitGroup, ctx)
	go VerifyAllocatedNodesDoNotExist(allocatedNodes1, &waitGroup, ctx)
	go VerifyAllocatedNodesDoNotExist(allocatedNodes2, &waitGroup, ctx)
	go VerifyNodeAllocationRequestDoesNotExist(nodeAllocationRequest1, &waitGroup, ctx)
	go VerifyNodeAllocationRequestDoesNotExist(nodeAllocationRequest2, &waitGroup, ctx)

	waitGroup.Wait()

	for _, bmh := range bmhs1 {
		VerifyBmhIsAvailable(bmh, OCloudConfig.InventoryPoolNamespace)
	}

	for _, bmh := range bmhs2 {
		VerifyBmhIsAvailable(bmh, OCloudConfig.InventoryPoolNamespace)
	}
}

// VerifySimultaneousSnoProvisioningDifferentClusterTemplates verifies the successful provisioning of
// two SNO clusters simultaneously with different cluster templates.
func VerifySimultaneousSnoProvisioningDifferentClusterTemplates(ctx SpecContext) {
	// Verify that both BMHs are available before provisioning the SNO cluster because either of them
	// can be used to provision the SNO cluster.
	VerifyBmhIsAvailable(OCloudConfig.BmhSpoke1, OCloudConfig.InventoryPoolNamespace)
	VerifyBmhIsAvailable(OCloudConfig.BmhSpoke2, OCloudConfig.InventoryPoolNamespace)

	provisioningRequest1 := VerifyProvisionSnoCluster(
		OCloudConfig.TemplateName,
		OCloudConfig.TemplateVersionSimultaneous1,
		OCloudConfig.NodeClusterName1,
		OCloudConfig.OCloudSiteID,
		ocloudparams.PolicyTemplateParameters,
		ocloudparams.ClusterInstanceParameters1)

	provisioningRequest2 := VerifyProvisionSnoCluster(
		OCloudConfig.TemplateName,
		OCloudConfig.TemplateVersionSimultaneous2,
		OCloudConfig.NodeClusterName2,
		OCloudConfig.OCloudSiteID,
		ocloudparams.PolicyTemplateParameters,
		ocloudparams.ClusterInstanceParameters2)

	VerifyOcloudCRsExist(provisioningRequest1)
	nsname1 := provisioningRequest1.Object.Status.Extensions.ClusterDetails.Name

	VerifyOcloudCRsExist(provisioningRequest2)
	nsname2 := provisioningRequest2.Object.Status.Extensions.ClusterDetails.Name

	var waitGroup sync.WaitGroup

	var mutex sync.Mutex

	waitGroup.Add(2)

	go VerifyAllPoliciesInNamespaceAreCompliant(nsname1, ctx, &waitGroup, &mutex)
	go VerifyAllPoliciesInNamespaceAreCompliant(nsname2, ctx, &waitGroup, &mutex)

	waitGroup.Wait()
}

// VerifySimultaneousSnoDeprovisioningDifferentClusterTemplates verifies the successful deletion of
// two SNO clusters with different cluster templates.
func VerifySimultaneousSnoDeprovisioningDifferentClusterTemplates(ctx SpecContext) {
	prName1 := GetProvisioningRequestName(OCloudConfig.ClusterName1)
	prName2 := GetProvisioningRequestName(OCloudConfig.ClusterName2)

	provisioningRequest1, err := oran.PullPR(HubAPIClient, prName1)
	Expect(err).ToNot(HaveOccurred(), "Failed to get PR %s", prName1)
	VerifyProvisioningRequestIsFulfilled(provisioningRequest1)

	provisioningRequest2, err := oran.PullPR(HubAPIClient, prName2)
	Expect(err).ToNot(HaveOccurred(), "Failed to get PR %s", prName2)
	VerifyProvisioningRequestIsFulfilled(provisioningRequest2)

	By(fmt.Sprintf("Verify that %s PR and %s PR are using different cluster template versions", prName1, prName2))

	pr1TemplateVersion := provisioningRequest1.Object.Spec.TemplateName + provisioningRequest1.Object.Spec.TemplateVersion
	pr2TemplateVersion := provisioningRequest2.Object.Spec.TemplateName + provisioningRequest2.Object.Spec.TemplateVersion

	Expect(pr1TemplateVersion).NotTo(Equal(pr2TemplateVersion),
		fmt.Sprintf("PR %s and %s are using the same cluster template", prName1, prName2))

	nodeAllocationRequest1, allocatedNodes1, namespace1 := VerifyOcloudCRsExist(provisioningRequest1)
	clusterInstance1 := VerifyClusterInstanceCompleted(provisioningRequest1, ctx)
	bmhs1 := GetBMHsFromAllocatedNodes(allocatedNodes1)
	nodeAllocationRequest2, allocatedNodes2, namespace2 := VerifyOcloudCRsExist(provisioningRequest2)
	clusterInstance2 := VerifyClusterInstanceCompleted(provisioningRequest2, ctx)
	bmhs2 := GetBMHsFromAllocatedNodes(allocatedNodes2)

	var waitGroup sync.WaitGroup

	waitGroup.Add(10)

	go VerifyProvisioningRequestIsDeleted(provisioningRequest1, &waitGroup, ctx)
	go VerifyProvisioningRequestIsDeleted(provisioningRequest2, &waitGroup, ctx)
	go VerifyNamespaceDoesNotExist(namespace1, &waitGroup, ctx)
	go VerifyNamespaceDoesNotExist(namespace2, &waitGroup, ctx)
	go VerifyClusterInstanceDoesNotExist(clusterInstance1, &waitGroup, ctx)
	go VerifyClusterInstanceDoesNotExist(clusterInstance2, &waitGroup, ctx)
	go VerifyAllocatedNodesDoNotExist(allocatedNodes1, &waitGroup, ctx)
	go VerifyAllocatedNodesDoNotExist(allocatedNodes2, &waitGroup, ctx)
	go VerifyNodeAllocationRequestDoesNotExist(nodeAllocationRequest1, &waitGroup, ctx)
	go VerifyNodeAllocationRequestDoesNotExist(nodeAllocationRequest2, &waitGroup, ctx)

	waitGroup.Wait()

	for _, bmh := range bmhs1 {
		VerifyBmhIsAvailable(bmh, OCloudConfig.InventoryPoolNamespace)
	}

	for _, bmh := range bmhs2 {
		VerifyBmhIsAvailable(bmh, OCloudConfig.InventoryPoolNamespace)
	}
}
