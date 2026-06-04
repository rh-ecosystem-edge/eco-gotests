package nhcparams

const (
	// Label represents nhc operator label that can be used for test cases selection.
	Label = "nhc"

	// LabelSuddenLoss is the label for the sudden-loss test scenario.
	LabelSuddenLoss = "sudden-loss"

	// NHCResourceName is the name of the NodeHealthCheck CR.
	NHCResourceName = "nhc-worker-self"

	// AppNamespace is the namespace for the stateful test application.
	AppNamespace = "stateful-app-test"

	// AppName is the name of the stateful test deployment.
	AppName = "stateful-app"

	// AppLabelKey is the label key for the stateful test application.
	AppLabelKey = "app"

	// AppLabelValue is the label value for the stateful test application.
	AppLabelValue = "stateful-app"

	// AppWorkerLabel is the node label used to select worker nodes for the test app.
	AppWorkerLabel = "node-role.kubernetes.io/appworker"

	// PVCName is the name of the PersistentVolumeClaim for the test app.
	PVCName = "app-data"

	// PVCSize is the size of the PVC for the test app.
	PVCSize = "1Gi"
)
