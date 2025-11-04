package rdscoreparams

const (
	// Label is used to select tests for RDS Core setup.
	Label = "rdscore"

	// RDSCoreLogLevel configures logging level for RDS Core related tests.
	RDSCoreLogLevel = 90

	// NMStateInstanceName is a name of the NMState instance.
	NMStateInstanceName = "nmstate"

	// MachineConfidDaemonPodSelector is a a label selector for all machine-config-daemon pods.
	MachineConfidDaemonPodSelector = "k8s-app=machine-config-daemon"

	// LabelValidatePerformanceProfile is a test selector for performance profile validation.
	LabelValidatePerformanceProfile = "rds-core-performance-profile"

	// MachineConfigDaemonContainerName is a name of container within machine-config-daemon pod.
	MachineConfigDaemonContainerName = "machine-config-daemon"

	// LabelValidateNMState a label to select tests for NMState validation.
	LabelValidateNMState = "rds-core-validate-nmstate"

	// ConditionTypeReadyString constant to fix linter warning.
	ConditionTypeReadyString = "Ready"

	// ConstantTrueString constant to fix linter warning.
	ConstantTrueString = "True"

	// LabelValidateSRIOV a label to select tests for SR-IOV validation.
	LabelValidateSRIOV = "rds-core-validate-sriov"

	// MetalLBOperatorNamespace MetalLB operator namespace.
	MetalLBOperatorNamespace = "metallb-system"

	// MetalLBFRRPodSelector pod selector for MetalLB-FRR pods.
	MetalLBFRRPodSelector = "app=frr-k8s"

	// MetalLBFRRContainerName name of the FRR container within a pod.
	MetalLBFRRContainerName = "frr"

	// CLONamespace namespace of the CLO deployment.
	CLONamespace = "openshift-logging"

	// CLOName is a cluster logging operator name.
	CLOName = "cluster-logging"

	// CLODeploymentName is a cluster logging operator deployment name.
	CLODeploymentName = "cluster-logging-operator"

	// CLOInstanceName is a cluster logging instance name.
	CLOInstanceName = "instance"

	// PureStorageFileSCName is a pure storage file storage class name.
	PureStorageFileSCName = "fa-file-sc"
)
