package rdscorecommon

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"
)

const (
	// IptablesAlerterNamespace is the namespace where iptables-alerter runs.
	IptablesAlerterNamespace = "openshift-network-operator"
	// IptablesAlerterConfigMapName is the ConfigMap used to disable iptables-alerter.
	IptablesAlerterConfigMapName = "iptables-alerter-config"
	// IptablesAlerterDisableKey is the ConfigMap key to disable iptables-alerter.
	IptablesAlerterDisableKey = "enabled"
	// IptablesAlerterPodLabel is the label selector for iptables-alerter pods.
	IptablesAlerterPodLabel = "app=iptables-alerter"
)

// VerifyIptablesAlerterDisabled verifies that the iptables-alerter component is disabled
// in the cluster. This is a critical configuration for Telco production clusters to avoid
// CPU usage issues and cluster instability.
//
// References:
// - KCS: https://access.redhat.com/solutions/7134521
// - OCPBUGS-87026: NF Pod Readiness Probe Failures in Production Cluster
// - OCPBUGS-73767: High CPU usage for iptables-alerter pods
//
// The test verifies:
// 1. ConfigMap "iptables-alerter-config" exists in "openshift-network-operator" namespace
// 2. ConfigMap contains "enabled: false"
// 3. No iptables-alerter pods are running in the cluster.
func VerifyIptablesAlerterDisabled(ctx SpecContext) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Verifying iptables-alerter is disabled")

	By("Checking iptables-alerter ConfigMap exists")

	alerterConfigMap, err := configmap.Pull(
		APIClient,
		IptablesAlerterConfigMapName,
		IptablesAlerterNamespace,
	)

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to get ConfigMap %s/%s: %v",
			IptablesAlerterNamespace, IptablesAlerterConfigMapName, err))
	Expect(alerterConfigMap).ToNot(BeNil(),
		fmt.Sprintf("ConfigMap %s/%s not found",
			IptablesAlerterNamespace, IptablesAlerterConfigMapName))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Found ConfigMap %s/%s",
		IptablesAlerterNamespace,
		IptablesAlerterConfigMapName,
	)

	By("Verifying 'enabled' is set to 'false'")

	enabledValue, exists := alerterConfigMap.Object.Data[IptablesAlerterDisableKey]

	Expect(exists).To(BeTrue(),
		fmt.Sprintf("ConfigMap %s/%s missing key %q",
			IptablesAlerterNamespace, IptablesAlerterConfigMapName, IptablesAlerterDisableKey))
	Expect(enabledValue).To(Equal("false"),
		fmt.Sprintf("Expected %q to be 'false', got %q", IptablesAlerterDisableKey, enabledValue))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"ConfigMap %s/%s correctly has %s='false'",
		IptablesAlerterNamespace,
		IptablesAlerterConfigMapName,
		IptablesAlerterDisableKey,
	)

	By("Verifying no iptables-alerter pods are running")

	alerterPods, err := pod.List(
		APIClient,
		IptablesAlerterNamespace,
		metav1.ListOptions{LabelSelector: IptablesAlerterPodLabel},
	)

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to list iptables-alerter pods: %v", err))
	Expect(len(alerterPods)).To(Equal(0),
		fmt.Sprintf("Found %d iptables-alerter pods running, expected 0 (iptables-alerter should be disabled)",
			len(alerterPods)))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Verified: no iptables-alerter pods running in cluster",
	)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"iptables-alerter is correctly disabled - verification complete",
	)
}
