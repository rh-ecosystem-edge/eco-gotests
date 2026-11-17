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

	ibuv1 "github.com/openshift-kni/lifecycle-agent/api/imagebasedupgrade/v1"
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
	CertManagerCryptoDir              = "certmanager-crypto"
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

	// DefaultOperatorNamespace is used when MY_POD_NAMESPACE is unset (e.g. local dev).
	// It aliases the exported IBU API constant so both stay in sync.
	DefaultOperatorNamespace = ibuv1.DefaultNamespace
	// OperatorDeploymentName is the OLM-managed operator Deployment name.
	OperatorDeploymentName = "lifecycle-agent-controller-manager"
	// OperatorNamespaceEnvVar is set on the operator Deployment via downward API.
	OperatorNamespaceEnvVar = "MY_POD_NAMESPACE"
	Host                    = "/host"

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

	// SeedFormatVersion and SeedFormatOCILabel are re-exported from the public API
	// package, which is the single source of truth (a hub consumer imports them
	// from there). Bump SeedFormatVersion in api/imagebasedupgrade/v1 every time
	// the seed format changes in a backwards incompatible way.
	SeedFormatVersion  = ibuv1.SeedFormatVersion
	SeedFormatOCILabel = ibuv1.SeedFormatOCILabel

	SeedClusterInfoOCILabel = "com.openshift.lifecycle-agent.seed_cluster_info"

	PullSecretName      = "pull-secret"
	PullSecretEmptyData = "{\"auths\":{\"registry.connect.redhat.com\":{\"username\":\"empty\",\"password\":\"empty\",\"auth\":\"ZW1wdHk6ZW1wdHk=\",\"email\":\"\"}}}" //nolint:gosec
	// IBIEmptyPullSecret is a truly empty authfile used for the disconnected IBI prepare
	// direct-import path, which needs no credentials. It is deliberately distinct from
	// PullSecretEmptyData, which embeds a registry.connect.redhat.com credential.
	IBIEmptyPullSecret       = "{\"auths\":{}}" //nolint:gosec
	OpenshiftConfigNamespace = "openshift-config"

	NMConnectionFolder = "/etc/NetworkManager/system-connections"
	NetworkDir         = "network-configuration"

	CaBundleDataKey                  = "ca-bundle.crt"
	ClusterAdditionalTrustBundleName = "user-ca-bundle"

	IBIWorkspace = "var/tmp"

	// LocalImagesFSLabel is the filesystem label of the USB "p2" partition that
	// carries the on-media OCI image layout for a disconnected IBI prepare. The
	// prepare phase discovers the partition by this label and mounts it read-only
	// at the configured LocalImagesPath.
	LocalImagesFSLabel = "ibi-images"

	// LocalImagesMountPath is the on-node mount point create-usb records as the
	// embedded IBIPrepareConfig.LocalImagesPath and that the prepare phase mounts the
	// p2 (ibi-images) partition at. It is the cross-story contract between create-usb's
	// embedded-config rendering and the prepare mount stages (CNF-27161/CNF-27162);
	// both sides must use this constant so the path never diverges. /mnt resolves
	// to /var/mnt in the RHCOS live environment; installing the target root at
	// /mnt would hide any USB mount beneath it. Keep this mount under /run.
	LocalImagesMountPath = "/run/ibi-images"

	// LocalImagesLCABinaryPath is where create-usb stages the exact shipping lca-cli binary
	// on the p2 (ibi-images) partition, mounted at LocalImagesMountPath on the node. The
	// live-ISO embedded ignition's lca-ibi.service execs the binary from this path; the
	// media-assembly wiring (CNF-27171) copies the extracted binary here. The binary is
	// staged on p2 rather than embedded in the ISO ignition because it is far larger
	// (~90 MB uncompressed) than the fixed RHCOS ISO ignition embed area (~256 KB), which
	// the embed would overflow. Kept next to LocalImagesMountPath so the unit's exec path
	// and the staged file never diverge.
	LocalImagesLCABinaryPath = LocalImagesMountPath + "/lca-cli"

	// LocalImagesStorePath is where create-usb stages the read-only containers/storage durability
	// store on the p2 (ibi-images) partition for self-contained media, mounted at
	// LocalImagesMountPath on the node. It is the path the prepare/persist stages
	// (CNF-27165) read the store from once p2 is mounted; both sides must use this constant so the
	// path never diverges.
	LocalImagesStorePath = LocalImagesMountPath + "/" + LocalImagesStoreDirName

	// LocalImagesStoreDirName is the directory name, relative to the p2 (ibi-images) mount point,
	// of the read-only containers/storage durability store create-usb stages for self-contained
	// media. The prepare/persist stage (CNF-27165) reads the store from this subdirectory of the
	// configured LocalImagesPath.
	LocalImagesStoreDirName = "store"

	// IBIStatusFSLabel is the filesystem label of the writable USB "p4" partition
	// that records per-phase machine-readable results for a USB-based IBI prepare.
	// The prepare phase discovers the partition by this label, mounts it read-write,
	// writes the result file at its root, and writes a diagnostic log bundle under
	// the logs directory. The partition itself (empty, writable, ext4, labelled) is
	// created by create-usb (CNF-28670); this story consumes it.
	IBIStatusFSLabel = "ibi-status"

	// IBIStatusMountPath is the mount point for the writable ibi-status (p4)
	// partition during a disconnected IBI prepare.
	// /mnt resolves to /var/mnt on the live ISO. The target root is mounted there
	// during preparation, so the result partition must live outside that tree.
	IBIStatusMountPath = "/run/ibi-status"

	// PrepareResultFileName is the machine-readable per-phase result file written at
	// the root of the ibi-status (p4) partition on both the success and failure paths.
	PrepareResultFileName = "prepare.json"

	// ReconfigureResultFileName is the machine-readable per-phase result file written at
	// the root of the ibi-status (p4) partition by the post-pivot reconfigure phase. The
	// pre-recert clock gate writes it on the failure path (an implausible clock halts the
	// node before recert runs); the reconfigure result-writer (CNF-27173) owns the final
	// success record.
	ReconfigureResultFileName = "reconfigure.json"

	// IBIStatusLogsDir is the directory, relative to the ibi-status (p4) root, under
	// which the compact diagnostic log bundle is written.
	IBIStatusLogsDir = "logs"

	ContainerStoragePath = "/var/lib/containers"

	// RODurabilityStoreDirName is the directory name under /var/lib where self-contained prepare
	// persists the read-only store copied from the media. Keep it outside ContainerStoragePath:
	// /var/lib/containers may be a separate mount that would hide a store copied into the stateroot.
	RODurabilityStoreDirName = "lca-ro-store"

	// RODurabilityStorePath is the post-pivot absolute path of the persisted read-only additional
	// image store. It is the value the CRI-O drop-in and LCA Podman command register (not the
	// prepare-time /mnt path, not the USB mount), so CRI-O finds the store after pivot.
	RODurabilityStorePath = "/var/lib/" + RODurabilityStoreDirName

	// CrioConfigDropInDir is CRI-O's drop-in config directory. OpenShift and customers already
	// use this directory for CRI-O-specific configuration, and unlike the older host Podman
	// storage.conf.d loader, CRI-O reads it on the affected RHCOS build.
	CrioConfigDropInDir = "/etc/crio/crio.conf.d"

	// RODurabilityCrioDropInName is the CRI-O drop-in filename the self-contained prepare
	// writes to register the persisted read-only additional image store.
	RODurabilityCrioDropInName = "99-lca-ro-store.conf"

	// RODurabilityCrioDropInPath is the absolute path of the CRI-O drop-in that registers
	// the persisted read-only additional image store.
	RODurabilityCrioDropInPath = CrioConfigDropInDir + "/" + RODurabilityCrioDropInName

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
