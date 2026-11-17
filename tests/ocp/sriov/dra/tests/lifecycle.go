package tests

import (
	"context"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/daemonset"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/resource"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriovdra"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/dra/internal/draenv"
	draparams "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/dra/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	featureGate   = "dynamicResourceAllocation"
	driverDS      = "sriov-dra-driver"
	pluginDS      = "sriov-device-plugin"
	ownerLabel    = "sriovnetwork.openshift.io/generated-by"
	ownerValue    = "sriov-network-operator"
	resourceLabel = "sriovnetwork.openshift.io/resource-name"
	poolLabel     = "sriovnetwork.openshift.io/resource-pool"
	runLabel      = "eco-gotests.openshift.io/dra-run"
	nodeLabel     = "sriovnetwork.openshift.io/node"
)

type lifecycle struct {
	namespace, node, pf, name, hostname, runID string
	owned                                      map[string]types.UID
	flagPresent                                bool
}

// TODO: Assign official test-management IDs to the six cases before merge.
var _ = Describe("DRA provider lifecycle", Ordered, Serial, Label(tsparams.LabelDRA), func() {
	var state *lifecycle

	BeforeAll(func() {
		state = &lifecycle{
			namespace: SriovOcpConfig.OcpSriovOperatorNamespace,
			node:      SriovOcpConfig.DRANode, pf: SriovOcpConfig.DRAPF,
			name: "ecodra" + string(uuid.NewUUID())[:8], owned: map[string]types.UID{}, runID: string(uuid.NewUUID()),
		}

		By("Checking operator-wide prerequisites without deleting existing configuration")
		Expect(state.preflight()).To(Succeed())
		DeferCleanup(func() {
			Expect(state.cleanup()).To(Succeed(),
				"DRA cleanup failed; inspect test-owned policies and restore the recorded feature flag manually")
		})
	})
	It("deploys the DRA driver and removes the device plugin", func() {
		By("Enabling the operator-wide DRA provider")
		Eventually(func() error { return state.setFlag(true, true) },
			draparams.ReconcileTimeout, draparams.PollInterval).Should(Succeed())
		Eventually(func() error { return state.ready(driverDS, true) },
			draparams.ReconcileTimeout, draparams.PollInterval).Should(Succeed())
		Eventually(func() error { return state.absent(pluginDS) },
			draparams.ReconcileTimeout, draparams.PollInterval).Should(Succeed())
	})
	It("creates the operator-managed base DeviceClass", func() {
		Eventually(func() error { return state.class(draenv.Driver, false) },
			draparams.ReconcileTimeout, draparams.PollInterval).Should(Succeed())
	})
	It("reconciles a two-VF policy into DRA resources", func() {
		By("Creating a uniquely named policy on the explicitly selected node and PF")
		Expect(state.createPolicy()).To(Succeed())
		Eventually(func() error { return state.configured(draparams.InitialVFs) },
			draparams.ReconcileTimeout, draparams.PollInterval).Should(Succeed())
		Eventually(func() error { return state.ready(driverDS, true) },
			draparams.ReconcileTimeout, draparams.PollInterval).Should(Succeed())
		Eventually(func() error { return state.generated() },
			draparams.ReconcileTimeout, draparams.PollInterval).Should(Succeed())
	})
	It("advertises the initial VFs only on the selected node", func() {
		Eventually(func() error { return state.counts(draparams.InitialVFs) },
			draparams.ReconcileTimeout, draparams.PollInterval).Should(Succeed())
	})
	It("updates advertised VFs after an in-place policy update", func() {
		By("Updating NumVfs on the existing policy from two to four")
		Eventually(func() error {
			policy, err := state.policy()
			if err != nil {
				return err
			}

			if policy.Object.UID != state.owned[state.name] {
				return fmt.Errorf("policy ownership changed")
			}

			policy.Definition.Spec.NumVfs = draparams.UpdatedVFs
			_, err = policy.Update()

			return err
		},
			draparams.ReconcileTimeout, draparams.PollInterval).Should(Succeed())
		Eventually(func() error { return state.configured(draparams.UpdatedVFs) },
			draparams.ReconcileTimeout, draparams.PollInterval).Should(Succeed())
		Eventually(func() error { return state.counts(draparams.UpdatedVFs) },
			draparams.ReconcileTimeout, draparams.PollInterval).Should(Succeed())
	})
	It("removes DRA resources and restores the device plugin", func() {
		By("Removing the test policy before switching providers")
		Expect(state.deletePolicies()).To(Succeed())
		Eventually(func() error { return state.configured(0) },
			draparams.ReconcileTimeout, draparams.PollInterval).Should(Succeed())
		Eventually(func() error { return state.setFlag(false, true) },
			draparams.ReconcileTimeout, draparams.PollInterval).Should(Succeed())
		Eventually(func() error { return state.absent(driverDS) },
			draparams.ReconcileTimeout, draparams.PollInterval).Should(Succeed())
		Eventually(state.generatedAbsent, draparams.ReconcileTimeout, draparams.PollInterval).Should(Succeed())
		By("Creating the test policy in device-plugin mode")
		Expect(state.createPolicy()).To(Succeed())
		Eventually(func() error { return state.configured(draparams.InitialVFs) },
			draparams.ReconcileTimeout, draparams.PollInterval).Should(Succeed())
		Eventually(func() error { return state.ready(pluginDS, false) },
			draparams.ReconcileTimeout, draparams.PollInterval).Should(Succeed())
	})
})

func (s *lifecycle) preflight() error {
	cfg, err := pullConfig(s.namespace)
	if err != nil {
		return err
	}

	value, present := cfg.Object.Spec.FeatureGates[featureGate]
	s.flagPresent = present

	if value {
		return fmt.Errorf("DRA must be disabled before lifecycle testing")
	}

	policies, err := sriov.ListPolicy(APIClient, s.namespace, runtimeclient.ListOptions{Namespace: s.namespace})
	if err != nil {
		return err
	}

	for _, policyBuilder := range policies {
		if policyBuilder.Object.Name != "default" || policyBuilder.Object.Spec.NumVfs != 0 ||
			policyBuilder.Object.Spec.ResourceName != "" {
			return fmt.Errorf("existing policy %s prevents exclusive provider transition", policyBuilder.Object.Name)
		}
	}

	classes, err := resource.ListDeviceClasses(APIClient)
	if err != nil {
		return err
	}

	for _, deviceClass := range classes {
		if deviceClass.Object.Labels[ownerLabel] == ownerValue {
			return fmt.Errorf("existing operator DRA class %s must be resolved first", deviceClass.Object.Name)
		}
	}

	attrs, err := sriovdra.ListDeviceAttributes(APIClient, s.namespace)
	if err != nil {
		return err
	}

	if len(attrs) != 0 {
		return fmt.Errorf("existing DeviceAttributes prevent isolated run")
	}

	policiesDRA, err := sriovdra.ListSriovResourcePolicies(APIClient, s.namespace)
	if err != nil {
		return err
	}

	if len(policiesDRA) != 0 {
		return fmt.Errorf("existing SriovResourcePolicies prevent isolated run")
	}

	return s.preflightNode()
}

func (s *lifecycle) preflightNode() error {
	selected, err := nodes.List(APIClient, metav1.ListOptions{FieldSelector: "metadata.name=" + s.node})
	if err != nil {
		return err
	}

	if len(selected) != 1 {
		return fmt.Errorf("selected node %s not found", s.node)
	}

	s.hostname = selected[0].Object.Labels[corev1.LabelHostname]
	if s.hostname == "" {
		return fmt.Errorf("selected node lacks hostname label")
	}

	matches, err := nodes.List(APIClient, metav1.ListOptions{LabelSelector: corev1.LabelHostname + "=" + s.hostname})
	if err != nil {
		return err
	}

	if len(matches) != 1 {
		return fmt.Errorf("hostname label must select exactly one node")
	}

	nodeState := sriov.NewNetworkNodeStateBuilder(APIClient, s.node, s.namespace)
	if err := nodeState.Discover(); err != nil {
		return err
	}

	for _, nic := range nodeState.Objects.Status.Interfaces {
		if nic.Name == s.pf {
			if nic.TotalVfs < draparams.UpdatedVFs {
				return fmt.Errorf("PF %s supports fewer than four VFs", s.pf)
			}

			return s.configured(0)
		}
	}

	return fmt.Errorf("PF %s not found on operator-managed node %s", s.pf, s.node)
}

func (s *lifecycle) setFlag(value, present bool) error {
	policies, err := sriov.ListPolicy(APIClient, s.namespace, runtimeclient.ListOptions{Namespace: s.namespace})
	if err != nil {
		return err
	}

	for _, policy := range policies {
		if policy.Object.Name != "default" || policy.Object.Spec.NumVfs != 0 || policy.Object.Spec.ResourceName != "" {
			return fmt.Errorf("refusing provider change while policy %s exists", policy.Object.Name)
		}
	}

	cfg, err := pullConfig(s.namespace)
	if err != nil {
		return err
	}

	if present {
		if cfg.Definition.Spec.FeatureGates == nil {
			cfg.Definition.Spec.FeatureGates = map[string]bool{}
		}

		cfg.Definition.Spec.FeatureGates[featureGate] = value
	} else {
		delete(cfg.Definition.Spec.FeatureGates, featureGate)
	}

	_, err = cfg.Update()

	return err
}
func (s *lifecycle) createPolicy() error {
	existing, err := sriov.ListPolicy(APIClient, s.namespace, runtimeclient.ListOptions{Namespace: s.namespace})
	if err != nil {
		return err
	}

	for _, candidate := range existing {
		if candidate.Object.Name == s.name {
			return fmt.Errorf("refusing to adopt existing policy %s", s.name)
		}
	}

	policyBuilder := sriov.NewPolicyBuilder(APIClient, s.name, s.namespace, s.name, draparams.InitialVFs,
		[]string{s.pf}, map[string]string{corev1.LabelHostname: s.hostname}).WithDevType("netdevice")
	delete(s.owned, s.name)
	policyBuilder.Definition.Labels = map[string]string{runLabel: s.runID}

	created, err := policyBuilder.Create()
	if err != nil {
		// The server may have accepted a create whose response was lost. Cleanup
		// resolves ownership through the full run token, never the name alone.
		return err
	}

	if created.Object.UID == "" {
		return fmt.Errorf("created policy has no UID; refusing to adopt it")
	}

	s.owned[s.name] = created.Object.UID

	return nil
}
func (s *lifecycle) deletePolicies() error {
	policies, err := sriov.ListPolicy(APIClient, s.namespace, runtimeclient.ListOptions{Namespace: s.namespace})
	if err != nil {
		return err
	}

	var failures []error

	for _, policyBuilder := range policies {
		uid, owned := s.owned[policyBuilder.Object.Name]
		if !owned && policyBuilder.Object.Name == s.name && policyBuilder.Object.Labels[runLabel] == s.runID {
			uid, owned = policyBuilder.Object.UID, true
			s.owned[policyBuilder.Object.Name] = uid
		}

		if !owned {
			continue
		}

		if policyBuilder.Object.UID != uid {
			failures = append(failures, fmt.Errorf("refusing deletion of replacement policy %s", policyBuilder.Object.Name))

			continue
		}

		if err := policyBuilder.DeleteWithUID(uid); err != nil && !apierrors.IsNotFound(err) {
			failures = append(failures, err)
		}
	}

	return errors.Join(failures...)
}
func (s *lifecycle) configured(vfs int) error {
	policies, err := sriov.ListPolicy(APIClient, s.namespace, runtimeclient.ListOptions{Namespace: s.namespace})
	if err != nil {
		return err
	}

	if vfs == 0 {
		for _, policyBuilder := range policies {
			if _, ok := s.owned[policyBuilder.Object.Name]; ok {
				return fmt.Errorf("policy %s still exists", policyBuilder.Object.Name)
			}
		}
	}

	nodeState := sriov.NewNetworkNodeStateBuilder(APIClient, s.node, s.namespace)
	if err := nodeState.Discover(); err != nil {
		return err
	}

	if nodeState.Objects.Status.SyncStatus != "Succeeded" {
		return fmt.Errorf("node sync status: %s", nodeState.Objects.Status.SyncStatus)
	}

	desired := 0

	for _, nic := range nodeState.Objects.Spec.Interfaces {
		if nic.Name == s.pf {
			desired = nic.NumVfs
		}
	}

	for _, nic := range nodeState.Objects.Status.Interfaces {
		if nic.Name == s.pf {
			if nic.NumVfs == vfs && desired == vfs {
				return nil
			}

			return fmt.Errorf("PF %s desired=%d observed=%d expected=%d", s.pf, desired, nic.NumVfs, vfs)
		}
	}

	return fmt.Errorf("selected PF missing")
}
func (s *lifecycle) ready(name string, initRequired bool) error {
	sets, err := daemonset.List(APIClient, s.namespace)
	if err != nil {
		return err
	}

	var daemonSet *daemonset.Builder

	for _, candidate := range sets {
		if candidate.Object.Name == name {
			daemonSet = candidate

			break
		}
	}

	if daemonSet == nil {
		return fmt.Errorf("DaemonSet %s not found", name)
	}

	status := daemonSet.Object.Status
	if status.DesiredNumberScheduled == 0 ||
		status.ObservedGeneration < daemonSet.Object.Generation ||
		status.NumberReady != status.DesiredNumberScheduled ||
		status.UpdatedNumberScheduled != status.DesiredNumberScheduled {
		return fmt.Errorf("DaemonSet %s rollout incomplete: %+v", name, status)
	}

	pods, err := pod.List(APIClient, s.namespace, metav1.ListOptions{LabelSelector: "app=" + name})
	if err != nil {
		return err
	}

	if len(pods) == 0 {
		return fmt.Errorf("no pods for %s", name)
	}

	for _, driverPod := range pods {
		if err := draenv.PodReady(*driverPod.Object, initRequired); err != nil {
			return err
		}
	}

	return nil
}
func (s *lifecycle) absent(name string) error {
	sets, err := daemonset.List(APIClient, s.namespace)
	if err != nil {
		return err
	}

	for _, daemonSet := range sets {
		if daemonSet.Object.Name == name {
			return fmt.Errorf("DaemonSet %s still exists", name)
		}
	}

	return nil
}
func (s *lifecycle) class(name string, perResource bool) error {
	classes, err := resource.ListDeviceClasses(APIClient)
	if err != nil {
		return err
	}

	for _, deviceClass := range classes {
		if deviceClass.Object.Name == name {
			if deviceClass.Object.Labels[ownerLabel] != ownerValue {
				return fmt.Errorf("class %s lacks owner label", name)
			}

			if perResource && deviceClass.Object.Labels[resourceLabel] != s.name {
				return fmt.Errorf("class resource label mismatch")
			}

			return nil
		}
	}

	return fmt.Errorf("DeviceClass %s not found", name)
}
func (s *lifecycle) generated() error {
	if err := s.class(s.name, true); err != nil {
		return err
	}

	attrs, err := sriovdra.PullDeviceAttributes(APIClient, s.name+"-attrs", s.namespace)
	if err != nil {
		return err
	}

	if attrs.Object.Labels[ownerLabel] != ownerValue || attrs.Object.Labels[poolLabel] != s.name {
		return fmt.Errorf("DeviceAttributes labels incorrect")
	}

	attribute := attrs.Object.Spec.Attributes[resourcev1.QualifiedName(draenv.ResourceAttribute)]
	if attribute.StringValue == nil || *attribute.StringValue != "openshift.io/"+s.name {
		return fmt.Errorf("DeviceAttributes resource attribute mismatch")
	}

	policy, err := sriovdra.PullSriovResourcePolicy(APIClient, s.node, s.namespace)
	if err != nil {
		return err
	}

	if policy.Object.Labels[ownerLabel] != ownerValue || policy.Object.Labels[nodeLabel] != s.node ||
		len(policy.Object.Spec.Configs) == 0 {
		return fmt.Errorf("SriovResourcePolicy labels/configs incorrect")
	}

	return nil
}
func (s *lifecycle) generatedAbsent() error {
	labels := runtimeclient.MatchingLabels{ownerLabel: ownerValue}

	attrs, err := sriovdra.ListDeviceAttributes(APIClient, s.namespace, labels)
	if err != nil {
		return err
	}

	policies, err := sriovdra.ListSriovResourcePolicies(APIClient, s.namespace, labels)
	if err != nil {
		return err
	}

	classes, err := resource.ListDeviceClasses(APIClient, labels)
	if err != nil {
		return err
	}

	if len(attrs)+len(policies)+len(classes) != 0 {
		return fmt.Errorf("operator-generated DRA resources still exist")
	}

	all, err := resource.ListDeviceClasses(APIClient)
	if err != nil {
		return err
	}

	for _, deviceClass := range all {
		if deviceClass.Object.Name == draenv.Driver {
			return fmt.Errorf("base DeviceClass still exists")
		}
	}

	return nil
}
func (s *lifecycle) counts(expected int) error {
	builders, err := resource.ListResourceSlicesByDriver(APIClient, draenv.Driver)
	if err != nil {
		return err
	}

	slices := make([]resourcev1.ResourceSlice, 0, len(builders))
	names := map[string]bool{s.node: true}

	for _, b := range builders {
		slices = append(slices, *b.Object)
		if b.Object.Spec.NodeName != nil {
			names[*b.Object.Spec.NodeName] = true
		}
	}

	for node := range names {
		count, err := draenv.CountDevices(slices, node, "openshift.io/"+s.name)
		if err != nil {
			return err
		}

		want := 0
		if node == s.node {
			want = expected
		}

		if count != want {
			return fmt.Errorf("node %s advertises %d devices, expected %d", node, count, want)
		}
	}

	return nil
}
func poll(fn func() error) error {
	var last error

	err := wait.PollUntilContextTimeout(context.Background(), draparams.PollInterval, draparams.ReconcileTimeout,
		true, func(context.Context) (bool, error) {
			last = fn()

			return last == nil, nil
		})
	if err != nil {
		return errors.Join(err, last)
	}

	return nil
}
func (s *lifecycle) cleanup() error {
	if err := s.deletePolicies(); err != nil {
		return err
	}

	if err := poll(func() error { return s.configured(0) }); err != nil {
		return fmt.Errorf("refusing provider switch before policy/PF cleanup: %w", err)
	}

	if err := poll(func() error { return s.setFlag(false, s.flagPresent) }); err != nil {
		return err
	}

	return errors.Join(poll(func() error { return s.absent(driverDS) }), poll(s.generatedAbsent))
}

func pullConfig(namespace string) (*sriov.OperatorConfigBuilder, error) {
	builder := sriov.NewOperatorConfigBuilder(APIClient, namespace)

	object, err := builder.Get()
	if err != nil {
		return nil, err
	}

	builder.Object = object
	builder.Definition = object.DeepCopy()

	return builder, nil
}

func (s *lifecycle) policy() (*sriov.PolicyBuilder, error) {
	policies, err := sriov.ListPolicy(APIClient, s.namespace, runtimeclient.ListOptions{Namespace: s.namespace})
	if err != nil {
		return nil, err
	}

	for _, policy := range policies {
		if policy.Object.Name == s.name {
			return policy, nil
		}
	}

	return nil, fmt.Errorf("policy %s not found", s.name)
}
