package ocloudcommon

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/o-cloud/internal/ocloudinittools"

	"fmt"
	"sync"

	"github.com/golang/glog"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/o-cloud/internal/ocloudparams"
)

// VerifySuccessfulSnoProvisioning verifies the successful provisioning of a SNO cluster using
// Assisted Installer.
func VerifySuccessfulSnoProvisioning(ctx SpecContext) {
	provisioningRequest := VerifyProvisionSnoCluster(
		OCloudConfig.TemplateName,
		OCloudConfig.TemplateVersionAISuccess,
		OCloudConfig.NodeClusterName1,
		OCloudConfig.OCloudSiteID,
		ocloudparams.PolicyTemplateParameters,
		ocloudparams.ClusterInstanceParameters1)

	allocatedNode, nodeAR, namespace, clusterInstance := VerifyAndRetrieveAssociatedCRsForAI(
		provisioningRequest.Object.Name, OCloudConfig.ClusterName1, ctx)

	VerifyAllPoliciesInNamespaceAreCompliant(namespace.Object.Name, ctx, nil, nil)
	glog.V(ocloudparams.OCloudLogLevel).Infof("All the policies in namespace %s are Complete", namespace.Object.Name)

	VerifyProvisioningRequestIsFulfilled(provisioningRequest)
	glog.V(ocloudparams.OCloudLogLevel).Infof("Provisioning request %s is fulfilled", provisioningRequest.Object.Name)

	DeprovisionAiSnoCluster(provisioningRequest, namespace, clusterInstance, allocatedNode, nodeAR, ctx, nil)
}

// VerifyFailedSnoProvisioning verifies that the provisioning of a SNO cluster using
// Assisted Installer fails.
func VerifyFailedSnoProvisioning(ctx SpecContext) {
	provisioningRequest := VerifyProvisionSnoCluster(
		OCloudConfig.TemplateName,
		OCloudConfig.TemplateVersionAIFailure,
		OCloudConfig.NodeClusterName1,
		OCloudConfig.OCloudSiteID,
		ocloudparams.PolicyTemplateParameters,
		ocloudparams.ClusterInstanceParameters1)

	allocatedNode, nodeAR, namespace, clusterInstance := VerifyAndRetrieveAssociatedCRsForAI(
		provisioningRequest.Object.Name, OCloudConfig.ClusterName1, ctx)

	VerifyProvisioningRequestTimeout(provisioningRequest)

	DeprovisionAiSnoCluster(provisioningRequest, namespace, clusterInstance, allocatedNode, nodeAR, ctx, nil)
}

// VerifySimultaneousSnoProvisioningSameClusterTemplate verifies the successful provisioning of two SNO clusters
// simultaneously with the same cluster templates.
func VerifySimultaneousSnoProvisioningSameClusterTemplate(ctx SpecContext) {
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

	_, _, ns1, _ := VerifyAndRetrieveAssociatedCRsForAI(
		provisioningRequest1.Object.Name, OCloudConfig.ClusterName1, ctx)
	_, _, ns2, _ := VerifyAndRetrieveAssociatedCRsForAI(
		provisioningRequest2.Object.Name, OCloudConfig.ClusterName2, ctx)

	var waitGroup sync.WaitGroup

	var mutex sync.Mutex

	waitGroup.Add(2)

	go VerifyAllPoliciesInNamespaceAreCompliant(ns1.Object.Name, ctx, &waitGroup, &mutex)
	go VerifyAllPoliciesInNamespaceAreCompliant(ns2.Object.Name, ctx, &waitGroup, &mutex)

	waitGroup.Wait()

	VerifyProvisioningRequestIsFulfilled(provisioningRequest1)
	VerifyProvisioningRequestIsFulfilled(provisioningRequest2)
}

// VerifySimultaneousSnoDeprovisioningSameClusterTemplate verifies the successful deletion of
// two SNO clusters with the same cluster template.
func VerifySimultaneousSnoDeprovisioningSameClusterTemplate(ctx SpecContext) {
	prName1 := getProvisioningRequestName(OCloudConfig.ClusterName1)
	prName2 := getProvisioningRequestName(OCloudConfig.ClusterName2)

	provisioningRequest1, err := oran.PullPR(HubAPIClient, prName1)
	Expect(err).ToNot(HaveOccurred(), "Failed to get PR %s", prName1)
	VerifyProvisioningRequestIsFulfilled(provisioningRequest1)

	provisioningRequest2, err := oran.PullPR(HubAPIClient, prName2)
	Expect(err).ToNot(HaveOccurred(), "Failed to get PR %s", prName2)
	VerifyProvisioningRequestIsFulfilled(provisioningRequest2)

	By(fmt.Sprintf("Verify that %s PR and %s PR are using the same template version", prName1, prName2))

	pr1TemplateVersion := provisioningRequest1.Object.Spec.TemplateName + provisioningRequest1.Object.Spec.TemplateVersion
	pr2TemplateVersion := provisioningRequest2.Object.Spec.TemplateName + provisioningRequest1.Object.Spec.TemplateVersion
	Expect(pr1TemplateVersion).To(Equal(pr2TemplateVersion),
		fmt.Sprintf("PR %s and %s are not using the same cluster template", prName1, prName2))

	allocatedNode1, nodeAllocationRequest1, namespace1, clusterInstance1 := VerifyAndRetrieveAssociatedCRsForAI(
		prName1, OCloudConfig.ClusterName1, ctx)
	allocatedNode2, nodeAllocationRequest2, namespace2, clusterInstance2 := VerifyAndRetrieveAssociatedCRsForAI(
		prName2, OCloudConfig.ClusterName2, ctx)

	var waitGroup sync.WaitGroup

	waitGroup.Add(10)

	go VerifyProvisioningRequestIsDeleted(provisioningRequest1, &waitGroup, ctx)
	go VerifyProvisioningRequestIsDeleted(provisioningRequest2, &waitGroup, ctx)
	go VerifyNamespaceDoesNotExist(namespace1, &waitGroup, ctx)
	go VerifyNamespaceDoesNotExist(namespace2, &waitGroup, ctx)
	go VerifyClusterInstanceDoesNotExist(clusterInstance1, &waitGroup, ctx)
	go VerifyClusterInstanceDoesNotExist(clusterInstance2, &waitGroup, ctx)
	go VerifyAllocatedNodeDoesNotExist(allocatedNode1, &waitGroup, ctx)
	go VerifyAllocatedNodeDoesNotExist(allocatedNode2, &waitGroup, ctx)
	go VerifyNARDoesNotExist(nodeAllocationRequest1, &waitGroup, ctx)
	go VerifyNARDoesNotExist(nodeAllocationRequest2, &waitGroup, ctx)

	waitGroup.Wait()
}

// VerifySimultaneousSnoProvisioningDifferentClusterTemplates verifies the successful provisioning of
// two SNO clusters simultaneously with different cluster templates.
func VerifySimultaneousSnoProvisioningDifferentClusterTemplates(ctx SpecContext) {
	provisioningRequest1 := VerifyProvisionSnoCluster(
		OCloudConfig.TemplateName,
		OCloudConfig.TemplateVersionAISuccess,
		OCloudConfig.NodeClusterName1,
		OCloudConfig.OCloudSiteID,
		ocloudparams.PolicyTemplateParameters,
		ocloudparams.ClusterInstanceParameters1)

	provisioningRequest2 := VerifyProvisionSnoCluster(
		OCloudConfig.TemplateName,
		OCloudConfig.TemplateVersionDifferentTemplates,
		OCloudConfig.NodeClusterName2,
		OCloudConfig.OCloudSiteID,
		ocloudparams.PolicyTemplateParameters,
		ocloudparams.ClusterInstanceParameters2)

	_, _, ns1, _ := VerifyAndRetrieveAssociatedCRsForAI(
		provisioningRequest1.Object.Name, OCloudConfig.ClusterName1, ctx)
	_, _, ns2, _ := VerifyAndRetrieveAssociatedCRsForAI(
		provisioningRequest2.Object.Name, OCloudConfig.ClusterName2, ctx)

	var waitGroup sync.WaitGroup

	var mutex sync.Mutex

	waitGroup.Add(2)

	go VerifyAllPoliciesInNamespaceAreCompliant(ns1.Object.Name, ctx, &waitGroup, &mutex)
	go VerifyAllPoliciesInNamespaceAreCompliant(ns2.Object.Name, ctx, &waitGroup, &mutex)

	waitGroup.Wait()
}

// VerifySimultaneousSnoDeprovisioningDifferentClusterTemplates verifies the successful deletion of
// two SNO clusters with different cluster templates.
func VerifySimultaneousSnoDeprovisioningDifferentClusterTemplates(ctx SpecContext) {
	prName1 := getProvisioningRequestName(OCloudConfig.ClusterName1)
	prName2 := getProvisioningRequestName(OCloudConfig.ClusterName2)

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

	allocatedNode1, nodeAllocationRequest1, ns1, clusterInstance1 := VerifyAndRetrieveAssociatedCRsForAI(
		prName1, OCloudConfig.ClusterName1, ctx)
	allocatedNode2, nodeAllocationRequest2, ns2, clusterInstance2 := VerifyAndRetrieveAssociatedCRsForAI(
		prName2, OCloudConfig.ClusterName2, ctx)

	var waitGroup sync.WaitGroup

	waitGroup.Add(10)

	go VerifyProvisioningRequestIsDeleted(provisioningRequest1, &waitGroup, ctx)
	go VerifyProvisioningRequestIsDeleted(provisioningRequest2, &waitGroup, ctx)
	go VerifyNamespaceDoesNotExist(ns1, &waitGroup, ctx)
	go VerifyNamespaceDoesNotExist(ns2, &waitGroup, ctx)
	go VerifyClusterInstanceDoesNotExist(clusterInstance1, &waitGroup, ctx)
	go VerifyClusterInstanceDoesNotExist(clusterInstance2, &waitGroup, ctx)
	go VerifyAllocatedNodeDoesNotExist(allocatedNode1, &waitGroup, ctx)
	go VerifyAllocatedNodeDoesNotExist(allocatedNode2, &waitGroup, ctx)
	go VerifyNARDoesNotExist(nodeAllocationRequest1, &waitGroup, ctx)
	go VerifyNARDoesNotExist(nodeAllocationRequest2, &waitGroup, ctx)

	waitGroup.Wait()
}

func getProvisioningRequestName(clusterName string) string {
	nodeAllocationRequest, err := oran.PullNodeAllocationRequest(
		HubAPIClient, clusterName, ocloudparams.OCloudHardwareManagerPluginNamespace)
	Expect(err).NotTo(HaveOccurred(), "Failed to retrieve node allocation request %s", OCloudConfig.ClusterName1)

	for _, ownerReference := range nodeAllocationRequest.Object.OwnerReferences {
		if ownerReference.Kind == "ProvisioningRequest" {
			return ownerReference.Name
		}
	}

	return ""
}
