package iface

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"time"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/ptpdaemon"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	sriovOperatorNamespace     = "openshift-sriov-network-operator"
	sriovOperatorConfigName    = "default"
	sriovConfigDaemonDaemonSet = "sriov-network-config-daemon"
	configDaemonPausedLabel    = "sriov-paused"
	configDaemonPausedValue    = "true"
	configDaemonRemovedTimeout = 2 * time.Minute
)

var (
	configDaemonPaused        bool
	savedConfigDaemonSelector map[string]string
	// downInterfaces records whether NetworkManager was managing the interface before it was set down.
	downInterfaces = map[string]bool{}
)

// setInterfaceDown keeps the interface down against NetworkManager and, when required, the SR-IOV config daemon.
// An interface that is already down is left as it is.
func setInterfaceDown(apiClient *clients.Settings, nodeName string, ifaceName Iface) error {
	down, err := interfaceIsDown(apiClient, nodeName, ifaceName)
	if err != nil || down {
		return err
	}

	if err := pauseConfigDaemonForInterface(apiClient, nodeName, ifaceName); err != nil {
		return err
	}

	managed, err := networkManagerManaged(apiClient, nodeName, ifaceName)
	if err != nil {
		return err
	}

	if managed {
		if err := setNetworkManagerManaged(apiClient, nodeName, ifaceName, false); err != nil {
			return err
		}
	}

	downInterfaces[downInterfaceKey(nodeName, ifaceName)] = managed

	return setLinkState(apiClient, nodeName, ifaceName, InterfaceStateDown)
}

// setInterfaceUp brings the interface up, returns it to NetworkManager, and schedules the config daemon again once
// every interface set down is up. The link is up before the daemon is scheduled. A failed NetworkManager restore keeps
// its record, so a later up retries it and the daemon stays paused until that retry succeeds.
func setInterfaceUp(apiClient *clients.Settings, nodeName string, ifaceName Iface) error {
	if err := setLinkState(apiClient, nodeName, ifaceName, InterfaceStateUp); err != nil {
		return err
	}

	key := downInterfaceKey(nodeName, ifaceName)
	if managed, tracked := downInterfaces[key]; tracked {
		if managed {
			if err := setNetworkManagerManaged(apiClient, nodeName, ifaceName, true); err != nil {
				return err
			}
		}

		delete(downInterfaces, key)
	}

	if len(downInterfaces) == 0 && configDaemonPaused {
		return resumeConfigDaemon(apiClient)
	}

	return nil
}

// interfaceIsDown reports whether ip link show already contains state DOWN.
func interfaceIsDown(apiClient *clients.Settings, nodeName string, ifaceName Iface) (bool, error) {
	output, err := ptpdaemon.ExecuteCommandInPtpDaemonPod(
		apiClient, nodeName, fmt.Sprintf("ip link show %s", ifaceName))
	if err != nil {
		return false, err
	}

	return strings.Contains(output, " state DOWN "), nil
}

// pauseConfigDaemonForInterface unschedules the config daemon when this interface is an SR-IOV physical function that
// the daemon would repair.
func pauseConfigDaemonForInterface(apiClient *clients.Settings, nodeName string, ifaceName Iface) error {
	needsPause, err := interfaceNeedsConfigDaemonPause(apiClient, nodeName, ifaceName)
	if err != nil || !needsPause || configDaemonPaused {
		return err
	}

	config, err := sriov.PullOperatorConfig(apiClient, sriovOperatorNamespace)
	if err != nil {
		return fmt.Errorf("failed to get SriovOperatorConfig %s: %w", sriovOperatorConfigName, err)
	}

	if config.Definition == nil {
		return fmt.Errorf("failed to get SriovOperatorConfig %s", sriovOperatorConfigName)
	}

	savedConfigDaemonSelector = maps.Clone(config.Definition.Spec.ConfigDaemonNodeSelector)

	err = updateConfigDaemonSelector(apiClient, map[string]string{
		configDaemonPausedLabel: configDaemonPausedValue,
	})
	if err != nil {
		return err
	}

	configDaemonPaused = true

	klog.V(tsparams.LogLevel).Infof(
		"Paused the SR-IOV config daemon with configDaemonNodeSelector %s=%s",
		configDaemonPausedLabel, configDaemonPausedValue)

	return waitForConfigDaemonGone(apiClient)
}

// interfaceNeedsConfigDaemonPause reports whether bringing this interface down would make the config daemon repair it.
// A missing SR-IOV installation is not an error.
func interfaceNeedsConfigDaemonPause(apiClient *clients.Settings, nodeName string, ifaceName Iface) (bool, error) {
	state := sriov.NewNetworkNodeStateBuilder(apiClient, nodeName, sriovOperatorNamespace)
	if state == nil {
		return false, fmt.Errorf("failed to create the SriovNetworkNodeState builder for node %s", nodeName)
	}

	err := state.Discover()
	if k8serrors.IsNotFound(err) || meta.IsNoMatchError(err) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("failed to get SriovNetworkNodeState %s: %w", nodeName, err)
	}

	return pfRequiresConfigDaemonPause(state.Objects, ifaceName), nil
}

// pfRequiresConfigDaemonPause reports whether the config daemon treats this netdev as a physical function it must
// keep administratively up. Externally managed functions are left alone by the daemon.
func pfRequiresConfigDaemonPause(state *sriovv1.SriovNetworkNodeState, ifaceName Iface) bool {
	if state == nil {
		return false
	}

	name := string(ifaceName)
	pciAddress := ""
	statusNumVfs := 0

	for _, statusIface := range state.Status.Interfaces {
		if statusIface.Name == name {
			pciAddress = statusIface.PciAddress
			statusNumVfs = statusIface.NumVfs

			break
		}
	}

	for _, specIface := range state.Spec.Interfaces {
		matchesName := specIface.Name == name

		matchesPCI := pciAddress != "" && specIface.PciAddress == pciAddress
		if !matchesName && !matchesPCI {
			continue
		}

		if specIface.ExternallyManaged {
			return false
		}

		return specIface.NumVfs > 0 || statusNumVfs > 0
	}

	return false
}

// resumeConfigDaemon writes the saved configDaemonNodeSelector back.
func resumeConfigDaemon(apiClient *clients.Settings) error {
	if err := updateConfigDaemonSelector(apiClient, savedConfigDaemonSelector); err != nil {
		return err
	}

	configDaemonPaused = false
	savedConfigDaemonSelector = nil

	klog.V(tsparams.LogLevel).Info("Restored the SR-IOV config daemon node selector")

	return nil
}

// updateConfigDaemonSelector writes configDaemonNodeSelector. An empty selector clears the field. The operator-config
// builder rejects an empty selector, so that restore updates the object directly.
func updateConfigDaemonSelector(apiClient *clients.Settings, selector map[string]string) error {
	if len(selector) == 0 {
		return clearConfigDaemonSelector(apiClient)
	}

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		config, err := sriov.PullOperatorConfig(apiClient, sriovOperatorNamespace)
		if err != nil {
			return err
		}

		_, err = config.WithConfigDaemonNodeSelector(selector).Update()

		return err
	})
	if err != nil {
		return fmt.Errorf("failed to update configDaemonNodeSelector on %s: %w", sriovOperatorConfigName, err)
	}

	return nil
}

// clearConfigDaemonSelector removes configDaemonNodeSelector. The operator-config builder rejects an empty map.
func clearConfigDaemonSelector(apiClient *clients.Settings) error {
	if err := apiClient.AttachScheme(sriovv1.AddToScheme); err != nil {
		return fmt.Errorf("failed to register the SR-IOV scheme: %w", err)
	}

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		config := &sriovv1.SriovOperatorConfig{}
		if err := apiClient.Get(context.Background(), sriovOperatorConfigKey(), config); err != nil {
			return err
		}

		config.Spec.ConfigDaemonNodeSelector = nil

		return apiClient.Update(context.Background(), config)
	})
	if err != nil {
		return fmt.Errorf("failed to clear configDaemonNodeSelector on %s: %w", sriovOperatorConfigName, err)
	}

	return nil
}

// waitForConfigDaemonGone waits until the config daemon DaemonSet has no pods.
func waitForConfigDaemonGone(apiClient *clients.Settings) error {
	err := wait.PollUntilContextTimeout(
		context.Background(), 2*time.Second, configDaemonRemovedTimeout, true,
		func(ctx context.Context) (bool, error) {
			daemon, err := apiClient.DaemonSets(sriovOperatorNamespace).Get(
				ctx, sriovConfigDaemonDaemonSet, metav1.GetOptions{})
			if err != nil {
				return false, err
			}

			return daemon.Status.DesiredNumberScheduled == 0 && daemon.Status.CurrentNumberScheduled == 0, nil
		})
	if err != nil {
		return fmt.Errorf("timed out waiting for DaemonSet %s to have desired 0: %w", sriovConfigDaemonDaemonSet, err)
	}

	return nil
}

// networkManagerManaged reports whether NetworkManager manages the device.
func networkManagerManaged(apiClient *clients.Settings, nodeName string, ifaceName Iface) (bool, error) {
	output, err := execOnHost(apiClient, nodeName,
		fmt.Sprintf("nmcli -g GENERAL.NM-MANAGED device show %s", ifaceName))
	if err != nil {
		return false, fmt.Errorf("failed to read NetworkManager state for interface %s on node %s: %w",
			ifaceName, nodeName, err)
	}

	switch strings.ToLower(strings.TrimSpace(output)) {
	case "yes":
		return true, nil
	case "no":
		return false, nil
	default:
		return false, fmt.Errorf(
			"unexpected NetworkManager managed state %q for interface %s on node %s", output, ifaceName, nodeName)
	}
}

// setNetworkManagerManaged sets the runtime managed flag for the device.
func setNetworkManagerManaged(apiClient *clients.Settings, nodeName string, ifaceName Iface, managed bool) error {
	value := "no"
	if managed {
		value = "yes"
	}

	_, err := execOnHost(apiClient, nodeName, fmt.Sprintf("nmcli device set %s managed %s", ifaceName, value))
	if err != nil {
		return fmt.Errorf("failed to set NetworkManager managed %s for interface %s on node %s: %w",
			value, ifaceName, nodeName, err)
	}

	return nil
}

// execOnHost runs a command in the host mount namespace from the PTP daemon pod.
func execOnHost(apiClient *clients.Settings, nodeName string, command string) (string, error) {
	return ptpdaemon.ExecuteCommandInPtpDaemonPod(apiClient, nodeName, "nsenter -t 1 -m -- "+command,
		ptpdaemon.WithRetries(3), ptpdaemon.WithRetryOnError(true))
}

// sriovOperatorConfigKey is the object key of the default SriovOperatorConfig.
func sriovOperatorConfigKey() runtimeclient.ObjectKey {
	return runtimeclient.ObjectKey{Name: sriovOperatorConfigName, Namespace: sriovOperatorNamespace}
}

// downInterfaceKey identifies one interface set down on one node.
func downInterfaceKey(nodeName string, ifaceName Iface) string {
	return nodeName + "/" + string(ifaceName)
}
