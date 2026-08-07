package tsparams

import (
	"time"

	"k8s.io/klog/v2"
)

const (
	// LabelSuite represents ibipreinstall label that can be used for test cases selection.
	LabelSuite = "ibipreinstall"
	// LabelEndToEndPreinstall represents e2e label that can be used for test cases selection.
	LabelEndToEndPreinstall = "preinstall-e2e"

	// LogLevel custom loglevel for ibi preinstall verbose mode.
	LogLevel klog.Level = 90

	// PreinstallBMHNamespace is the namespace for the preinstall BMH.
	PreinstallBMHNamespace = "openshift-machine-api"

	// PreinstallServiceUnit is the systemd unit that performs the IBI
	// preinstall (RHCOS write + seed restore) on the spoke node.
	PreinstallServiceUnit = "install-rhcos-and-restore-seed.service"

	// PreinstallWaitTimeout is the timeout for preinstall completion polling.
	PreinstallWaitTimeout = 60 * time.Minute

	// TargetNodeSSHUser is the SSH user for the preinstalled target node (always core on OCP).
	TargetNodeSSHUser = "core"

	// ISOFilename is the output filename from openshift-install image-based create image.
	ISOFilename = "rhcos-ibi.iso"
)
