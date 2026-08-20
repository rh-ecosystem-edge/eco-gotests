//go:build unit_test

package client_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	ptpv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ptp/v1"
	ptpv2alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ptp/v2alpha1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/client"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/clock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clientgotesting "k8s.io/client-go/testing"
	"sigs.k8s.io/yaml"
)

// updateGoldenEnvVar, when "true", regenerates golden fixtures instead of comparing against them.
const updateGoldenEnvVar = "UPDATE_GOLDEN"

// pluginJSONAsMap compares plugin JSON by decoded content, not raw byte order.
var pluginJSONAsMap = cmp.Transformer("pluginJSONAsMap", func(raw apiextv1.JSON) interface{} {
	var decoded interface{}
	if err := json.Unmarshal(raw.Raw, &decoded); err != nil {
		return string(raw.Raw)
	}

	return decoded
})

// assertGoldenPtpConfigSpec diffs actual's own Spec against the golden fixture, or regenerates it
// under UPDATE_GOLDEN=true.
func assertGoldenPtpConfigSpec(t *testing.T, filename string, actual *ptpv1.PtpConfig) {
	t.Helper()

	if os.Getenv(updateGoldenEnvVar) == "true" {
		out, err := yaml.Marshal(actual)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile("testdata/"+filename, out, 0o644))

		return
	}

	golden := loadPtpConfig(t, filename)
	if diff := cmp.Diff(golden.Spec, actual.Spec, pluginJSONAsMap); diff != "" {
		t.Errorf("PtpConfig spec mismatch (-golden +actual):\n%s", diff)
	}
}

// assertGoldenHardwareConfigSpec diffs actual's own Spec against the golden fixture, or regenerates
// it under UPDATE_GOLDEN=true.
func assertGoldenHardwareConfigSpec(t *testing.T, filename string, actual *ptpv2alpha1.HardwareConfig) {
	t.Helper()

	if os.Getenv(updateGoldenEnvVar) == "true" {
		out, err := yaml.Marshal(actual)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile("testdata/"+filename, out, 0o644))

		return
	}

	golden := loadHardwareConfig(t, filename)
	if diff := cmp.Diff(golden.Spec, actual.Spec); diff != "" {
		t.Errorf("HardwareConfig spec mismatch (-golden +actual):\n%s", diff)
	}
}

// masterNode is the single node every fixture's own recommend rule matches against
// (node-role.kubernetes.io/master).
func masterNode() *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "master-0",
			Labels: map[string]string{"node-role.kubernetes.io/master": ""},
		},
	}
}

// loadPtpConfig unmarshals a PtpConfig fixture from testdata.
func loadPtpConfig(t *testing.T, filename string) *ptpv1.PtpConfig {
	t.Helper()

	data, err := os.ReadFile("testdata/" + filename)
	require.NoError(t, err)

	var cfg ptpv1.PtpConfig

	require.NoError(t, yaml.Unmarshal(data, &cfg))

	return &cfg
}

// loadHardwareConfig unmarshals a HardwareConfig fixture from testdata.
func loadHardwareConfig(t *testing.T, filename string) *ptpv2alpha1.HardwareConfig {
	t.Helper()

	data, err := os.ReadFile("testdata/" + filename)
	require.NoError(t, err)

	var hwConfig ptpv2alpha1.HardwareConfig

	require.NoError(t, yaml.Unmarshal(data, &hwConfig))

	return &hwConfig
}

// initFakePtpClient wires a fake PtpClient against the given fixture objects, exactly matching real
// fetchPtpResources' own List calls (nodes.List, ptp.ListPtpConfigs, ptp.ListHardwareConfigs). Returns
// the fake client itself, for a test that needs to inspect state client.PtpClient doesn't expose.
func initFakePtpClient(t *testing.T, objects ...runtime.Object) *clients.Settings {
	t.Helper()

	fakeClient := clients.GetTestClients(clients.TestClientParams{
		K8sMockObjects:  objects,
		SchemeAttachers: []clients.SchemeAttacher{ptpv1.AddToScheme, ptpv2alpha1.AddToScheme},
	})

	client.Init(fakeClient, nil)

	return fakeClient
}

// initFakePtpClientPlainTracker is initFakePtpClient, but the fake client's own ObjectTracker skips
// field management (structured-merge-diff via a deduced type converter, needed only for Server-Side
// Apply, which this codebase never uses) -- that converter's reflection walk panics on a plain uint64
// field (ptpv2alpha1.HoldoverParameters' own fields), a known controller-runtime limitation:
// https://github.com/kubernetes-sigs/controller-runtime/issues/3418.
func initFakePtpClientPlainTracker(t *testing.T, objects ...runtime.Object) *clients.Settings {
	t.Helper()

	clientSet, testBuilder := clients.GetModifiableTestClients(clients.TestClientParams{
		K8sMockObjects:  objects,
		SchemeAttachers: []clients.SchemeAttacher{ptpv1.AddToScheme, ptpv2alpha1.AddToScheme},
	})

	trackerScheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(trackerScheme))
	require.NoError(t, ptpv1.AddToScheme(trackerScheme))
	require.NoError(t, ptpv2alpha1.AddToScheme(trackerScheme))

	objectTracker := clientgotesting.NewObjectTracker(trackerScheme, serializer.NewCodecFactory(trackerScheme).UniversalDecoder())

	clientSet.Client = testBuilder.WithObjectTracker(objectTracker).Build()
	client.Init(clientSet, nil)

	return clientSet
}

// Fixture source: ztp-site-configs' own policygentemplates/source-crs/ptp-operator/tbc-single-card/
// PtpConfigTbcSingleCard.yaml -- verified live on helix60 for CNF-26069 itself.
func TestDeriveAllClusterClocks_TBC_WPC_SingleNIC_HoldoverCapable(t *testing.T) {
	ptpConfig := loadPtpConfig(t, "PtpConfigTBCWpcSingleNICHoldoverCapable.yaml")
	initFakePtpClient(t, masterNode(), ptpConfig)

	clks, err := client.DeriveAllClusterClocks()
	require.NoError(t, err)
	require.Len(t, clks, 1)

	clk := clks[0]
	require.Equal(t, clock.ClockTypeTBC, clk.Type)
	require.Equal(t, "master-0", clk.NodeName)
	require.True(t, clk.HoldoverEnabled)
	require.Equal(t, clock.HoldoverSourcePlugin, clk.HoldoverSource)
	require.Len(t, clk.Interfaces, 3)
	require.Len(t, clk.TimeReceiverIfaces(), 1)
	require.Len(t, clk.TimeTransmitterIfaces(), 2)
}

// Fixture source: telco-reference's own telco-ran/configuration/source-crs/ptp-operator/configuration/
// PtpConfigTTSCWpc.yaml -- https://github.com/openshift-kni/telco-reference.
func TestDeriveAllClusterClocks_TTSC_WPC_SingleNIC_HoldoverCapable(t *testing.T) {
	ptpConfig := loadPtpConfig(t, "PtpConfigTTSCWpcSingleNICHoldoverCapable.yaml")
	initFakePtpClient(t, masterNode(), ptpConfig)

	clks, err := client.DeriveAllClusterClocks()
	require.NoError(t, err)
	require.Len(t, clks, 1)

	clk := clks[0]
	require.Equal(t, clock.ClockTypeTTSC, clk.Type)
	require.Equal(t, "master-0", clk.NodeName)
	require.True(t, clk.HoldoverEnabled)
	require.Equal(t, clock.HoldoverSourcePlugin, clk.HoldoverSource)
	require.Len(t, clk.Interfaces, 1)
	require.Len(t, clk.TimeReceiverIfaces(), 1)
	require.Len(t, clk.TimeTransmitterIfaces(), 0)
}

// Fixture source: official OKD/OCP 4.22 docs, "Applying unassisted holdover for boundary clocks and
// time synchronous clocks" (multi-card T-BC, three E810 NICs) --
// https://docs.okd.io/4.22/networking/advanced_networking/ptp/configuring-ptp.html#nw-ptp-t-bc-t-tsc-holdover_configuring-ptp
// Same real-world scenario as telco-reference's own telco-ran/configuration/source-crs/ptp-operator/
// configuration/PtpConfigDualCardTBCWpc.yaml (placeholder interface names there; the official doc's own
// values, already resolved, are used here instead).
func TestDeriveAllClusterClocks_TBC_WPC_TripleNIC_HoldoverCapable(t *testing.T) {
	ptpConfig := loadPtpConfig(t, "PtpConfigTBCWpcTripleNICHoldoverCapable.yaml")
	initFakePtpClient(t, masterNode(), ptpConfig)

	clks, err := client.DeriveAllClusterClocks()
	require.NoError(t, err)
	require.Len(t, clks, 1)

	clk := clks[0]
	require.Equal(t, clock.ClockTypeTBC, clk.Type)
	require.Equal(t, "master-0", clk.NodeName)
	require.True(t, clk.HoldoverEnabled)
	require.Equal(t, clock.HoldoverSourcePlugin, clk.HoldoverSource)
	require.Len(t, clk.Interfaces, 4)
	require.Len(t, clk.TimeReceiverIfaces(), 1)
	require.Len(t, clk.TimeTransmitterIfaces(), 3)
}

// Fixture source: official OKD/OCP 4.22 docs, "Configuring linuxptp services as a boundary clock
// without holdover on Intel Granite Rapids-D hardware" --
// https://docs.okd.io/4.22/networking/advanced_networking/ptp/configuring-ptp.html#ptp-configuring-linuxptp-services-as-boundary-clock-gnrd_configuring-ptp
// Same real-world scenario as telco-reference's own telco-ran/configuration/source-crs/ptp-operator/
// configuration/PtpConfigGnrdBcNoHoldover.yaml. No HardwareConfig CR exists for this scenario --
// proves the discrimination from the other side: 01-bc-tr's own e825 plugin settings must not be
// mistaken for a valid holdover source.
func TestDeriveAllClusterClocks_TBC_GNRD_NACDualCarterFlat_HoldoverIncapable(t *testing.T) {
	ptpConfig := loadPtpConfig(t, "PtpConfigTBCGnrdNACDualCarterFlatHoldoverIncapable.yaml")
	initFakePtpClient(t, masterNode(), ptpConfig)

	clks, err := client.DeriveAllClusterClocks()
	require.NoError(t, err)
	require.Len(t, clks, 1)

	clk := clks[0]
	require.Equal(t, clock.ClockTypeTBC, clk.Type)
	require.Equal(t, "master-0", clk.NodeName)
	require.False(t, clk.HoldoverEnabled)
	require.Equal(t, clock.HoldoverSourceNone, clk.HoldoverSource)
	require.Len(t, clk.Interfaces, 23)
	require.Len(t, clk.TimeReceiverIfaces(), 1)
	require.Len(t, clk.TimeTransmitterIfaces(), 22)
}

// Fixture source: PtpConfig shape matches
// testdata/PtpConfigTBCGnrdNACDualCarterFlatHoldoverIncapable.yaml (see that test's own citations),
// paired here with a real HardwareConfig CR. HardwareConfig shape confirmed independently
// from ztp-site-configs' own HwConfigBcGnrd.yaml (branches cnfdg55-4.20/4.22, cnfdg59-4.20/4.21/4.22,
// helix98-4.22/5.0, helix103/104-4.21/4.22/5.0) and the official OKD/OCP 4.22 docs' "Configuring GNR-D
// T-BC holdover on a GNR-D platform" example --
// https://docs.okd.io/4.22/networking/advanced_networking/ptp/configuring-ptp.html#nw-ptp-gnrd-t-bc-holdover_configuring-ptp
func TestDeriveAllClusterClocks_TBC_GNRD_NACDualCarterFlat_HoldoverCapable(t *testing.T) {
	ptpConfig := loadPtpConfig(t, "PtpConfigTBCGnrdNACDualCarterFlatHoldoverCapable.yaml")
	hwConfig := loadHardwareConfig(t, "HardwareConfigTBCGnrdNACDualCarterFlatHoldoverCapable.yaml")
	initFakePtpClient(t, masterNode(), ptpConfig, hwConfig)

	clks, err := client.DeriveAllClusterClocks()
	require.NoError(t, err)
	require.Len(t, clks, 1)

	clk := clks[0]
	require.Equal(t, clock.ClockTypeTBC, clk.Type)
	require.Equal(t, "master-0", clk.NodeName)
	require.True(t, clk.HoldoverEnabled)
	require.Equal(t, clock.HoldoverSourceHardwareConfig, clk.HoldoverSource)
	require.Len(t, clk.Interfaces, 23)
	require.Len(t, clk.TimeReceiverIfaces(), 1)
	require.Len(t, clk.TimeTransmitterIfaces(), 22)
}
