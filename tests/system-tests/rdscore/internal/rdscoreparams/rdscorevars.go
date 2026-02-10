package rdscoreparams

import (
	"github.com/openshift-kni/k8sreporter"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = []string{Label}

	// ReporterNamespacesToDump tells to the reporter from where to collect logs.
	// Updated to include all RDS Core test namespaces instead of demo namespaces.
	ReporterNamespacesToDump = map[string]string{
		// SR-IOV workload namespaces
		"rds-sriov-wlkd": "rds-sriov-wlkd",

		// Storage (ODF/Ceph) test namespaces
		"rds-cephfs-ns":        "rds-cephfs-ns",
		"rds-cephrbd-ns":       "rds-cephrbd-ns",
		"rds-cephrbd-block-ns": "rds-cephrbd-block-ns",

		// Whereabouts IP management namespaces
		"rds-whereabouts": "rds-whereabouts",

		// MACVLAN test namespaces
		"rds-macvlan": "rds-macvlan",

		// IPVLAN test namespaces
		"rds-ipvlan": "rds-ipvlan",

		// EgressIP test namespaces
		"rds-egressip-ns-one": "rds-egressip-ns-one",
		"rds-egressip-ns-two": "rds-egressip-ns-two",

		// Egress Service test namespace
		"rds-egress-ns": "rds-egress-ns",

		// MetalLB and FRR test namespaces
		"rds-metallb-supporttools-ns": "rds-metallb-supporttools-ns",
		"openshift-frr-k8s":           "openshift-frr-k8s",

		// NROP (NUMA Resources Operator) test namespaces
		"rds-nrop": "rds-nrop",

		// Pod-level bonding test namespace
		"rds-pod-level-bond": "rds-pod-level-bond",

		// Rootless DPDK test namespace
		"rds-dpdk": "rds-dpdk",

		// OpenShift system namespaces (for cluster-wide resources)
		"openshift-multus": "openshift-multus",
	}

	// ReporterCRDsToDump tells to the reporter what CRs to dump.
	// Enhanced to include Deployments, StatefulSets, ReplicaSets, and Events for better failure debugging.
	ReporterCRDsToDump = []k8sreporter.CRData{
		// Core workload resources
		{Cr: &corev1.PodList{}},
		{Cr: &appsv1.DeploymentList{}},
		{Cr: &appsv1.StatefulSetList{}},
		{Cr: &appsv1.ReplicaSetList{}},

		// Events are critical for understanding scheduling failures and other issues
		{Cr: &corev1.EventList{}},
	}
)
