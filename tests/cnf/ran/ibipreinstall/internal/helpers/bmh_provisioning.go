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

	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ibipreinstall/internal/tsparams"
)

// CreateBMCSecret creates a secret containing the BMC credentials.
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

// bmhProvisionedErrorThreshold controls how many times the BMH's own
// ErrorCount must reach before we treat the error as permanent. Metal3
// retries transient errors (Ironic busy, BMC hiccup) automatically and
// increments ErrorCount each time; a low threshold risks false positives.
const bmhProvisionedErrorThreshold = 3

// WaitForBMHProvisioned polls the BareMetalHost until its provisioning state
// reaches "provisioned". If the BMH enters an error state and its ErrorCount
// reaches bmhProvisionedErrorThreshold, this function fails fast.
func WaitForBMHProvisioned(
	parentCtx context.Context,
	apiClient *clients.Settings,
	name, namespace string,
	timeout, pollInterval time.Duration,
) error {
	klog.V(tsparams.LogLevel).Infof("Waiting for BMH %s/%s to reach provisioned state", namespace, name)

	return wait.PollUntilContextTimeout(parentCtx, pollInterval, timeout, false, func(_ context.Context) (bool, error) {
		bmhBuilder, err := bmh.Pull(apiClient, name, namespace)
		if err != nil {
			klog.V(tsparams.LogLevel).Infof("Could not pull BMH %s: %v (will retry)", name, err)

			return false, nil
		}

		status := bmhBuilder.Object.Status
		state := status.Provisioning.State
		opStatus := status.OperationalStatus

		klog.V(tsparams.LogLevel).Infof("BMH %s: state=%s operationalStatus=%s errorCount=%d",
			name, state, opStatus, status.ErrorCount)

		if state == bmhv1alpha1.StateProvisioned {
			return true, nil
		}

		if opStatus == bmhv1alpha1.OperationalStatusError && status.ErrorCount >= bmhProvisionedErrorThreshold {
			return false, fmt.Errorf(
				"BMH %s in error state (errorCount=%d, errorType=%q): %s",
				name, status.ErrorCount, status.ErrorType, status.ErrorMessage)
		}

		return false, nil
	})
}

// sshFailureTimeout is the maximum duration SSH can remain unreachable.
// This covers both the initial boot (where the node may take a while to
// come up) and mid-run connectivity loss (node powered off/deprovisioned).
const sshFailureTimeout = 15 * time.Minute

// WaitForPreinstallCompletion polls the spoke node via SSH using
// "systemctl is-active" for the preinstall service unit. It returns
// success when the service reaches "active" (oneshot completed), fails
// immediately on "failed" (with journal output), and treats any other
// state as an unexpected error. If SSH remains unreachable for longer
// than sshFailureTimeout, it fails early.
func WaitForPreinstallCompletion(
	parentCtx context.Context,
	host, user, sshKeyPath string,
	timeout, pollInterval time.Duration,
) error {
	klog.V(tsparams.LogLevel).Infof("Waiting for preinstall service to complete on %s", host)

	startTime := time.Now()

	var (
		lastOutput      string
		sshFailingSince *time.Time
	)

	err := wait.PollUntilContextTimeout(parentCtx, pollInterval, timeout, false, func(ctx context.Context) (bool, error) {
		statusCmd := fmt.Sprintf("systemctl is-active %s 2>&1 || true", tsparams.PreinstallServiceUnit)

		output, sshErr := SSHExec(ctx, host, user, sshKeyPath, statusCmd)
		if sshErr != nil {
			now := time.Now()
			if sshFailingSince == nil {
				sshFailingSince = &now
			}

			failDuration := now.Sub(*sshFailingSince)
			klog.V(tsparams.LogLevel).Infof("SSH to %s unavailable for %v (timeout %v): %v, waiting %v...",
				host, failDuration.Round(time.Second), sshFailureTimeout, sshErr, pollInterval)

			if failDuration >= sshFailureTimeout {
				return false, fmt.Errorf("SSH to %s unreachable for %v — node may have been powered off or deprovisioned: %w",
					host, failDuration.Round(time.Second), sshErr)
			}

			return false, nil
		}

		sshFailingSince = nil

		unitStatus := strings.TrimSpace(output)
		klog.V(tsparams.LogLevel).Infof("%s on %s: %s", tsparams.PreinstallServiceUnit, host, unitStatus)

		switch unitStatus {
		case "activating", "inactive", "unknown", "deactivating", "reloading", "":
			lastOutput = unitStatus

			return false, nil

		case "active":
			klog.V(tsparams.LogLevel).Infof("Preinstall service completed on %s", host)

			return true, nil

		case "failed":
			journalCmd := fmt.Sprintf("journalctl -u %s --no-pager -n 50 2>&1", tsparams.PreinstallServiceUnit)
			journalOut, _ := SSHExec(ctx, host, user, sshKeyPath, journalCmd)
			lastOutput = journalOut

			return false, fmt.Errorf("preinstall service failed on %s:\n%s", host, journalOut)

		default:
			lastOutput = unitStatus

			return false, fmt.Errorf("unexpected service state %q on %s", unitStatus, host)
		}
	})
	if err != nil {
		return fmt.Errorf("waiting for preinstall on %s (started %v, timeout %v): output=%q: %w",
			host, startTime.Format(time.RFC3339), timeout, lastOutput, err)
	}

	return nil
}
