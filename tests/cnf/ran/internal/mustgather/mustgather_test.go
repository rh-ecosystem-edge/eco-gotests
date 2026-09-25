//go:build unit_test

package mustgather

import (
	"testing"

	"github.com/onsi/ginkgo/v2/types"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaticImage(t *testing.T) {
	image, err := StaticImage("registry.example.com/must-gather:latest")(nil)

	require.NoError(t, err)
	assert.Equal(t, "registry.example.com/must-gather:latest", image)
}

func TestCollectIfFailedDoesNotResolveImageForPassingSpec(t *testing.T) {
	providerCalled := false
	imageProvider := func(_ *clients.Settings) (string, error) {
		providerCalled = true

		return "registry.example.com/must-gather:latest", nil
	}

	CollectIfFailed(types.SpecReport{State: types.SpecStatePassed}, "suite", nil, "test", imageProvider)

	assert.False(t, providerCalled)
}

func TestResourceNames(t *testing.T) {
	namespaceName, clusterRoleBindingName := resourceNames("o2ims", 12345)

	assert.Equal(t, "o2ims-must-gather-12345", namespaceName)
	assert.Equal(t, "o2ims-must-gather-admin-12345", clusterRoleBindingName)
}
