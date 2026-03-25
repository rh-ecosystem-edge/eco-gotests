package helpers

import (
	"encoding/json"
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/mco"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	"k8s.io/klog/v2"
)

// GetPullSecretFromHub retrieves the pull secret from the hub cluster.
// It fetches the secret from openshift-config/pull-secret and decodes the .dockerconfigjson data.
func GetPullSecretFromHub(apiClient *clients.Settings) (string, error) {
	klog.Infof("Fetching pull secret from hub cluster")

	secretBuilder, err := secret.Pull(apiClient, "pull-secret", "openshift-config")
	if err != nil {
		return "", fmt.Errorf("failed to pull secret: %w", err)
	}

	if !secretBuilder.Exists() {
		return "", fmt.Errorf("pull-secret not found in openshift-config namespace")
	}

	dockerConfigJSON, ok := secretBuilder.Object.Data[".dockerconfigjson"]
	if !ok {
		return "", fmt.Errorf(".dockerconfigjson key not found in pull-secret")
	}

	var pullSecretJSON interface{}

	err = json.Unmarshal(dockerConfigJSON, &pullSecretJSON)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal pull secret JSON: %w", err)
	}

	compactJSON, err := json.Marshal(pullSecretJSON)
	if err != nil {
		return "", fmt.Errorf("failed to marshal pull secret: %w", err)
	}

	klog.Infof("Successfully retrieved pull secret from hub cluster")

	return string(compactJSON), nil
}

// GetSSHKeyFromHub retrieves the SSH public key from the hub cluster.
// It fetches the MachineConfig 99-master-ssh and extracts the first SSH authorized key.
func GetSSHKeyFromHub(apiClient *clients.Settings) (string, error) {
	klog.Infof("Fetching SSH key from hub cluster")

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

	var configData map[string]interface{}

	err = json.Unmarshal(raw, &configData)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal MachineConfig config: %w", err)
	}

	passwd, okPasswd := configData["passwd"].(map[string]interface{})
	if !okPasswd {
		return "", fmt.Errorf("failed to extract passwd from MachineConfig config")
	}

	users, okUsers := passwd["users"].([]interface{})
	if !okUsers || len(users) == 0 {
		return "", fmt.Errorf("failed to extract users from MachineConfig passwd")
	}

	firstUser, okUser := users[0].(map[string]interface{})
	if !okUser {
		return "", fmt.Errorf("failed to extract first user from MachineConfig users")
	}

	sshKeys, okKeys := firstUser["sshAuthorizedKeys"].([]interface{})
	if !okKeys || len(sshKeys) == 0 {
		return "", fmt.Errorf("failed to extract sshAuthorizedKeys from MachineConfig user")
	}

	sshKey, okKey := sshKeys[0].(string)
	if !okKey {
		return "", fmt.Errorf("failed to extract SSH key string")
	}

	klog.Infof("Successfully retrieved SSH key from hub cluster")

	return sshKey, nil
}

// GetCACertFromHub retrieves the CA certificate bundle from the hub cluster.
// It fetches the configmap from openshift-config/user-ca-bundle.
func GetCACertFromHub(apiClient *clients.Settings) (string, error) {
	klog.Infof("Fetching CA certificate from hub cluster")

	cmBuilder, err := configmap.Pull(apiClient, "user-ca-bundle", "openshift-config")
	if err != nil {
		return "", fmt.Errorf("failed to pull configmap: %w", err)
	}

	if !cmBuilder.Exists() {
		return "", fmt.Errorf("user-ca-bundle configmap not found in openshift-config namespace")
	}

	caCert, ok := cmBuilder.Object.Data["ca-bundle.crt"]
	if !ok {
		return "", fmt.Errorf("ca-bundle.crt key not found in user-ca-bundle configmap")
	}

	klog.Infof("Successfully retrieved CA certificate from hub cluster")

	return caCert, nil
}
