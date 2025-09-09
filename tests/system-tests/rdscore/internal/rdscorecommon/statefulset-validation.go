package rdscorecommon

import (
	"fmt"
	"time"

	"github.com/golang/glog"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/service"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/statefulset"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WaitAllStatefulsetsReady checks all statefulset objects are in Ready state.
func WaitAllStatefulsetsReady(ctx SpecContext) {
	By("Checking all statefulsets are Ready")

	Eventually(func() bool {
		var (
			statefulsetList []*statefulset.Builder
			err             error
			notReadySets    int
		)

		statefulsetList, err = statefulset.ListInAllNamespaces(APIClient, metav1.ListOptions{})

		if err != nil {
			glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to list statefulsets in all namespaces: %s", err)

			return false
		}

		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Found %d statefulsets", len(statefulsetList))

		notReadySets = 0

		for _, sfSet := range statefulsetList {
			glog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Processing statefulset %q in %q namespace", sfSet.Definition.Name, sfSet.Definition.Namespace)

			if sfSet.Object.Status.ReadyReplicas != *sfSet.Definition.Spec.Replicas {
				msg := fmt.Sprintf("Statefulset %q in %q namespace has %d ready replicas but expects %d",
					sfSet.Definition.Name, sfSet.Definition.Namespace,
					sfSet.Object.Status.ReadyReplicas, *sfSet.Definition.Spec.Replicas)

				glog.V(rdscoreparams.RDSCoreLogLevel).Infof(msg)

				notReadySets++
			} else {
				msg := fmt.Sprintf("Statefulset %q in %q namespace has %d ready replicas and expects %d",
					sfSet.Definition.Name, sfSet.Definition.Namespace,
					sfSet.Object.Status.ReadyReplicas, *sfSet.Definition.Spec.Replicas)

				glog.V(rdscoreparams.RDSCoreLogLevel).Infof(msg)
			}
		}

		return notReadySets == 0
	}).WithContext(ctx).WithPolling(15*time.Second).WithTimeout(15*time.Minute).Should(BeTrue(),
		"There are not Ready statefulsets all namespaces")
}

func defineHeadlessService(svcName, svcNSName string,
	svcSelector map[string]string, svcPort corev1.ServicePort) *service.Builder {
	glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Defining headless service %q in %q ns", svcName, svcNSName)

	svcBuilder := service.NewBuilder(APIClient, svcName, svcNSName, svcSelector, svcPort)

	// Make it headless by setting ClusterIP to None
	svcBuilder.Definition.Spec.ClusterIP = "None"

	return svcBuilder
}

func defineStatefulsetContainer(
	cName, cImage string,
	cCmd []string,
	cRequests, cLimits map[string]string) *pod.ContainerBuilder {
	glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Defining container %q with %q image and command %q",
		cName, cImage, cCmd)

	cBuilder := pod.NewContainerBuilder(cName, cImage, cCmd)

	if cRequests != nil {
		containerRequests := corev1.ResourceList{}

		for key, val := range cRequests {
			glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Parsing container's request: %q - %q", key, val)

			containerRequests[corev1.ResourceName(key)] = resource.MustParse(val)
		}

		cBuilder = cBuilder.WithCustomResourcesRequests(containerRequests)
	}

	if cLimits != nil {
		containerLimits := corev1.ResourceList{}

		for key, val := range cLimits {
			glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Parsing container's limit: %q - %q", key, val)

			containerLimits[corev1.ResourceName(key)] = resource.MustParse(val)
		}

		cBuilder = cBuilder.WithCustomResourcesLimits(containerLimits)
	}

	return cBuilder
}

func withRequiredLabelPodAffinity(
	stBuilder *statefulset.Builder,
	matchLabels map[string]string,
	nsNames []string,
	topologyKey string) error {
	glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Adding required pod affinity to statefulset %q",
		stBuilder.Definition.Name)
	glog.V(rdscoreparams.RDSCoreLogLevel).Infof("RequiredLabelPodAffinity 'matchLabels': %q", matchLabels)
	glog.V(rdscoreparams.RDSCoreLogLevel).Infof("RequiredLabelPodAffinity 'namespaces': %q", nsNames)

	if matchLabels == nil {
		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'matchLabels' is not set")

		return fmt.Errorf("option 'matchLabels' is not set")
	}

	if topologyKey == "" {
		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'topologyKey' is not set")

		return fmt.Errorf("option 'topologyKey' is not set")
	}

	if len(nsNames) == 0 {
		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'namespaces' is not set")

		return fmt.Errorf("option 'namespaces' is not set")
	}

	stBuilder.Definition.Spec.Template.Spec.Affinity = &corev1.Affinity{
		PodAffinity: &corev1.PodAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
				{
					LabelSelector: &metav1.LabelSelector{
						MatchLabels: matchLabels,
					},
					TopologyKey: topologyKey,
					Namespaces:  nsNames,
				},
			},
		},
	}

	return nil
}

func withRequiredLabelPodAntiAffinity(
	stBuilder *statefulset.Builder,
	matchLabels map[string]string,
	nsNames []string,
	topologyKey string) error {
	glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Adding required pod anti-affinity to statefulset %q",
		stBuilder.Definition.Name)
	glog.V(rdscoreparams.RDSCoreLogLevel).Infof("RequiredPodAntiAffinity 'matchLabels': %q", matchLabels)
	glog.V(rdscoreparams.RDSCoreLogLevel).Infof("RequiredPodAntiAffinity 'namespaces': %q", nsNames)

	if matchLabels == nil {
		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'matchLabels' is not set")

		return fmt.Errorf("option 'matchLabels' is not set")
	}

	if topologyKey == "" {
		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'topologyKey' is not set")

		return fmt.Errorf("option 'topologyKey' is not set")
	}

	if len(nsNames) == 0 {
		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'namespaces' is not set")

		return fmt.Errorf("option 'namespaces' is not set")
	}

	stBuilder.Definition.Spec.Template.Spec.Affinity = &corev1.Affinity{
		PodAntiAffinity: &corev1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
				{
					LabelSelector: &metav1.LabelSelector{
						MatchLabels: matchLabels,
					},
					TopologyKey: topologyKey,
					Namespaces:  nsNames,
				},
			},
		},
	}

	return nil
}

//nolint:unused
func withPreferredLabelPodAffinity(
	stBuilder *statefulset.Builder,
	matchLabels map[string]string,
	nsNames []string,
	topologyKey string,
	weight int32) error {
	glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Adding preferred pod affinity to statefulset %q",
		stBuilder.Definition.Name)
	glog.V(rdscoreparams.RDSCoreLogLevel).Infof("PreferredPodAffinity 'matchLabels': %q", matchLabels)
	glog.V(rdscoreparams.RDSCoreLogLevel).Infof("PreferredPodAffinity 'namespaces': %q", nsNames)

	if matchLabels == nil {
		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'matchLabels' is not set")

		return fmt.Errorf("option 'matchLabels' is not set")
	}

	if topologyKey == "" {
		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'topologyKey' is not set")

		return fmt.Errorf("option 'topologyKey' is not set")
	}

	if len(nsNames) == 0 {
		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'namespaces' is not set")

		return fmt.Errorf("option 'namespaces' is not set")
	}

	if weight < 0 || weight > 100 {
		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'weight' is invalid: %d. Must be between 0 and 100", weight)

		return fmt.Errorf("option 'weight' is invalid: %d. Must be between 0 and 100", weight)
	}

	stBuilder.Definition.Spec.Template.Spec.Affinity = &corev1.Affinity{
		PodAffinity: &corev1.PodAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
				{
					Weight: weight,
					PodAffinityTerm: corev1.PodAffinityTerm{
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: matchLabels,
						},
						TopologyKey: topologyKey,
						Namespaces:  nsNames,
					},
				},
			},
		},
	}

	return nil
}

//nolint:unused
func withPreferredLabelPodAntiAffinity(
	stBuilder *statefulset.Builder,
	matchLabels map[string]string,
	nsNames []string,
	topologyKey string,
	weight int32) error {
	glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Adding preferred pod anti-affinity to statefulset %q",
		stBuilder.Definition.Name)
	glog.V(rdscoreparams.RDSCoreLogLevel).Infof("PreferredPodAntiAffinity 'matchLabels': %q", matchLabels)
	glog.V(rdscoreparams.RDSCoreLogLevel).Infof("PreferredPodAntiAffinity 'namespaces': %q", nsNames)

	if matchLabels == nil {
		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'matchLabels' is not set")

		return fmt.Errorf("option 'matchLabels' is not set")
	}

	if topologyKey == "" {
		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'topologyKey' is not set")

		return fmt.Errorf("option 'topologyKey' is not set")
	}

	if len(nsNames) == 0 {
		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'namespaces' is not set")

		return fmt.Errorf("option 'namespaces' is not set")
	}

	if weight < 0 || weight > 100 {
		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'weight' is invalid: %d. Must be between 0 and 100", weight)

		return fmt.Errorf("option 'weight' is invalid: %d. Must be between 0 and 100", weight)
	}

	stBuilder.Definition.Spec.Template.Spec.Affinity = &corev1.Affinity{
		PodAntiAffinity: &corev1.PodAntiAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
				{
					Weight: weight,
					PodAffinityTerm: corev1.PodAffinityTerm{
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: matchLabels,
						},
						TopologyKey: topologyKey,
						Namespaces:  nsNames,
					},
				},
			},
		},
	}

	return nil
}

//nolint:unused
func withNodeSelector(
	stBuilder *statefulset.Builder,
	selector map[string]string) error {
	glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Adding node selector to statefulset %q in %q namespace",
		stBuilder.Definition.Name, stBuilder.Definition.Namespace)
	glog.V(rdscoreparams.RDSCoreLogLevel).Infof("NodeSelector: %q", selector)

	if len(selector) == 0 {
		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'nodeSelector' is not set")

		return fmt.Errorf("option 'nodeSelector' is not set")
	}

	stBuilder.Definition.Spec.Template.Spec.NodeSelector = selector

	return nil
}

func withPodAnnotations(
	stBuilder *statefulset.Builder,
	annotations map[string]string) error {
	glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Adding annotations to statefulset %q in %q namespace",
		stBuilder.Definition.Name, stBuilder.Definition.Namespace)
	glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Annotations: %q", annotations)

	if len(annotations) == 0 {
		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'annotations' is not set")

		return fmt.Errorf("option 'annotations' is not set")
	}

	if stBuilder.Definition.Spec.Template.Annotations == nil {
		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'annotations' is not set. Creating new map")

		stBuilder.Definition.Spec.Template.Annotations = make(map[string]string)
	}

	for key, val := range annotations {
		glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Adding annotation %q with value %q", key, val)

		stBuilder.Definition.Spec.Template.Annotations[key] = val
	}

	return nil
}
