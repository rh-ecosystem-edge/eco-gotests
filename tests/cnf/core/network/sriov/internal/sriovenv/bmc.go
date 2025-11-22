package sriovenv

import (
	"context"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmc"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/tsparams"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// ConfigureSecureBoot enables or disables a SecureBoot on a BMC machine.
func ConfigureSecureBoot(bmcClient *bmc.BMC, action string) error {
	switch action {
	case "enable":
		klog.V(90).Infof("Enabling SecureBoot")

		err := wait.PollUntilContextTimeout(
			context.TODO(),
			tsparams.PollingIntervalBMC,
			tsparams.DefaultTimeout,
			true,
			func(ctx context.Context) (bool, error) {
				err := bmcClient.SecureBootEnable()
				if err == nil {
					klog.V(90).Infof("Successfully enabled Secure Boot")

					return true, nil
				}

				if err.Error() == "secure boot is already enabled" {
					klog.V(90).Infof("Secure Boot is already enabled")

					return true, nil
				}

				return false, nil
			})
		if err != nil {
			klog.V(90).Infof("Failed to enable secure boot")

			return err
		}
	case "disable":
		klog.V(90).Infof("Disabling SecureBoot")

		err := wait.PollUntilContextTimeout(
			context.TODO(),
			tsparams.PollingIntervalBMC,
			tsparams.DefaultTimeout,
			true,
			func(ctx context.Context) (bool, error) {
				err := bmcClient.SecureBootDisable()
				if err == nil {
					klog.V(90).Infof("Successfully disabled Secure Boot")

					return true, nil
				}

				return false, nil
			})
		if err != nil {
			klog.V(90).Infof("Failed to disable secure boot")

			return err
		}
	default:
		klog.V(90).Infof("Wrong action")

		return fmt.Errorf("invalid action provided: %s. Allowed actions are 'enable' or 'disable'", action)
	}

	klog.V(90).Infof("Rebooting the node")

	err := powerCycleNode(bmcClient)
	if err != nil {
		klog.V(90).Infof("Failed to reboot the node")

		return err
	}

	klog.V(90).Infof("Waiting for the cluster becomes stable")

	return netenv.WaitForMcpStable(
		netinittools.APIClient, tsparams.MCOWaitTimeout, 1*time.Minute, netinittools.NetConfig.CnfMcpLabel)
}

// CreateBMCClient creates BMC client instance.
func CreateBMCClient() (*bmc.BMC, error) {
	klog.V(90).Infof("Creating BMC instance")

	bmcHosts, err := netinittools.NetConfig.GetBMCHostNames()
	if err != nil {
		klog.V(90).Infof("Failed to get BMC host names")

		return nil, err
	}

	bmcUser, err := netinittools.NetConfig.GetBMCHostUser()
	if err != nil {
		klog.V(90).Infof("Failed to get BMC host user")

		return nil, err
	}

	bmcPass, err := netinittools.NetConfig.GetBMCHostPass()
	if err != nil {
		klog.V(90).Infof("Failed to get BMC host password")

		return nil, err
	}

	return bmc.New(bmcHosts[0]).
		WithRedfishUser(bmcUser, bmcPass).
		WithRedfishTimeout(5*time.Minute).
		WithSSHUser(bmcUser, bmcPass), nil
}

func powerCycleNode(bmcClient *bmc.BMC) error {
	klog.V(90).Infof("Starting power cycle for the system")

	err := wait.PollUntilContextTimeout(
		context.TODO(),
		tsparams.PollingIntervalBMC,
		tsparams.DefaultTimeout,
		true,
		func(ctx context.Context) (bool, error) {
			if err := bmcClient.SystemPowerCycle(); err == nil {
				klog.V(90).Info("Successfully initiated the power cycle")

				return true, nil
			}

			klog.V(90).Infof("Retrying power cycle")

			return false, nil
		})
	if err != nil {
		klog.V(90).Infof("Failed to power cycle the system")

		return err
	}

	klog.V(90).Infof("Power cycle completed successfully")

	return nil
}
