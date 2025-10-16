/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"errors"
	"fmt"
	"net"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	"golang.org/x/crypto/ssh"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var icilog = logf.Log.WithName("imageclusterinstall-resource")

func (r *ImageClusterInstall) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

var _ webhook.Validator = &ImageClusterInstall{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *ImageClusterInstall) ValidateCreate() (admission.Warnings, error) {
	icilog.Info("validate create", "name", r.Name)
	if err := r.validate(); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *ImageClusterInstall) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	icilog.Info("validate update", "name", r.Name)

	if err := r.validate(); err != nil {
		return nil, err
	}

	oldClusterInstall, ok := old.(*ImageClusterInstall)
	if !ok {
		return nil, fmt.Errorf("old object is not an ImageClusterInstall")
	}

	// Allow update if it's not the spec
	if !isSpecUpdate(oldClusterInstall, r) {
		return nil, nil
	}
	// block update if the installation started
	if installationStarted(oldClusterInstall) {
		return nil, fmt.Errorf("cannot update ImageClusterInstall when the configImage is ready")
	}
	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *ImageClusterInstall) ValidateDelete() (admission.Warnings, error) {
	return nil, nil
}

func (r *ImageClusterInstall) validate() error {
	if err := isValidSSHPublicKey(r.Spec.SSHKey); err != nil {
		return fmt.Errorf("invalid ssh key: %v", err)
	}
	if err := isValidHostname(r.Spec.Hostname); err != nil {
		return fmt.Errorf("invalid hostname: %w", err)
	}

	if err := isValidMachineNetworks(r.Spec.MachineNetwork, r.Spec.MachineNetworks); err != nil {
		return fmt.Errorf("invalid machine network: %w", err)
	}

	effectiveMachineNetworks := getEffectiveMachineNetworks(r.Spec.MachineNetwork, r.Spec.MachineNetworks, icilog)
	if err := isValidProxy(r.Spec.Proxy, effectiveMachineNetworks); err != nil {
		return fmt.Errorf("invalid proxy: %w", err)
	}
	return nil
}

func installationStarted(r *ImageClusterInstall) bool {
	// if the BareMetalHostRef is set on the status than the installation has started.
	return r.Status.BareMetalHostRef != nil
}

func isSpecUpdate(oldClusterInstall *ImageClusterInstall, newClusterInstall *ImageClusterInstall) bool {
	oldSpec := oldClusterInstall.Spec.DeepCopy()
	newSpec := newClusterInstall.Spec.DeepCopy()
	oldSpec.ClusterMetadata = nil
	newSpec.ClusterMetadata = nil

	return !reflect.DeepEqual(oldSpec, newSpec)
}

func isValidSSHPublicKey(pubKeyString string) error {
	if pubKeyString == "" {
		return nil
	}
	// Trim any leading/trailing whitespaces
	pubKeyString = strings.TrimSpace(pubKeyString)
	pubKeyBytes := []byte(pubKeyString)
	_, _, _, _, err := ssh.ParseAuthorizedKey(pubKeyBytes)
	if err != nil {
		return fmt.Errorf("error parsing public key: %w", err)
	}
	// If parsing is successful, the key is valid
	return nil
}

func isValidHostname(hostname string) error {
	if hostname == "" {
		return nil
	}
	errs := validation.IsDNS1123Subdomain(hostname)
	if len(errs) != 0 {
		return errors.New(strings.Join(errs, ";"))
	}
	return nil
}

// isValidMachineNetworks validates both legacy MachineNetwork and new MachineNetworks fields
func isValidMachineNetworks(legacyMachineNetwork string, machineNetworks []MachineNetworkEntry) error {
	// If both are specified, MachineNetworks takes precedence but we still validate both
	if legacyMachineNetwork != "" {
		if err := isValidNetworkCidr(legacyMachineNetwork); err != nil {
			return err
		}
	}

	for i, network := range machineNetworks {
		if err := isValidNetworkCidr(network.CIDR); err != nil {
			return fmt.Errorf("machine network %d: %w", i, err)
		}
	}

	return nil
}

// getEffectiveMachineNetworks returns the effective machine networks for other validations
func getEffectiveMachineNetworks(legacyMachineNetwork string, machineNetworks []MachineNetworkEntry, log logr.Logger) []string {
	if len(machineNetworks) > 0 {
		networks := make([]string, len(machineNetworks))
		for i, network := range machineNetworks {
			networks[i] = network.CIDR
		}
		return networks
	}

	if legacyMachineNetwork != "" {
		log.Info("Using legacy MachineNetwork field", "legacyMachineNetwork", legacyMachineNetwork)
		return []string{legacyMachineNetwork}
	}

	return nil
}

func isValidNetworkCidr(networkCIDR string) error {
	if networkCIDR == "" {
		return fmt.Errorf("network CIDR is empty")
	}

	_, _, err := net.ParseCIDR(networkCIDR)
	if err != nil {
		return fmt.Errorf("error parsing network, check that it is valid CIDR: %w", err)
	}
	return nil
}

func isValidProxy(proxy *Proxy, machineNetworks []string) error {
	if proxy != nil && len(machineNetworks) == 0 {
		return errors.New("machine network must be set when proxy is defined")
	}
	return nil
}
