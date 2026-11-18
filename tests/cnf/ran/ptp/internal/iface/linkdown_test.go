//go:build unit_test

package iface

import (
	"testing"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	"github.com/stretchr/testify/assert"
)

//nolint:funlen // long due to number of test cases
func TestPFRequiresConfigDaemonPause(t *testing.T) {
	t.Parallel()

	withVFs := &sriovv1.SriovNetworkNodeState{
		Spec: sriovv1.SriovNetworkNodeStateSpec{
			Interfaces: []sriovv1.Interface{{
				Name:       "ens5f0np0",
				PciAddress: "0000:17:00.0",
				NumVfs:     8,
			}},
		},
		Status: sriovv1.SriovNetworkNodeStateStatus{
			Interfaces: []sriovv1.InterfaceExt{{
				Name:       "ens5f0np0",
				PciAddress: "0000:17:00.0",
				NumVfs:     8,
			}},
		},
	}

	testCases := []struct {
		name      string
		state     *sriovv1.SriovNetworkNodeState
		ifaceName Iface
		want      bool
	}{
		{
			name:      "nil node state",
			ifaceName: "ens5f0np0",
			want:      false,
		},
		{
			name:      "physical function with virtual functions",
			state:     withVFs,
			ifaceName: "ens5f0np0",
			want:      true,
		},
		{
			name: "externally managed physical function",
			state: &sriovv1.SriovNetworkNodeState{
				Spec: sriovv1.SriovNetworkNodeStateSpec{
					Interfaces: []sriovv1.Interface{{
						Name:              "ens5f0np0",
						PciAddress:        "0000:17:00.0",
						NumVfs:            8,
						ExternallyManaged: true,
					}},
				},
				Status: sriovv1.SriovNetworkNodeStateStatus{
					Interfaces: []sriovv1.InterfaceExt{{
						Name:       "ens5f0np0",
						PciAddress: "0000:17:00.0",
						NumVfs:     8,
					}},
				},
			},
			ifaceName: "ens5f0np0",
			want:      false,
		},
		{
			name: "physical function without virtual functions",
			state: &sriovv1.SriovNetworkNodeState{
				Spec: sriovv1.SriovNetworkNodeStateSpec{
					Interfaces: []sriovv1.Interface{{
						Name:       "ens5f0np0",
						PciAddress: "0000:17:00.0",
					}},
				},
				Status: sriovv1.SriovNetworkNodeStateStatus{
					Interfaces: []sriovv1.InterfaceExt{{
						Name:       "ens5f0np0",
						PciAddress: "0000:17:00.0",
					}},
				},
			},
			ifaceName: "ens5f0np0",
			want:      false,
		},
		{
			name:      "different interface",
			state:     withVFs,
			ifaceName: "ens5f1np1",
			want:      false,
		},
		{
			name: "virtual functions in status only",
			state: &sriovv1.SriovNetworkNodeState{
				Spec: sriovv1.SriovNetworkNodeStateSpec{
					Interfaces: []sriovv1.Interface{{
						PciAddress: "0000:17:00.0",
						NumVfs:     8,
					}},
				},
				Status: sriovv1.SriovNetworkNodeStateStatus{
					Interfaces: []sriovv1.InterfaceExt{{
						Name:       "ens5f0np0",
						PciAddress: "0000:17:00.0",
						NumVfs:     8,
					}},
				},
			},
			ifaceName: "ens5f0np0",
			want:      true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, testCase.want, pfRequiresConfigDaemonPause(testCase.state, testCase.ifaceName))
		})
	}
}
