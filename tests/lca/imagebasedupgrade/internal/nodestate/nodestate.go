package nodestate

import (
	"context"
	"net"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/lca"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/internal/ibuparams"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// WaitForNodeToBeUnreachable waits the specified timeout for the host to be unreachable on the provided TCP port.
func WaitForNodeToBeUnreachable(host, port string, timeout time.Duration) (bool, error) {
	err := wait.PollUntilContextTimeout(
		context.TODO(), time.Second*3, timeout, true, func(ctx context.Context) (bool, error) {
			klog.V(ibuparams.IBULogLevel).Infof("Waiting for node %s to become unreachable", host)

			return !CheckIfNodeReachableOnPort(host, port), nil
		},
	)
	if err != nil {
		return false, err
	}

	return true, nil
}

// WaitForNodeToBeReachable waits the specified timeout for the host to be reachable on the provided TCP port.
func WaitForNodeToBeReachable(host, port string, timeout time.Duration) (bool, error) {
	err := wait.PollUntilContextTimeout(
		context.TODO(), time.Second*3, timeout, true, func(ctx context.Context) (bool, error) {
			klog.V(ibuparams.IBULogLevel).Infof("Waiting for node %s to become reachable", host)

			return CheckIfNodeReachableOnPort(host, port), nil
		},
	)
	if err != nil {
		return false, err
	}

	return true, nil
}

// CheckIfNodeReachableOnPort checks that the provided node is reachable on a specific TCP port.
func CheckIfNodeReachableOnPort(host, port string) bool {
	klog.V(ibuparams.IBULogLevel).Infof("Attempting to contact node %s on port %s", host, port)

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), time.Second*3)
	if err != nil {
		return false
	}

	if conn != nil {
		defer conn.Close()

		return true
	}

	return false
}

// WaitForIBUToBeAvailable waits the specified timeout for the ibu resource to be retrievable.
func WaitForIBUToBeAvailable(
	apiClient *clients.Settings,
	ibu *lca.ImageBasedUpgradeBuilder,
	timeout time.Duration) error {
	err := wait.PollUntilContextTimeout(
		context.TODO(), time.Second*3, timeout, true, func(ctx context.Context) (bool, error) {
			var err error

			ibu.Object, err = ibu.Get()
			if err != nil {
				return false, nil
			}

			ibu.Definition = ibu.Object

			return true, nil
		},
	)

	return err
}
