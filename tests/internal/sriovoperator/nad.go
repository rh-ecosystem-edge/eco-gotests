package sriovoperator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
)

// CNIType is the Multus/CNI plugin type embedded in a NetworkAttachmentDefinition config.
type CNIType string

const (
	// CNITypeSriov is the sriov CNI plugin type.
	CNITypeSriov CNIType = "sriov"
	// CNITypeOVS is the ovs CNI plugin type.
	CNITypeOVS CNIType = "ovs"

	resourceNameAnnotation = "k8s.v1.cni.cncf.io/resourceName"
	resourceNamePrefix     = "openshift.io/"
)

type nadConfigType struct {
	Type          string `json:"type"`
	Bridge        string `json:"bridge,omitempty"`
	InterfaceType string `json:"interface_type,omitempty"`
}

// WaitForNADCreation waits for NetworkAttachmentDefinition to be created.
func WaitForNADCreation(apiClient *clients.Settings, name, namespace string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(context.Background(), PollingInterval, timeout, true,
		func(ctx context.Context) (bool, error) {
			_, err := nad.Pull(apiClient, name, namespace)

			return err == nil, nil
		})
}

// WaitForNADDeletion waits for the NAD to be deleted.
// Only NotFound (or eco-goinfra's "does not exist" wrap) ends the wait; other
// Pull errors keep polling so transient API failures do not false-pass.
func WaitForNADDeletion(apiClient *clients.Settings, name, namespace string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(context.Background(), PollingInterval, timeout, true,
		func(ctx context.Context) (bool, error) {
			_, err := nad.Pull(apiClient, name, namespace)
			if err == nil {
				return false, nil
			}

			if k8serrors.IsNotFound(err) || strings.Contains(err.Error(), "does not exist") {
				return true, nil
			}

			return false, nil
		})
}

// AssertNAD verifies a NAD exists with the expected CNI type and device-plugin resource annotation.
// wantResource may be bare (e.g. "hwolresource") or fully qualified ("openshift.io/hwolresource").
// wantBridge, when non-empty, requires the NAD CNI config bridge field to match (ovs CNI / OVSNetwork).
func AssertNAD(
	apiClient *clients.Settings,
	name, namespace string,
	wantCNIType CNIType,
	wantResource string,
	wantBridge string,
) error {
	if apiClient == nil {
		return fmt.Errorf("apiClient cannot be nil")
	}

	if name == "" || namespace == "" {
		return fmt.Errorf("NAD name and namespace cannot be empty")
	}

	if wantCNIType == "" {
		return fmt.Errorf("wantCNIType cannot be empty")
	}

	if wantResource == "" {
		return fmt.Errorf("wantResource cannot be empty")
	}

	nadBuilder, err := nad.Pull(apiClient, name, namespace)
	if err != nil {
		return fmt.Errorf("failed to pull NAD %s/%s: %w", namespace, name, err)
	}

	var cfg nadConfigType
	if err := json.Unmarshal([]byte(nadBuilder.Object.Spec.Config), &cfg); err != nil {
		return fmt.Errorf("failed to parse NAD %s/%s config: %w", namespace, name, err)
	}

	if cfg.Type != string(wantCNIType) {
		return fmt.Errorf("NAD %s/%s type is %q, want %q", namespace, name, cfg.Type, wantCNIType)
	}

	if nadBuilder.Object.Annotations == nil {
		return fmt.Errorf("NAD %s/%s has nil annotations", namespace, name)
	}

	gotResource, ok := nadBuilder.Object.Annotations[resourceNameAnnotation]
	if !ok {
		return fmt.Errorf("NAD %s/%s missing annotation %s", namespace, name, resourceNameAnnotation)
	}

	if !resourceNamesMatch(gotResource, wantResource) {
		return fmt.Errorf("NAD %s/%s resource annotation is %q, want %q",
			namespace, name, gotResource, wantResource)
	}

	if wantBridge != "" && cfg.Bridge != wantBridge {
		return fmt.Errorf("NAD %s/%s bridge is %q, want %q",
			namespace, name, cfg.Bridge, wantBridge)
	}

	return nil
}

// AssertNADInterfaceType checks the representor interface type passed to ovs-cni.
func AssertNADInterfaceType(apiClient *clients.Settings, name, namespace, want string) error {
	if apiClient == nil || name == "" || namespace == "" || want == "" {
		return fmt.Errorf("API client, NAD name, namespace, and interface type are required")
	}

	obj, err := nad.Pull(apiClient, name, namespace)
	if err != nil {
		return fmt.Errorf("failed to pull NAD %s/%s: %w", namespace, name, err)
	}

	var cfg nadConfigType
	if err := json.Unmarshal([]byte(obj.Object.Spec.Config), &cfg); err != nil {
		return fmt.Errorf("failed to parse NAD %s/%s: %w", namespace, name, err)
	}

	if cfg.InterfaceType != want {
		return fmt.Errorf("NAD %s/%s interface_type is %q, want %q", namespace, name, cfg.InterfaceType, want)
	}

	return nil
}

func resourceNamesMatch(got, want string) bool {
	return normalizeResourceName(got) == normalizeResourceName(want)
}

func normalizeResourceName(name string) string {
	if strings.HasPrefix(name, resourceNamePrefix) {
		return name
	}

	return resourceNamePrefix + name
}
