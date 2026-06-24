package helper

// UNIT_TEST=true go test ./tests/cnf/ran/gitopsztp/internal/helper/... -run TestHubHasClusterInstance

import (
	"testing"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	siteconfigv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/siteconfig/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestHubHasClusterInstance(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		objects  []runtime.Object
		expected bool
	}{
		{
			name:     "no cluster instances",
			objects:  nil,
			expected: false,
		},
		{
			name: "cluster instance present",
			objects: []runtime.Object{
				&siteconfigv1alpha1.ClusterInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "spoke1",
						Namespace: "spoke1",
					},
				},
			},
			expected: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			apiClient := clients.GetTestClients(clients.TestClientParams{
				K8sMockObjects: testCase.objects,
				SchemeAttachers: []clients.SchemeAttacher{
					siteconfigv1alpha1.AddToScheme,
				},
			})

			hasClusterInstance, err := HubHasClusterInstance(apiClient)
			assert.NoError(t, err)
			assert.Equal(t, testCase.expected, hasClusterInstance)
		})
	}

	_, err := HubHasClusterInstance(nil)
	assert.Error(t, err)
}
