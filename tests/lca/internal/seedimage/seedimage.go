package seedimage

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/containers/image/v5/pkg/sysregistriesv2"
	"github.com/openshift-kni/lifecycle-agent/lca-cli/seedclusterinfo"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/lca"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/internal/lcaparams"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

const (
	seedImageLabel    = "com.openshift.lifecycle-agent.seed_cluster_info"
	seedGeneratorName = "seedimage"
	defaultTimeout    = 30 * time.Minute
)

// GetContent returns the structured contents of a seed image as SeedImageContent.
//
//nolint:funlen
func GetContent(apiClient *clients.Settings, seedImageLocation string) (*SeedImageContent, error) {
	if apiClient == nil {
		return nil, fmt.Errorf("nil apiclient passed to seed image function")
	}

	if seedImageLocation == "" {
		return nil, fmt.Errorf("empty seed image location passed to seed image function")
	}

	ibuNodes, err := nodes.List(apiClient)
	if err != nil {
		return nil, err
	}

	if len(ibuNodes) == 0 {
		return nil, fmt.Errorf("node list was empty")
	}

	seedNode := ibuNodes[0].Object.Name

	targetProxy, err := cluster.GetOCPProxy(apiClient)
	if err != nil {
		return nil, err
	}

	var connectionString string

	switch {
	case len(targetProxy.Object.Spec.HTTPSProxy) != 0:
		connectionString =
			fmt.Sprintf("sudo HTTPS_PROXY=%s", targetProxy.Object.Spec.HTTPSProxy)
	case len(targetProxy.Object.Spec.HTTPProxy) != 0:
		connectionString =
			fmt.Sprintf("sudo HTTP_PROXY=%s", targetProxy.Object.Spec.HTTPProxy)
	default:
		connectionString = "sudo"
	}

	skopeoInspectCmd := fmt.Sprintf("%s skopeo inspect docker://%s", connectionString, seedImageLocation)

	skopeoInspectJSONOutput, err := cluster.ExecCmdWithStdout(
		apiClient, skopeoInspectCmd, metav1.ListOptions{
			FieldSelector: fmt.Sprintf("metadata.name=%s", seedNode),
		})
	if err != nil {
		return nil, err
	}

	skopeoInspectJSON := skopeoInspectJSONOutput[seedNode]

	var imageMeta ImageInspect

	err = json.Unmarshal([]byte(skopeoInspectJSON), &imageMeta)
	if err != nil {
		return nil, err
	}

	if _, ok := imageMeta.Labels[seedImageLabel]; !ok {
		return nil, fmt.Errorf("%s image did not contain expected label: %s", seedImageLocation, seedImageLabel)
	}

	seedInfo := new(SeedImageContent)
	seedInfo.SeedClusterInfo = new(seedclusterinfo.SeedClusterInfo)

	err = json.Unmarshal([]byte(imageMeta.Labels[seedImageLabel]), seedInfo.SeedClusterInfo)
	if err != nil {
		return nil, err
	}

	var mountedFilePath string

	var unmount func()

	if seedInfo.HasProxy {
		podmanPullCmd := fmt.Sprintf("%s podman pull", connectionString)

		mountedFilePath, unmount, err = pullAndMountImage(apiClient, seedNode, podmanPullCmd, seedImageLocation)
		if err != nil {
			return nil, err
		}

		defer unmount()

		proxyEnvOutput, err := cluster.ExecCmdWithStdout(
			apiClient, fmt.Sprintf("sudo tar xzf %s/etc.tgz -O etc/mco/proxy.env", mountedFilePath), metav1.ListOptions{
				FieldSelector: fmt.Sprintf("metadata.name=%s", seedNode),
			})
		if err != nil {
			return nil, err
		}

		proxyEnv := proxyEnvOutput[seedNode]

		seedInfo.ParseProxyEnv(proxyEnv)

		if seedInfo.Proxy.HTTPProxy == "" || seedInfo.Proxy.HTTPSProxy == "" {
			return nil, fmt.Errorf("encountered an error gathering proxy info: %v", seedInfo.Proxy)
		}
	}

	if seedInfo.MirrorRegistryConfigured {
		if mountedFilePath == "" {
			podmanPullCmd := fmt.Sprintf("%s podman pull", connectionString)

			mountedFilePath, unmount, err = pullAndMountImage(apiClient, seedNode, podmanPullCmd, seedImageLocation)
			if err != nil {
				return nil, err
			}

			defer unmount()
		}

		mirrorConfigOutput, err := cluster.ExecCmdWithStdout(
			apiClient, fmt.Sprintf("sudo tar xzf %s/etc.tgz -O etc/containers/registries.conf",
				mountedFilePath), metav1.ListOptions{FieldSelector: fmt.Sprintf("metadata.name=%s", seedNode)})
		if err != nil {
			return nil, err
		}

		mirrorConfig := mirrorConfigOutput[seedNode]

		var registriesConfig sysregistriesv2.V2RegistriesConf

		err = toml.Unmarshal([]byte(mirrorConfig), &registriesConfig)
		if err != nil {
			return nil, err
		}

		seedInfo.ParseMirrorConf(registriesConfig)

		if len(seedInfo.MirrorConfig.Spec.ImageDigestMirrors) == 0 {
			return nil, fmt.Errorf("encountered an error gathering mirror info: %v",
				seedInfo.MirrorConfig.Spec.ImageDigestMirrors)
		}
	}

	return seedInfo, nil
}

// ParseProxyEnv reads a proxy.env config and sets SeedImageContent.Proxy values accordingly.
func (s *SeedImageContent) ParseProxyEnv(config string) {
	httpProxyRE := regexp.MustCompile(`HTTP_PROXY=(.+)`)
	httpProxyResult := httpProxyRE.FindString(config)

	if len(httpProxyResult) > 0 {
		httpProxyKeyVal := strings.Split(httpProxyResult, "=")
		if len(httpProxyKeyVal) == 2 {
			s.Proxy.HTTPProxy = httpProxyKeyVal[1]
		}
	}

	httpsProxyRE := regexp.MustCompile(`HTTPS_PROXY=(.+)`)
	httpsProxyResult := httpsProxyRE.FindString(config)

	if len(httpsProxyResult) > 0 {
		httpsKeyVal := strings.Split(httpsProxyResult, "=")
		if len(httpsKeyVal) == 2 {
			s.Proxy.HTTPSProxy = httpsKeyVal[1]
		}
	}

	noProxyRE := regexp.MustCompile(`NO_PROXY=(.*)`)
	noProxyResult := noProxyRE.FindString(config)

	if len(noProxyResult) > 0 {
		noProxyKeyVal := strings.Split(noProxyResult, "=")
		if len(noProxyKeyVal) == 2 {
			s.Proxy.NOProxy = noProxyKeyVal[1]
		}
	}
}

// ParseMirrorConf reads a registries.conf config and sets SeedImageContent.MirrorConfig accordingly.
func (s *SeedImageContent) ParseMirrorConf(config sysregistriesv2.V2RegistriesConf) {
	s.MirrorConfig = &configv1.ImageDigestMirrorSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "seed-image-digest-mirror",
		},
	}

	for _, reg := range config.Registries {
		var registryMirrors []configv1.ImageMirror
		for _, mirror := range reg.Mirrors {
			registryMirrors = append(registryMirrors, configv1.ImageMirror(mirror.Location))
		}

		s.MirrorConfig.Spec.ImageDigestMirrors = append(s.MirrorConfig.Spec.ImageDigestMirrors, configv1.ImageDigestMirrors{
			Source:  reg.Location,
			Mirrors: registryMirrors,
		})
	}
}

func pullAndMountImage(apiClient *clients.Settings, node, pullCommand, image string) (string, func(), error) {
	_, err := cluster.ExecCmdWithStdout(
		apiClient, fmt.Sprintf("%s %s", pullCommand, image), metav1.ListOptions{
			FieldSelector: fmt.Sprintf("metadata.name=%s", node),
		})
	if err != nil {
		return "", nil, err
	}

	mountedFilePathOutput, err := cluster.ExecCmdWithStdout(
		apiClient, fmt.Sprintf("sudo podman image mount %s", image), metav1.ListOptions{
			FieldSelector: fmt.Sprintf("metadata.name=%s", node),
		})
	if err != nil {
		return "", nil, err
	}

	mountedFilePath := regexp.MustCompile(`\n`).ReplaceAllString(mountedFilePathOutput[node], "")

	return mountedFilePath, func() {
		_, err := cluster.ExecCmdWithStdout(
			apiClient, fmt.Sprintf("sudo podman image unmount %s", image), metav1.ListOptions{
				FieldSelector: fmt.Sprintf("metadata.name=%s", node),
			})
		if err != nil {
			klog.V(lcaparams.LCALogLevel).Info("Error occurred while unmounting image")
		}
	}, nil
}

// GenerateSeedImage creates a SeedGenerator CR on the spoke cluster, waits for the seed image
// to be generated, and verifies it was created successfully.
//
// Parameters:
//   - apiClient: The Kubernetes API client for the spoke cluster
//   - seedImageLocation: The full pull-spec of the seed container image to be created
//   - recertImage: Optional recert image to use. If empty, the default will be used
//   - timeout: Maximum time to wait for seed generation to complete. If zero, defaults to 30 minutes
//
// Returns:
//   - The seed image location (full pull-spec)
//   - An error if any step fails
func GenerateSeedImage(
	apiClient *clients.Settings,
	seedImageLocation string,
	recertImage string,
	timeout time.Duration,
) (string, error) {
	if apiClient == nil {
		return "", fmt.Errorf("nil apiclient passed to GenerateSeedImage")
	}

	if seedImageLocation == "" {
		return "", fmt.Errorf("seedImageLocation cannot be empty")
	}

	if timeout == 0 {
		timeout = defaultTimeout
	}

	klog.V(lcaparams.LCALogLevel).Infof("Creating SeedGenerator CR with seed image location: %s", seedImageLocation)

	// Create SeedGenerator builder
	seedGenerator := lca.NewSeedGeneratorBuilder(apiClient, seedGeneratorName)
	if seedGenerator == nil {
		return "", fmt.Errorf("failed to create SeedGenerator builder")
	}

	// Set seed image location
	seedGenerator.WithSeedImage(seedImageLocation)

	// Set recert image if provided
	if recertImage != "" {
		seedGenerator.WithRecertImage(recertImage)
	}

	// Create the SeedGenerator CR
	seedGenerator, err := seedGenerator.Create()
	if err != nil {
		return "", fmt.Errorf("failed to create SeedGenerator CR: %w", err)
	}

	klog.V(lcaparams.LCALogLevel).Info("Waiting for SeedGenerator to complete seed image generation")

	// Wait for seed generation to complete
	_, err = seedGenerator.WaitUntilComplete(timeout)
	if err != nil {
		return "", fmt.Errorf("seed generation did not complete within timeout: %w", err)
	}

	klog.V(lcaparams.LCALogLevel).Info("SeedGenerator completed successfully, verifying seed image exists")

	// Verify the seed image exists by inspecting it
	err = verifySeedImageExists(apiClient, seedImageLocation)
	if err != nil {
		return "", fmt.Errorf("failed to verify seed image exists: %w", err)
	}

	klog.V(lcaparams.LCALogLevel).Info("Seed image verified successfully")

	klog.V(lcaparams.LCALogLevel).Infof("Successfully generated seed image at: %s", seedImageLocation)

	return seedImageLocation, nil
}

// verifySeedImageExists verifies that the seed image exists in the registry.
// If skopeo inspect succeeds, the image exists and is accessible.
// Uses ExecCommandOnSNOWithRetries to handle temporary cluster unavailability
// after seed generation completes.
func verifySeedImageExists(apiClient *clients.Settings, seedImageLocation string) error {
	skopeoInspectCmd := fmt.Sprintf("sudo skopeo inspect docker://%s", seedImageLocation)

	// Use retries to handle temporary cluster unavailability after seed generation
	// 3 retries with 5 second intervals gives us ~15 seconds total retry time
	_, err := cluster.ExecCommandOnSNOWithRetries(
		apiClient,
		3,             // retries
		5*time.Second, // interval
		skopeoInspectCmd,
	)
	if err != nil {
		return fmt.Errorf("failed to verify seed image exists at %s: %w", seedImageLocation, err)
	}

	klog.V(lcaparams.LCALogLevel).Info("Seed image verified successfully")

	return nil
}
