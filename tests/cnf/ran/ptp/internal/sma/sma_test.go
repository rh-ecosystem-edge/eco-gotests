package sma

import (
	"testing"

	"github.com/mdlayher/genetlink"
	"github.com/mdlayher/netlink"
	"github.com/stretchr/testify/require"
)

type fakeDpllExecutor struct {
	requests  []genetlink.Message
	responses []genetlink.Message
}

func (f *fakeDpllExecutor) Execute(
	message genetlink.Message, _ uint16, _ netlink.HeaderFlags) ([]genetlink.Message, error) {
	f.requests = append(f.requests, message)

	return f.responses, nil
}

func TestDpllNetlinkClientPinID(t *testing.T) {
	t.Parallel()

	response, err := marshalDpllAttributes(func(ae *netlink.AttributeEncoder) {
		ae.Uint32(dpllPinAttrID, 41)
	})
	require.NoError(t, err)

	executor := &fakeDpllExecutor{responses: []genetlink.Message{{Data: response}}}
	client := dpllNetlinkClient{
		conn:   executor,
		family: genetlink.Family{ID: 100, Version: dpllFamilyVersion},
	}

	pinID, err := client.pinID(0x0123456789abcdef, "SMA1")
	require.NoError(t, err)
	require.Equal(t, uint32(41), pinID)
	require.Len(t, executor.requests, 1)
	require.Equal(t, uint8(dpllCmdPinIDGet), executor.requests[0].Header.Command)

	attrs, err := netlink.UnmarshalAttributes(executor.requests[0].Data)
	require.NoError(t, err)
	require.Len(t, attrs, 2)
	require.Equal(t, uint16(dpllPinAttrClockID), attrs[0].Type)
	require.Equal(t, uint16(dpllPinAttrBoardLabel), attrs[1].Type)
}

func TestDpllPinStateFromAttributes(t *testing.T) {
	t.Parallel()

	data, err := marshalDpllAttributes(func(ae *netlink.AttributeEncoder) {
		ae.Uint32(dpllPinAttrID, 41)
		ae.Nested(dpllPinAttrParentDevice, func(nae *netlink.AttributeEncoder) error {
			nae.Uint32(dpllPinAttrParentID, 7)
			nae.Uint32(dpllPinAttrDirection, dpllPinDirectionInput)
			nae.Uint32(dpllPinAttrPriority, 3)
			nae.Uint32(dpllPinAttrState, dpllPinStateConnected)

			return nil
		})
	})
	require.NoError(t, err)

	state, err := dpllPinStateFromAttributes(data)
	require.NoError(t, err)
	require.Equal(t, dpllPinState{parents: []dpllParentDevice{{
		id:        7,
		direction: dpllPinDirectionInput,
		priority:  3,
		state:     dpllPinStateConnected,
	}}}, state)
}

func TestDpllNetlinkClientSetPinParents(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		reconnect     bool
		inputPriority uint32
		inputState    uint32
		outputState   uint32
	}{
		{
			name:          "disconnects input and output parents",
			inputPriority: dpllDisconnectedPriority,
			inputState:    dpllPinStateSelectable,
			outputState:   dpllPinStateDisconnected,
		},
		{
			name:          "reconnects input and output parents",
			reconnect:     true,
			inputPriority: dpllDefaultPriority,
			inputState:    dpllPinStateSelectable,
			outputState:   dpllPinStateConnected,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			executor := &fakeDpllExecutor{}
			client := dpllNetlinkClient{
				conn:   executor,
				family: genetlink.Family{ID: 100, Version: dpllFamilyVersion},
			}
			parents := []dpllParentDevice{
				{id: 7, direction: dpllPinDirectionInput},
				{id: 8, direction: dpllPinDirectionOutput},
			}

			require.NoError(t, client.setPinParents(41, parents, testCase.reconnect))
			require.Len(t, executor.requests, 1)
			require.Equal(t, uint8(dpllCmdPinSet), executor.requests[0].Header.Command)

			attrs, err := netlink.UnmarshalAttributes(executor.requests[0].Data)
			require.NoError(t, err)
			require.Len(t, attrs, 3)
			require.Equal(t, uint16(dpllPinAttrID), attrs[0].Type)

			inputParent, err := dpllParentDeviceFromAttributes(attrs[1].Data)
			require.NoError(t, err)
			require.Equal(t, dpllParentDevice{
				id:        7,
				direction: 0,
				priority:  testCase.inputPriority,
				state:     testCase.inputState,
			}, inputParent)

			outputParent, err := dpllParentDeviceFromAttributes(attrs[2].Data)
			require.NoError(t, err)
			require.Equal(t, dpllParentDevice{
				id:    8,
				state: testCase.outputState,
			}, outputParent)
		})
	}
}
