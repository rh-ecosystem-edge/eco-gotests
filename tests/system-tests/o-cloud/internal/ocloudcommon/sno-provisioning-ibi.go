package ocloudcommon

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/o-cloud/internal/ocloudinittools"

	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ibi"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"github.com/Masterminds/sprig/v3"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmc"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/lca"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/files"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/shell"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/sshcommand"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/o-cloud/internal/ocloudparams"
)

// ImageBasedInstallConfigData struct holds the configuration data required for the image based install.
type ImageBasedInstallConfigData struct {
	BaseImageName       string
	SeedImage           string
	SeedVersion         string
	Registry            string
	PullSecret          string
	SSHKey              string
	RegistryCertificate string
	InterfaceName       string
	InterfaceIpv6       string
	DNSIpv6             string
	NextHopIpv6         string
	NextHopInterface    string
}

// VerifySuccessfulIbiSnoProvisioning verifies the successful provisioning of a SNO cluster using
// the Image Based Installer.
func VerifySuccessfulIbiSnoProvisioning(ctx SpecContext) {
	VerifyBmhIsAvailable(OCloudConfig.BmhSpoke1, OCloudConfig.InventoryPoolNamespace)
	VerifyBmhIsAvailable(OCloudConfig.BmhSpoke2, OCloudConfig.InventoryPoolNamespace)

	if OCloudConfig.GenerateSeedImage && !baseImageExists() {
		generateBaseImage(ctx)
	}

	installBaseImage(
		OCloudConfig.Spoke2BMC,
		OCloudConfig.IbiBaseImageURL,
		OCloudConfig.VirtualMediaID,
		OCloudConfig.SSHCluster2,
		ocloudparams.SpokeSSHUser,
		ocloudparams.SpokeSSHPasskeyPath,
		ctx,
		time.Minute)

	provisioningRequest := VerifyProvisionSnoCluster(
		OCloudConfig.TemplateName,
		OCloudConfig.TemplateVersionIBISuccess,
		OCloudConfig.NodeClusterName2,
		OCloudConfig.OCloudSiteID,
		ocloudparams.PolicyTemplateParameters,
		ocloudparams.ClusterInstanceParameters2)

	VerifyOcloudCRsExist(provisioningRequest)

	imageClusterInstall := VerifyImageClusterInstallCompleted(
		OCloudConfig.ClusterName2, OCloudConfig.ClusterName2, ctx)
	nsname := provisioningRequest.Object.Status.Extensions.ClusterDetails.Name

	VerifyAllPoliciesInNamespaceAreCompliant(nsname, ctx, nil, nil)
	klog.V(ocloudparams.OCloudLogLevel).Infof("All the policies in namespace %s are Complete", nsname)

	VerifyProvisioningRequestIsFulfilled(provisioningRequest)
	klog.V(ocloudparams.OCloudLogLevel).Infof("Provisioning request %s is fulfilled", provisioningRequest.Object.Name)

	DeprovisionIbiSnoCluster(provisioningRequest, imageClusterInstall, ctx)
}

// VerifyFailedIbiSnoProvisioning verifies the failed provisioning of a SNO cluster using
// the Image Based Installer.
func VerifyFailedIbiSnoProvisioning(ctx SpecContext) {
	VerifyBmhIsAvailable(OCloudConfig.BmhSpoke1, OCloudConfig.InventoryPoolNamespace)
	VerifyBmhIsAvailable(OCloudConfig.BmhSpoke2, OCloudConfig.InventoryPoolNamespace)

	if OCloudConfig.GenerateSeedImage && !baseImageExists() {
		generateBaseImage(ctx)
	}

	installBaseImage(
		OCloudConfig.Spoke2BMC,
		OCloudConfig.IbiBaseImageURL,
		OCloudConfig.VirtualMediaID,
		OCloudConfig.SSHCluster2,
		ocloudparams.SpokeSSHUser,
		ocloudparams.SpokeSSHPasskeyPath,
		ctx,
		time.Minute)

	provisioningRequest := VerifyProvisionSnoCluster(
		OCloudConfig.TemplateName,
		OCloudConfig.TemplateVersionIBIFailure,
		OCloudConfig.NodeClusterName2,
		OCloudConfig.OCloudSiteID,
		ocloudparams.PolicyTemplateParameters,
		ocloudparams.ClusterInstanceParameters2)

	VerifyOcloudCRsExist(provisioningRequest)

	imageClusterInstall := VerifyImageClusterInstallCompleted(
		OCloudConfig.ClusterName2, OCloudConfig.ClusterName2, ctx)

	VerifyProvisioningRequestTimeout(provisioningRequest)
	klog.V(ocloudparams.OCloudLogLevel).Infof("Provisioning request %s has timed out", provisioningRequest.Object.Name)

	DeprovisionIbiSnoCluster(provisioningRequest, imageClusterInstall, ctx)
}

// installBaseImage boots a given spoke cluster from CD using the specified base image and virtual media ID,
// and uses ssh to verify that the installation of the base image has finished before a given time.
func installBaseImage(
	spoke *bmc.BMC,
	isoURL, virtualMediaID, sshHost, sshUser, sshPassKey string,
	ctx context.Context,
	timeout time.Duration) {
	By("Installing base image in target SNO")

	err := spoke.BootFromCD(isoURL, virtualMediaID)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error setting virtual media to %s", isoURL))

	powerState, err := spoke.SystemPowerState()
	Expect(err).NotTo(HaveOccurred(), "Error getting system power state")

	if powerState != "Off" {
		err = spoke.SystemForceReset()
		Expect(err).NotTo(HaveOccurred(), "Error resetting the system")
	} else {
		err = spoke.SystemPowerOn()
		Expect(err).NotTo(HaveOccurred(), "Error powering on the system")
	}

	By("Checking if image based install finished in target SNO")

	Eventually(func(ctx context.Context) bool {
		output := sshcommand.SSHCommand(ocloudparams.CheckIbiCompleted, sshHost, sshUser, sshPassKey)
		if output.Err == nil && output.SSHOutput != "" {
			return true
		}

		return false
	}).WithTimeout(80*time.Minute).WithPolling(timeout).WithContext(ctx).Should(BeTrue(),
		"Image based install did not completed")
}

// generateBaseImage provisions a seed SNO cluster and generates a base image to be used with Image Based Installation.
//
//nolint:funlen
func generateBaseImage(ctx SpecContext) {
	By("Generating a base image from seed SNO")

	provisioningRequest := VerifyProvisionSnoCluster(
		OCloudConfig.TemplateName,
		OCloudConfig.TemplateVersionSeed,
		OCloudConfig.NodeClusterName1,
		OCloudConfig.OCloudSiteID,
		ocloudparams.PolicyTemplateParameters,
		ocloudparams.ClusterInstanceParameters1)

	VerifyOcloudCRsExist(provisioningRequest)
	nsname := provisioningRequest.Object.Status.Extensions.ClusterDetails.Name
	clusterInstance := VerifyClusterInstanceCompleted(provisioningRequest, ctx)

	VerifyAllPoliciesInNamespaceAreCompliant(nsname, ctx, nil, nil)
	klog.V(ocloudparams.OCloudLogLevel).Infof("All the policies are compliant")

	VerifyProvisioningRequestIsFulfilled(provisioningRequest)
	klog.V(ocloudparams.OCloudLogLevel).Infof("Provisioning request %s is fulfilled", provisioningRequest.Object.Name)

	snoAPIClient := CreateSnoAPIClient(OCloudConfig.ClusterName1)

	By("Verifying the SR-IOV network node states")

	networkNodeStates, err := sriov.ListNetworkNodeState(snoAPIClient, ocloudparams.SriovNamespace)
	Expect(err).NotTo(HaveOccurred(),
		fmt.Sprintf("Error getting the list of SR-IOV network node states in namespace %s", ocloudparams.SriovNamespace))

	for _, networkNodeState := range networkNodeStates {
		err = networkNodeState.WaitUntilSyncStatus("Succeeded", 30*time.Minute)
		Expect(err).NotTo(HaveOccurred(),
			fmt.Sprintf("SR-IOV network node state %s did not succeeded", networkNodeState.Objects.Name))
	}

	By("Detaching the seed SNO from the hub")

	cluster, err := ocm.PullManagedCluster(HubAPIClient, OCloudConfig.ClusterName1)
	if err == nil {
		err = cluster.DeleteAndWait(time.Minute * 10)
		Expect(err).NotTo(HaveOccurred(),
			fmt.Sprintf("Error deleting managed cluster %s", OCloudConfig.ClusterName1))
		time.Sleep(30 * time.Second)
	}

	By("Creating a seedgen secret in the LCA namespace")

	secret := secret.NewBuilder(snoAPIClient, "seedgen", ocloudparams.LifecycleAgentNamespace, corev1.SecretTypeOpaque)
	data := make(map[string][]byte)
	data["seedAuth"] = []byte(OCloudConfig.LocalRegistryAuth)
	secret.WithData(data)
	_, err = secret.Create()
	Expect(err).NotTo(HaveOccurred(),
		fmt.Sprintf("Error creating seedgen secret in namespace %s: %v", ocloudparams.LifecycleAgentNamespace, err))

	By("Creating a seed generator")

	seedGenerator := lca.NewSeedGeneratorBuilder(snoAPIClient, ocloudparams.SeedGeneratorName)
	seedGenerator.WithSeedImage(OCloudConfig.SeedImage)
	seedGenerator, err = seedGenerator.Create()
	Expect(err).NotTo(HaveOccurred(),
		fmt.Sprintf("Error creating seedgenerator seedimage: %v", err))

	_, err = seedGenerator.WaitUntilComplete(30 * time.Minute)
	Expect(err).NotTo(HaveOccurred(),
		fmt.Sprintf("Seedgenerator seedimage did not completed: %v", err))

	By("Creating the base image")

	_, err = shell.ExecuteCmd(ocloudparams.CreateImageBasedInstallationConfig)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error creating image-based-installation-config.yaml file: %v", err))

	content, err := os.ReadFile(ocloudparams.IbiConfigTemplate)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error reading config template: %v", err))

	templateText := string(content)
	tmpl, _ := template.New("config").Funcs(sprig.FuncMap()).Parse(templateText)

	certContent, err := os.ReadFile(ocloudparams.RegistryCertPath)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error reading certificate file: %v", err))

	registryCertificate := removeLastNewline(string(certContent))

	imageBasedConfigData := ImageBasedInstallConfigData{
		BaseImageName:       OCloudConfig.BaseImageName,
		SeedImage:           OCloudConfig.SeedImage,
		SeedVersion:         OCloudConfig.SeedVersion,
		Registry:            OCloudConfig.Registry5005,
		PullSecret:          OCloudConfig.PullSecret,
		SSHKey:              OCloudConfig.SSHKey,
		RegistryCertificate: registryCertificate,
		InterfaceName:       OCloudConfig.InterfaceName,
		InterfaceIpv6:       OCloudConfig.InterfaceIpv6_2,
		DNSIpv6:             OCloudConfig.DNSIpv6,
		NextHopIpv6:         OCloudConfig.NextHopIpv6,
		NextHopInterface:    OCloudConfig.NextHopInterface,
	}

	var rendered bytes.Buffer

	err = tmpl.Execute(&rendered, imageBasedConfigData)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error executing template file: %v", err))

	err = os.WriteFile(ocloudparams.IbiConfigTemplateYaml, rendered.Bytes(), 0644)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error writing YAML file: %v", err))

	_, err = shell.ExecuteCmd(ocloudparams.CreateIsoImage)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error creating rhcos-ibi.iso image: %v", err))

	err = files.CopyFile(ocloudparams.IbiBasedImageSourcePath, OCloudConfig.IbiBaseImagePath)
	Expect(err).NotTo(HaveOccurred(),
		fmt.Sprintf("Error copying rhcos-ibi.iso image to %s: %v", OCloudConfig.IbiBaseImagePath, err))

	DeprovisionAiSnoCluster(provisioningRequest, clusterInstance, ctx, nil)
}

// verifyImageClusterInstallDoesNotExist verifies that a given ORAN node pool does not exist.
func verifyImageClusterInstallDoesNotExist(
	imageClusterInstall *ibi.ImageClusterInstallBuilder, waitGroup *sync.WaitGroup, ctx SpecContext) {
	if waitGroup != nil {
		defer waitGroup.Done()
		defer GinkgoRecover()
	}

	iciName := imageClusterInstall.Object.Name
	By(fmt.Sprintf("Verifying that image cluster install %s does not exist", iciName))
	Eventually(func(ctx context.Context) bool {
		return !imageClusterInstall.Exists()
	}).WithTimeout(5*time.Second).WithPolling(time.Second).WithContext(ctx).Should(BeTrue(),
		fmt.Sprintf("Image cluster install %s still exists", iciName))
}

// DeprovisionIbiSnoCluster deprovisions a SNO cluster that has been deployed using IBI.
func DeprovisionIbiSnoCluster(
	provisioningRequest *oran.ProvisioningRequestBuilder,
	imageClusterInstall *ibi.ImageClusterInstallBuilder,
	ctx SpecContext) {
	By(fmt.Sprintf("Tearing down PR %s", provisioningRequest.Object.Name))

	prName := provisioningRequest.Object.Name
	By(fmt.Sprintf("Tearing down PR %s", prName))

	nodeAllocationRequest, allocatedNodes, namespace := VerifyOcloudCRsExist(provisioningRequest)
	bmhs := GetBMHsFromAllocatedNodes(allocatedNodes)

	var tearDownWg sync.WaitGroup

	tearDownWg.Add(5)

	go VerifyProvisioningRequestIsDeleted(provisioningRequest, &tearDownWg, ctx)
	go VerifyNamespaceDoesNotExist(namespace, &tearDownWg, ctx)
	go VerifyAllocatedNodesDoNotExist(allocatedNodes, &tearDownWg, ctx)
	go verifyImageClusterInstallDoesNotExist(imageClusterInstall, &tearDownWg, ctx)
	go VerifyNodeAllocationRequestDoesNotExist(nodeAllocationRequest, &tearDownWg, ctx)

	tearDownWg.Wait()

	klog.V(ocloudparams.OCloudLogLevel).Infof("Provisioning request %s has been removed", provisioningRequest.Object.Name)

	// Verify the BMHs are available after the SNO cluster is deprovisioned.
	for _, bmh := range bmhs {
		VerifyBmhIsAvailable(bmh, OCloudConfig.InventoryPoolNamespace)
	}
}

// VerifyImageClusterInstallCompleted verifies that the Image Cluster Install has completed.
func VerifyImageClusterInstallCompleted(
	nodeID string, nsName string, ctx SpecContext) *ibi.ImageClusterInstallBuilder {
	By(fmt.Sprintf("Verifying that Image Cluster Install %s in namespace %s has succeeded", nodeID, nsName))

	imageClusterInstall, err := ibi.PullImageClusterInstall(HubAPIClient, nodeID, nsName)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull Image Cluster install %s from namespace %s; %v", nodeID, nsName, err)

	Eventually(func(ctx context.Context) bool {
		condition, _ := imageClusterInstall.GetCompletedCondition()
		if condition != nil {
			return condition.Status == "True"
		}

		return false
	}).WithTimeout(60*time.Minute).WithPolling(20*time.Second).WithContext(ctx).Should(BeTrue(),
		fmt.Sprintf("Image Cluster Install %s is not Completed", nodeID))

	klog.V(ocloudparams.OCloudLogLevel).Infof("Cluster installation %s has succeeded ", nodeID)

	return imageClusterInstall
}

// baseImageExists returns true if the IBI base image exists false otherwise.
func baseImageExists() bool {
	By(fmt.Sprintf("Verifying that file %s exists", OCloudConfig.IbiBaseImagePath))

	_, err := os.Stat(OCloudConfig.IbiBaseImagePath)
	if err != nil {
		klog.V(ocloudparams.OCloudLogLevel).Infof("file %s (base image) does not exist", OCloudConfig.IbiBaseImagePath)
	}

	return !os.IsNotExist(err)
}

// removeLastNewline removes the last new line.
func removeLastNewline(originalString string) string {
	lastNewline := strings.LastIndex(originalString, "\n")
	if lastNewline == -1 {
		return originalString
	}

	return originalString[:lastNewline] + originalString[lastNewline+1:]
}
