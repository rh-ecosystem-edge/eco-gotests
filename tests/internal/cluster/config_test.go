package cluster

import (
	"fmt"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	configv1fake "github.com/openshift/client-go/config/clientset/versioned/fake"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

type MockAPIClientGetter struct {
	testClients *clients.Settings
}

// NewMockAPIClientGetter returns a new instance of MockAPIClientGetter.
func NewMockAPIClientGetter() *MockAPIClientGetter {
	return &MockAPIClientGetter{}
}

// SetFakeOCPClient sets a fake k8s client for testing purposes.
func (m *MockAPIClientGetter) SetFakeOCPClient(runtimeObjs []runtime.Object) {
	configv1Client := configv1fake.NewSimpleClientset(runtimeObjs...)

	myClient := clients.GetTestClients(clients.TestClientParams{
		K8sMockObjects: runtimeObjs,
		SchemeAttachers: []clients.SchemeAttacher{
			configv1.Install,
		},
	})

	myClient.ConfigV1Interface = configv1Client.ConfigV1()

	m.testClients = myClient
}

// SetFakeK8sClient sets a fake k8s client for testing purposes.
func (m *MockAPIClientGetter) SetFakeK8sClient(runtimeObjs []runtime.Object) {
	m.testClients = &clients.Settings{
		K8sClient:       k8sfake.NewSimpleClientset(runtimeObjs...),
		CoreV1Interface: k8sfake.NewSimpleClientset(runtimeObjs...).CoreV1(),
	}
}

// GetAPIClient returns the fake k8s client settings struct.
func (m *MockAPIClientGetter) GetAPIClient() (*clients.Settings, error) {
	if m.testClients == nil {
		return nil, fmt.Errorf("apiClient cannot be nil")
	}

	return m.testClients, nil
}

func generateFakeClusterVersion(version string) runtime.Object {
	return &configv1.ClusterVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name: "version",
		},
		Status: configv1.ClusterVersionStatus{
			Desired: configv1.Release{
				Version: version,
			},
		},

		Spec: configv1.ClusterVersionSpec{
			ClusterID: "fake-cluster-id",
		},
	}
}

func generateFakeClusterVersionWithConnectionInfo(connected bool) runtime.Object {
	if connected {
		return &configv1.ClusterVersion{
			ObjectMeta: metav1.ObjectMeta{
				Name: "version",
			},
			Status: configv1.ClusterVersionStatus{
				Conditions: []configv1.ClusterOperatorStatusCondition{
					{
						Type:   configv1.RetrievedUpdates,
						Reason: "VersionNotFound",
					},
				},
			},
		}
	}

	return &configv1.ClusterVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name: "version",
		},
		Status: configv1.ClusterVersionStatus{
			Conditions: []configv1.ClusterOperatorStatusCondition{
				{
					Type:   configv1.RetrievedUpdates,
					Reason: "RemoteFailed",
				},
			},
		},
	}
}

func generateFakeNetworkConfig() runtime.Object {
	return &configv1.Network{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
	}
}

func generateFakeK8sSecret(containsDockerJSON bool, name, namespace string) runtime.Object {
	testSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	if containsDockerJSON {
		testSecret.Data = map[string][]byte{
			".dockerconfigjson": []byte("fake-docker-json"),
		}
	}

	return testSecret
}

func generateFakeConfigObject(name string) runtime.Object {
	return &configv1.Proxy{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

func TestGetOCPClusterVersion(t *testing.T) {
	testCases := []struct {
		versionExists bool
	}{
		{
			versionExists: true,
		},
		{
			versionExists: false,
		},
	}

	for _, testCase := range testCases {
		var runtimeObjs []runtime.Object

		if testCase.versionExists {
			runtimeObjs = append(runtimeObjs, generateFakeClusterVersion("4.6.0"))
		}

		mockAPIClient := NewMockAPIClientGetter()
		mockAPIClient.SetFakeOCPClient(runtimeObjs)

		clusterVersion, err := GetOCPClusterVersion(mockAPIClient)
		if testCase.versionExists {
			assert.Nil(t, err)
			assert.Equal(t, "4.6.0", clusterVersion.Object.Status.Desired.Version)
		} else {
			assert.NotNil(t, err)
		}
	}
}

func TestGetOCPNetworkConfig(t *testing.T) {
	testCases := []struct {
		networkConfigExists bool
	}{
		{
			networkConfigExists: true,
		},
		{
			networkConfigExists: false,
		},
	}

	for _, testCase := range testCases {
		var runtimeObjs []runtime.Object
		if testCase.networkConfigExists {
			runtimeObjs = append(runtimeObjs, generateFakeNetworkConfig())
		}

		mockAPIClient := NewMockAPIClientGetter()
		mockAPIClient.SetFakeOCPClient(runtimeObjs)

		networkConfig, err := GetOCPNetworkConfig(mockAPIClient)

		if testCase.networkConfigExists {
			assert.Nil(t, err)
			assert.Equal(t, "cluster", networkConfig.Object.Name)
		} else {
			assert.NotNil(t, err)
		}
	}
}

func TestGetOCPPullSecret(t *testing.T) {
	testCases := []struct {
		containsDockerJSON bool
		expectedError      bool
		secretExists       bool
		name               string
		namespace          string
	}{
		{
			containsDockerJSON: true,
			expectedError:      false,
			secretExists:       true,
			name:               "pull-secret",
			namespace:          "openshift-config",
		},
		{
			containsDockerJSON: false,
			expectedError:      true,
			secretExists:       true,
			name:               "pull-secret",
			namespace:          "openshift-config",
		},
		{
			containsDockerJSON: true,
			expectedError:      true,
			secretExists:       false,
			name:               "pull-secret",
			namespace:          "openshift-config",
		},
	}

	for _, testCase := range testCases {
		mockAPIClient := NewMockAPIClientGetter()

		var runtimeObjs []runtime.Object

		if testCase.secretExists {
			runtimeObjs = append(runtimeObjs,
				generateFakeK8sSecret(testCase.containsDockerJSON, testCase.name, testCase.namespace))
		}

		mockAPIClient.SetFakeK8sClient(runtimeObjs)

		secretObj, err := GetOCPPullSecret(mockAPIClient)

		if testCase.expectedError {
			assert.NotNil(t, err)
		} else {
			assert.Nil(t, err)
			assert.Equal(t, "pull-secret", secretObj.Object.Name)
			assert.Equal(t, "openshift-config", secretObj.Object.Namespace)
		}
	}
}

func TestGetOCPProxy(t *testing.T) {
	testCases := []struct {
		proxyExists bool
	}{
		{
			proxyExists: true,
		},
		{
			proxyExists: false,
		},
	}

	for _, testCase := range testCases {
		mockAPIClient := NewMockAPIClientGetter()

		var runtimeObjs []runtime.Object
		if testCase.proxyExists {
			runtimeObjs = append(runtimeObjs, generateFakeConfigObject("cluster"))
		}

		mockAPIClient.SetFakeOCPClient(runtimeObjs)

		proxy, err := GetOCPProxy(mockAPIClient)

		if testCase.proxyExists {
			assert.Nil(t, err)
			assert.Equal(t, "cluster", proxy.Object.Name)
		} else {
			assert.NotNil(t, err)
		}
	}
}

func TestConnected(t *testing.T) {
	testCases := []struct {
		versionExists bool
		connected     bool
		validAPI      bool
	}{
		{
			versionExists: true,
			connected:     true,
			validAPI:      true,
		},
		{
			versionExists: true,
			connected:     false,
			validAPI:      true,
		},
		{
			versionExists: false,
			connected:     false,
			validAPI:      true,
		},
		{
			versionExists: true,
			connected:     true,
			validAPI:      false,
		},
	}

	for _, testCase := range testCases {
		var runtimeObjs []runtime.Object

		if testCase.versionExists {
			runtimeObjs = append(runtimeObjs, generateFakeClusterVersionWithConnectionInfo(testCase.connected))
		}

		mockAPIClient := NewMockAPIClientGetter()

		if testCase.validAPI {
			mockAPIClient.SetFakeOCPClient(runtimeObjs)
		}

		connectionResult, err := Connected(mockAPIClient)

		if testCase.validAPI {
			if testCase.versionExists {
				assert.Nil(t, err)

				if testCase.connected {
					assert.True(t, connectionResult)
				} else {
					assert.False(t, connectionResult)
				}
			} else {
				assert.NotNil(t, err)
			}
		} else {
			assert.NotNil(t, err)
			assert.Equal(t, err.Error(), "failed to get clusterversion from cluster: apiClient cannot be nil")
		}
	}
}

func TestDisconnected(t *testing.T) {
	testCases := []struct {
		versionExists bool
		connected     bool
		validAPI      bool
	}{
		{
			versionExists: true,
			connected:     true,
			validAPI:      true,
		},
		{
			versionExists: true,
			connected:     false,
			validAPI:      true,
		},
		{
			versionExists: false,
			connected:     false,
			validAPI:      true,
		},
		{
			versionExists: true,
			connected:     true,
			validAPI:      false,
		},
	}

	for _, testCase := range testCases {
		var runtimeObjs []runtime.Object

		if testCase.versionExists {
			runtimeObjs = append(runtimeObjs, generateFakeClusterVersionWithConnectionInfo(testCase.connected))
		}

		mockAPIClient := NewMockAPIClientGetter()

		if testCase.validAPI {
			mockAPIClient.SetFakeOCPClient(runtimeObjs)
		}

		connectionResult, err := Disconnected(mockAPIClient)

		if testCase.validAPI {
			if testCase.versionExists {
				assert.Nil(t, err)

				if testCase.connected {
					assert.False(t, connectionResult)
				} else {
					assert.True(t, connectionResult)
				}
			} else {
				assert.NotNil(t, err)
			}
		} else {
			assert.NotNil(t, err)
			assert.Equal(t, err.Error(), "failed to get clusterversion from cluster: apiClient cannot be nil")
		}
	}
}
