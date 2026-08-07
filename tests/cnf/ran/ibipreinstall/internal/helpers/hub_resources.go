package helpers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/idms"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/mco"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ibipreinstall/internal/tsparams"
)

// GetPullSecretFromHub retrieves the pull secret from the hub cluster.
func GetPullSecretFromHub(apiClient *clients.Settings) (string, error) {
	klog.V(tsparams.LogLevel).Infof("Fetching pull secret from hub cluster")

	secretBuilder, err := secret.Pull(apiClient, "pull-secret", "openshift-config")
	if err != nil {
		return "", fmt.Errorf("failed to pull secret: %w", err)
	}

	dockerConfigJSON, ok := secretBuilder.Object.Data[".dockerconfigjson"]
	if !ok {
		return "", fmt.Errorf(".dockerconfigjson key not found in pull-secret")
	}

	var pullSecretJSON any

	err = json.Unmarshal(dockerConfigJSON, &pullSecretJSON)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal pull secret JSON: %w", err)
	}

	compactJSON, err := json.Marshal(pullSecretJSON)
	if err != nil {
		return "", fmt.Errorf("failed to marshal pull secret: %w", err)
	}

	klog.V(tsparams.LogLevel).Infof("Successfully retrieved pull secret from hub cluster")

	return string(compactJSON), nil
}

// GetSSHKeyFromHub retrieves the SSH public key from the hub cluster via MachineConfig 99-master-ssh.
func GetSSHKeyFromHub(apiClient *clients.Settings) (string, error) {
	klog.V(tsparams.LogLevel).Infof("Fetching SSH key from hub cluster")

	mcBuilder, err := mco.PullMachineConfig(apiClient, "99-master-ssh")
	if err != nil {
		return "", fmt.Errorf("failed to pull MachineConfig 99-master-ssh: %w", err)
	}

	if mcBuilder == nil || mcBuilder.Object == nil {
		return "", fmt.Errorf("MachineConfig 99-master-ssh not found")
	}

	raw := mcBuilder.Object.Spec.Config.Raw
	if len(raw) == 0 {
		return "", fmt.Errorf("MachineConfig 99-master-ssh has missing Spec.Config.Raw")
	}

	var configData map[string]any

	err = json.Unmarshal(raw, &configData)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal MachineConfig config: %w", err)
	}

	passwd, okPasswd := configData["passwd"].(map[string]any)
	if !okPasswd {
		return "", fmt.Errorf("failed to extract passwd from MachineConfig config")
	}

	users, okUsers := passwd["users"].([]any)
	if !okUsers || len(users) == 0 {
		return "", fmt.Errorf("failed to extract users from MachineConfig passwd")
	}

	firstUser, okUser := users[0].(map[string]any)
	if !okUser {
		return "", fmt.Errorf("failed to extract first user from MachineConfig users")
	}

	sshKeys, okKeys := firstUser["sshAuthorizedKeys"].([]any)
	if !okKeys || len(sshKeys) == 0 {
		return "", fmt.Errorf("failed to extract sshAuthorizedKeys from MachineConfig user")
	}

	sshKey, okKey := sshKeys[0].(string)
	if !okKey {
		return "", fmt.Errorf("failed to extract SSH key string")
	}

	klog.V(tsparams.LogLevel).Infof("Successfully retrieved SSH key from hub cluster")

	return sshKey, nil
}

// GetCACertFromHub retrieves the CA certificate bundle from the hub cluster.
func GetCACertFromHub(apiClient *clients.Settings) (string, error) {
	klog.V(tsparams.LogLevel).Infof("Fetching CA certificate from hub cluster")

	cmBuilder, err := configmap.Pull(apiClient, "user-ca-bundle", "openshift-config")
	if err != nil {
		return "", fmt.Errorf("failed to pull configmap: %w", err)
	}

	caCert, ok := cmBuilder.Object.Data["ca-bundle.crt"]
	if !ok {
		return "", fmt.Errorf("ca-bundle.crt key not found in user-ca-bundle configmap")
	}

	klog.V(tsparams.LogLevel).Infof("Successfully retrieved CA certificate from hub cluster")

	return caCert, nil
}

const (
	mceSourcePrefix = "registry.redhat.io/multicluster-engine"
	acmSourcePrefix = "registry.redhat.io/rhacm2"
)

// matchesSourcePrefix returns true if source equals prefix or starts with prefix + "/".
func matchesSourcePrefix(source, prefix string) bool {
	return source == prefix || strings.HasPrefix(source, prefix+"/")
}

// MCEACMMirrors holds the mirror locations for MCE and ACM discovered from hub IDMS.
type MCEACMMirrors struct {
	MCE string
	ACM string
}

// DiscoverMCEACMMirrorsFromHub queries the hub's IDMS resources to find
// the mirror locations for multicluster-engine and rhacm2 images.
func DiscoverMCEACMMirrorsFromHub(apiClient *clients.Settings) (*MCEACMMirrors, error) {
	if apiClient == nil {
		return nil, fmt.Errorf("hub api client is nil")
	}

	idmsBuilders, err := idms.ListImageDigestMirrorSets(apiClient)
	if err != nil {
		return nil, fmt.Errorf("list IDMS from hub: %w", err)
	}

	mirrors := &MCEACMMirrors{}

	for _, builder := range idmsBuilders {
		for _, idm := range builder.Object.Spec.ImageDigestMirrors {
			if len(idm.Mirrors) == 0 {
				continue
			}

			if matchesSourcePrefix(idm.Source, mceSourcePrefix) && mirrors.MCE == "" {
				mirrors.MCE = string(idm.Mirrors[0])
			}

			if matchesSourcePrefix(idm.Source, acmSourcePrefix) && mirrors.ACM == "" {
				mirrors.ACM = string(idm.Mirrors[0])
			}
		}
	}

	if mirrors.MCE == "" || mirrors.ACM == "" {
		return nil, fmt.Errorf(
			"MCE/ACM mirror locations not found in hub IDMS (mce=%q, acm=%q); "+
				"ensure hub has IDMS entries for %s and %s",
			mirrors.MCE, mirrors.ACM, mceSourcePrefix, acmSourcePrefix)
	}

	klog.V(tsparams.LogLevel).Infof("Discovered MCE/ACM mirrors from hub: MCE=%s, ACM=%s", mirrors.MCE, mirrors.ACM)

	return mirrors, nil
}
