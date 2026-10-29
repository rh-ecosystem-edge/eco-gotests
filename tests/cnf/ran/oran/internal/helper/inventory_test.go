//go:build unit_test

package helper

import (
	"testing"

	bmhv1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsFreeInventoryBMH(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		host *bmhv1alpha1.BareMetalHost
		want bool
	}{
		{name: "nil", host: nil, want: false},
		{
			name: "available o-cloud host",
			host: ocloudBMH(bmhv1alpha1.StateAvailable, ""),
			want: true,
		},
		{
			name: "provisioned o-cloud host is not free",
			host: ocloudBMH(bmhv1alpha1.StateProvisioned, ""),
			want: false,
		},
		{
			name: "available without o-cloud labels",
			host: &bmhv1alpha1.BareMetalHost{
				Status: bmhv1alpha1.BareMetalHostStatus{
					Provisioning: bmhv1alpha1.ProvisionStatus{State: bmhv1alpha1.StateAvailable},
				},
			},
			want: false,
		},
		{
			name: "available with resource selector label",
			host: &bmhv1alpha1.BareMetalHost{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						labelPrefixResourceSelector + "site": "rdu3",
					},
				},
				Status: bmhv1alpha1.BareMetalHostStatus{
					Provisioning: bmhv1alpha1.ProvisionStatus{State: bmhv1alpha1.StateAvailable},
				},
			},
			want: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, testCase.want, IsFreeInventoryBMH(testCase.host))
		})
	}
}

func TestShouldReclaimStuckBMH(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		host *bmhv1alpha1.BareMetalHost
		want bool
	}{
		{name: "nil", host: nil, want: false},
		{
			name: "available free host is not reclaimed",
			host: ocloudBMH(bmhv1alpha1.StateAvailable, ""),
			want: false,
		},
		{
			name: "available host with allocated-node label is reclaimed",
			host: ocloudBMH(bmhv1alpha1.StateAvailable, "node-1"),
			want: true,
		},
		{
			name: "provisioned o-cloud host is reclaimed",
			host: ocloudBMH(bmhv1alpha1.StateProvisioned, ""),
			want: true,
		},
		{
			name: "provisioning o-cloud host is reclaimed",
			host: ocloudBMH(bmhv1alpha1.StateProvisioning, "node-1"),
			want: true,
		},
		{
			name: "inspecting o-cloud host is not reclaimed",
			host: ocloudBMH(bmhv1alpha1.StateInspecting, ""),
			want: false,
		},
		{
			name: "registering o-cloud host is not reclaimed",
			host: ocloudBMH(bmhv1alpha1.StateRegistering, ""),
			want: false,
		},
		{
			name: "preparing o-cloud host is reclaimed",
			host: ocloudBMH(bmhv1alpha1.StatePreparing, ""),
			want: true,
		},
		{
			name: "non o-cloud provisioned host is not reclaimed",
			host: &bmhv1alpha1.BareMetalHost{
				Status: bmhv1alpha1.BareMetalHostStatus{
					Provisioning: bmhv1alpha1.ProvisionStatus{State: bmhv1alpha1.StateProvisioned},
				},
			},
			want: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, testCase.want, ShouldReclaimStuckBMH(testCase.host))
		})
	}
}

func ocloudBMH(state bmhv1alpha1.ProvisioningState, allocatedNode string) *bmhv1alpha1.BareMetalHost {
	labels := map[string]string{
		labelResourcePoolName: "master-pool",
	}
	if allocatedNode != "" {
		labels[AllocatedNodeLabel] = allocatedNode
	}

	return &bmhv1alpha1.BareMetalHost{
		ObjectMeta: metav1.ObjectMeta{Labels: labels},
		Status: bmhv1alpha1.BareMetalHostStatus{
			Provisioning: bmhv1alpha1.ProvisionStatus{State: state},
		},
	}
}
