package kmmparams

import (
	"fmt"

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

	// DTKImage represents Driver Toolkit image in internal image registry.
	DTKImage = "image-registry.openshift-image-registry.svc:5000/openshift/driver-toolkit"

	trueVar        = true
	capabilityAll  = []corev1.Capability{"ALL"}
	defaultGroupID = int64(3000)
	defaultUserID  = int64(2000)

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
)
