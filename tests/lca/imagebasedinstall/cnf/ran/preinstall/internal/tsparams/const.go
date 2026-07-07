package tsparams

import "github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedinstall/cnf/ran/internal/ranparams"

const (
	// LabelSuite represents preinstall label that can be used for test cases selection.
	LabelSuite = "preinstall"
	// LabelEndToEndPreinstall represents e2e label that can be used for test cases selection.
	LabelEndToEndPreinstall = "e2e"

	// LogLevel custom loglevel for preinstall verbose mode.
	LogLevel = ranparams.RANLogLevel

	// PreinstallBMHName matches Ansible baremetal host metadata.name.
	PreinstallBMHName = "ibi-sno"
	// PreinstallBMHNamespace matches Ansible BMH namespace.
	PreinstallBMHNamespace = "openshift-machine-api"
	// PreinstallBMCSecretName matches Ansible BMC secret metadata.name.
	PreinstallBMCSecretName = "ibi-sno-bmc-secret"
)
