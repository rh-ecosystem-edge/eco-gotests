package rdscorecommon

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const (
	uncordonNodeInterval = 15 * time.Second
	uncordonNodeTimeout  = 3 * time.Minute
)

// UncordonNode uncordons a node referenced by nodeToUncordon parameter.
// It retries uncordoning for the specified timeout duration at regular intervals.
func UncordonNode(nodeToUncordon *nodes.Builder, interval, timeout time.Duration) {
	By(fmt.Sprintf("Uncordoning node %q", nodeToUncordon.Definition.Name))

	err := wait.PollUntilContextTimeout(context.TODO(), interval, timeout, true,
		func(context.Context) (bool, error) {
			err := nodeToUncordon.Uncordon()
			if err != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to uncordon %q: %v", nodeToUncordon.Definition.Name, err)

				return false, nil
			}

			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Successfully uncordon %q", nodeToUncordon.Definition.Name)

			return err == nil, nil
		})
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to uncordon %q: %v", nodeToUncordon.Definition.Name, err)
	}
}
