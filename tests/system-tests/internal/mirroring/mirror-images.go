package mirroring

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/platform"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/remote"
	"k8s.io/klog/v2"
)

// MirrorImageToTheLocalRegistry downloads image to the local mirror registry.
func MirrorImageToTheLocalRegistry(
	apiClient *clients.Settings,
	originServerURL,
	imageName,
	imageTag,
	host,
	user,
	pass,
	localRegistryRepository,
	kubeconfigPath string) (string, string, error) {
	if originServerURL == "" {
		klog.V(100).Infof("The originServerURL is empty")

		return "", "", fmt.Errorf("the originServerURL could not be empty")
	}

	if imageName == "" {
		klog.V(100).Infof("The imageName is empty")

		return "", "", fmt.Errorf("the imageName could not be empty")
	}

	if imageTag == "" {
		klog.V(100).Infof("The imageTag is empty")

		return "", "", fmt.Errorf("the imageTag could not be empty")
	}

	originalImageURL := fmt.Sprintf("%s/%s", originServerURL, imageName)

	isDisconnected, err := platform.IsDisconnectedDeployment(apiClient)
	if err != nil {
		return "", "", err
	}

	if !isDisconnected {
		klog.Info("Deployment type is not disconnected, images mirroring not required")

		return originalImageURL, imageTag, nil
	}

	localRegistryURL, err := platform.GetLocalMirrorRegistryURL(apiClient)
	if err != nil {
		return "", "", err
	}

	localImageURL := fmt.Sprintf("%s/%s/%s", localRegistryURL, localRegistryRepository, imageName)

	klog.Infof("Mirror image %s to the local registry %s", originalImageURL, localRegistryURL)

	imageMirrorCmd := fmt.Sprintf("oc --kubeconfig %s image mirror --insecure=true %s:%s %s:%s",
		kubeconfigPath, originalImageURL, imageTag, localImageURL, imageTag)

	_, err = remote.ExecCmdOnHost(host, user, pass, imageMirrorCmd)
	if err != nil {
		klog.Infof("failed to execute %s command due to %s", imageMirrorCmd, err)

		return "", "", err
	}

	return localImageURL, imageTag, nil
}
