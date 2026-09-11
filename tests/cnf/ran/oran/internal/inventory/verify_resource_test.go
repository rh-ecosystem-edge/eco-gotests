//go:build unit_test

package inventory

import (
	"testing"

	"github.com/google/uuid"
	bmhv1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// createMatchingResourceFixture creates matching API, host, pool, and hardware fixtures.
func createMatchingResourceFixture() (
	oranapi.Resource, *bmhv1alpha1.BareMetalHost, uuid.UUID, *bmhv1alpha1.HardwareDetails) {
	resourceID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	poolID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	host := createProvisionedOnlineHost()
	host.ObjectMeta = metav1.ObjectMeta{
		UID:         types.UID(resourceID.String()),
		Annotations: map[string]string{resourceInfoDescriptionAnnotation: "server description"},
	}
	hardware := &bmhv1alpha1.HardwareDetails{
		SystemVendor: bmhv1alpha1.HardwareSystemVendor{
			SerialNumber: "serial-1",
			Manufacturer: "Acme",
			ProductName:  "Model A",
		},
	}
	resource := oranapi.Resource{
		ResourceId:     resourceID,
		ResourcePoolId: poolID,
		Description:    "server description",
		GlobalAssetId:  "serial-1",
		Extensions: map[string]any{
			extVendor:           "Acme",
			extModel:            "Model A",
			extAdminState:       "UNLOCKED",
			extOperationalState: "ENABLED",
			extUsageState:       "ACTIVE",
			extPowerState:       "ON",
		},
	}

	return resource, host, poolID, hardware
}

func TestVerifyResourceMatchesBMHProjection(t *testing.T) {
	t.Parallel()

	resource, host, poolID, hardware := createMatchingResourceFixture()
	require.NoError(t, verifyResourceMatchesBMH(resource, host, poolID, hardware))

	resource.Description = "different description"
	resource.Extensions[extVendor] = "Different Vendor"

	err := verifyResourceMatchesBMH(resource, host, poolID, hardware)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Description")
	assert.Contains(t, err.Error(), `"vendor"`)
}

func TestVerifyResourceMatchesBMHNilAnnotations(t *testing.T) {
	t.Parallel()

	resource, host, poolID, hardware := createMatchingResourceFixture()
	host.Annotations = nil
	resource.Description = ""

	assert.NoError(t, verifyResourceMatchesBMH(resource, host, poolID, hardware))
}

func TestReadExtensionString(t *testing.T) {
	t.Parallel()

	extensions := map[string]any{
		"string":    "value",
		"notString": 123,
	}

	assert.Equal(t, "value", readExtensionString(extensions, "string"))
	assert.Empty(t, readExtensionString(extensions, "missing"))
	assert.Empty(t, readExtensionString(extensions, "notString"))
}

// createProvisionedOnlineHost creates a provisioned, online, powered-on host fixture.
func createProvisionedOnlineHost() *bmhv1alpha1.BareMetalHost {
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

// createOfflineHost creates an offline host fixture.
func createOfflineHost() *bmhv1alpha1.BareMetalHost {
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
			assert.Equal(t, testCase.want, deriveAdminState(host))
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
			host: createProvisionedOnlineHost(),
			want: "ENABLED",
		},
		{
			name: "disabled when offline",
			host: createOfflineHost(),
			want: "DISABLED",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, testCase.want, deriveOperationalState(testCase.host))
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
			host: createProvisionedOnlineHost(),
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

			assert.Equal(t, testCase.want, deriveUsageState(testCase.host))
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
			assert.Equal(t, testCase.want, derivePowerState(host))
		})
	}
}
