package tsparams

const (
	// LabelSuite represents preinstall label that can be used for test cases selection.
	LabelSuite = "preinstall"
	// LabelEndToEndPreinstall represents e2e label that can be used for test cases selection.
	LabelEndToEndPreinstall = "e2e"

	// PreinstallBMHName matches Ansible baremetal host metadata.name.
	PreinstallBMHName = "ibi-sno"
	// PreinstallBMHNamespace matches Ansible BMH namespace.
	PreinstallBMHNamespace = "openshift-machine-api"
	// PreinstallBMCSecretName matches Ansible BMC secret metadata.name.
	PreinstallBMCSecretName = "ibi-sno-bmc-secret"
)
