package mount

import (
	"fmt"
	"strings"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/talm/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	"k8s.io/klog/v2"
)

// PrepareEnvWithSmallMountPoint creates a virtual filesystem mounted at the backup path using a loopback device backed
// by a file in the test path. Some helpful links for understanding the process: https://stackoverflow.com/q/16044204
// and https://youtu.be/r9CQhwci4tE
func PrepareEnvWithSmallMountPoint(client *clients.Settings) (string, error) {
	// create the backup and test paths if they don't exist
	_, err := cluster.ExecCommandOnSNOWithRetries(client, ranparam.RetryCount, ranparam.RetryInterval,
		fmt.Sprintf("sudo mkdir -p %s", tsparams.BackupPath))
	if err != nil {
		return "", err
	}

	_, err = cluster.ExecCommandOnSNOWithRetries(client, ranparam.RetryCount, ranparam.RetryInterval,
		fmt.Sprintf("sudo mkdir -p %s", tsparams.RANTestPath))
	if err != nil {
		return "", err
	}

	// find the next available loopback device (OS takes care of creating a new one if needed)
	loopbackDevicePath, err := execWhileBlank(client, "sudo losetup -f")
	if err != nil {
		return "", err
	}

	klog.V(tsparams.LogLevel).Info("loopback device path: ", loopbackDevicePath)

	// create a file with desired size for the filesystem to use
	_, err = cluster.ExecCommandOnSNOWithRetries(
		client, ranparam.RetryCount, ranparam.RetryInterval,
		fmt.Sprintf("sudo fallocate -l %s %s/%s.img", tsparams.FSSize, tsparams.RANTestPath, tsparams.FSSize))
	if err != nil {
		return "", err
	}

	// create the loopback device by assigning it with the file
	_, err = cluster.ExecCommandOnSNOWithRetries(
		client, ranparam.RetryCount, ranparam.RetryInterval,
		fmt.Sprintf("sudo losetup %s %s/%s.img", loopbackDevicePath, tsparams.RANTestPath, tsparams.FSSize))
	if err != nil {
		return "", err
	}

	// format the loopback device with xfs
	_, err = cluster.ExecCommandOnSNOWithRetries(client, ranparam.RetryCount, ranparam.RetryInterval,
		fmt.Sprintf("sudo mkfs.xfs -f -q %s", loopbackDevicePath))
	if err != nil {
		return "", err
	}

	// mount the fs to backup dir
	_, err = cluster.ExecCommandOnSNOWithRetries(client, ranparam.RetryCount, ranparam.RetryInterval,
		fmt.Sprintf("sudo mount %s %s",
			loopbackDevicePath, tsparams.BackupPath))

	return loopbackDevicePath, err
}

// DiskFullEnvCleanup clean all the resources created for single cluster backup fail.
func DiskFullEnvCleanup(client *clients.Settings, loopbackDevicePath string) error {
	findmntOutput, err := execWhileBlank(client, fmt.Sprintf("findmnt -n -o SOURCE --target %s", tsparams.BackupPath))
	if err != nil {
		return err
	}

	safeToDeleteBackupDir, err := unmountLoopback(client, loopbackDevicePath, findmntOutput)
	if err != nil {
		return err
	}

	if safeToDeleteBackupDir {
		_, err = cluster.ExecCommandOnSNOWithRetries(client, ranparam.RetryCount, ranparam.RetryInterval,
			fmt.Sprintf("sudo rm -rf %s", tsparams.BackupPath))
		if err != nil {
			return err
		}
	} else {
		// if false there was a partition (most likely ZTP w/ MC) so delete content instead of the whole thing
		_, err = cluster.ExecCommandOnSNOWithRetries(client, ranparam.RetryCount, ranparam.RetryInterval,
			fmt.Sprintf("sudo rm -rf %s/*", tsparams.BackupPath))
		if err != nil {
			return err
		}
	}

	_, err = cluster.ExecCommandOnSNOWithRetries(client, ranparam.RetryCount, ranparam.RetryInterval,
		fmt.Sprintf("sudo rm -rf %s", tsparams.RANTestPath))

	return err
}

func unmountLoopback(client *clients.Settings, loopbackDevicePath, findmntOutput string) (bool, error) {
	if findmntOutput == "" {
		return true, nil
	}

	safeToDeleteBackupDir := true

	outputArr := strings.Split(findmntOutput, "\n")
	for _, devicePath := range outputArr {
		if devicePath == "" {
			continue
		}

		deviceType, err := execWhileBlank(client, fmt.Sprintf("lsblk %s -o TYPE -n", devicePath))
		if err != nil {
			return false, err
		}

		switch deviceType {
		case "part":
			safeToDeleteBackupDir = false

			klog.V(tsparams.LogLevel).Infof(
				"partition detected for %s, will not attempt to delete the folder (only the content if any)", tsparams.BackupPath)
		case "loop":
			if loopbackDevicePath == devicePath {
				// unmount and detach the loop device
				_, err := cluster.ExecCommandOnSNOWithRetries(client, ranparam.RetryCount,
					ranparam.RetryInterval, fmt.Sprintf("sudo umount --detach-loop %s",
						tsparams.BackupPath))
				if err != nil {
					return false, err
				}
			} else {
				safeToDeleteBackupDir = false

				klog.V(tsparams.LogLevel).Info("found unexpected loopback device, likely error with previous clean up")
				/*
					Assuming loop0 is the unwanted one...
					look for clues with lsblk
					$ lsblk
					NAME   MAJ:MIN RM   SIZE RO TYPE MOUNTPOINT
					loop0    7:0    0   100M  0 loop /var/recovery -----> this line should not be there

					unmount it with: `sudo umount --detach-loop /var/recovery`
					check lsblk to verify there's nothing mounted to loop0 and line is gone completely

					if line is still there (but unmounted) make use `losetup` to see the status of loopdevice (loop0)
					$ losetup
					NAME       SIZELIMIT OFFSET AUTOCLEAR RO BACK-FILE                                      DIO LOG-SEC
					/dev/loop0         0      0         1  0 /var/ran-test-talm-recovery/100M.img (deleted)   0     512

					if you see (deleted) -- reboot the node. i.e sudo reboot.
					Once back loop0 should not appear anywhere (lsblk + losetup)

				*/
				klog.V(tsparams.LogLevel).Infof("see comments for manual cleanup of %s", devicePath)
			}
		}
	}

	return safeToDeleteBackupDir, nil
}

// execWhileBlank runs the provided command on the cluster until the output, after trimming whitespace, is not empty.
func execWhileBlank(client *clients.Settings, command string) (string, error) {
	var (
		output  string
		err     error
		retries uint
	)

	// Just in case an infinite loop forms, allow a maximum of 10 retries. It should never require that many.
	for output == "" && retries < 10 {
		output, err = cluster.ExecCommandOnSNOWithRetries(client, ranparam.RetryCount, ranparam.RetryInterval, command)
		if err != nil {
			return "", err
		}

		output = strings.TrimSpace(output)
		retries++
	}

	return output, nil
}
