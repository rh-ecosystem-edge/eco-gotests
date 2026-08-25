package kmmparams

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/internal/hwaccelparams"
	corev1 "k8s.io/api/core/v1"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = []string{hwaccelparams.Label, Label}

	// LocalImageRegistry represents the local registry used in KMM tests.
	LocalImageRegistry = "image-registry.openshift-image-registry.svc:5000"

	// KmmHubSelector represents MCM object generic selector.
	KmmHubSelector = map[string]string{"cluster.open-cluster-management.io/clusterset": "default"}

	// KmmTestHelperLabelName represents label set on the helper resources.
	KmmTestHelperLabelName = "kmm-test-helper"

	// DTKImageStream represents DTK imagestream name.
	DTKImageStream = "driver-toolkit"

	// DTKImageStreamNamespace represents namespace where imagestream is found.
	DTKImageStreamNamespace = "openshift"

	// DTKImage represents Driver Toolkit image in internal image registry.
	DTKImage = "image-registry.openshift-image-registry.svc:5000/openshift/driver-toolkit"

	trueVar        = true
	capabilityAll  = []corev1.Capability{"ALL"}
	defaultGroupID = int64(3000)
	defaultUserID  = int64(0)

	// PrivilegedSC represents a privileged security context definition.
	PrivilegedSC = &corev1.SecurityContext{
		Privileged:     &trueVar,
		RunAsGroup:     &defaultGroupID,
		RunAsUser:      &defaultUserID,
		SeccompProfile: &corev1.SeccompProfile{Type: "RuntimeDefault"},
		Capabilities: &corev1.Capabilities{
			Add: capabilityAll,
		},
	}

	// DRAPresetEnvNames represents the preset environment variable names injected by KMM into DRA DaemonSets.
	DRAPresetEnvNames = []string{
		"NODE_NAME", "POD_UID", "CDI_ROOT",
		"KUBELET_REGISTRAR_DIRECTORY_PATH",
		"KUBELET_PLUGINS_DIRECTORY_PATH",
		"HEALTHCHECK_PORT",
		"DRIVER_NAME",
	}

	// ReasonBuildList represents expected events to be found for a successful build job.
	ReasonBuildList = []string{ReasonBuildCreated, ReasonBuildStarted, ReasonBuildCompleted, ReasonBuildSucceeded}
	// ReasonSignList represents expected events to be found for a successful sign job.
	ReasonSignList = []string{ReasonSignCreated, ReasonBuildStarted, ReasonBuildCompleted, ReasonBuildSucceeded}

	// TolerationNoScheduleK8sUnschedulable represents definition of specific K8s unschedulable taint
	// seen during cluster upgrades.
	TolerationNoScheduleK8sUnschedulable = corev1.Toleration{
		Key:      fmt.Sprintf("kmm-%s", corev1.TaintNodeUnschedulable),
		Effect:   corev1.TaintEffectNoSchedule,
		Operator: corev1.TolerationOpExists,
	}

	// TolerationNoScheduleK8sUnreachable represents definition of speficic K8s unreachable taint seen
	// during cluster upgrades.
	TolerationNoScheduleK8sUnreachable = corev1.Toleration{
		Key:      fmt.Sprintf("kmm-%s", corev1.TaintNodeUnreachable),
		Effect:   corev1.TaintEffectNoSchedule,
		Operator: corev1.TolerationOpExists,
	}

	// TolerationNoExecuteK8sUnreachable represents definition of specific K8s unreachable taint seen
	// during cluster upgrades.
	TolerationNoExecuteK8sUnreachable = corev1.Toleration{
		Key:      fmt.Sprintf("kmm-%s", corev1.TaintNodeUnreachable),
		Effect:   corev1.TaintEffectNoExecute,
		Operator: corev1.TolerationOpExists,
	}

	// TolerationNoScheduleK8sDiskPressure represents definition of specific K8s disk-pressure taint seen
	// on nodes with low disk space.
	TolerationNoScheduleK8sDiskPressure = corev1.Toleration{
		Key:      fmt.Sprintf("kmm-%s", corev1.TaintNodeDiskPressure),
		Effect:   corev1.TaintEffectNoSchedule,
		Operator: corev1.TolerationOpExists,
	}

	// TolerationNoScheduleKeyValue represents definition of dummy taint used in tests.
	TolerationNoScheduleKeyValue = corev1.Toleration{
		Key:      "kmm-key",
		Value:    "value",
		Operator: corev1.TolerationOpEqual,
		Effect:   corev1.TaintEffectNoSchedule,
	}

	// TolerationNoExecuteKeyValue represents definition of dummy taint used in tests.
	TolerationNoExecuteKeyValue = corev1.Toleration{
		Key:      "kmm-key",
		Value:    "value",
		Operator: corev1.TolerationOpEqual,
		Effect:   corev1.TaintEffectNoExecute,
	}

	// DRADriverImage is the DRA example driver image, resolved at runtime
	// from the cluster's Kubernetes version. Call SetDRADriverImage before
	// running DRA tests. Empty until SetDRADriverImage is called.
	DRADriverImage string

	draDriverTags = map[int]string{
		32: "v0.1.0",
		33: "v0.2.0",
		34: "v0.2.1",
		35: "v0.2.1",
		36: "v0.3.0",
		37: "v0.4.0",
	}
)

// SetDRADriverImage resolves the DRA example driver image tag from the
// cluster's Kubernetes version (e.g. "v1.35.3") and stores it in
// DRADriverImage. repo is the image repository set via
// ECO_HWACCEL_KMM_DRA_DRIVER_IMAGE_REPO. If repo is empty,
// DRADriverImage remains empty and DRA tests should be skipped.
func SetDRADriverImage(repo, serverVersion string) {
	if repo == "" {
		return
	}

	tag := "v0.4.0"

	v := strings.TrimPrefix(serverVersion, "v")
	parts := strings.SplitN(v, ".", 3)

	if len(parts) >= 2 {
		if minor, err := strconv.Atoi(parts[1]); err == nil {
			if t, ok := draDriverTags[minor]; ok {
				tag = t
			}
		}
	}

	DRADriverImage = fmt.Sprintf("%s:%s", repo, tag)
}
