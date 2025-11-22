package vcorecommon

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/mirroring"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/platform"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreparams"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

func getImageURL(repository, name, tag string) (string, error) {
	imageURL := fmt.Sprintf("%s/%s", repository, name)

	isDisconnected, err := platform.IsDisconnectedDeployment(APIClient)
	if err != nil {
		return "", err
	}

	if !isDisconnected {
		klog.V(vcoreparams.VCoreLogLevel).Info("The connected deployment type was detected, " +
			"the images mirroring is not required")
	} else {
		klog.V(vcoreparams.VCoreLogLevel).Infof("Mirror image %s:%s locally", imageURL, tag)

		imageURL, _, err = mirroring.MirrorImageToTheLocalRegistry(
			APIClient,
			repository,
			name,
			tag,
			VCoreConfig.Host,
			VCoreConfig.User,
			VCoreConfig.Pass,
			VCoreConfig.RegistryRepository,
			VCoreConfig.KubeconfigPath)
		if err != nil {
			return "", fmt.Errorf("failed to mirror image %s:%s locally due to %w",
				name, tag, err)
		}
	}

	return fmt.Sprintf("%s:%s", imageURL, tag), nil
}

func ensureNamespaceNotExists(nsName string) bool {
	watchNamespace := namespace.NewBuilder(APIClient, nsName)
	if watchNamespace.Exists() {
		err := watchNamespace.Delete()
		gomega.Expect(err).ToNot(gomega.HaveOccurred(),
			fmt.Sprintf("Failed to delete watch namespace %s due to: %v",
				vcoreparams.KedaWatchNamespace, err))

		err = wait.PollUntilContextTimeout(
			context.TODO(),
			time.Second,
			time.Minute*10,
			true,
			func(ctx context.Context) (bool, error) {
				isExists := watchNamespace.Exists()

				if !isExists {
					return true, nil
				}

				return false, nil
			})
		gomega.Expect(err).ToNot(gomega.HaveOccurred(),
			fmt.Sprintf("Failed to delete watch namespace %s due to: %v",
				vcoreparams.KedaWatchNamespace, err))
	}

	return true
}

func ensureNamespaceExists(nsName string) bool {
	klog.V(vcoreparams.VCoreLogLevel).Infof("Ensure namespace %q exists", nsName)

	createNs := namespace.NewBuilder(APIClient, nsName)

	if !createNs.Exists() {
		createNs, err := createNs.Create()
		if err != nil {
			klog.V(vcoreparams.VCoreLogLevel).Infof("Error creating namespace %q: %v", nsName, err)

			return false
		}

		err = wait.PollUntilContextTimeout(
			context.TODO(),
			time.Second,
			3*time.Second,
			true,
			func(ctx context.Context) (bool, error) {
				if !createNs.Exists() {
					klog.V(vcoreparams.VCoreLogLevel).Infof("Error creating namespace %q", nsName)

					return false, nil
				}

				klog.V(vcoreparams.VCoreLogLevel).Infof("Created namespace %q", createNs.Definition.Name)

				return true, nil
			})
		if err != nil {
			return false
		}
	}

	return true
}
