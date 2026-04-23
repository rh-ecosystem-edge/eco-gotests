package helpers

import (
	"context"
	"fmt"
	"strings"
	"time"

	bmhv1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmh"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedinstall/cnf/ran/preinstall/internal/tsparams"
)

const (
	deleteResourcePollInterval = 5 * time.Second
	deleteResourceWaitTimeout  = 5 * time.Minute
)

// CreateBMCSecret creates a secret containing the BMC credentials.
// Values are stored as plain text in Secret data (Kubernetes API base64-encodes on persist);
// provide ECO_LCA_IBI_BMC_USERNAME and ECO_LCA_IBI_BMC_PASSWORD as plain text, not pre-encoded.
func CreateBMCSecret(apiClient *clients.Settings, name, namespace, username, password string) (*secret.Builder, error) {
	klog.V(tsparams.LogLevel).Infof("Creating BMC secret %s in namespace %s", name, namespace)

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
	klog.V(tsparams.LogLevel).Infof("Creating BareMetalHost %s in namespace %s", name, namespace)

	bmhBuilder := bmh.NewBuilder(
		apiClient, name, namespace, bmcAddress, bmcSecretName, macAddress, "UEFI")

	bmhBuilder.Definition.Spec.AutomatedCleaningMode = "disabled"

	liveISO := "live-iso"

	bmhBuilder.Definition.Spec.Image = &bmhv1alpha1.Image{
		URL:        isoURL,
		DiskFormat: &liveISO,
	}

	bmhBuilder.Definition.Spec.Online = true

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
func WaitForPreinstallCompletion(
	parentCtx context.Context,
	host, user, sshKeyPath string,
	timeout, pollInterval time.Duration,
) error {
	klog.V(tsparams.LogLevel).Infof("Waiting for preinstall to complete on %s", host)

	startTime := time.Now()

	var (
		lastErr    error
		lastOutput string
	)

	err := wait.PollUntilContextTimeout(parentCtx, pollInterval, timeout, false, func(ctx context.Context) (bool, error) {
		cmd := "journalctl -l -u install-rhcos-and-restore-seed.service | tail -2"

		output, err := SSHExecOnProvisioningHost(ctx, host, user, sshKeyPath, cmd)
		lastOutput = output

		if err != nil {
			lastErr = err

			klog.V(tsparams.LogLevel).Infof("Preinstall not yet complete, waiting %v...", pollInterval)

			return false, nil
		}

		lastErr = nil

		if strings.Contains(output, "Finished SNO Image-based Installation") ||
			strings.Contains(output, "Finished SNO Image Based Installation") {
			klog.V(tsparams.LogLevel).Infof("Preinstall completed successfully on %s", host)

			return true, nil
		}

		klog.V(tsparams.LogLevel).Infof("Preinstall not yet complete, waiting %v...", pollInterval)

		return false, nil
	})
	if err == nil {
		return nil
	}

	if lastErr != nil {
		return fmt.Errorf(
			"timed out waiting for preinstall to complete after %v (started %v): output=%q: %w",
			timeout, startTime, lastOutput, lastErr)
	}

	return fmt.Errorf(
		"timed out waiting for preinstall to complete after %v (started %v): output=%q",
		timeout, startTime, lastOutput)
}

// DeletePreinstallBMHResources deletes the BareMetalHost and BMC secret if they exist.
func DeletePreinstallBMHResources(apiClient *clients.Settings, bmhName, bmhNamespace, bmcSecretName string) error {
	if apiClient == nil {
		return fmt.Errorf("api client is nil")
	}

	bmhBuilder, err := bmh.Pull(apiClient, bmhName, bmhNamespace)
	if err == nil {
		_, err = bmhBuilder.DeleteAndWaitUntilDeleted(deleteResourceWaitTimeout)
		if err != nil {
			return fmt.Errorf("delete bmh: %w", err)
		}
	} else if !strings.Contains(err.Error(), "does not exist") {
		return fmt.Errorf("pull bmh: %w", err)
	}

	secretBuilder := secret.NewBuilder(apiClient, bmcSecretName, bmhNamespace, corev1.SecretTypeOpaque)
	if secretBuilder.Exists() {
		if err := secretBuilder.Delete(); err != nil {
			return fmt.Errorf("delete bmc secret: %w", err)
		}

		if err := waitSecretDeleted(apiClient, bmhNamespace, bmcSecretName, deleteResourceWaitTimeout); err != nil {
			return err
		}
	}

	return nil
}

func waitSecretDeleted(apiClient *clients.Settings, namespace, name string, timeout time.Duration) error {
	builder := secret.NewBuilder(apiClient, name, namespace, corev1.SecretTypeOpaque)

	err := wait.PollUntilContextTimeout(
		context.TODO(), deleteResourcePollInterval, timeout, false, func(_ context.Context) (bool, error) {
			if !builder.Exists() {
				return true, nil
			}

			return false, nil
		})
	if err != nil {
		return fmt.Errorf(
			"timed out after %v waiting for Secret %s/%s to be removed (still present; finalizers may be pending): %w",
			timeout, namespace, name, err)
	}

	return nil
}
