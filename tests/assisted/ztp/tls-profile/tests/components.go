package tls_profile_test

import (
	"crypto/tls"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/tlsprofile"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/tls-profile/internal/tsparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var capoa = &tlsprofile.Component{
	Name:        "CAPOA",
	Label:       tsparams.LabelCAPOATLSProfile,
	Namespace:   "multicluster-engine",
	RestartMode: tlsprofile.RestartModeContainerRestart,
	Endpoints: []tlsprofile.Endpoint{
		{
			ServiceName:    "capoa-bootstrap-webhook-service",
			LocalPort:      19443,
			RemotePort:     9443,
			DeploymentName: "capoa-bootstrap-controller-manager",
		},
		{
			ServiceName:    "capoa-controlplane-webhook-service",
			LocalPort:      19444,
			RemotePort:     9443,
			DeploymentName: "capoa-controlplane-controller-manager",
		},
	},
	Deployments: []tlsprofile.Deployment{
		{Name: "capoa-bootstrap-controller-manager", ContainerName: "manager"},
		{Name: "capoa-controlplane-controller-manager", ContainerName: "manager"},
	},
	ListPods: func(client *clients.Settings, ns string) ([]*pod.Builder, error) {
		return pod.ListByNamePattern(client, "capoa", ns)
	},
	ExpectedHealthyPods: 2,
	PodReadyTimeout:     5 * time.Minute,
	AutoRestartTimeout:  10 * time.Minute,
	HonoringLogPattern:  "honoring cluster-wide TLS profile",
	AllowedCipher:       tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	AllowedCipherAlt:    tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	DisallowedCipher:    tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,
	OldProfileCipher:    tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,
	WebhookTest: &tlsprofile.WebhookTestConfig{
		TestNamespace:      "tls-test-capoa",
		APIVersion:         "bootstrap.cluster.x-k8s.io/v1alpha2",
		Kind:               "OpenshiftAssistedConfig",
		ResourceName:       "test-webhook-validation",
		CreateSpec:         map[string]interface{}{"cpuArchitecture": "x86_64"},
		MutationPatch:      []byte(`{"spec":{"cpuArchitecture":"aarch64"}}`),
		RejectionSubstring: "immutable",
	},
}

var ibio = &tlsprofile.Component{
	Name:        "IBIO",
	Label:       tsparams.LabelIBIOTLSProfile,
	Namespace:   "multicluster-engine",
	RestartMode: tlsprofile.RestartModeContainerRestart,
	Endpoints: []tlsprofile.Endpoint{
		{
			ServiceName:    "image-based-install-webhook",
			LocalPort:      19445,
			RemotePort:     9443,
			DeploymentName: "image-based-install-operator",
		},
		{
			ServiceName:    "image-based-install-config",
			LocalPort:      19446,
			RemotePort:     8000,
			DeploymentName: "image-based-install-operator",
		},
	},
	Deployments: []tlsprofile.Deployment{
		{Name: "image-based-install-operator", ContainerName: "manager"},
	},
	ListPods: func(client *clients.Settings, ns string) ([]*pod.Builder, error) {
		return pod.List(client, ns, metav1.ListOptions{
			LabelSelector: "app=image-based-install-operator",
		})
	},
	ExpectedHealthyPods: 1,
	PodReadyTimeout:     5 * time.Minute,
	AutoRestartTimeout:  10 * time.Minute,
	HonoringLogPattern:  "Reconciling APIServer TLS profile",
	AllowedCipher:       tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	AllowedCipherAlt:    tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	DisallowedCipher:    tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,
	OldProfileCipher:    tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,
	WebhookTest: &tlsprofile.WebhookTestConfig{
		TestNamespace: "tls-test-ibio",
		APIVersion:    "extensions.hive.openshift.io/v1alpha1",
		Kind:          "ImageClusterInstall",
		ResourceName:  "test-webhook-validation",
		CreateSpec: map[string]interface{}{
			"clusterDeploymentRef": map[string]interface{}{"name": "tls-test-cd"},
			"imageSetRef":          map[string]interface{}{"name": "tls-test-imageset"},
			"hostname":             "tls-test-host",
			"version":              "4.17.0",
		},
		MutationPatch:      []byte(`{"spec":{"clusterDeploymentRef":{"name":"different-cd-name"}}}`),
		RejectionSubstring: "immutable",
	},
}

var allComponents = []*tlsprofile.Component{capoa, ibio}
