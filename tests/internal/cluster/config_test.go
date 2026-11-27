//go:build unit_test

package cluster

import (
	"errors"
	"fmt"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type MockAPIClientGetter struct {
	testClients    *clients.Settings
	apiClientError error
	returnNil      bool
}

// NewMockAPIClientGetter returns a new instance of MockAPIClientGetter.
func NewMockAPIClientGetter() *MockAPIClientGetter {
	return &MockAPIClientGetter{}
}

// SetFakeOCPClient sets a fake k8s client for testing purposes.
func (m *MockAPIClientGetter) SetFakeOCPClient(runtimeObjs []runtime.Object) {
	m.testClients = clients.GetTestClients(clients.TestClientParams{
		K8sMockObjects: runtimeObjs,
		SchemeAttachers: []clients.SchemeAttacher{
			configv1.Install,
			corev1.AddToScheme,
		},
	})
}

// SetAPIClientError configures the mock to return an error when getting the API client.
func (m *MockAPIClientGetter) SetAPIClientError(err error) {
	m.apiClientError = err
}

// SetNilAPIClient configures the mock to return a nil API client without an error.
func (m *MockAPIClientGetter) SetNilAPIClient() {
	m.returnNil = true
}

// GetAPIClient returns the fake k8s client settings struct.
func (m *MockAPIClientGetter) GetAPIClient() (*clients.Settings, error) {
	if m.apiClientError != nil {
		return nil, m.apiClientError
	}

	if m.returnNil {
		return nil, nil //nolint:nilnil // The nil client is the behavior under test.
	}

	if m.testClients == nil {
		return nil, fmt.Errorf("apiClient cannot be nil")
	}

	return m.testClients, nil
}

//nolint:funlen // The table covers every cluster information helper and client failure mode.
func TestClusterInformationHelpersUnavailableAPIClient(t *testing.T) {
	const getterErrorMessage = "api client is unavailable"

	testCases := []struct {
		name    string
		call    func(APIClientGetter) (any, error)
		wrapErr string
	}{
		{
			name: "cluster version",
			call: func(clusterObj APIClientGetter) (any, error) {
				return GetOCPClusterVersion(clusterObj)
			},
		},
		{
			name: "network config",
			call: func(clusterObj APIClientGetter) (any, error) {
				return GetOCPNetworkConfig(clusterObj)
			},
		},
		{
			name: "network operator config",
			call: func(clusterObj APIClientGetter) (any, error) {
				return GetOCPNetworkOperatorConfig(clusterObj)
			},
		},
		{
			name: "pull secret",
			call: func(clusterObj APIClientGetter) (any, error) {
				return GetOCPPullSecret(clusterObj)
			},
		},
		{
			name: "proxy",
			call: func(clusterObj APIClientGetter) (any, error) {
				return GetOCPProxy(clusterObj)
			},
		},
		{
			name: "connected",
			call: func(clusterObj APIClientGetter) (any, error) {
				return Connected(clusterObj)
			},
			wrapErr: "failed to get clusterversion from cluster: ",
		},
		{
			name: "disconnected",
			call: func(clusterObj APIClientGetter) (any, error) {
				return Disconnected(clusterObj)
			},
			wrapErr: "failed to get clusterversion from cluster: ",
		},
		{
			name: "cluster name",
			call: func(clusterObj APIClientGetter) (any, error) {
				return GetOCPClusterName(clusterObj)
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			for _, unavailableClient := range []struct {
				name      string
				configure func(*MockAPIClientGetter)
			}{
				{
					name: "getter error",
					configure: func(mockAPIClient *MockAPIClientGetter) {
						mockAPIClient.SetAPIClientError(errors.New(getterErrorMessage))
					},
				},
				{
					name: "nil client",
					configure: func(mockAPIClient *MockAPIClientGetter) {
						mockAPIClient.SetNilAPIClient()
					},
				},
			} {
				t.Run(unavailableClient.name, func(t *testing.T) {
					mockAPIClient := NewMockAPIClientGetter()
					unavailableClient.configure(mockAPIClient)

					result, err := testCase.call(mockAPIClient)

					assert.Zero(t, result)

					expectedError := expectedUnavailableClientError(unavailableClient.name)
					switch {
					case testCase.wrapErr != "":
						expectedError = testCase.wrapErr + expectedError
					case unavailableClient.name == "getter error":
						expectedError = getterErrorMessage
					}

					assert.EqualError(t, err, expectedError)
				})
			}
		})
	}
}

func expectedUnavailableClientError(clientState string) string {
	if clientState == "getter error" {
		return "api client is unavailable"
	}

	return "cannot discover cluster information when APIClient is nil"
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

		mockAPIClient.SetFakeOCPClient(runtimeObjs)

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
