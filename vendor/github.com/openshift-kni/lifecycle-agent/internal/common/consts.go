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

package common

import (
	"math"
	"path/filepath"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Common constants mainly used by packages in lca-cli
const (
	VarFolder       = "/var"
	BackupDir       = "/var/tmp/backup"
	BackupCertsDir  = "/var/tmp/backupCertsDir"
	BackupChecksDir = "/var/tmp/checks"

	// Workload partitioning annotation key and value
	WorkloadManagementAnnotationKey   = "target.workload.openshift.io/management"
	WorkloadManagementAnnotationValue = `{"effect": "PreferredDuringScheduling"}`

	// ImageRegistryAuthFile is the pull secret. Written by the machine-config-operator
	ImageRegistryAuthFile = "/var/lib/kubelet/config.json"
	KubeconfigFile        = "/etc/kubernetes/static-pod-resources/kube-apiserver-certs/secrets/node-kubeconfigs/lb-ext.kubeconfig"

	RecertImageEnvKey      = "RELATED_IMAGE_RECERT_IMAGE"
	DefaultRecertImage     = "quay.io/edge-infrastructure/recert:v0"
	EtcdStaticPodFile      = "/etc/kubernetes/manifests/etcd-pod.yaml"
	EtcdStaticPodContainer = "etcd"
	EtcdDefaultEndpoint    = "localhost:2379"

	OvnIcEtcFolder = "/var/lib/ovn-ic/etc"
	OvnNodeCerts   = OvnIcEtcFolder + "/ovnkube-node-certs"

	MultusCerts   = "/etc/cni/multus/certs"
	ChronyConfig  = "/etc/chrony.conf"
	OvsConfDb     = "/etc/openvswitch/conf.db"
	OvsConfDbLock = "/etc/openvswitch/.conf.db.~lock~"

	SSHServerKeysDirectory = "/etc/ssh"

	MCDCurrentConfig = "/etc/machine-config-daemon/currentconfig"

	InstallationConfigurationFilesDir = "/usr/local/installation_configuration_files"
	IPConfigurationFilesDir           = "/usr/local/ip_configuration_files"
	OptOpenshift                      = "/opt/openshift"
	NmstateConfigDir                  = "/etc/nmstate/openshift"
	NmstateConfigFileName             = "nmstate.yaml"
	SeedDataDir                       = "/var/seed_data"
	KubeconfigCryptoDir               = "kubeconfig-crypto"
	ClusterConfigDir                  = "cluster-configuration"
	ContainersListFileName            = "containers.list"
	SeedClusterInfoFileName           = "manifest.json"
	SeedReconfigurationFileName       = "manifest.json"
	ManifestsDir                      = "manifests"
	ExtraManifestsDir                 = "extra-manifests"
	EtcdContainerName                 = "recert_etcd"
	LvmConfigDir                      = "lvm-configuration"
	LvmDevicesPath                    = "/etc/lvm/devices/system.devices"
	CABundleFilePath                  = "/etc/pki/ca-trust/source/anchors/openshift-config-user-ca-bundle.crt"

	LCAConfigDir               = "/var/lib/lca"
	LCAWorkspaceDir            = LCAConfigDir + "/workspace"
	IPConfigPrePivotFlagsFile  = LCAWorkspaceDir + "/ip-config-pre-pivot.json"
	IPConfigPostPivotFlagsFile = LCAWorkspaceDir + "/ip-config-post-pivot.json"
	IPConfigPullSecretFile     = LCAWorkspaceDir + "/recert-pull-secret.json"
	IPCAutoRollbackConfigFile  = LCAWorkspaceDir + "/ip-config-autorollback-config.json"
	RecertDataFile             = LCAWorkspaceDir + "/recert-data.json"

	IBUAutoRollbackConfigFile                       = LCAConfigDir + "/autorollback_config.json"
	IBUAutoRollbackInitMonitorTimeoutDefaultSeconds = 1800 // 30 minutes
	IBUInitMonitorService                           = "lca-init-monitor.service"
	IBUInitMonitorServiceFile                       = "/etc/systemd/system/" + IBUInitMonitorService
	IPCAutoRollbackInitMonitorTimeoutDefaultSeconds = 1800 // 30 minutes
	IPCInitMonitorService                           = "lca-init-monitor.service"
	IPConfigurationService                          = "ip-configuration.service"
	IPCFilePath                                     = LCAConfigDir + "/ipc.json"
	// InitMonitorModeFile configures which mode the init-monitor should operate in ("ibu" or "ipconfig")
	InitMonitorModeFile = LCAWorkspaceDir + "/initmonitor_mode"
	// AutoRollbackOnFailurePostRebootConfigAnnotation configure automatic rollback when the reconfiguration of the cluster fails upon the first reboot.
	// Only acceptable value is AutoRollbackDisableValue. Any other value is treated as "Enabled".
	AutoRollbackOnFailurePostRebootConfigAnnotation = "auto-rollback-on-failure.lca.openshift.io/post-reboot-config"
	// AutoRollbackOnFailureUpgradeCompletionAnnotation configure automatic rollback after the Lifecycle Agent reports a failed upgrade upon completion.
	// Only acceptable value is AutoRollbackDisableValue. Any other value is treated as "Enabled".
	AutoRollbackOnFailureUpgradeCompletionAnnotation = "auto-rollback-on-failure.lca.openshift.io/upgrade-completion"
	// AutoRollbackOnFailureInitMonitorAnnotation configure automatic rollback LCA Init Monitor watchdog, which triggers auto-rollback if timeout occurs before upgrade completion
	// Only acceptable value is AutoRollbackDisableValue. Any other value is treated as "Enabled".
	AutoRollbackOnFailureInitMonitorAnnotation = "auto-rollback-on-failure.lca.openshift.io/init-monitor"
	// AutoRollbackOnFailureIPConfigRunAnnotation configure automatic rollback when the IP config run fails.
	// Only acceptable value is AutoRollbackDisableValue. Any other value is treated as "Enabled".
	AutoRollbackOnFailureIPConfigRunAnnotation = "auto-rollback-on-failure.lca.openshift.io/ip-config-run"
	// AutoRollbackDisableValue value that decides if rollback is disabled
	AutoRollbackDisableValue = "Disabled"
	// ContainerStorageUsageThresholdPercentAnnotation overrides default /var/lib/containers disk usage threshold for image cleanup
	ContainerStorageUsageThresholdPercentAnnotation = "image-cleanup.lca.openshift.io/disk-usage-threshold-percent"
	// ContainerStorageUsageThresholdPercentDefault is the default /var/lib/containers disk usage threshold for image cleanu
	ContainerStorageUsageThresholdPercentDefault = 50
	// ImageCleanupOnPrepAnnotation configures automatic image cleanup during IBU Prep, if disk usage threshold is exceeded
	// Only acceptable value is ImageCleanupDisabledValue. Any other value is treated as "Enabled".
	ImageCleanupOnPrepAnnotation = "image-cleanup.lca.openshift.io/on-prep"
	// ImageCleanupDisabledValue value to disable image cleanup
	ImageCleanupDisabledValue = "Disabled"

	LcaNamespace = "openshift-lifecycle-agent"
	Host         = "/host"

	CsvDeploymentName      = "cluster-version-operator"
	CsvDeploymentNamespace = "openshift-cluster-version"
	// InstallConfigCM cm name
	InstallConfigCM = "cluster-config-v1"
	// InstallConfigCMNamespace cm namespace
	InstallConfigCMNamespace = "kube-system"
	// InstallConfigCMNamespace data key
	InstallConfigCMInstallConfigDataKey = "install-config"
	OpenshiftInfraCRName                = "cluster"
	OpenshiftProxyCRName                = "cluster"

	// Env var to configure auto rollback for post-reboot config failure
	IBUPostRebootConfigAutoRollbackOnFailureEnv = "LCA_IBU_AUTO_ROLLBACK_ON_CONFIG_FAILURE"

	// Bump this every time the seed format changes in a backwards incompatible way
	SeedFormatVersion  = 4
	SeedFormatOCILabel = "com.openshift.lifecycle-agent.seed_format_version"

	SeedClusterInfoOCILabel = "com.openshift.lifecycle-agent.seed_cluster_info"

	PullSecretName           = "pull-secret"
	PullSecretEmptyData      = "{\"auths\":{\"registry.connect.redhat.com\":{\"username\":\"empty\",\"password\":\"empty\",\"auth\":\"ZW1wdHk6ZW1wdHk=\",\"email\":\"\"}}}" //nolint:gosec
	OpenshiftConfigNamespace = "openshift-config"

	NMConnectionFolder = "/etc/NetworkManager/system-connections"
	NetworkDir         = "network-configuration"

	CaBundleDataKey                  = "ca-bundle.crt"
	ClusterAdditionalTrustBundleName = "user-ca-bundle"

	IBIWorkspace = "var/tmp"

	ContainerStoragePath = "/var/lib/containers"

	DnsmasqOverrides = "/etc/default/sno_dnsmasq_configuration_overrides"

	IPConfigName = "ipconfig"

	// DNS family names
	IPv4FamilyName = "ipv4"
	IPv6FamilyName = "ipv6"
	DNSFamilyNone  = "none"

	OvsBridgeInterfaceType = "ovs-bridge"
	OvsInterfaceType       = "ovs-interface"
	OvsBridgeExternalName  = "br-ex"
)

// DNSMasq-related constants
const (
	// MachineConfig name that carries dnsmasq configuration
	DnsmasqMachineConfigName = "50-master-dnsmasq-configuration"
	// Path where the single-node filter configuration is placed
	DnsmasqFilterTargetPath = "/etc/dnsmasq.d/single-node-filter.conf"
	// Filter directives based on the IP family being filtered out:
	// - filter-A removes A records (IPv4)
	// - filter-AAAA removes AAAA records (IPv6)
	DnsmasqFilterOutIPv6 = "filter-AAAA"
	DnsmasqFilterOutIPv4 = "filter-A"
	// Header written into the dnsmasq filter file to indicate it is managed by IP configuration.
	// dnsmasq treats lines beginning with '#' as comments.
	DnsmasqFilterManagedByIPConfigHeader = "# Generated by IP configuration flow (lifecycle-agent). Do not edit.\n"
	// Data URL template used for embedding file contents in ignition (URL-encoded body)
	DataURLBase64Template = "data:text/plain;charset=utf-8,%s"
	// Ignition version used when creating/updating configs
	IgnitionVersion32 = "3.2.0"
	// Environment key written to dnsmasq override file for the IP override
	DnsmasqOverrideEnvKeyIP = "SNO_DNSMASQ_IP_OVERRIDE"
	// Common file permission modes
	FileMode0644 = 0o644
	FileMode0600 = 0o600

	LcaCliBinaryHostPath = "/var/usrlocal/bin/lca-cli"
)

// Annotation names and values related to extra manifest
const (
	ApplyWaveAnn        = "lca.openshift.io/apply-wave"
	defaultApplyWave    = math.MaxInt32 // 2147483647, an enough large number
	ApplyTypeAnnotation = "lca.openshift.io/apply-type"
	ApplyTypeReplace    = "replace" // default if annotation doesn't exist
	ApplyTypeMerge      = "merge"
)

var (
	BackupGvk  = schema.GroupVersionKind{Group: "velero.io", Kind: "Backup", Version: "v1"}
	RestoreGvk = schema.GroupVersionKind{Group: "velero.io", Kind: "Restore", Version: "v1"}
)

var (
	ContainersListFilePath      = filepath.Join(IBIWorkspace, ContainersListFileName)
	IBIPostDeploymentScriptPath = filepath.Join(IBIWorkspace, "post.sh")
	IBIPullSecretFilePath       = filepath.Join(IBIWorkspace, "pull-secret.json")
	IBISeedInfoFilePath         = filepath.Join(IBIWorkspace, SeedClusterInfoFileName)
)

// CertPrefixes is the list of certificate prefixes to be backed up
// before creating the seed image
var CertPrefixes = []string{
	"loadbalancer-serving-signer",
	"localhost-serving-signer",
	"service-network-serving-signer",
}

var TarOpts = []string{"--selinux", "--xattrs", "--xattrs-include=*", "--acls"}
