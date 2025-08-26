package mgmtparams

import (
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/internal/ibuparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/internal/brutil"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	veleroScheme "github.com/vmware-tanzu/velero/pkg/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = []string{ibuparams.Label, Label}
)

// WorkloadBackup represents ibu-workload backup.
var WorkloadBackup = brutil.BackupRestoreObject{
	Scheme: veleroScheme.VeleroScheme,
	GVR:    velerov1.SchemeGroupVersion,
	Object: &velerov1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      LCAWorkloadName,
			Namespace: LCAOADPNamespace,
			Labels: map[string]string{
				"velero.io/storage-location": "default",
			},
		},
		Spec: velerov1.BackupSpec{
			IncludedNamespaces: []string{
				LCAWorkloadName,
			},
			IncludedNamespaceScopedResources: []string{
				"deployments",
				"services",
				"routes",
			},
			ExcludedClusterScopedResources: []string{
				"persistentVolumes",
			},
		},
	},
}

// WorkloadRestore represents ibu-workload restore.
var WorkloadRestore = brutil.BackupRestoreObject{
	Scheme: veleroScheme.VeleroScheme,
	GVR:    velerov1.SchemeGroupVersion,
	Object: &velerov1.Restore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      LCAWorkloadName,
			Namespace: LCAOADPNamespace,
			Labels: map[string]string{
				"velero.io/storage-location": "default",
			},
		},
		Spec: velerov1.RestoreSpec{
			BackupName: LCAWorkloadName,
		},
	},
}

// KlusterletBackup represents ibu klusterlet backup.
var KlusterletBackup = brutil.BackupRestoreObject{
	Scheme: veleroScheme.VeleroScheme,
	GVR:    velerov1.SchemeGroupVersion,
	Object: &velerov1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      LCAKlusterletName,
			Namespace: LCAOADPNamespace,
			Labels: map[string]string{
				"velero.io/storage-location": "default",
			},
		},
		Spec: velerov1.BackupSpec{
			IncludedNamespaces: []string{
				LCAKlusterletNamespace,
			},
			IncludedClusterScopedResources: []string{
				"klusterlets.operator.open-cluster-management.io",
				"clusterclaims.cluster.open-cluster-management.io",
				"clusterroles.rbac.authorization.k8s.io",
				"clusterrolebindings.rbac.authorization.k8s.io",
			},
			IncludedNamespaceScopedResources: []string{
				"deployments",
				"serviceaccounts",
				"secrets",
			},
		},
	},
}

// KlusterletRestore represents ibu klusterlet restore.
var KlusterletRestore = brutil.BackupRestoreObject{
	Scheme: veleroScheme.VeleroScheme,
	GVR:    velerov1.SchemeGroupVersion,
	Object: &velerov1.Restore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      LCAKlusterletName,
			Namespace: LCAOADPNamespace,
			Labels: map[string]string{
				"velero.io/storage-location": "default",
			},
		},
		Spec: velerov1.RestoreSpec{
			BackupName: LCAKlusterletName,
		},
	},
}
