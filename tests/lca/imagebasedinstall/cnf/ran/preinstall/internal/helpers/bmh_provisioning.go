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
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
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
	klog.Infof("Waiting for preinstall to complete on %s", host)

	startTime := time.Now()

	var (
		lastErr    error
		lastOutput string
	)

	for time.Since(startTime) < timeout {
		cmd := "journalctl -l -u install-rhcos-and-restore-seed.service | tail -2"

		output, err := SSHExecOnProvisioningHost(parentCtx, host, user, sshKeyPath, cmd)
		lastOutput = output

		if err != nil {
			lastErr = err
		} else {
			lastErr = nil

			if strings.Contains(output, "Finished SNO Image-based Installation") ||
				strings.Contains(output, "Finished SNO Image Based Installation") {
				klog.Infof("Preinstall completed successfully on %s", host)

				return nil
			}
		}

		klog.V(5).Infof("Preinstall not yet complete, waiting %v...", pollInterval)
		time.Sleep(pollInterval)
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

const (
	deleteResourcePollInterval = 5 * time.Second
	deleteResourceWaitTimeout  = 5 * time.Minute
)

// DeletePreinstallBMHResources deletes the BareMetalHost and BMC secret if they exist (Ansible cleanup).
func DeletePreinstallBMHResources(apiClient *clients.Settings, bmhName, bmhNamespace, bmcSecretName string) error {
	if apiClient == nil {
		return fmt.Errorf("api client is nil")
	}

	ctx := context.Background()

	err := apiClient.AttachScheme(bmhv1alpha1.AddToScheme)
	if err != nil {
		return fmt.Errorf("attach bmh scheme: %w", err)
	}

	bmhObj := &bmhv1alpha1.BareMetalHost{}

	err = apiClient.Get(ctx, goclient.ObjectKey{Namespace: bmhNamespace, Name: bmhName}, bmhObj)
	if err == nil {
		err = apiClient.Delete(ctx, bmhObj)
		if err != nil && !k8serrors.IsNotFound(err) {
			return fmt.Errorf("delete bmh: %w", err)
		}

		if err := waitBareMetalHostDeleted(
			ctx, apiClient, bmhNamespace, bmhName, deleteResourceWaitTimeout, deleteResourcePollInterval); err != nil {
			return err
		}
	} else if !k8serrors.IsNotFound(err) {
		return fmt.Errorf("get bmh: %w", err)
	}

	err = apiClient.Secrets(bmhNamespace).Delete(ctx, bmcSecretName, metav1.DeleteOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("delete bmc secret: %w", err)
	}

	if err := waitSecretDeleted(
		ctx, apiClient, bmhNamespace, bmcSecretName, deleteResourceWaitTimeout, deleteResourcePollInterval); err != nil {
		return err
	}

	return nil
}

func waitBareMetalHostDeleted(
	ctx context.Context,
	apiClient *clients.Settings,
	namespace, name string,
	timeout, pollInterval time.Duration,
) error {
	key := goclient.ObjectKey{Namespace: namespace, Name: name}
	obj := &bmhv1alpha1.BareMetalHost{}

	start := time.Now()
	for time.Since(start) < timeout {
		err := apiClient.Get(ctx, key, obj)
		if k8serrors.IsNotFound(err) {
			return nil
		}

		if err != nil {
			return fmt.Errorf(
				"BareMetalHost %s/%s: Get while waiting for deletion after Delete: %w",
				namespace, name, err)
		}

		time.Sleep(pollInterval)
	}

	return fmt.Errorf(
		"timed out after %v waiting for BareMetalHost %s/%s to be removed (still present; finalizers may be pending)",
		timeout, namespace, name)
}

func waitSecretDeleted(
	ctx context.Context,
	apiClient *clients.Settings,
	namespace, name string,
	timeout, pollInterval time.Duration,
) error {
	start := time.Now()
	for time.Since(start) < timeout {
		_, err := apiClient.Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
		if k8serrors.IsNotFound(err) {
			return nil
		}

		if err != nil {
			return fmt.Errorf(
				"secret %s/%s: get while waiting for deletion after delete: %w",
				namespace, name, err)
		}

		time.Sleep(pollInterval)
	}

	return fmt.Errorf(
		"timed out after %v waiting for Secret %s/%s to be removed (still present; finalizers may be pending)",
		timeout, namespace, name)
}
