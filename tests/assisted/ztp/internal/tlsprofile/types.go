package tlsprofile

import (
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
)

// RestartMode indicates how a component restarts after TLS profile changes.
type RestartMode int

const (
	// RestartModeContainerRestart means the container restarts in-place (same pod UID, restart count increases).
	RestartModeContainerRestart RestartMode = iota
	// RestartModePodReplacement means the pod is replaced entirely (new pod name).
	RestartModePodReplacement
)

// Endpoint represents a TLS-secured service endpoint to probe.
type Endpoint struct {
	ServiceName    string
	LocalPort      int
	RemotePort     int
	DeploymentName string
}

// Deployment represents a deployment whose pods should be tracked for logs and restarts.
type Deployment struct {
	Name          string
	ContainerName string
}

// WebhookTestConfig holds details for webhook validation testing after a TLS profile change.
type WebhookTestConfig struct {
	TestNamespace      string
	APIVersion         string
	Kind               string
	ResourceName       string
	CreateSpec         map[string]interface{}
	MutationPatch      []byte
	RejectionSubstring string
}

// Component fully describes a TLS-adherent component for testing.
type Component struct {
	Name                string
	Label               string
	Namespace           string
	RestartMode         RestartMode
	Endpoints           []Endpoint
	Deployments         []Deployment
	ListPods            func(*clients.Settings, string) ([]*pod.Builder, error)
	ExpectedHealthyPods int
	PodReadyTimeout     time.Duration
	AutoRestartTimeout  time.Duration
	HonoringLogPattern  string
	OldProfileLog       string
	ModernProfileLog    string
	WebhookTest         *WebhookTestConfig
	AllowedCipher       uint16
	AllowedCipherAlt    uint16
	DisallowedCipher    uint16
	OldProfileCipher    uint16
}
