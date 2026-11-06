package rhwaparams

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// SnrGVR is the GroupVersionResource for SelfNodeRemediation resources.
	SnrGVR = schema.GroupVersionResource{
		Group:    "self-node-remediation.medik8s.io",
		Version:  "v1alpha1",
		Resource: "selfnoderemediations",
	}
)
