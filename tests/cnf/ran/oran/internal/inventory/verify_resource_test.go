package inventory

import (
	"testing"

	bmhv1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/stretchr/testify/assert"
)

func provisionedOnlineHost() *bmhv1alpha1.BareMetalHost {
	return &bmhv1alpha1.BareMetalHost{
		Spec: bmhv1alpha1.BareMetalHostSpec{Online: true},
		Status: bmhv1alpha1.BareMetalHostStatus{
			OperationalStatus: bmhv1alpha1.OperationalStatusOK,
			PoweredOn:         true,
			Provisioning: bmhv1alpha1.ProvisionStatus{
				State: bmhv1alpha1.StateProvisioned,
			},
		},
	}
}

func offlineHost() *bmhv1alpha1.BareMetalHost {
	return &bmhv1alpha1.BareMetalHost{
		Spec: bmhv1alpha1.BareMetalHostSpec{Online: false},
	}
}

func TestExpectedAdminState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		online bool
		want   string
	}{
		{name: "online", online: true, want: "UNLOCKED"},
		{name: "offline", online: false, want: "LOCKED"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			host := &bmhv1alpha1.BareMetalHost{Spec: bmhv1alpha1.BareMetalHostSpec{Online: testCase.online}}
			assert.Equal(t, testCase.want, expectedAdminState(host))
		})
	}
}

func TestExpectedOperationalState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		host *bmhv1alpha1.BareMetalHost
		want string
	}{
		{
			name: "enabled when provisioned online and powered on",
			host: provisionedOnlineHost(),
			want: "ENABLED",
		},
		{
			name: "disabled when offline",
			host: offlineHost(),
			want: "DISABLED",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, testCase.want, expectedOperationalState(testCase.host))
		})
	}
}

func TestExpectedUsageState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		host *bmhv1alpha1.BareMetalHost
		want string
	}{
		{
			name: "provisioned active",
			host: provisionedOnlineHost(),
			want: "ACTIVE",
		},
		{
			name: "available idle",
			host: &bmhv1alpha1.BareMetalHost{
				Status: bmhv1alpha1.BareMetalHostStatus{
					OperationalStatus: bmhv1alpha1.OperationalStatusOK,
					Provisioning:      bmhv1alpha1.ProvisionStatus{State: bmhv1alpha1.StateAvailable},
				},
			},
			want: "IDLE",
		},
		{
			name: "provisioning busy",
			host: &bmhv1alpha1.BareMetalHost{
				Status: bmhv1alpha1.BareMetalHostStatus{
					Provisioning: bmhv1alpha1.ProvisionStatus{State: bmhv1alpha1.StateProvisioning},
				},
			},
			want: "BUSY",
		},
		{
			name: "registering unknown",
			host: &bmhv1alpha1.BareMetalHost{
				Status: bmhv1alpha1.BareMetalHostStatus{
					Provisioning: bmhv1alpha1.ProvisionStatus{State: bmhv1alpha1.StateRegistering},
				},
			},
			want: "UNKNOWN",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, testCase.want, expectedUsageState(testCase.host))
		})
	}
}

func TestExpectedPowerState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		poweredOn bool
		want      string
	}{
		{name: "on", poweredOn: true, want: "ON"},
		{name: "off", poweredOn: false, want: "OFF"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			host := &bmhv1alpha1.BareMetalHost{
				Status: bmhv1alpha1.BareMetalHostStatus{PoweredOn: testCase.poweredOn},
			}
			assert.Equal(t, testCase.want, expectedPowerState(host))
		})
	}
}
