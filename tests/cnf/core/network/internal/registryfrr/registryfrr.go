package registryfrr

import (
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"

	"github.com/golang/glog"
)

const (
	// FRRImage is the FRR container image.
	FRRImage = "quay.io/frrouting/frr:8.5.3"
	// TestContainerImage is the network test container image.
	TestContainerImage = "quay.io/ocp-edge-qe/eco-gotests-network-client:v4.18"
	// PodName is the name of the Podman pod.
	PodName = "frrpod"
	// FRRContainerName is the name of the FRR container.
	FRRContainerName = "frr"
	// TestContainerName is the name of the test container.
	TestContainerName = "testcontainer"
	// MacvlanNetworkName is the name of the macvlan network.
	MacvlanNetworkName = "frr-macvlan"
	// BaremetalInterface is the network interface on the registry VM.
	BaremetalInterface = "baremetal"
)

// RegistryVMConfig holds the configuration for connecting to the registry VM.
type RegistryVMConfig struct {
	Host    string
	User    string
	KeyPath string
}

// ExecuteSSHCommand executes a command on the registry VM via SSH.
func (r *RegistryVMConfig) ExecuteSSHCommand(command string) (string, error) {
	sshCmd := fmt.Sprintf("ssh -i %s -o StrictHostKeyChecking=no %s@%s '%s'",
		r.KeyPath, r.User, r.Host, command)

	glog.V(100).Infof("Executing SSH command: %s", sshCmd)

	cmd := exec.Command("bash", "-c", sshCmd)
	output, err := cmd.CombinedOutput()

	if err != nil {
		glog.V(100).Infof("SSH command failed: %v, output: %s", err, string(output))

		return string(output), fmt.Errorf("SSH command failed: %w, output: %s", err, string(output))
	}

	glog.V(100).Infof("SSH command output: %s", string(output))

	return string(output), nil
}

// CreateMacvlanNetwork creates a macvlan network on the registry VM.
func (r *RegistryVMConfig) CreateMacvlanNetwork(subnet, gateway string) error {
	glog.V(100).Infof("Creating macvlan network '%s' on registry VM", MacvlanNetworkName)

	cmd := fmt.Sprintf("sudo podman network create -d macvlan --subnet=%s --gateway=%s -o parent=%s %s",
		subnet, gateway, BaremetalInterface, MacvlanNetworkName)
	output, err := r.ExecuteSSHCommand(cmd)

	if err != nil && !strings.Contains(output, "already exists") {
		return fmt.Errorf("failed to create macvlan network: %w", err)
	}

	glog.V(100).Infof("Macvlan network created successfully")

	return nil
}

// CreatePodmanPod creates a Podman pod with host networking on the registry VM (as root).
func (r *RegistryVMConfig) CreatePodmanPod() error {
	glog.V(100).Infof("Creating Podman pod '%s' with host network on registry VM as root", PodName)

	cmd := fmt.Sprintf("sudo podman pod create --name %s --network host", PodName)
	output, err := r.ExecuteSSHCommand(cmd)

	if err != nil && !strings.Contains(output, "already exists") {
		return fmt.Errorf("failed to create Podman pod: %w", err)
	}

	glog.V(100).Infof("Podman pod created successfully")

	return nil
}

// CreateFRRConfigFiles creates the FRR configuration files on the registry VM.
func (r *RegistryVMConfig) CreateFRRConfigFiles(frrConf, daemonsConf string) error {
	glog.V(100).Infof("Creating FRR configuration files on registry VM")

	// Create /etc/frr directory and ensure it's owned by root
	mkdirCmd := "sudo mkdir -p /etc/frr && sudo chown root:root /etc/frr"
	if _, err := r.ExecuteSSHCommand(mkdirCmd); err != nil {
		return fmt.Errorf("failed to create /etc/frr directory: %w", err)
	}

	// Write frr.conf using base64 encoding to safely transfer content
	frrConfEncoded := base64.StdEncoding.EncodeToString([]byte(frrConf))
	frrConfCmd := fmt.Sprintf("echo '%s' | base64 -d | sudo tee /etc/frr/frr.conf > /dev/null", frrConfEncoded)

	if _, err := r.ExecuteSSHCommand(frrConfCmd); err != nil {
		return fmt.Errorf("failed to create frr.conf: %w", err)
	}

	// Write daemons using base64 encoding
	daemonsEncoded := base64.StdEncoding.EncodeToString([]byte(daemonsConf))
	daemonsCmd := fmt.Sprintf("echo '%s' | base64 -d | sudo tee /etc/frr/daemons > /dev/null", daemonsEncoded)

	if _, err := r.ExecuteSSHCommand(daemonsCmd); err != nil {
		return fmt.Errorf("failed to create daemons file: %w", err)
	}

	// Create empty vtysh.conf (required by FRR)
	vtyshCmd := "sudo touch /etc/frr/vtysh.conf"

	if _, err := r.ExecuteSSHCommand(vtyshCmd); err != nil {
		return fmt.Errorf("failed to create vtysh.conf: %w", err)
	}

	// Set permissions and keep ownership as root (matching the document example)
	chmodCmd := "sudo chmod 644 /etc/frr/frr.conf /etc/frr/daemons /etc/frr/vtysh.conf"

	if _, err := r.ExecuteSSHCommand(chmodCmd); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	glog.V(100).Infof("FRR configuration files created successfully")

	return nil
}

// CreateFRRContainer creates the FRR container in the Podman pod with volume mount (as root).
func (r *RegistryVMConfig) CreateFRRContainer() error {
	glog.V(100).Infof("Creating FRR container in Podman pod with /etc/frr volume as root")

	cmd := fmt.Sprintf(`sudo podman run -dit --pod %s --name %s \
		--replace \
		--privileged \
		--cap-add NET_ADMIN \
		--cap-add NET_RAW \
		--cap-add SYS_ADMIN \
		--volume /etc/frr:/etc/frr:Z \
		%s`, PodName, FRRContainerName, FRRImage)

	output, err := r.ExecuteSSHCommand(cmd)

	if err != nil && !strings.Contains(output, "already exists") {
		return fmt.Errorf("failed to create FRR container: %w", err)
	}

	glog.V(100).Infof("FRR container created successfully")

	return nil
}

// CopyConfigFilesToContainer copies FRR config files from host into the running container (as root).
func (r *RegistryVMConfig) CopyConfigFilesToContainer(frrConf, daemonsConf string) error {
	glog.V(100).Infof("Copying configuration files into FRR container")

	// Write configs using base64 directly into the container
	frrConfEncoded := base64.StdEncoding.EncodeToString([]byte(frrConf))
	frrConfCmd := fmt.Sprintf("sudo podman exec %s bash -c \"echo '%s' | base64 -d > /etc/frr/frr.conf\"",
		FRRContainerName, frrConfEncoded)

	if _, err := r.ExecuteSSHCommand(frrConfCmd); err != nil {
		return fmt.Errorf("failed to copy frr.conf into container: %w", err)
	}

	daemonsEncoded := base64.StdEncoding.EncodeToString([]byte(daemonsConf))
	daemonsCmd := fmt.Sprintf("sudo podman exec %s bash -c \"echo '%s' | base64 -d > /etc/frr/daemons\"",
		FRRContainerName, daemonsEncoded)

	if _, err := r.ExecuteSSHCommand(daemonsCmd); err != nil {
		return fmt.Errorf("failed to copy daemons into container: %w", err)
	}

	// Create empty vtysh.conf
	vtyshCmd := fmt.Sprintf("sudo podman exec %s touch /etc/frr/vtysh.conf", FRRContainerName)

	if _, err := r.ExecuteSSHCommand(vtyshCmd); err != nil {
		return fmt.Errorf("failed to create vtysh.conf in container: %w", err)
	}

	// Set permissions inside container
	chmodCmd := fmt.Sprintf("sudo podman exec %s chmod 644 /etc/frr/frr.conf /etc/frr/daemons /etc/frr/vtysh.conf",
		FRRContainerName)

	if _, err := r.ExecuteSSHCommand(chmodCmd); err != nil {
		return fmt.Errorf("failed to set permissions in container: %w", err)
	}

	glog.V(100).Infof("Configuration files copied into container successfully")

	return nil
}

// RestartFRRContainer restarts the FRR container to pick up new configuration (as root).
func (r *RegistryVMConfig) RestartFRRContainer() error {
	glog.V(100).Infof("Restarting FRR container to apply configuration")

	cmd := fmt.Sprintf("sudo podman restart %s", FRRContainerName)
	if _, err := r.ExecuteSSHCommand(cmd); err != nil {
		return fmt.Errorf("failed to restart FRR container: %w", err)
	}

	glog.V(100).Infof("FRR container restarted successfully")

	return nil
}

// CreateTestContainer creates the test container in the Podman pod (as root).
func (r *RegistryVMConfig) CreateTestContainer() error {
	glog.V(100).Infof("Creating test container in Podman pod as root")

	cmd := fmt.Sprintf("sudo podman run -dit --pod %s --name %s --replace %s",
		PodName, TestContainerName, TestContainerImage)

	output, err := r.ExecuteSSHCommand(cmd)

	if err != nil && !strings.Contains(output, "already exists") {
		return fmt.Errorf("failed to create test container: %w", err)
	}

	glog.V(100).Infof("Test container created successfully")

	return nil
}

// ExecInFRRContainer executes a command in the FRR container (as root).
func (r *RegistryVMConfig) ExecInFRRContainer(command string) (string, error) {
	glog.V(100).Infof("Executing command in FRR container: %s", command)

	cmd := fmt.Sprintf("sudo podman exec %s %s", FRRContainerName, command)

	return r.ExecuteSSHCommand(cmd)
}

// ExecVtyshCommand executes a vtysh command in the FRR container.
func (r *RegistryVMConfig) ExecVtyshCommand(vtyshCmd string) (string, error) {
	glog.V(100).Infof("Executing vtysh command: %s", vtyshCmd)

	command := fmt.Sprintf("vtysh -c \"%s\"", vtyshCmd)

	return r.ExecInFRRContainer(command)
}

// StopPodmanPod stops the Podman pod (as root).
func (r *RegistryVMConfig) StopPodmanPod() error {
	glog.V(100).Infof("Stopping Podman pod '%s'", PodName)

	cmd := fmt.Sprintf("sudo podman pod stop %s", PodName)
	_, err := r.ExecuteSSHCommand(cmd)

	if err != nil {
		glog.V(100).Infof("Warning: failed to stop Podman pod: %v", err)
	}

	return nil
}

// RemovePodmanPod removes the Podman pod and all its containers (as root).
func (r *RegistryVMConfig) RemovePodmanPod() error {
	glog.V(100).Infof("Removing Podman pod '%s'", PodName)

	cmd := fmt.Sprintf("sudo podman pod rm -f %s", PodName)
	_, err := r.ExecuteSSHCommand(cmd)

	if err != nil {
		glog.V(100).Infof("Warning: failed to remove Podman pod: %v", err)
	}

	return nil
}

// VerifyPodRunning verifies that the Podman pod and containers are running.
func (r *RegistryVMConfig) VerifyPodRunning() error {
	glog.V(100).Infof("Verifying Podman pod and containers are running")

	output, err := r.ExecuteSSHCommand("sudo podman ps --pod")
	if err != nil {
		return fmt.Errorf("failed to check pod status: %w", err)
	}

	if !strings.Contains(output, PodName) {
		return fmt.Errorf("pod %s not found in running containers", PodName)
	}

	if !strings.Contains(output, FRRContainerName) {
		return fmt.Errorf("FRR container not found in running containers")
	}

	glog.V(100).Infof("Podman pod verified: %s", output)

	return nil
}

// AddSecondaryIP adds a secondary IP address to the baremetal interface on the registry VM.
func (r *RegistryVMConfig) AddSecondaryIP(ipAddress, interfaceName string) error {
	glog.V(100).Infof("Adding secondary IP %s to interface %s on registry VM", ipAddress, interfaceName)

	// Check if IP already exists
	checkCmd := fmt.Sprintf("ip addr show %s | grep %s || true", interfaceName, ipAddress)
	output, _ := r.ExecuteSSHCommand(checkCmd)

	if strings.Contains(output, ipAddress) {
		glog.V(100).Infof("Secondary IP %s already exists on interface %s", ipAddress, interfaceName)

		return nil
	}

	// Add secondary IP
	cmd := fmt.Sprintf("sudo ip addr add %s dev %s", ipAddress, interfaceName)

	if _, err := r.ExecuteSSHCommand(cmd); err != nil {
		return fmt.Errorf("failed to add secondary IP: %w", err)
	}

	glog.V(100).Infof("Secondary IP %s added successfully to interface %s", ipAddress, interfaceName)

	return nil
}

// RemoveSecondaryIP removes a secondary IP address from the baremetal interface.
func (r *RegistryVMConfig) RemoveSecondaryIP(ipAddress, interfaceName string) error {
	glog.V(100).Infof("Removing secondary IP %s from interface %s on registry VM", ipAddress, interfaceName)

	cmd := fmt.Sprintf("sudo ip addr del %s dev %s || true", ipAddress, interfaceName)

	if _, err := r.ExecuteSSHCommand(cmd); err != nil {
		glog.V(100).Infof("Warning: failed to remove secondary IP: %v", err)
	}

	return nil
}

// PullImages pulls the required container images on the registry VM (as root).
func (r *RegistryVMConfig) PullImages() error {
	glog.V(100).Infof("Pulling container images on registry VM as root")

	// Pull FRR image
	cmd := fmt.Sprintf("sudo podman pull %s", FRRImage)

	if _, err := r.ExecuteSSHCommand(cmd); err != nil {
		return fmt.Errorf("failed to pull FRR image: %w", err)
	}

	// Pull test container image
	cmd = fmt.Sprintf("sudo podman pull %s", TestContainerImage)

	if _, err := r.ExecuteSSHCommand(cmd); err != nil {
		return fmt.Errorf("failed to pull test container image: %w", err)
	}

	glog.V(100).Infof("Container images pulled successfully")

	return nil
}

// RemoveMacvlanNetwork removes the macvlan network (as root).
func (r *RegistryVMConfig) RemoveMacvlanNetwork() error {
	glog.V(100).Infof("Removing macvlan network '%s'", MacvlanNetworkName)

	cmd := fmt.Sprintf("sudo podman network rm %s || true", MacvlanNetworkName)

	if _, err := r.ExecuteSSHCommand(cmd); err != nil {
		glog.V(100).Infof("Warning: failed to remove macvlan network: %v", err)
	}

	return nil
}

// CleanupPodmanPod performs cleanup of the Podman pod.
func (r *RegistryVMConfig) CleanupPodmanPod() error {
	glog.V(100).Infof("Cleaning up Podman pod '%s'", PodName)

	// Stop and remove pod (force remove to ensure cleanup even if containers are running)
	if err := r.RemovePodmanPod(); err != nil {
		glog.V(100).Infof("Warning during cleanup: %v", err)
	}

	// Clean up FRR config files (safe - only removes specific test files, not the directory)
	cleanupCmd := "sudo rm -f /etc/frr/frr.conf /etc/frr/daemons /etc/frr/vtysh.conf"

	if _, err := r.ExecuteSSHCommand(cleanupCmd); err != nil {
		glog.V(100).Infof("Warning: failed to cleanup FRR config files: %v", err)
	}

	glog.V(100).Infof("Cleanup completed - pod removed, config files deleted, VM unharmed")

	return nil
}

// CleanupPodmanPodWithSecondaryIP performs cleanup of the Podman pod and removes secondary IP.
func (r *RegistryVMConfig) CleanupPodmanPodWithSecondaryIP(secondaryIP, interfaceName string) error {
	// First cleanup the pod
	if err := r.CleanupPodmanPod(); err != nil {
		glog.V(100).Infof("Warning during pod cleanup: %v", err)
	}

	// Remove secondary IP if provided
	if secondaryIP != "" && interfaceName != "" {
		if err := r.RemoveSecondaryIP(secondaryIP, interfaceName); err != nil {
			glog.V(100).Infof("Warning: failed to remove secondary IP: %v", err)
		}
	}

	return nil
}
