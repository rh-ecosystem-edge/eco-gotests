package preinstall

import (
	"fmt"
	"strings"
	"time"

	bmhv1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmh"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

// CreateBMCSecret creates a secret containing the BMC credentials.
func CreateBMCSecret(apiClient *clients.Settings, name, namespace, username, password string) (*secret.Builder, error) {
	klog.Infof("Creating BMC secret %s in namespace %s", name, namespace)

	secretBuilder := secret.NewBuilder(
		apiClient, name, namespace, corev1.SecretTypeOpaque).WithData(map[string][]byte{
		"username": []byte(username),
		"password": []byte(password),
	})

	_, err := secretBuilder.Create()
	if err != nil {
		return nil, fmt.Errorf("failed to create BMC secret: %w", err)
	}

	return secretBuilder, nil
}

// CreateBareMetalHost creates a BareMetalHost CR pointing to the IBI ISO.
func CreateBareMetalHost(
	apiClient *clients.Settings,
	name, namespace, bmcAddress, macAddress, bmcSecretName, isoURL string) (*bmh.BmhBuilder, error) {
	klog.Infof("Creating BareMetalHost %s in namespace %s", name, namespace)

	// Create BMH builder
	bmhBuilder := bmh.NewBuilder(
		apiClient, name, namespace, bmcAddress, bmcSecretName, macAddress, "UEFI")

	// Set automated cleaning mode to disabled
	bmhBuilder.Definition.Spec.AutomatedCleaningMode = "disabled"

	// Set the image URL to the ISO
	bmhBuilder.Definition.Spec.Image = &bmhv1alpha1.Image{
		URL: isoURL,
	}

	// Set online to true
	bmhBuilder.Definition.Spec.Online = true

	// Add annotation to disable inspection
	if bmhBuilder.Definition.Annotations == nil {
		bmhBuilder.Definition.Annotations = make(map[string]string)
	}

	bmhBuilder.Definition.Annotations["inspect.metal3.io"] = "disabled"

	_, err := bmhBuilder.Create()
	if err != nil {
		return nil, fmt.Errorf("failed to create BareMetalHost: %w", err)
	}

	return bmhBuilder, nil
}

// WaitForPreinstallCompletion waits for the preinstall process to finish by checking journalctl on the node.
func WaitForPreinstallCompletion(host, user, sshKeyPath string, timeout, pollInterval time.Duration) error {
	klog.Infof("Waiting for preinstall to complete on %s", host)

	startTime := time.Now()

	for time.Since(startTime) < timeout {
		// command to check journalctl
		cmd := "journalctl -l -u install-rhcos-and-restore-seed.service | tail -2"

		output, err := SSHExecOnProvisioningHost(host, user, sshKeyPath, cmd)
		if err == nil {
			// Check for success strings
			if strings.Contains(output, "Finished SNO Image-based Installation") ||
				strings.Contains(output, "Finished SNO Image Based Installation") {
				klog.Infof("Preinstall completed successfully on %s", host)

				return nil
			}
		}

		klog.V(5).Infof("Preinstall not yet complete, waiting %v...", pollInterval)
		time.Sleep(pollInterval)
	}

	return fmt.Errorf("timed out waiting for preinstall to complete after %v", timeout)
}
