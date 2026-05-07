package utils

import (
	"path/filepath"

	"github.com/openshift-kni/lifecycle-agent/internal/common"
)

const (
	IBUWorkspacePath string = common.LCAConfigDir + "/workspace"
	IPCWorkspacePath string = common.LCAConfigDir + "/workspace"
	// IBUName defines the valid name of the CR for the controller to reconcile
	IBUName     string = "upgrade"
	IBUFilePath string = common.LCAConfigDir + "/ibu.json"

	ManualCleanupAnnotation                                    string = "lca.openshift.io/manual-cleanup-done"
	TriggerReconcileAnnotation                                 string = "lca.openshift.io/trigger-reconcile"
	RecertImageAnnotation                                      string = "lca.openshift.io/recert-image"
	RecertPullSecretAnnotation                                 string = "lca.openshift.io/recert-pull-secret" //nolint:gosec // annotation key, not credentials
	RecertCachedImageAnnotation                                string = "lca.openshift.io/recert-image-cached"
	SkipIPConfigPreConfigurationClusterHealthChecksAnnotation  string = "lca.openshift.io/ipconfig-skip-pre-configuration-cluster-health-checks"
	SkipIPConfigPostConfigurationClusterHealthChecksAnnotation string = "lca.openshift.io/ipconfig-skip-post-configuration-cluster-health-checks"

	// SeedGenName defines the valid name of the CR for the controller to reconcile
	SeedGenName          string = "seedimage"
	SeedGenSecretName    string = "seedgen"
	SeedgenWorkspacePath string = common.LCAConfigDir + "/ibu-seedgen-orch" // The LCAConfigDir folder is excluded from the var.tgz backup in seed image creation
)

var (
	SeedGenStoredCR       = filepath.Join(SeedgenWorkspacePath, "seedgen-cr.json")
	SeedGenStoredSecretCR = filepath.Join(SeedgenWorkspacePath, "seedgen-secret.json")

	StoredPullSecret = filepath.Join(SeedgenWorkspacePath, "pull-secret.json")
)

const (
	LcaCliBinaryContainerPath = "/usr/local/bin/lca-cli"
)

// IP configuration/controller related constants used across handlers to avoid magic strings
const (
	// systemd-run common property
	SystemdExitTypeCgroup = "ExitType=cgroup"

	// systemd unit names for IPConfig flows
	IPConfigPrePivotUnit  = "lca-ipconfig-pre-pivot"
	IPConfigPostPivotUnit = "lca-ipconfig-post-pivot"
	IPConfigRollbackUnit  = "lca-ipconfig-rollback"

	// systemd unit descriptions
	IPConfigPrePivotDescription    = "lifecycle-agent: ip-config pre-pivot"
	IPConfigPostPivotDescription   = "lifecycle-agent: ip-config post-pivot"
	IPConfigRollbackDescription    = "lifecycle-agent: ip-config rollback"
	IPConfigInitMonitorDescription = "lifecycle-agent: ip-config init monitor"

	// common binary name
	LcaCliBinaryName = "lca-cli"

	PodContainerWaitingReasonImagePullBackOff = "ImagePullBackOff"
	PodContainerWaitingReasonErrImagePull     = "ErrImagePull"
)

// Networking related constants
const (
	BridgeExternalName = "br-ex"
	OvsInterfaceType   = "ovs-interface"
	DefaultRouteV4     = "0.0.0.0/0"
	DefaultRouteV6     = "::/0"

	IPv4FamilyName = "ipv4"
	IPv6FamilyName = "ipv6"

	IPv4TotalBits = 32
	IPv6TotalBits = 128
)
