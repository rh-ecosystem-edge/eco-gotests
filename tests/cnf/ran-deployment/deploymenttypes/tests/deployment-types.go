package tests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/argocd"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/hive"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran-deployment/deploymenttypes/internal/gitdetails"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran-deployment/deploymenttypes/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran-deployment/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran-deployment/internal/ranparam"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	sigyaml "sigs.k8s.io/yaml"
)

const (
	gitSiteConfigCloneDir      string = "ztp-deployment-siteconfig"
	gitPolicyTemplatesCloneDir string = "ztp-deployment-policy-templates"
	// ibiExtraManifestsCMName is the common name for IBI extra-manifests ConfigMap.
	ibiExtraManifestsCMName string = "extra-manifests-cm"
	// ibiExtraManifestsCMSuffix is the suffix for the IBI extra-manifests ConfigMap name.
	// The full name is: <cluster-name>-extras-cm0
	// This is used as a fallback if the common name is not found.
	ibiExtraManifestsCMSuffix string = "-extras-cm0"
)

var (
	reHubSideTemplate           = regexp.MustCompile(`\{\{\s*hub[^\r\n]+hub\s*\}\}`)
	ignorePaths       [5]string = [5]string{
		"source-crs/",
		"custom-crs/",
		"extra-manifest/",
		"extra-manifests/",
		"ztp-test/",
	}
)

var _ = Describe("Cluster Deployment Types Tests", Ordered, Label(tsparams.LabelDeploymentTypeTestCases), func() {
	var (
		siteconfigRepo *git.Repository
		policiesRepo   *git.Repository
		pathSiteConfig string
		pathPolicies   string

		deploymentMethod tsparams.DeploymentType
		policyTemplate   tsparams.PolicyType
		isMultiCluster   tsparams.MultiClusterType
		clusterKind      tsparams.ClusterType

		policiesApp *argocd.ApplicationBuilder
		clustersApp *argocd.ApplicationBuilder
	)

	BeforeAll(func() {
		// Determine if cluster deployments were successful, check for compliant policies for each cluster
		Expect(HubAPIClient).ToNot(BeNil(), "HubAPIClient is nil")
		Expect(HubAPIClient.KubeconfigPath).ToNot(BeEmpty(), "KUBECONFIG for hub cluster is not provided.")

		Expect(Spoke1APIClient).ToNot(BeNil(), "Spoke1APIClient is nil")
		Expect(Spoke1APIClient.KubeconfigPath).ToNot(BeEmpty(), "KUBECONFIG for first cluster is not provided.")

		err := ocm.WaitForAllPoliciesComplianceState(
			HubAPIClient, policiesv1.Compliant, time.Minute, runtimeclient.ListOptions{Namespace: RANConfig.Spoke1Name})

		Expect(err).ToNot(HaveOccurred(), "Failed to verify all policies are compliant for spoke %s", RANConfig.Spoke1Name)

		clusterKind = getClusterType(Spoke1APIClient, RANConfig.Spoke1Name)

		if Spoke2APIClient != nil && Spoke2APIClient.KubeconfigPath != "" {
			err = ocm.WaitForAllPoliciesComplianceState(
				HubAPIClient, policiesv1.Compliant, time.Minute, runtimeclient.ListOptions{Namespace: RANConfig.Spoke2Name})

			Expect(err).ToNot(HaveOccurred(), "Failed to verify all policies are compliant for spoke %s", RANConfig.Spoke2Name)

			isMultiCluster = tsparams.MultiCluster

		} else {
			klog.V(tsparams.LogLevel).Infof("Second cluster is not available")
			isMultiCluster = tsparams.SingleCluster
		}

		policiesApp, err = argocd.PullApplication(
			HubAPIClient, tsparams.ArgoCdPoliciesAppName, ranparam.OpenshiftGitOpsNamespace)
		Expect(err).ToNot(HaveOccurred(), "Failed to get the policies app")

		policiesSource, err := gitdetails.GetGitSource(policiesApp)
		Expect(err).ToNot(HaveOccurred(), "Failed to get the policies app git source")

		pathPolicies = policiesSource.Path

		clustersApp, err = argocd.PullApplication(
			HubAPIClient, tsparams.ArgoCdClustersAppName, ranparam.OpenshiftGitOpsNamespace)
		Expect(err).ToNot(HaveOccurred(), "Failed to get the clusters app")

		clustersSource, err := gitdetails.GetGitSource(clustersApp)
		Expect(err).ToNot(HaveOccurred(), "Failed to get the clusters app git source")

		pathSiteConfig = clustersSource.Path

		klog.V(tsparams.LogLevel).Infof("Successful retreival of apps git details")

		mkGitCloneDirs()

		siteconfigRepo, policiesRepo = gitCloneToDirs(clustersApp, policiesApp)

		// Examine files in repos
		deploymentMethod, _ = getFilesInfo(siteconfigRepo, pathSiteConfig)
		_, policyTemplate = getFilesInfo(policiesRepo, pathPolicies)

		// Check for siteconfig deployment type
		deploymentMethod = getDeploymentMethod(HubAPIClient, RANConfig.Spoke1Name, deploymentMethod)
	})

	AfterAll(func() {

		// Clean up git clone directories
		rmGitCloneDirs()
	})

	It(fmt.Sprintf("checks if deployment is %s", tsparams.MultiCluster), reportxml.ID("80498"), func() {

		Expect(isMultiCluster == tsparams.SingleCluster || isMultiCluster == tsparams.MultiCluster).To(BeTrueBecause(
			"Deployment must either be single cluster or multi cluster"))

		if isMultiCluster == tsparams.SingleCluster {
			Skip(fmt.Sprintf("Not %s deployment", tsparams.MultiCluster))
		}

		klog.V(tsparams.LogLevel).Info("Deployment is multi cluster")
	})

	DescribeTable("checks deployment method",
		func(methodValue *tsparams.DeploymentType, kindValue tsparams.DeploymentType) {

			Expect(*methodValue).ToNot(BeEmpty(), "deployMethod should not be empty")

			if *methodValue != kindValue {
				Skip(fmt.Sprintf("Not %s install method", kindValue))
			}

			klog.V(tsparams.LogLevel).Infof("Install method is %s", kindValue)
		},
		func(methodValue *tsparams.DeploymentType, kindValue tsparams.DeploymentType) string {
			return fmt.Sprintf("checks if deployment method is %s", kindValue)
		},
		Entry(nil, &deploymentMethod, tsparams.DeploymentImageBasedCI, reportxml.ID("80495")),
		Entry(nil, &deploymentMethod, tsparams.DeploymentAssistedCI, reportxml.ID("80494")),
		Entry(nil, &deploymentMethod, tsparams.DeploymentSiteConfig, reportxml.ID("80493")),
	)

	DescribeTable("checks policy kind",
		func(policyValue *tsparams.PolicyType, kindValue tsparams.PolicyType) {

			Expect(*policyValue).ToNot(BeEmpty(), "policyTemplate should not be empty")

			if *policyValue != kindValue {
				Skip(fmt.Sprintf("Not %s policy type", kindValue))
			}

			klog.V(tsparams.LogLevel).Infof("Policy type is %s", kindValue)
		},
		func(policyValue *tsparams.PolicyType, kindValue tsparams.PolicyType) string {
			return fmt.Sprintf("checks if policy type is %s", kindValue)
		},
		Entry(nil, &policyTemplate, tsparams.PolicyPGT, reportxml.ID("80496")),
		Entry(nil, &policyTemplate, tsparams.PolicyACMPG, reportxml.ID("80502")),
		Entry(nil, &policyTemplate, tsparams.PolicyPGTHST, reportxml.ID("80501")),
		Entry(nil, &policyTemplate, tsparams.PolicyACMPGHST, reportxml.ID("80503")),
	)

	DescribeTable("checks cluster type",
		func(clusterValue *tsparams.ClusterType, kindValue tsparams.ClusterType) {

			Expect(*clusterValue).ToNot(BeEmpty(), "clusterKind should not be empty")

			if *clusterValue != kindValue {
				Skip(fmt.Sprintf("Not %s cluster type", kindValue))
			}

			klog.V(tsparams.LogLevel).Infof("Cluster type is %s", kindValue)
		},
		func(clusterValue *tsparams.ClusterType, kindValue tsparams.ClusterType) string {
			return fmt.Sprintf("checks if cluster type is %s", kindValue)
		},
		Entry(nil, &clusterKind, tsparams.ClusterSNO, reportxml.ID("80497")),
		Entry(nil, &clusterKind, tsparams.ClusterSNOPlusWorker, reportxml.ID("81679")),
		Entry(nil, &clusterKind, tsparams.ClusterThreeNode, reportxml.ID("80499")),
		Entry(nil, &clusterKind, tsparams.ClusterStandard, reportxml.ID("80500")),
	)

	// IBI Extra-Manifests Validation
	// This test verifies that when a cluster is deployed using Image-Based Installation (IBI),
	// the extra-manifests (specifically ImageDigestMirrorSets and ImageTagMirrorSets) from the
	// seed cluster are properly carried over to the target spoke cluster.
	// It compares the IDMS stored in the hub's extraManifests ConfigMap with the IDMS
	// actually present on the target spoke cluster.
	// The spoke may have MORE IDMS than the ConfigMap because:
	// 1. The ConfigMap contains IDMS defined in ZTP siteconfig (from GitOps)
	// 2. The seed image contains additional IDMS baked in during seed generation
	// Both are merged on the spoke cluster, so we validate the ConfigMap IDMS are a SUBSET.
	Describe("IBI Extra-Manifests Validation", Label(tsparams.LabelIBIExtraManifests), func() {
		It("verifies ImageDigestMirrorSets from extraManifests ConfigMap are applied to IBI deployed spoke",
			reportxml.ID("00000"), func() {
				// Skip if this is not an IBI deployment
				if deploymentMethod != tsparams.DeploymentImageBasedCI {
					Skip(fmt.Sprintf("Skipping: deployment method is %s, not %s",
						deploymentMethod, tsparams.DeploymentImageBasedCI))
				}

				By("Retrieving the extraManifests ConfigMap from the hub cluster")
				// The IBI deployment creates a ConfigMap on the hub containing IDMS.
				// ConfigMap name could be "extra-manifests-cm" or "<cluster-name>-extras-cm0"
				// depending on the deployment method (ZTP GitOps vs direct IBI)
				cmNamespace := RANConfig.Spoke1Name
				var extraManifestsCM *configmap.Builder
				var err error
				var cmName string

				// Try the common name first
				cmName = ibiExtraManifestsCMName
				klog.V(tsparams.LogLevel).Infof("Looking for ConfigMap %s in namespace %s on hub cluster",
					cmName, cmNamespace)

				extraManifestsCM, err = configmap.Pull(HubAPIClient, cmName, cmNamespace)
				if err != nil {
					// Fallback to the cluster-specific name
					cmName = RANConfig.Spoke1Name + ibiExtraManifestsCMSuffix
					klog.V(tsparams.LogLevel).Infof("ConfigMap not found, trying fallback name %s", cmName)

					extraManifestsCM, err = configmap.Pull(HubAPIClient, cmName, cmNamespace)
				}
				Expect(err).ToNot(HaveOccurred(),
					"Failed to get extraManifests ConfigMap from hub in namespace %s. "+
						"Tried names: %s, %s", cmNamespace, ibiExtraManifestsCMName,
					RANConfig.Spoke1Name+ibiExtraManifestsCMSuffix)

				klog.V(tsparams.LogLevel).Infof("Found ConfigMap %s in namespace %s", cmName, cmNamespace)

				By("Extracting IDMS content from the ConfigMap")
				// The ConfigMap may have different keys depending on how it was created:
				// - "99_seed_idms" for IDMS extracted from seed cluster
				// - "04-rh-internal-icsp.yaml" or similar for ZTP siteconfig-defined IDMS
				// We need to parse all keys that contain IDMS YAML
				var expectedSources []string

				for key, value := range extraManifestsCM.Object.Data {
					klog.V(tsparams.LogLevel).Infof("Processing ConfigMap key: %s (length: %d bytes)", key, len(value))

					// Try to parse IDMS from this key's value
					sources := parseIDMSSources(value)
					if len(sources) > 0 {
						klog.V(tsparams.LogLevel).Infof("Found %d IDMS source(s) in key %s: %v",
							len(sources), key, sources)
						expectedSources = append(expectedSources, sources...)
					}
				}

				Expect(expectedSources).ToNot(BeEmpty(),
					"Failed to parse any IDMS sources from the extraManifests ConfigMap %s/%s. "+
						"The ConfigMap may not contain valid IDMS YAML.", cmNamespace, cmName)

				// Remove duplicates from expected sources
				expectedSources = removeDuplicates(expectedSources)

				klog.V(tsparams.LogLevel).Infof("Parsed %d unique expected IDMS source(s) from ConfigMap: %v",
					len(expectedSources), expectedSources)

				By("Listing ImageDigestMirrorSets on the target spoke cluster")
				// List all IDMS resources on the spoke cluster
				spokeIDMSList, err := Spoke1APIClient.ImageDigestMirrorSets().List(
					context.TODO(), metav1.ListOptions{})
				Expect(err).ToNot(HaveOccurred(), "Failed to list ImageDigestMirrorSets on spoke cluster")

				Expect(spokeIDMSList.Items).ToNot(BeEmpty(),
					"No ImageDigestMirrorSets found on spoke cluster. "+
						"Extra-manifests were not applied correctly.")

				// Build a map of actual sources present on the spoke
				actualSources := make(map[string]bool)
				for _, idms := range spokeIDMSList.Items {
					for _, mirror := range idms.Spec.ImageDigestMirrors {
						actualSources[mirror.Source] = true
					}
				}

				klog.V(tsparams.LogLevel).Infof("Found %d unique IDMS source(s) on spoke cluster",
					len(actualSources))

				By("Verifying all ConfigMap IDMS sources exist on the spoke cluster")
				// Compare: each expected source from the ConfigMap should exist on the spoke
				// Note: The spoke may have MORE sources than the ConfigMap (from seed image)
				var missingSources []string
				for _, expectedSource := range expectedSources {
					if !actualSources[expectedSource] {
						missingSources = append(missingSources, expectedSource)
					}
				}

				Expect(missingSources).To(BeEmpty(),
					"The following IDMS sources from the extraManifests ConfigMap are missing on the spoke: %v. "+
						"Extra-manifests were not fully applied during IBI deployment.", missingSources)

				klog.V(tsparams.LogLevel).Infof(
					"SUCCESS: All %d expected IDMS sources from ConfigMap are present on the spoke cluster "+
						"(spoke has %d total sources)", len(expectedSources), len(actualSources))
			})

		It("verifies ImageTagMirrorSets are properly configured on IBI deployed spoke",
			reportxml.ID("00001"), func() {
				// Skip if this is not an IBI deployment
				if deploymentMethod != tsparams.DeploymentImageBasedCI {
					Skip(fmt.Sprintf("Skipping: deployment method is %s, not %s",
						deploymentMethod, tsparams.DeploymentImageBasedCI))
				}

				By("Listing ImageTagMirrorSets on the target spoke cluster")
				// List all ITMS resources on the spoke cluster
				// ITMS may be present if the seed cluster had tag-based mirror configurations
				spokeITMSList, err := Spoke1APIClient.ImageTagMirrorSets().List(
					context.TODO(), metav1.ListOptions{})
				Expect(err).ToNot(HaveOccurred(), "Failed to list ImageTagMirrorSets on spoke cluster")

				// ITMS are optional - the seed cluster may or may not have them configured
				// We log the results but don't fail if they're empty
				if len(spokeITMSList.Items) == 0 {
					klog.V(tsparams.LogLevel).Infof(
						"No ImageTagMirrorSets found on spoke cluster. " +
							"This is acceptable if the seed cluster did not have ITMS configured.")

					Skip("No ImageTagMirrorSets present on spoke cluster - seed likely did not have ITMS")
				}

				By("Verifying ImageTagMirrorSets have valid configurations")
				// If ITMS exist, verify they have valid source configurations
				var itmsSources []string
				for _, itms := range spokeITMSList.Items {
					for _, mirror := range itms.Spec.ImageTagMirrors {
						if mirror.Source != "" {
							itmsSources = append(itmsSources, mirror.Source)
						}
					}
				}

				Expect(itmsSources).ToNot(BeEmpty(),
					"ImageTagMirrorSets exist on spoke but have no configured sources. "+
						"ITMS configuration may be invalid.")

				klog.V(tsparams.LogLevel).Infof(
					"SUCCESS: Found %d ImageTagMirrorSet(s) with %d source(s) on spoke cluster: %v",
					len(spokeITMSList.Items), len(itmsSources), itmsSources)
			})
	})

})

// Clean up git clone dirs if they exist and create empty dirctories for git clone targets.
func mkGitCloneDirs() {
	rmGitCloneDirs()

	err := os.MkdirAll(gitSiteConfigCloneDir, 0755)
	Expect(err).ToNot(HaveOccurred(), "Failed to create %s directory", gitSiteConfigCloneDir)

	err = os.MkdirAll(gitPolicyTemplatesCloneDir, 0755)
	Expect(err).ToNot(HaveOccurred(), "Failed to create %s directory", gitPolicyTemplatesCloneDir)
}

// Delete git clone directories.
func rmGitCloneDirs() {
	err := os.RemoveAll(gitSiteConfigCloneDir)
	Expect(err).ToNot(HaveOccurred(), "Failed to remove %s directory", gitSiteConfigCloneDir)

	err = os.RemoveAll(gitPolicyTemplatesCloneDir)
	Expect(err).ToNot(HaveOccurred(), "Failed to remove %s directory", gitPolicyTemplatesCloneDir)
}

// git clone siteconfig and policy templates to target directories.
// clusters and policies apps are cloned separately to allow for
// the case where they point to different repos/branches/paths.
func gitCloneToDirs(clustersApp,
	policiesApp *argocd.ApplicationBuilder) (
	siteconfigRepo, policiesRepo *git.Repository) {
	clustersSource, err := gitdetails.GetGitSource(clustersApp)
	Expect(err).ToNot(HaveOccurred(), "Failed to get clusters app git source details")

	policiesSource, err := gitdetails.GetGitSource(policiesApp)
	Expect(err).ToNot(HaveOccurred(), "Failed to get policies app git source details")

	siteconfigRepo, err = git.PlainClone(gitSiteConfigCloneDir, false, &git.CloneOptions{
		URL:             clustersSource.RepoURL,
		Tags:            git.NoTags,
		ReferenceName:   plumbing.ReferenceName(clustersSource.TargetRevision),
		Depth:           1,
		SingleBranch:    true,
		Progress:        nil,
		InsecureSkipTLS: RANConfig.SkipTLSVerify,
	})
	Expect(err).ToNot(HaveOccurred(), "Failed to git clone siteconfig repo %s branch %s to directory %s",
		clustersSource.RepoURL, clustersSource.TargetRevision, gitSiteConfigCloneDir)
	klog.V(tsparams.LogLevel).Infof("Successful git clone of sitconfig repo %s branch %s",
		clustersSource.RepoURL, clustersSource.TargetRevision)
	klog.V(tsparams.LogLevel).Infof("Path in worktree: %s", clustersSource.Path)

	policiesRepo, err = git.PlainClone(gitPolicyTemplatesCloneDir, false, &git.CloneOptions{
		URL:             policiesSource.RepoURL,
		Tags:            git.NoTags,
		ReferenceName:   plumbing.ReferenceName(policiesSource.TargetRevision),
		Depth:           1,
		SingleBranch:    true,
		Progress:        nil,
		InsecureSkipTLS: RANConfig.SkipTLSVerify,
	})
	Expect(err).ToNot(HaveOccurred(), "Failed to git clone policies repo %s branch %s to directory %s",
		policiesSource.RepoURL, policiesSource.TargetRevision, gitPolicyTemplatesCloneDir)
	klog.V(tsparams.LogLevel).Infof("Successful git clone of policies repo %s branch %s",
		policiesSource.RepoURL, policiesSource.TargetRevision)
	klog.V(tsparams.LogLevel).Infof("Path in worktree: %s", policiesSource.Path)

	return siteconfigRepo, policiesRepo
}

// Get information from the files in the repo, filtering files by extensions, path, and "kind".
func getFilesInfo(repo *git.Repository, path string) (tsparams.DeploymentType, tsparams.PolicyType) {
	var (
		deploymentMethod tsparams.DeploymentType = ""
		policyTemplate   tsparams.PolicyType     = ""
	)

	remotes, err := repo.Remotes()

	Expect(err).ToNot(HaveOccurred(), "Failed to get list of remotes")
	klog.V(tsparams.LogLevel).Infof("Remote: %s", remotes[0].Config().URLs[0])

	head, err := repo.Head()
	Expect(err).ToNot(HaveOccurred(), "Failed to get branch head")

	commit, err := repo.CommitObject(head.Hash())
	Expect(err).ToNot(HaveOccurred(), "Failed to get commit")

	tree, err := commit.Tree()
	Expect(err).ToNot(HaveOccurred(), "Failed to get file tree")

	subtree, err := tree.Tree(path)
	Expect(err).ToNot(HaveOccurred(), "Failed to get file subtree for path %s", path)

	err = subtree.Files().ForEach(func(fileEntry *object.File) error {
		for _, ignorePath := range ignorePaths {
			if strings.Contains(fileEntry.Name, ignorePath) {
				klog.V(tsparams.LogLevel).Infof("Skipping reference or test CR file: %s", fileEntry.Name)

				return nil
			}
		}

		if filepath.Ext(fileEntry.Name) == ".yaml" || filepath.Ext(fileEntry.Name) == ".yml" {
			klog.V(tsparams.LogLevel).Infof("Path: %s", fileEntry.Name)

			content, err := fileEntry.Contents()
			Expect(err).ToNot(HaveOccurred(), "Failed to get file content")

			contentBytes := []byte(content)

			// Get YAML Kind value.
			kind := getYAMLKind(contentBytes, fileEntry.Name)

			klog.V(tsparams.LogLevel).Infof("Kind from YAML: %s", kind)

			// Determine deployment and policy types
			switch kind {
			case string(tsparams.DeploymentSiteConfig):
				deploymentMethod = tsparams.DeploymentSiteConfig
			case string(tsparams.PolicyPGT):
				hasHST := checkForHubSideTemplate(contentBytes)

				if !hasHST && policyTemplate != tsparams.PolicyPGTHST {
					policyTemplate = tsparams.PolicyPGT
				} else if hasHST {
					policyTemplate = tsparams.PolicyPGTHST
				}
			case string(tsparams.PolicyACMPG):
				hasHST := checkForHubSideTemplate(contentBytes)

				if !hasHST && policyTemplate != tsparams.PolicyACMPGHST {
					policyTemplate = tsparams.PolicyACMPG
				} else if hasHST {
					policyTemplate = tsparams.PolicyACMPGHST
				}
			}

			return nil
		}

		klog.V(tsparams.LogLevel).Infof("Skipping non-YAML file: %s", fileEntry.Name)

		return nil
	})
	Expect(err).ToNot(HaveOccurred(), "Failed to get file iterator")

	return deploymentMethod, policyTemplate
}

// unmarshal YAML and get CR kind. Return empty string if kind is not found in YAML.
func getYAMLKind(fileData []byte, fileName string) string {
	fileContent := make(map[string]any)
	err := yaml.Unmarshal(fileData, &fileContent)
	Expect(err).ToNot(HaveOccurred(), "Failed to unmarshal file %s as yaml", fileName)

	kind, result := fileContent["kind"].(string)
	if !result {
		klog.V(tsparams.LogLevel).Infof("Failed to determine kind from file %s", fileName)

		return ""
	}

	return kind
}

// Check file for hub-side templating syntax.
func checkForHubSideTemplate(content []byte) bool {
	return reHubSideTemplate.Match(content)
}

// getCluterType determines the cluster type as one of: standard, 3node, SNO, SNO+Worker.
func getClusterType(cluster *clients.Settings, clusterName string) tsparams.ClusterType {
	var (
		workerCount                            = 0
		controlPlaneCount                      = 0
		clusterKind       tsparams.ClusterType = ""
	)

	if cluster.KubeconfigPath == "" {
		klog.V(tsparams.LogLevel).Infof("Cluster %s KUBECONFIG is not availabled", clusterName)

		return clusterKind
	}

	nodes, err := nodes.List(cluster)
	Expect(err).ToNot(HaveOccurred(), "Failed to get nodes list")

	for _, node := range nodes {
		nodeName := node.Definition.Name

		_, isControlPlane := node.Object.Labels[RANConfig.ControlPlaneLabel]
		_, isWorker := node.Object.Labels[RANConfig.WorkerLabel]

		Expect(isWorker || isControlPlane).To(BeTrue(), "Node %s has neither control-plane nor worker label?", nodeName)

		// node can be both control-plane and worker, so check each separately
		if isControlPlane {
			controlPlaneCount++
		}

		if isWorker {
			workerCount++
		}
	}

	klog.V(tsparams.LogLevel).Infof(
		"controlPlaneCount: %d -- workerCount: %d", controlPlaneCount, workerCount)

	switch {
	case (controlPlaneCount == 3) && (workerCount == 2):
		clusterKind = tsparams.ClusterStandard
	case (controlPlaneCount == 3) && (workerCount == 3):
		clusterKind = tsparams.ClusterThreeNode
	case (controlPlaneCount == 1) && (workerCount == 2):
		clusterKind = tsparams.ClusterSNOPlusWorker
	case (controlPlaneCount == 1) && (workerCount == 1):
		clusterKind = tsparams.ClusterSNO
	}

	return clusterKind
}

// getDeploymentMethod determines the deployment type as one of: SiteConfig with AgentClusterInstall,
// ClusterInstance with AgentClusterInstall, or ClusterInstance with ImageClusterInstall.
func getDeploymentMethod(
	hub *clients.Settings,
	clusterName string,
	deploymentMethod tsparams.DeploymentType) tsparams.DeploymentType {
	var (
		deployment tsparams.DeploymentType = ""
	)

	if deploymentMethod == tsparams.DeploymentSiteConfig {
		return deploymentMethod
	}

	clusterDeployment, err := hive.PullClusterDeployment(hub, clusterName, clusterName)

	Expect(err).ToNot(HaveOccurred(), "Failed to get ClusterDeployment for cluster %s", clusterName)

	Expect(hive.PullClusterDeployment(
		hub, clusterName, clusterName)).ToNot(BeNil(), "ClusterDeployment for cluster %s is nil", clusterName)

	installKind := clusterDeployment.Object.Spec.ClusterInstallRef.Kind
	Expect(installKind).ToNot(BeEmpty(),
		fmt.Sprintf("clusterdeployment %s does not have ClusterInstallRef.Kind value",
			clusterName))

	switch installKind {
	case "ImageClusterInstall":
		deployment = tsparams.DeploymentImageBasedCI
	case "AgentClusterInstall":
		deployment = tsparams.DeploymentAssistedCI
	}

	return deployment
}

// removeDuplicates removes duplicate strings from a slice while preserving order.
func removeDuplicates(slice []string) []string {
	seen := make(map[string]bool)
	result := []string{}

	for _, item := range slice {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}

	return result
}

// parseIDMSSources extracts the list of IDMS sources from IDMS YAML content.
// The YAML content can be an ImageDigestMirrorSetList, a single ImageDigestMirrorSet,
// or a raw YAML document containing an IDMS definition.
// Returns a slice of source strings (e.g., "registry.redhat.io", "quay.io", etc.)
func parseIDMSSources(idmsYaml string) []string {
	var sources []string

	// The seed IDMS YAML is an ImageDigestMirrorSetList
	var idmsList configv1.ImageDigestMirrorSetList

	err := sigyaml.Unmarshal([]byte(idmsYaml), &idmsList)
	if err != nil {
		klog.V(tsparams.LogLevel).Infof("Failed to unmarshal IDMS list: %v", err)
		// Try parsing as a single IDMS (in case it's not a list)
		var singleIDMS configv1.ImageDigestMirrorSet

		err = sigyaml.Unmarshal([]byte(idmsYaml), &singleIDMS)
		if err != nil {
			klog.V(tsparams.LogLevel).Infof("Failed to unmarshal single IDMS: %v", err)

			return sources
		}

		// Extract sources from single IDMS
		for _, mirror := range singleIDMS.Spec.ImageDigestMirrors {
			if mirror.Source != "" {
				sources = append(sources, mirror.Source)
			}
		}

		return sources
	}

	// Extract sources from IDMS list
	for _, idms := range idmsList.Items {
		for _, mirror := range idms.Spec.ImageDigestMirrors {
			if mirror.Source != "" {
				sources = append(sources, mirror.Source)
			}
		}
	}

	return sources
}
