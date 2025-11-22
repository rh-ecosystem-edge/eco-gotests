package vcorecommon

import (
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodesconfig"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/cgroup"
	"k8s.io/klog/v2"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreparams"
)

// VerifyCGroupDefault container that contains tests for cgroup verification.
func VerifyCGroupDefault() {
	Describe(
		"cgroup verification",
		Label(vcoreparams.LabelVCoreDeployment), func() {
			It("Verifies cgroupv2 is a default for the cluster deployment",
				Label("cgroupv2"), reportxml.ID("73370"), VerifyCGroupV2IsADefault)

			It("Verifies that the cluster can be moved to the cgroupv1",
				Label("cgroupv2"), reportxml.ID("73371"), VerifySwitchBetweenCGroupVersions)
		})
}

// VerifyCGroupV2IsADefault assert cGroupV2 is a default for the cluster deployment.
func VerifyCGroupV2IsADefault(ctx SpecContext) {
	klog.V(vcoreparams.VCoreLogLevel).Infof("Verify cgroupv2 is a default for the cluster deployment")

	klog.V(vcoreparams.VCoreLogLevel).Infof("Get current cgroup mode configured for the cluster")

	nodesConfigObj, err := nodesconfig.Pull(APIClient, "cluster")
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to get nodes.config 'cluster' object due to %v", err))

	cgroupMode, err := nodesConfigObj.GetCGroupMode()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to get cluster cgroup mode due to %v", err))
	Expect(cgroupMode).ToNot(Equal(configv1.CgroupModeV1), "wrong cgroup mode default found, v1 instead of v2")

	klog.V(vcoreparams.VCoreLogLevel).Infof("Verify actual cgroup version configured for the nodes")

	nodesList, err := nodes.List(APIClient)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to get cluster nodes list due to %v", err))

	for _, node := range nodesList {
		klog.V(vcoreparams.VCoreLogLevel).Infof("Verify actual cgroup version configured for node %s",
			node.Definition.Name)

		currentCgroupMode, err := cgroup.GetNodeLinuxCGroupVersion(APIClient, node.Definition.Name)
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("failed to get cgroup mode value for the node %s due to %v",
				node.Definition.Name, err))
		Expect(currentCgroupMode).To(Equal(configv1.CgroupModeV2), fmt.Sprintf("wrong cgroup mode default "+
			"found configured for the node %s; expected cgroupMode is %v, found cgroupMode is %v",
			node.Definition.Name, configv1.CgroupModeV2, currentCgroupMode))
	}
} // func VerifyCGroupV2IsADefault (ctx SpecContext)

// VerifySwitchBetweenCGroupVersions assert that the cluster can be moved to the cgroupv1 and back.
func VerifySwitchBetweenCGroupVersions(ctx SpecContext) {
	klog.V(vcoreparams.VCoreLogLevel).Infof("Verify that the cluster can be moved to the cgroupv1 and back")

	err := cgroup.SetLinuxCGroupVersion(APIClient, configv1.CgroupModeV1)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to change cluster cgroup mode to the %v due to %v",
		configv1.CgroupModeV1, err))

	klog.V(vcoreparams.VCoreLogLevel).Infof("For the vCore test run cgroup version will stay to be configured " +
		"to the v1 because cpu load balancing not supported on cgroupv2")
} // func VerifySwitchBetweenCGroupVersions (ctx SpecContext)
