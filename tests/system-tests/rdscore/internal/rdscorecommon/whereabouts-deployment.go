package rdscorecommon

import (
	"fmt"
	"maps"
	"math/rand"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmc"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// WhereaboutsDeploymentConfig holds configuration for creating whereabouts deployments.
type WhereaboutsDeploymentConfig struct {
	Name                string
	Label               string
	Port                string
	Image               string
	Command             []string
	NAD                 string
	ServiceAccount      string
	RBACRole            string
	TopologyKey         string
	Replicas            int32
	UseAntiAffinity     bool
	UseRequiredAffinity bool
	AffinityLabels      string
	Description         string
}

const (
	myDeploymentOne                    = "rds-whereabouts-one"
	myDeploymentOneLabel               = "app=rds-whereabouts-one"
	myDeploymentOneSA                  = "rds-whereabouts-one-sa"
	myDeploymentOneRBACRole            = "system:openshift:scc:nonroot-v2"
	myDeploymentOneTopologyKey         = "kubernetes.io/hostname"
	myDeploymentOneReplicas            = 1
	myDeploymentOneUseAntiAffinity     = false
	myDeploymentOneUseRequiredAffinity = false
	myDeploymentOneAffinityLabels      = "app=rds-whereabouts-two"

	myDeploymentTwo                    = "rds-whereabouts-two"
	myDeploymentTwoLabel               = "app=rds-whereabouts-two"
	myDeploymentTwoSA                  = "rds-whereabouts-two-sa"
	myDeploymentTwoRBACRole            = "system:openshift:scc:nonroot-v2"
	myDeploymentTwoTopologyKey         = "kubernetes.io/hostname"
	myDeploymentTwoReplicas            = 1
	myDeploymentTwoUseAntiAffinity     = false
	myDeploymentTwoUseRequiredAffinity = true
	myDeploymentTwoAffinityLabels      = "app=rds-whereabouts-one"

	// DefaultAffinityWeight value for the weight of the pod affinity or anti-affinity.
	DefaultAffinityWeight = 100
	// DefaultDeploymentTimeout value for the timeout of the deployment creation.
	DefaultDeploymentTimeout = 5 * time.Minute
	// DefaultDeploymentPollingInterval value for the polling interval of the deployment creation.
	DefaultDeploymentPollingInterval = 15 * time.Second
	// DefaultCleanupPollingInterval value for the polling interval of the cleanup.
	DefaultCleanupPollingInterval = 15 * time.Second
	// DefaultCleanupTimeout value for the timeout of the cleanup.
	DefaultCleanupTimeout = 5 * time.Minute
	// DefaultPodTerminationPollingInterval value for the polling interval of the pod termination.
	DefaultPodTerminationPollingInterval = 15 * time.Second
	// DefaultPodTerminationTimeout value for the timeout of the pod termination.
	DefaultPodTerminationTimeout = 5 * time.Minute
	// WorkerNodeSelector value for the selector of the worker nodes.
	WorkerNodeSelector = "node-role.kubernetes.io/worker"
	// ExpectedReadyNodes value for the minimum number of nodes in Ready state.
	ExpectedReadyNodes = 1
	// DefaultNodePollingInterval value for the polling interval of the node.
	DefaultNodePollingInterval = 5 * time.Second
	// DefaultNodePollingTimeout value for the timeout of the node.
	DefaultNodePollingTimeout = 6 * time.Minute
	// DefaultRedfishTimeout value for the timeout of the redfish.
	DefaultRedfishTimeout = 6 * time.Minute
	// DefaultNodePowerOffTimeout value for the timeout of the node power off.
	DefaultNodePowerOffTimeout = 15 * time.Minute

	waDeployment3                    = "rds-whereabouts-3"
	waDeployment3Label               = "app=rds-whereabouts-3"
	waDeployment3SA                  = "rds-whereabouts-3-sa"
	waDeployment3RBACRole            = "system:openshift:scc:nonroot-v2"
	waDeployment3TopologyKey         = "kubernetes.io/hostname"
	waDeployment3Replicas            = 1
	waDeployment3UseAntiAffinity     = true
	waDeployment3UseRequiredAffinity = false
	waDeployment3AffinityLabels      = "app=rds-whereabouts-4"

	waDeployment4                    = "rds-whereabouts-4"
	waDeployment4Label               = "app=rds-whereabouts-4"
	waDeployment4SA                  = "rds-whereabouts-4-sa"
	waDeployment4RBACRole            = "system:openshift:scc:nonroot-v2"
	waDeployment4TopologyKey         = "kubernetes.io/hostname"
	waDeployment4Replicas            = 1
	waDeployment4UseAntiAffinity     = true
	waDeployment4UseRequiredAffinity = true
	waDeployment4AffinityLabels      = "app=rds-whereabouts-3"
)

// Pre-defined configurations.
var (
	SameNodeDeploymentOneConfig = WhereaboutsDeploymentConfig{
		Name:                myDeploymentOne,
		Label:               myDeploymentOneLabel,
		Port:                "",  // Will be set from RDSCoreConfig
		Image:               "",  // Will be set from RDSCoreConfig
		Command:             nil, // Will be set from RDSCoreConfig
		NAD:                 "",  // Will be set from RDSCoreConfig
		ServiceAccount:      myDeploymentOneSA,
		RBACRole:            myDeploymentOneRBACRole,
		TopologyKey:         myDeploymentOneTopologyKey,
		Replicas:            myDeploymentOneReplicas,
		UseAntiAffinity:     myDeploymentOneUseAntiAffinity,
		UseRequiredAffinity: myDeploymentOneUseRequiredAffinity,
		AffinityLabels:      myDeploymentOneAffinityLabels,
		Description:         "pods from 1st deployment running on the same node",
	}

	SameNodeDeploymentTwoConfig = WhereaboutsDeploymentConfig{
		Name:                myDeploymentTwo,
		Label:               myDeploymentTwoLabel,
		Port:                "",  // Will be set from RDSCoreConfig
		Image:               "",  // Will be set from RDSCoreConfig
		Command:             nil, // Will be set from RDSCoreConfig
		NAD:                 "",  // Will be set from RDSCoreConfig
		ServiceAccount:      myDeploymentTwoSA,
		RBACRole:            myDeploymentTwoRBACRole,
		TopologyKey:         myDeploymentTwoTopologyKey,
		Replicas:            myDeploymentTwoReplicas,
		UseAntiAffinity:     myDeploymentTwoUseAntiAffinity,
		UseRequiredAffinity: myDeploymentTwoUseRequiredAffinity,
		AffinityLabels:      myDeploymentTwoAffinityLabels,
		Description:         "pods from 2nd deployment running on the same node",
	}

	DiffNodeDeploymentOneConfig = WhereaboutsDeploymentConfig{
		Name:                waDeployment3,
		Label:               waDeployment3Label,
		Port:                "",  // Will be set from RDSCoreConfig
		Image:               "",  // Will be set from RDSCoreConfig
		Command:             nil, // Will be set from RDSCoreConfig
		NAD:                 "",  // Will be set from RDSCoreConfig
		ServiceAccount:      waDeployment3SA,
		RBACRole:            waDeployment3RBACRole,
		TopologyKey:         waDeployment3TopologyKey,
		Replicas:            waDeployment3Replicas,
		UseAntiAffinity:     waDeployment3UseAntiAffinity,
		UseRequiredAffinity: waDeployment3UseRequiredAffinity,
		AffinityLabels:      waDeployment3AffinityLabels,
		Description:         "pods from 1st deployment running on different nodes",
	}

	DiffNodeDeploymentTwoConfig = WhereaboutsDeploymentConfig{
		Name:                waDeployment4,
		Label:               waDeployment4Label,
		Port:                "",  // Will be set from RDSCoreConfig
		Image:               "",  // Will be set from RDSCoreConfig
		Command:             nil, // Will be set from RDSCoreConfig
		NAD:                 "",  // Will be set from RDSCoreConfig
		ServiceAccount:      waDeployment4SA,
		RBACRole:            waDeployment4RBACRole,
		TopologyKey:         waDeployment4TopologyKey,
		Replicas:            waDeployment4Replicas,
		UseAntiAffinity:     waDeployment4UseAntiAffinity,
		UseRequiredAffinity: waDeployment4UseRequiredAffinity,
		AffinityLabels:      waDeployment4AffinityLabels,
		Description:         "pods from 2nd deployment running on different nodes",
	}
)

// defineWhereaboutsDeploymentContainer defines the container configuration for whereabouts deployment.
func defineWhereaboutsDeploymentContainer(
	cName, cImage string,
	cCmd []string,
	cRequests, cLimits map[string]string) *pod.ContainerBuilder {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Defining container %q with %q image and command %q",
		cName, cImage, cCmd)

	cBuilder := pod.NewContainerBuilder(cName, cImage, cCmd)

	if cRequests != nil {
		containerRequests := corev1.ResourceList{}

		for key, val := range cRequests {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Parsing container's request: %q - %q", key, val)

			containerRequests[corev1.ResourceName(key)] = resource.MustParse(val)
		}

		cBuilder = cBuilder.WithCustomResourcesRequests(containerRequests)
	}

	if cLimits != nil {
		containerLimits := corev1.ResourceList{}

		for key, val := range cLimits {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Parsing container's limit: %q - %q", key, val)

			containerLimits[corev1.ResourceName(key)] = resource.MustParse(val)
		}

		cBuilder = cBuilder.WithCustomResourcesLimits(containerLimits)
	}

	return cBuilder
}

// createDeploymentBuilder creates and configures the basic deployment.
func createDeploymentBuilder(config WhereaboutsDeploymentConfig) *deployment.Builder {
	By("Defining whereabouts deployment container")

	waContainer := defineWhereaboutsDeploymentContainer("whereabouts-container", config.Image, config.Command, nil, nil)
	Expect(waContainer).ToNot(BeNil(), "Failed to define whereabouts deployment container")

	By("Getting whereabouts deployment container config")

	waContainerCfg, err := waContainer.GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to get whereabouts deployment container config")

	By("Checking that label is set")

	Expect(config.Label).ToNot(BeEmpty(), "Label is not set")

	svcLabelsMap := parseLabelsMap(config.Label)

	By("Defining deployment")

	waBuilder := deployment.NewBuilder(APIClient, config.Name, RDSCoreConfig.WhereaboutNS, svcLabelsMap, *waContainerCfg)

	By("Adding pod annotations")

	Expect(config.NAD).ToNot(BeEmpty(), "NAD(network attachment definition) is not set")

	nadMap := map[string]string{"k8s.v1.cni.cncf.io/networks": config.NAD}

	if waBuilder.Definition.Spec.Template.Annotations == nil {
		waBuilder.Definition.Spec.Template.Annotations = make(map[string]string)
	}

	maps.Copy(waBuilder.Definition.Spec.Template.Annotations, nadMap)

	By("Setting replicas")

	waBuilder.Definition.Spec.Replicas = &config.Replicas

	return waBuilder
}

func cleanupDeployment(waName, namespace, waLabel string) {
	By(fmt.Sprintf("Checking that deployment %q doesn't exist in %q namespace",
		waName, namespace))

	var ctx SpecContext

	waOne, err := deployment.Pull(APIClient, waName, namespace)
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to get deployment %q in %q namespace: %s",
			waName, namespace, err)
	} else {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deleting deployment %q in %q namespace",
			waName, namespace)

		delError := waOne.Delete()

		Expect(delError).ToNot(HaveOccurred(), "Failed to delete deployment %q in %q namespace",
			waName, namespace)

		// wait for pods to be deleted
		By(fmt.Sprintf("Waiting for pods from %q deployment in %q namespace to be deleted",
			waName, namespace))

		Eventually(func() bool {
			pods, err := pod.List(APIClient, namespace, metav1.ListOptions{
				LabelSelector: waLabel,
			})
			if err != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to list pods from %q deployment in %q namespace: %s",
					waName, namespace, err)

				return false
			}

			return len(pods) == 0
		}).WithContext(ctx).WithPolling(DefaultCleanupPollingInterval).WithTimeout(DefaultCleanupTimeout).Should(BeTrue(),
			"Pods from %q deployment in %q namespace are not deleted", waName, namespace)
	}
}

func withDeploymentRequiredLabelPodAntiAffinity(
	waBuilder *deployment.Builder,
	matchLabels map[string]string,
	nsNames []string,
	topologyKey string) error {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Adding required pod anti-affinity to deployment %q",
		waBuilder.Definition.Name)
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("RequiredPodAntiAffinity 'matchLabels': %q", matchLabels)
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("RequiredPodAntiAffinity 'namespaces': %q", nsNames)

	if matchLabels == nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'matchLabels' is not set")

		return fmt.Errorf("option 'matchLabels' is not set")
	}

	if topologyKey == "" {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'topologyKey' is not set")

		return fmt.Errorf("option 'topologyKey' is not set")
	}

	if len(nsNames) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'namespaces' is not set")

		return fmt.Errorf("option 'namespaces' is not set")
	}

	waBuilder.Definition.Spec.Template.Spec.Affinity = &corev1.Affinity{
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

func withDeploymentRequiredLabelPodAffinity(
	waBuilder *deployment.Builder,
	matchLabels map[string]string,
	nsNames []string,
	topologyKey string) error {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Adding required pod affinity to deployment %q",
		waBuilder.Definition.Name)
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("RequiredLabelPodAffinity 'matchLabels': %q", matchLabels)
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("RequiredLabelPodAffinity 'namespaces': %q", nsNames)

	if matchLabels == nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'matchLabels' is not set")

		return fmt.Errorf("option 'matchLabels' is not set")
	}

	if topologyKey == "" {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'topologyKey' is not set")

		return fmt.Errorf("option 'topologyKey' is not set")
	}

	if len(nsNames) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'namespaces' is not set")

		return fmt.Errorf("option 'namespaces' is not set")
	}

	waBuilder.Definition.Spec.Template.Spec.Affinity = &corev1.Affinity{
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

func withDeploymentPreferredLabelPodAffinity(
	waBuilder *deployment.Builder,
	matchLabels map[string]string,
	nsNames []string,
	topologyKey string,
	weight int32) error {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Adding preferred pod affinity to deployment %q",
		waBuilder.Definition.Name)
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("PreferredPodAffinity 'matchLabels': %q", matchLabels)
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("PreferredPodAffinity 'namespaces': %q", nsNames)

	if matchLabels == nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'matchLabels' is not set")

		return fmt.Errorf("option 'matchLabels' is not set")
	}

	if topologyKey == "" {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'topologyKey' is not set")

		return fmt.Errorf("option 'topologyKey' is not set")
	}

	if len(nsNames) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'namespaces' is not set")

		return fmt.Errorf("option 'namespaces' is not set")
	}

	if weight < 1 || weight > 100 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'weight' is invalid: %d. Must be between 1 and 100", weight)

		return fmt.Errorf("option 'weight' is invalid: %d. Must be between 1 and 100", weight)
	}

	waBuilder.Definition.Spec.Template.Spec.Affinity = &corev1.Affinity{
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

func withDeploymentPreferredLabelPodAntiAffinity(
	waBuilder *deployment.Builder,
	matchLabels map[string]string,
	nsNames []string,
	topologyKey string,
	weight int32) error {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Adding preferred pod anti-affinity to deployment %q",
		waBuilder.Definition.Name)
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("PreferredPodAntiAffinity 'matchLabels': %q", matchLabels)
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("PreferredPodAntiAffinity 'namespaces': %q", nsNames)

	if matchLabels == nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'matchLabels' is not set")

		return fmt.Errorf("option 'matchLabels' is not set")
	}

	if topologyKey == "" {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'topologyKey' is not set")

		return fmt.Errorf("option 'topologyKey' is not set")
	}

	if len(nsNames) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'namespaces' is not set")

		return fmt.Errorf("option 'namespaces' is not set")
	}

	if weight < 1 || weight > 100 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Option 'weight' is invalid: %d. Must be between 1 and 100", weight)

		return fmt.Errorf("option 'weight' is invalid: %d. Must be between 1 and 100", weight)
	}

	waBuilder.Definition.Spec.Template.Spec.Affinity = &corev1.Affinity{
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

// configureAffinity sets up pod affinity or anti-affinity based on configuration.
func configureDeploymentAffinity(waBuilder *deployment.Builder, config WhereaboutsDeploymentConfig) {
	By(fmt.Sprintf("Adding pod %s to deployment %q",
		map[bool]string{true: "anti-affinity", false: "affinity"}[config.UseAntiAffinity],
		config.Name))

	var err error

	svcLabelsMap := parseLabelsMap(config.AffinityLabels)

	if config.UseAntiAffinity {
		if config.UseRequiredAffinity {
			err = withDeploymentRequiredLabelPodAntiAffinity(waBuilder, svcLabelsMap,
				[]string{RDSCoreConfig.WhereaboutNS}, config.TopologyKey)
		} else {
			err = withDeploymentPreferredLabelPodAntiAffinity(waBuilder, svcLabelsMap,
				[]string{RDSCoreConfig.WhereaboutNS}, config.TopologyKey, DefaultAffinityWeight)
		}
	} else {
		if config.UseRequiredAffinity {
			err = withDeploymentRequiredLabelPodAffinity(waBuilder, svcLabelsMap,
				[]string{RDSCoreConfig.WhereaboutNS}, config.TopologyKey)
		} else {
			err = withDeploymentPreferredLabelPodAffinity(waBuilder, svcLabelsMap,
				[]string{RDSCoreConfig.WhereaboutNS}, config.TopologyKey, DefaultAffinityWeight)
		}
	}

	Expect(err).ToNot(HaveOccurred(), "Failed to configure pod affinity for deployment %q", config.Name)
}

// CreateWhereaboutsDeployment creates a deployment with whereabouts IPAM based on configuration.
func CreateWhereaboutsDeployment(ctx SpecContext, config WhereaboutsDeploymentConfig) {
	By("Validating deployment configuration")

	Expect(config.Name).ToNot(BeEmpty(), "Deployment name must be set")

	Expect(config.Image).ToNot(BeEmpty(), "Image must be set for deployment %q", config.Name)

	Expect(len(config.Command)).To(BeNumerically(">", 0),
		"At least one command must be specified for deployment %q", config.Name)

	Expect(config.NAD).ToNot(BeEmpty(),
		fmt.Sprintf("NetworkAttachmentDefinition must be set for deployment %q", config.Name))

	if config.Port != "" {
		_, perr := strconv.Atoi(config.Port)
		Expect(perr).ToNot(HaveOccurred(),
			"Port %q is not a valid integer for deployment %q", config.Port, config.Name)
	}

	By(fmt.Sprintf("Setting up deployment with %s", config.Description))

	cleanupDeployment(config.Name, RDSCoreConfig.WhereaboutNS, config.Label)

	waBuilder := createDeploymentBuilder(config)

	configureDeploymentAffinity(waBuilder, config)

	By(fmt.Sprintf("Setting up service account %q", config.ServiceAccount))

	// Delete existing service account
	deleteServiceAccount(config.ServiceAccount, RDSCoreConfig.WhereaboutNS)

	// Create new service account
	createServiceAccount(config.ServiceAccount, RDSCoreConfig.WhereaboutNS)

	By(fmt.Sprintf("Setting up cluster role binding %q", config.RBACRole))

	// Delete existing cluster role binding
	deleteClusterRBAC(config.RBACRole)

	// Create new cluster role binding
	createClusterRBAC(config.ServiceAccount, config.RBACRole, config.ServiceAccount, RDSCoreConfig.WhereaboutNS)

	By(fmt.Sprintf("Assigning service account %q to deployment %q", config.ServiceAccount, config.Name))

	waBuilder = waBuilder.WithServiceAccountName(config.ServiceAccount)

	_, err := waBuilder.CreateAndWaitUntilReady(DefaultDeploymentTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to create deployment %q", config.Name)
}

// CreateDeploymentsOnSameNode creates deployments with pods scheduled on the same node.
func CreateDeploymentsOnSameNode(ctx SpecContext) {
	configOne := SameNodeDeploymentOneConfig
	// Set runtime configuration values
	configOne.Port = RDSCoreConfig.WhereaboutsDeployOnePort
	configOne.Image = RDSCoreConfig.WhereaboutsDeployImageOne
	configOne.Command = RDSCoreConfig.WhereaboutsDeployOneCMD
	configOne.NAD = RDSCoreConfig.WhereaboutsDeployOneNAD

	CreateWhereaboutsDeployment(ctx, configOne)

	configTwo := SameNodeDeploymentTwoConfig
	// Set runtime configuration values
	configTwo.Port = RDSCoreConfig.WhereaboutsDeployTwoPort
	configTwo.Image = RDSCoreConfig.WhereaboutsDeployImageTwo
	configTwo.Command = RDSCoreConfig.WhereaboutsDeployTwoCMD
	configTwo.NAD = RDSCoreConfig.WhereaboutsDeployTwoNAD

	CreateWhereaboutsDeployment(ctx, configTwo)
}

// VerifyWhereaboutsInterDeploymentPodCommunication verifies inter pod communication between two deployments.
func VerifyWhereaboutsInterDeploymentPodCommunication(
	ctx SpecContext,
	configOne, configTwo WhereaboutsDeploymentConfig) {
	By("Getting list of active pods from both deployments")

	activePods := getActivePods(configOne.Label, RDSCoreConfig.WhereaboutNS)

	Expect(len(activePods)).To(Equal(int(configOne.Replicas)),
		"Number of active pods is not equal to number of replicas")

	twoActivePods := getActivePods(configTwo.Label, RDSCoreConfig.WhereaboutNS)

	Expect(len(twoActivePods)).To(Equal(int(configTwo.Replicas)),
		"Number of active pods is not equal to number of replicas")

	activePods = append(activePods, twoActivePods...)

	By("Checking pods IP addresses")

	podWhereaboutsIPs := getPodWhereaboutsIPs(activePods, interfaceName)
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("PodWhereaboutsIPs: %+v", podWhereaboutsIPs)

	podOneName := activePods[0].Object.Name
	podTwoName := activePods[len(activePods)-1].Object.Name

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod one %q", podOneName)
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod two %q", podTwoName)

	podsMapping := make(map[string]string)

	podsMapping[podOneName] = podTwoName
	podsMapping[podTwoName] = podOneName

	By("Parsing port number")

	Expect(configOne.Port).ToNot(BeEmpty(), "Port is not set")
	Expect(configTwo.Port).ToNot(BeEmpty(), "Port is not set")

	Expect(configOne.Port).To(Equal(configTwo.Port), "Ports are not equal")

	parsedPort, err := strconv.Atoi(configOne.Port)

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to parse port number: %v", configOne.Port))

	verifyInterPodCommunication(activePods, podWhereaboutsIPs, podsMapping, parsedPort)
}

// terminateAndWaitPodFromDeployment terminates a random pod from a deployment and waits for it to be terminated.
func terminateAndWaitPodFromDeployment(config WhereaboutsDeploymentConfig) {
	By(fmt.Sprintf("Terminating pod from deployment %q", config.Name))

	activePods := getActivePods(config.Label, RDSCoreConfig.WhereaboutNS)

	Expect(len(activePods)).To(Equal(int(config.Replicas)),
		"Number of active pods %d is not equal to number of replicas %d", len(activePods), config.Replicas)

	// Ensure we have pods to terminate
	Expect(len(activePods)).To(BeNumerically(">", 0),
		"No active pods found for deployment %q", config.Name)

	By("Randomly picking up pod for termination")

	randomPodIndex := rand.Intn(len(activePods))
	terminatedPod := activePods[randomPodIndex]

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Picked up pod %q for termination", terminatedPod.Object.Name)

	By(fmt.Sprintf("Terminating pod %q", terminatedPod.Object.Name))

	terminatedPod, err := terminatedPod.DeleteAndWait(DefaultPodTerminationTimeout)

	Expect(err).ToNot(HaveOccurred(), "Failed to terminate pod %q due to %v", terminatedPod.Definition.Name, err)
}

// waitForDeploymentReplicas waits for a deployment to have a given number of replicas
// and checks that amount of active pods is equal to the number of replicas.
func waitForDeploymentReplicas(ctx SpecContext, config WhereaboutsDeploymentConfig, replicas int32) {
	By(fmt.Sprintf("Waiting for deployment %q to have %d replicas", config.Name, replicas))

	Eventually(func() bool {
		mDeploy, err := deployment.Pull(APIClient, config.Name, RDSCoreConfig.WhereaboutNS)
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to pull deployment %q: %v", config.Name, err)

			return false
		}

		return mDeploy.Object.Status.ReadyReplicas == replicas
	}).WithContext(ctx).WithTimeout(DefaultDeploymentTimeout).WithPolling(DefaultDeploymentPollingInterval).Should(
		BeTrue(),
		fmt.Sprintf("Deployment %q did not reach %d replicas", config.Name, replicas))

	Eventually(func() bool {
		activePods := getActivePods(config.Label, RDSCoreConfig.WhereaboutNS)

		return len(activePods) == int(replicas)
	}).WithContext(ctx).WithTimeout(DefaultDeploymentTimeout).WithPolling(DefaultDeploymentPollingInterval).Should(
		BeTrue(),
		fmt.Sprintf("Deployment %q did not reach %d active pods", config.Name, replicas))
}

// getRandomPodNode gets a node from a random pod matching the label.
func getRandomPodNode(podsLabel, namespace string, expectedReplicas int32) (string, error) {
	By("Getting list of active pods")

	activePods := getActivePods(podsLabel, namespace)

	Expect(int32(len(activePods))).To(Equal(expectedReplicas),
		"Number of active pods is not equal to number of expected replicas")

	for _, pod := range activePods {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod %q is running on node %q", pod.Object.Name, pod.Object.Spec.NodeName)
	}

	By("Generating random pod index")

	randomPodIndex := rand.Intn(len(activePods))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Random pod index: %d pod name: %q",
		randomPodIndex, activePods[randomPodIndex].Object.Name)

	selectedPod := activePods[randomPodIndex]

	selectedNode := selectedPod.Object.Spec.NodeName

	if selectedNode == "" {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Selected node is empty")

		return "", fmt.Errorf("selected node is empty")
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Selected node: %q", selectedNode)

	return selectedNode, nil
}

// checkEnoughReadyNodes checks if there are enough ready nodes in the cluster.
// nodeToDrain is the node to drain.
// nodeSelector is the selector of the nodes to check.
// expectedReadyNodes is the minimum number of nodes in Ready state.
func checkEnoughReadyNodes(ctx SpecContext, nodeToDrain, nodeSelector string, expectedReadyNodes int) {
	By("Checking there are enough Ready nodes")

	Expect(nodeSelector).ToNot(BeEmpty(), "nodeSelector parameter is empty")

	Expect(nodeToDrain).ToNot(BeEmpty(), "nodeToDrain parameter is empty")

	Expect(expectedReadyNodes).To(BeNumerically(">", 0), "expectedReadyNodes parameter is less than 1")

	Eventually(func() bool {
		nodesList, err := nodes.List(APIClient, metav1.ListOptions{LabelSelector: nodeSelector})
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to list nodes due to: %v", err)

			return false
		}

		var readyNodes int

		for _, node := range nodesList {
			if node.Object.Name == nodeToDrain {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Skipping node %q as it is the node to drain",
					node.Object.Name)

				continue
			}

			if node.Object.Spec.Unschedulable {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Skipping node %q as it is unschedulable",
					node.Object.Name)

				continue
			}

			for _, condition := range node.Object.Status.Conditions {
				if condition.Type == corev1.NodeReady {
					if condition.Status == corev1.ConditionTrue {
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Node %q is Ready and schedulable", node.Object.Name)

						readyNodes++

						break
					}
				}
			}
		}

		// We need at least one node in Ready state to host pods migrated away from the drained node
		return readyNodes >= expectedReadyNodes
	}).WithContext(ctx).WithTimeout(DefaultDeploymentTimeout).WithPolling(DefaultDeploymentPollingInterval).Should(
		BeTrue(), "Not enough nodes in Ready state found")
}

// VerifyConnectivityAfterNodeDrain verifies inter pod communication between the deployments
// after a node is drained.
//
// sameNode parameter distinguishes between the case where the deployments are scheduled on the same node
// and the case where they are scheduled on different nodes.
func VerifyConnectivityAfterNodeDrain(
	ctx SpecContext,
	configOne, configTwo WhereaboutsDeploymentConfig,
	sameNode bool) {
	By("Randomly picking up deployment for node drain")

	whereaboutsDeployments := []WhereaboutsDeploymentConfig{configOne, configTwo}

	randomIndex := rand.Intn(len(whereaboutsDeployments))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Picked up deployment %q for node drain",
		whereaboutsDeployments[randomIndex].Name)

	nodeToDrain, err := getRandomPodNode(whereaboutsDeployments[randomIndex].Label,
		RDSCoreConfig.WhereaboutNS, whereaboutsDeployments[randomIndex].Replicas)

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to get random pod node due to: %v", err))

	checkEnoughReadyNodes(ctx, nodeToDrain, WorkerNodeSelector, ExpectedReadyNodes)

	By(fmt.Sprintf("Pulling node object %q", nodeToDrain))

	nodeObj, err := nodes.Pull(APIClient, nodeToDrain)

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve node %s object due to: %v", nodeToDrain, err))

	By(fmt.Sprintf("Cordoning node %q", nodeToDrain))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Cordoning node %q", nodeToDrain)

	err = nodeObj.Cordon()

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to cordon node %s due to: %v", nodeToDrain, err))

	defer UncordonNode(nodeObj, uncordonNodeInterval, uncordonNodeTimeout)

	By(fmt.Sprintf("Draining node %q", nodeToDrain))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Draining node %q", nodeToDrain)

	err = nodeObj.Drain()

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to drain node %s due to: %v", nodeToDrain, err))

	By(fmt.Sprintf("Waiting for deployment %q to have pods running", whereaboutsDeployments[randomIndex].Name))

	waitForDeploymentReplicas(ctx, whereaboutsDeployments[randomIndex], whereaboutsDeployments[randomIndex].Replicas)

	for _, _deployment := range whereaboutsDeployments {
		if _deployment.Name != whereaboutsDeployments[randomIndex].Name {
			By(fmt.Sprintf("Waiting for deployment %q to have pods running", _deployment.Name))

			waitForDeploymentReplicas(ctx, _deployment, _deployment.Replicas)
		}
	}

	By("Verifying pods were rescheduled away from drained node")

	Eventually(func() bool {
		var allPods []*pod.Builder

		if sameNode {
			allPods = append(
				getActivePods(configOne.Label, RDSCoreConfig.WhereaboutNS),
				getActivePods(configTwo.Label, RDSCoreConfig.WhereaboutNS)...)
		} else {
			allPods = getActivePods(whereaboutsDeployments[randomIndex].Label, RDSCoreConfig.WhereaboutNS)
		}

		for _, pod := range allPods {
			if pod.Object.Spec.NodeName == nodeToDrain {
				return false // Pod still on drained node
			}
		}

		return true
	}).WithContext(ctx).WithTimeout(DefaultDeploymentTimeout).WithPolling(DefaultDeploymentPollingInterval).Should(
		BeTrue(), "Pods were not rescheduled away from drained node")

	By("Verifying inter pod communication between the deployments works after node drain")

	// Ensure inter pod communication between the deployments works after node drain
	VerifyWhereaboutsInterDeploymentPodCommunication(ctx, configOne, configTwo)
}

// waitForPodsToBeMarkedForDeletionOrDeleted waits for all pods to be marked for deletion or deleted.
func waitForPodsToBeMarkedForDeletionOrDeleted(
	ctx SpecContext,
	existingPods []*pod.Builder,
	pollingInterval,
	timeout time.Duration) {
	By("Waiting for pods to be marked for deletion or deleted")

	Eventually(func() bool {
		disruptedPods := make([]*pod.Builder, 0)

		for _, oldPod := range existingPods {
			if oldPod.Exists() {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod %q is still present in %q namespace",
					oldPod.Definition.Name, oldPod.Definition.Namespace)

				if oldPod.Object.DeletionTimestamp != nil {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod %q has DeletionTimestamp set", oldPod.Definition.Name)

					disruptedPods = append(disruptedPods, oldPod)

					continue
				}

				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Checking if pod %q is marked for termination", oldPod.Definition.Name)

				for _, condition := range oldPod.Object.Status.Conditions {
					if condition.Type == corev1.DisruptionTarget {
						if condition.Status == rdscoreparams.ConstantTrueString {
							klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod %q is about to be terminated due to %q",
								oldPod.Object.Name, condition.Message)

							disruptedPods = append(disruptedPods, oldPod)

							break
						}
					}
				}
			} else {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod %q is not present", oldPod.Definition.Name)

				disruptedPods = append(disruptedPods, oldPod)
			}
		}

		return len(disruptedPods) == len(existingPods)
	}).WithContext(ctx).WithPolling(pollingInterval).WithTimeout(timeout).Should(BeTrue(),
		"Pods from previous deployments are still present")
}

// waitForNodeToBeNotReady waits for the node to be not ready.
func waitForNodeToBeNotReady(ctx SpecContext, nodeToPowerOff string, pollingInterval, timeout time.Duration) {
	By(fmt.Sprintf("Waiting for node %q to get into NotReady state", nodeToPowerOff))

	Eventually(func() bool {
		currentNode, err := nodes.Pull(APIClient, nodeToPowerOff)
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to pull node: %v", err)

			return false
		}

		for _, condition := range currentNode.Object.Status.Conditions {
			if condition.Type == rdscoreparams.ConditionTypeReadyString {
				if condition.Status != rdscoreparams.ConstantTrueString {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Node %q is notReady", currentNode.Definition.Name)
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Reason: %s", condition.Reason)

					return true
				}
			}
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Node %q is Ready", currentNode.Definition.Name)

		return false
	}).WithContext(ctx).WithPolling(pollingInterval).WithTimeout(timeout).Should(BeTrue(),
		"Node %q hasn't reached NotReady state", nodeToPowerOff)
}

// VerifyConnectivityAfterNodePowerOff verifies inter pod communication between the deployments
// after a node is powered off.
//
// sameNode parameter distinguishes between the case where the deployments are scheduled on the same node
// and the case where they are scheduled on different nodes.
func VerifyConnectivityAfterNodePowerOff(
	ctx SpecContext,
	configOne, configTwo WhereaboutsDeploymentConfig,
	sameNode bool) {
	By("Ensuring configs are not empty")

	Expect(configOne).ToNot(Equal(WhereaboutsDeploymentConfig{}), "configOne parameter is empty")

	Expect(configTwo).ToNot(Equal(WhereaboutsDeploymentConfig{}), "configTwo parameter is empty")

	By("Randomly picking up deployment for node power off")

	whereaboutsDeployments := []WhereaboutsDeploymentConfig{configOne, configTwo}

	randomIndex := rand.Intn(len(whereaboutsDeployments))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Picked up deployment %q for node power off",
		whereaboutsDeployments[randomIndex].Name)

	nodeToPowerOff, err := getRandomPodNode(whereaboutsDeployments[randomIndex].Label,
		RDSCoreConfig.WhereaboutNS, whereaboutsDeployments[randomIndex].Replicas)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Node to power off: %q", nodeToPowerOff)

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to get random pod node due to: %v", err))

	checkEnoughReadyNodes(ctx, nodeToPowerOff, WorkerNodeSelector, ExpectedReadyNodes)

	var existingPods []*pod.Builder

	if sameNode {
		By(fmt.Sprintf("Getting list of existing pods from deployments %q and %q", configOne.Name, configTwo.Name))

		existingPods = append(
			getActivePods(configOne.Label, RDSCoreConfig.WhereaboutNS),
			getActivePods(configTwo.Label, RDSCoreConfig.WhereaboutNS)...)

		Expect(int32(len(existingPods))).To(Equal(configOne.Replicas+configTwo.Replicas),
			fmt.Sprintf("Number of existing pods %d from deployments %q and %q is not equal to number of replicas %d",
				len(existingPods), configOne.Name, configTwo.Name, configOne.Replicas+configTwo.Replicas))
	} else {
		By(fmt.Sprintf("Getting list of existing pods from %q deployment", whereaboutsDeployments[randomIndex].Name))

		existingPods = getActivePods(whereaboutsDeployments[randomIndex].Label, RDSCoreConfig.WhereaboutNS)

		Expect(int32(len(existingPods))).To(Equal(whereaboutsDeployments[randomIndex].Replicas),
			fmt.Sprintf("Number of existing pods %d from %q deployment is not equal to number of replicas %d",
				len(existingPods), whereaboutsDeployments[randomIndex].Name,
				whereaboutsDeployments[randomIndex].Replicas))
	}

	By(fmt.Sprintf("Powering off node %q", nodeToPowerOff))

	var (
		bmcClient *bmc.BMC
	)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Creating BMC client for node %s", nodeToPowerOff)

	if auth, ok := RDSCoreConfig.NodesCredentialsMap[nodeToPowerOff]; !ok {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("BMC Details for %q not found", nodeToPowerOff)

		Fail(fmt.Sprintf("BMC Details for %q not found", nodeToPowerOff))
	} else {
		bmcClient = bmc.New(auth.BMCAddress).
			WithRedfishUser(auth.Username, auth.Password).
			WithRedfishTimeout(DefaultRedfishTimeout)
	}

	Expect(bmcClient).ToNot(BeNil(), "Failed to create BMC client for node %q", nodeToPowerOff)

	stopCh := make(chan bool, 1)

	defer powerOnNodeWaitReady(bmcClient, nodeToPowerOff, stopCh)

	go keepNodePoweredOff(bmcClient, nodeToPowerOff, DefaultNodePowerOffTimeout, stopCh)

	waitForNodeToBeNotReady(ctx, nodeToPowerOff, DefaultNodePollingInterval, DefaultNodePollingTimeout)

	waitForPodsToBeMarkedForDeletionOrDeleted(ctx, existingPods, DefaultNodePollingInterval, 2*DefaultNodePollingTimeout)

	waitForDeploymentReplicas(ctx, configOne, configOne.Replicas)

	waitForDeploymentReplicas(ctx, configTwo, configTwo.Replicas)

	By("Verifying inter pod communication between the deployments works after node power off")

	// Ensure inter pod communication between the deployments works after pod termination
	VerifyWhereaboutsInterDeploymentPodCommunication(ctx, configOne, configTwo)
}

// configureSameNodeDeployments configures the same node deployments.
func configureSameNodeDeployments() (WhereaboutsDeploymentConfig, WhereaboutsDeploymentConfig, error) {
	By("Validating deployment configuration")

	switch {
	case strings.TrimSpace(RDSCoreConfig.WhereaboutsDeployOnePort) == "":
		return WhereaboutsDeploymentConfig{}, WhereaboutsDeploymentConfig{},
			fmt.Errorf("parameter 'Port' is not set for 1st deployment")
	case strings.TrimSpace(RDSCoreConfig.WhereaboutsDeployTwoPort) == "":
		return WhereaboutsDeploymentConfig{}, WhereaboutsDeploymentConfig{},
			fmt.Errorf("parameter 'Port' is not set for 2nd deployment")
	case strings.TrimSpace(RDSCoreConfig.WhereaboutsDeployImageOne) == "":
		return WhereaboutsDeploymentConfig{}, WhereaboutsDeploymentConfig{},
			fmt.Errorf("parameter 'Image' is not set for 1st deployment")
	case strings.TrimSpace(RDSCoreConfig.WhereaboutsDeployImageTwo) == "":
		return WhereaboutsDeploymentConfig{}, WhereaboutsDeploymentConfig{},
			fmt.Errorf("parameter 'Image' is not set for 2nd deployment")
	case len(RDSCoreConfig.WhereaboutsDeployOneCMD) == 0:
		return WhereaboutsDeploymentConfig{}, WhereaboutsDeploymentConfig{},
			fmt.Errorf("parameter 'Command' is not set for 1st deployment")
	case len(RDSCoreConfig.WhereaboutsDeployTwoCMD) == 0:
		return WhereaboutsDeploymentConfig{}, WhereaboutsDeploymentConfig{},
			fmt.Errorf("parameter 'Command' is not set for 2nd deployment")
	case strings.TrimSpace(RDSCoreConfig.WhereaboutsDeployOneNAD) == "":
		return WhereaboutsDeploymentConfig{}, WhereaboutsDeploymentConfig{},
			fmt.Errorf("parameter 'NAD' is not set for 1st deployment")
	case strings.TrimSpace(RDSCoreConfig.WhereaboutsDeployTwoNAD) == "":
		return WhereaboutsDeploymentConfig{}, WhereaboutsDeploymentConfig{},
			fmt.Errorf("parameter 'NAD' is not set for 2nd deployment")
	default:
		if port, err := strconv.Atoi(RDSCoreConfig.WhereaboutsDeployOnePort); err != nil {
			return WhereaboutsDeploymentConfig{}, WhereaboutsDeploymentConfig{},
				fmt.Errorf("parameter 'Port' %v is not a valid integer for 1st deployment",
					RDSCoreConfig.WhereaboutsDeployOnePort)
		} else if port < 1 || port > 65535 {
			return WhereaboutsDeploymentConfig{}, WhereaboutsDeploymentConfig{},
				fmt.Errorf("parameter 'Port' %v is not in the valid range for 1st deployment",
					RDSCoreConfig.WhereaboutsDeployOnePort)
		}

		if port, err := strconv.Atoi(RDSCoreConfig.WhereaboutsDeployTwoPort); err != nil {
			return WhereaboutsDeploymentConfig{}, WhereaboutsDeploymentConfig{},
				fmt.Errorf("parameter 'Port' %v is not a valid integer for 2nd deployment",
					RDSCoreConfig.WhereaboutsDeployTwoPort)
		} else if port < 1 || port > 65535 {
			return WhereaboutsDeploymentConfig{}, WhereaboutsDeploymentConfig{},
				fmt.Errorf("parameter 'Port' %v is not in the valid range for 2nd deployment",
					RDSCoreConfig.WhereaboutsDeployTwoPort)
		}
	}

	configOne := SameNodeDeploymentOneConfig
	configOne.Port = RDSCoreConfig.WhereaboutsDeployOnePort
	configOne.Image = RDSCoreConfig.WhereaboutsDeployImageOne
	configOne.Command = RDSCoreConfig.WhereaboutsDeployOneCMD
	configOne.NAD = RDSCoreConfig.WhereaboutsDeployOneNAD

	configTwo := SameNodeDeploymentTwoConfig
	configTwo.Port = RDSCoreConfig.WhereaboutsDeployTwoPort
	configTwo.Image = RDSCoreConfig.WhereaboutsDeployImageTwo
	configTwo.Command = RDSCoreConfig.WhereaboutsDeployTwoCMD
	configTwo.NAD = RDSCoreConfig.WhereaboutsDeployTwoNAD

	return configOne, configTwo, nil
}

// configureDifferentNodeDeployments validates and initializes the configuration for deployments on different nodes.
// It validates WhereaboutsDeploy3* and WhereaboutsDeploy4* configuration parameters and returns
// configured DiffNodeDeploymentOneConfig and DiffNodeDeploymentTwoConfig objects.
func configureDifferentNodeDeployments() (WhereaboutsDeploymentConfig, WhereaboutsDeploymentConfig, error) {
	By("Validating deployment configuration")

	switch {
	case strings.TrimSpace(RDSCoreConfig.WhereaboutsDeploy3Port) == "":
		return WhereaboutsDeploymentConfig{}, WhereaboutsDeploymentConfig{},
			fmt.Errorf("parameter 'Port' is not set for 1st deployment")
	case strings.TrimSpace(RDSCoreConfig.WhereaboutsDeploy4Port) == "":
		return WhereaboutsDeploymentConfig{}, WhereaboutsDeploymentConfig{},
			fmt.Errorf("parameter 'Port' is not set for 2nd deployment")
	case strings.TrimSpace(RDSCoreConfig.WhereaboutsDeployImage3) == "":
		return WhereaboutsDeploymentConfig{}, WhereaboutsDeploymentConfig{},
			fmt.Errorf("parameter 'Image' is not set for 1st deployment")
	case strings.TrimSpace(RDSCoreConfig.WhereaboutsDeployImage4) == "":
		return WhereaboutsDeploymentConfig{}, WhereaboutsDeploymentConfig{},
			fmt.Errorf("parameter 'Image' is not set for 2nd deployment")
	case len(RDSCoreConfig.WhereaboutsDeploy3CMD) == 0:
		return WhereaboutsDeploymentConfig{}, WhereaboutsDeploymentConfig{},
			fmt.Errorf("parameter 'Command' is not set for 1st deployment")
	case len(RDSCoreConfig.WhereaboutsDeploy4CMD) == 0:
		return WhereaboutsDeploymentConfig{}, WhereaboutsDeploymentConfig{},
			fmt.Errorf("parameter 'Command' is not set for 2nd deployment")
	case strings.TrimSpace(RDSCoreConfig.WhereaboutsDeploy3NAD) == "":
		return WhereaboutsDeploymentConfig{}, WhereaboutsDeploymentConfig{},
			fmt.Errorf("parameter 'NAD' is not set for 1st deployment")
	case strings.TrimSpace(RDSCoreConfig.WhereaboutsDeploy4NAD) == "":
		return WhereaboutsDeploymentConfig{}, WhereaboutsDeploymentConfig{},
			fmt.Errorf("parameter 'NAD' is not set for 2nd deployment")
	default:
		if port, err := strconv.Atoi(RDSCoreConfig.WhereaboutsDeploy3Port); err != nil {
			return WhereaboutsDeploymentConfig{}, WhereaboutsDeploymentConfig{},
				fmt.Errorf("parameter 'Port' %v is not a valid integer for 1st deployment",
					RDSCoreConfig.WhereaboutsDeploy3Port)
		} else if port < 1 || port > 65535 {
			return WhereaboutsDeploymentConfig{}, WhereaboutsDeploymentConfig{},
				fmt.Errorf("parameter 'Port' %v is not in the valid range for 1st deployment",
					RDSCoreConfig.WhereaboutsDeploy3Port)
		}

		if port, err := strconv.Atoi(RDSCoreConfig.WhereaboutsDeploy4Port); err != nil {
			return WhereaboutsDeploymentConfig{}, WhereaboutsDeploymentConfig{},
				fmt.Errorf("parameter 'Port' %v is not a valid integer for 2nd deployment",
					RDSCoreConfig.WhereaboutsDeploy4Port)
		} else if port < 1 || port > 65535 {
			return WhereaboutsDeploymentConfig{}, WhereaboutsDeploymentConfig{},
				fmt.Errorf("parameter 'Port' %v is not in the valid range for 2nd deployment",
					RDSCoreConfig.WhereaboutsDeploy4Port)
		}
	}

	configOne := DiffNodeDeploymentOneConfig
	// Set runtime configuration values
	configOne.Port = RDSCoreConfig.WhereaboutsDeploy3Port
	configOne.Image = RDSCoreConfig.WhereaboutsDeployImage3
	configOne.Command = RDSCoreConfig.WhereaboutsDeploy3CMD
	configOne.NAD = RDSCoreConfig.WhereaboutsDeploy3NAD

	configTwo := DiffNodeDeploymentTwoConfig
	// Set runtime configuration values
	configTwo.Port = RDSCoreConfig.WhereaboutsDeploy4Port
	configTwo.Image = RDSCoreConfig.WhereaboutsDeployImage4
	configTwo.Command = RDSCoreConfig.WhereaboutsDeploy4CMD
	configTwo.NAD = RDSCoreConfig.WhereaboutsDeploy4NAD

	return configOne, configTwo, nil
}

// VerifyWhereaboutsInterDeploymentPodCommunicationOnTheSameNode creates two deployments
// with pods scheduled on the same node and verifies inter pod communication between them.
func VerifyWhereaboutsInterDeploymentPodCommunicationOnTheSameNode(ctx SpecContext) {
	configOne, configTwo, err := configureSameNodeDeployments()

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to configure same node deployments: %v", err))

	CreateWhereaboutsDeployment(ctx, configOne)

	CreateWhereaboutsDeployment(ctx, configTwo)

	VerifyWhereaboutsInterDeploymentPodCommunication(ctx, configOne, configTwo)
}

// VerifyWhereaboutsInterDeploymentPodCommunicationOnDifferentNodes creates two deployments
// with pods scheduled on different nodes and verifies inter pod communication between them.
func VerifyWhereaboutsInterDeploymentPodCommunicationOnDifferentNodes(ctx SpecContext) {
	configOne, configTwo, err := configureDifferentNodeDeployments()

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to configure different node deployments: %v", err))

	CreateWhereaboutsDeployment(ctx, configOne)

	CreateWhereaboutsDeployment(ctx, configTwo)

	VerifyWhereaboutsInterDeploymentPodCommunication(ctx, configOne, configTwo)
}

// VerifyWhereaboutsInterDeploymentPodCommunicationOnTheSameNodeAfterPodTermination creates two deployments
// with pods scheduled on the same node, terminates a pod from one of the deployments and verifies
// inter pod communication between them.
func VerifyWhereaboutsInterDeploymentPodCommunicationOnTheSameNodeAfterPodTermination(ctx SpecContext) {
	configOne, configTwo, err := configureSameNodeDeployments()

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to configure same node deployments: %v", err))

	CreateWhereaboutsDeployment(ctx, configOne)

	CreateWhereaboutsDeployment(ctx, configTwo)

	// Ensure inter pod communication between the deployments works before pod termination
	VerifyWhereaboutsInterDeploymentPodCommunication(ctx, configOne, configTwo)

	By("Randomly picking up deployment for pod termination")

	whereaboutsDeployments := []WhereaboutsDeploymentConfig{configOne, configTwo}

	randomIndex := rand.Intn(len(whereaboutsDeployments))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Picked up deployment %q for pod termination",
		whereaboutsDeployments[randomIndex].Name)

	terminateAndWaitPodFromDeployment(whereaboutsDeployments[randomIndex])

	By("Waiting for deployment to have new pods running")

	waitForDeploymentReplicas(ctx, whereaboutsDeployments[randomIndex], whereaboutsDeployments[randomIndex].Replicas)

	By("Verifying inter pod communication between the deployments works after pod termination")

	// Ensure inter pod communication between the deployments works after pod termination
	VerifyWhereaboutsInterDeploymentPodCommunication(ctx, configOne, configTwo)
}

// VerifyWhereaboutsInterDeploymentPodCommunicationOnDifferentNodesAfterPodTermination creates two deployments
// with pods scheduled on different nodes, terminates a pod from one of the deployments and verifies
// inter pod communication between them.
func VerifyWhereaboutsInterDeploymentPodCommunicationOnDifferentNodesAfterPodTermination(ctx SpecContext) {
	configOne, configTwo, err := configureDifferentNodeDeployments()

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to configure different node deployments: %v", err))

	CreateWhereaboutsDeployment(ctx, configOne)

	CreateWhereaboutsDeployment(ctx, configTwo)

	// Ensure inter pod communication between the deployments works before pod termination
	VerifyWhereaboutsInterDeploymentPodCommunication(ctx, configOne, configTwo)

	By("Randomly picking up deployment for pod termination")

	whereaboutsDeployments := []WhereaboutsDeploymentConfig{configOne, configTwo}

	randomIndex := rand.Intn(len(whereaboutsDeployments))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Picked up deployment %q for pod termination",
		whereaboutsDeployments[randomIndex].Name)

	terminateAndWaitPodFromDeployment(whereaboutsDeployments[randomIndex])

	By("Waiting for deployment to have new pods running")

	waitForDeploymentReplicas(ctx, whereaboutsDeployments[randomIndex], whereaboutsDeployments[randomIndex].Replicas)

	By("Verifying inter pod communication between the deployments works after pod termination")

	// Ensure inter pod communication between the deployments works after pod termination
	VerifyWhereaboutsInterDeploymentPodCommunication(ctx, configOne, configTwo)
}

// VerifyWhereaboutsInterDeploymentPodCommunicationOnTheSameNodeAfterNodeDrain creates two deployments
// with pods scheduled on the same node, drains a node and verifies
// inter pod communication between them.
func VerifyWhereaboutsInterDeploymentPodCommunicationOnTheSameNodeAfterNodeDrain(ctx SpecContext) {
	configOne, configTwo, err := configureSameNodeDeployments()

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to configure same node deployments: %v", err))

	CreateWhereaboutsDeployment(ctx, configOne)

	CreateWhereaboutsDeployment(ctx, configTwo)

	// Ensure inter pod communication between the deployments works before node drain
	VerifyWhereaboutsInterDeploymentPodCommunication(ctx, configOne, configTwo)

	VerifyConnectivityAfterNodeDrain(ctx, configOne, configTwo, true)
}

// VerifyWhereaboutsInterDeploymentPodCommunicationOnDifferentNodesAfterNodeDrain creates two deployments
// with pods scheduled on different nodes, drains a node and verifies
// inter pod communication between them.
func VerifyWhereaboutsInterDeploymentPodCommunicationOnDifferentNodesAfterNodeDrain(ctx SpecContext) {
	configOne, configTwo, err := configureDifferentNodeDeployments()

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to configure different node deployments: %v", err))

	CreateWhereaboutsDeployment(ctx, configOne)

	CreateWhereaboutsDeployment(ctx, configTwo)

	// Ensure inter pod communication between the deployments works before node drain
	VerifyWhereaboutsInterDeploymentPodCommunication(ctx, configOne, configTwo)

	VerifyConnectivityAfterNodeDrain(ctx, configOne, configTwo, false)
}

// VerifyWhereaboutsInterDeploymentPodCommunicationOnTheSameNodeAfterNodePowerOff creates two deployments
// with pods scheduled on the same node, powers off a node and verifies
// inter pod communication between them.
func VerifyWhereaboutsInterDeploymentPodCommunicationOnTheSameNodeAfterNodePowerOff(ctx SpecContext) {
	configOne, configTwo, err := configureSameNodeDeployments()

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to configure same node deployments: %v", err))

	CreateWhereaboutsDeployment(ctx, configOne)

	CreateWhereaboutsDeployment(ctx, configTwo)

	// Ensure inter pod communication between the deployments works before node power off
	VerifyWhereaboutsInterDeploymentPodCommunication(ctx, configOne, configTwo)

	VerifyConnectivityAfterNodePowerOff(ctx, configOne, configTwo, true)
}

// VerifyWhereaboutsInterDeploymentPodCommunicationOnDifferentNodesAfterNodePowerOff creates two deployments
// with pods scheduled on the different nodes, powers off a node and verifies
// inter pod communication between them.
func VerifyWhereaboutsInterDeploymentPodCommunicationOnDifferentNodesAfterNodePowerOff(ctx SpecContext) {
	configOne, configTwo, err := configureDifferentNodeDeployments()

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to configure different node deployments: %v", err))

	CreateWhereaboutsDeployment(ctx, configOne)

	CreateWhereaboutsDeployment(ctx, configTwo)

	// Ensure inter pod communication between the deployments works before node power off
	VerifyWhereaboutsInterDeploymentPodCommunication(ctx, configOne, configTwo)

	VerifyConnectivityAfterNodePowerOff(ctx, configOne, configTwo, false)
}

// VerifyPodCommunicationOnSameNodeAfterClusterReboot validates inter pod communication
// on the same node after cluster reboot.
func VerifyPodCommunicationOnSameNodeAfterClusterReboot(ctx SpecContext) {
	configOne, configTwo, err := configureSameNodeDeployments()

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to configure same node deployments: %v", err))

	// Ensure inter pod communication between the deployments works after cluster reboot
	VerifyWhereaboutsInterDeploymentPodCommunication(ctx, configOne, configTwo)
}

// VerifyPodCommunicationOnDifferentNodesAfterClusterReboot validates inter pod communication
// on different nodes after cluster reboot.
func VerifyPodCommunicationOnDifferentNodesAfterClusterReboot(ctx SpecContext) {
	configOne, configTwo, err := configureDifferentNodeDeployments()

	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to configure different node deployments: %v", err))

	// Ensure inter pod communication between the deployments works after cluster reboot
	VerifyWhereaboutsInterDeploymentPodCommunication(ctx, configOne, configTwo)
}
