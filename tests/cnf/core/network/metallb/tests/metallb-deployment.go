package tests

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/daemonset"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/metallb"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/metallb/mlboperator"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/metallbenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/metallb/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	controllerPriorityClass   = "example"
	speakerPriorityClass      = "high-priority"
	runtimeClassName          = "myclass"
	tolerationKeyExample      = "example"
	controllerAnnotationKey   = "controller-annotation"
	controllerAnnotationValue = "demo-controller"
	controllerAffinityLabel   = "controller-test"
	speakerAnnotationKey      = "speaker-annotation"
	speakerAnnotationValue    = "demo-speaker"
	speakerAffinityLabel      = "speaker-test"
)

var _ = Describe("MetalLB Deployment", Label(tsparams.LabelMetalLBDeployment), Ordered, ContinueOnFailure, func() {
	BeforeAll(func() {
		By("Removing existing MetalLB configuration")

		err := metallbenv.DeleteMetalLbCRAndWait(tsparams.DefaultTimeout)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		By("Removing MetalLB configuration")

		err := metallbenv.DeleteMetalLbCRAndWait(tsparams.DefaultTimeout)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterAll(func() {
		By("Restoring baseline MetalLB deployment for subsequent test cases")

		err := metallbenv.CreateNewMetalLbDaemonSetAndWaitUntilItsRunning(
			tsparams.DefaultTimeout, NetConfig.WorkerLabelMap)
		Expect(err).ToNot(HaveOccurred(), "Failed to restore baseline MetalLB deployment")
	})

	It("Create deployment with all parameters set", reportxml.ID("54131"), func() {
		By("Creating MetalLB with controller and speaker configuration")

		configureMetalLb(true)

		By("Verifying MetalLB controller and speaker deployments")
		verifyMetalLbDeploymentsEventually()
	})

	It("Update deployment parameters with baseline deployment", reportxml.ID("54132"), func() {
		By("Creating a new instance of MetalLB Speakers on workers")

		err := metallbenv.CreateNewMetalLbDaemonSetAndWaitUntilItsRunning(
			tsparams.DefaultTimeout, NetConfig.WorkerLabelMap)
		Expect(err).ToNot(HaveOccurred(), "Failed to create baseline MetalLB deployment")

		By("Updating MetalLB controller and speaker configuration")

		configureMetalLb(false)

		By("Verifying MetalLB controller and speaker deployments")
		verifyMetalLbDeploymentsEventually()
	})
})

func configureMetalLb(createNew bool) {
	var (
		metalLbBuilder *metallb.Builder
		err            error
	)

	if createNew {
		metalLbBuilder = metallb.NewBuilder(
			APIClient, tsparams.MetalLbIo, NetConfig.MlbOperatorNamespace, NetConfig.WorkerLabelMap)
		defineMetalLbConfig(metalLbBuilder)
		_, err = metalLbBuilder.Create()
	} else {
		metalLbBuilder, err = metallb.Pull(APIClient, tsparams.MetalLbIo, NetConfig.MlbOperatorNamespace)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull MetalLB CR")
		defineMetalLbConfig(metalLbBuilder)
		_, err = metalLbBuilder.Update(false)
	}

	Expect(err).ToNot(HaveOccurred(), "Failed to persist MetalLB CR")
}

func defineMetalLbConfig(metalLbBuilder *metallb.Builder) {
	metallbCR := metalLbBuilder.Definition

	metallbCR.Spec.ControllerConfig = &mlboperator.Config{
		PriorityClassName: controllerPriorityClass,
		RuntimeClassName:  runtimeClassName,
		Annotations: map[string]string{
			controllerAnnotationKey: controllerAnnotationValue,
		},
		Affinity: defineMetalLbPodAffinity(controllerAffinityLabel),
	}

	metallbCR.Spec.ControllerTolerations = []corev1.Toleration{expectedMetalLbToleration()}

	metallbCR.Spec.SpeakerConfig = &mlboperator.Config{
		PriorityClassName: speakerPriorityClass,
		RuntimeClassName:  runtimeClassName,
		Annotations: map[string]string{
			speakerAnnotationKey: speakerAnnotationValue,
		},
		Affinity: defineMetalLbPodAffinity(speakerAffinityLabel),
	}

	metallbCR.Spec.SpeakerTolerations = []corev1.Toleration{expectedMetalLbToleration()}
}

func expectedMetalLbToleration() corev1.Toleration {
	return corev1.Toleration{
		Key:      tolerationKeyExample,
		Operator: corev1.TolerationOpExists,
		Effect:   corev1.TaintEffectNoExecute,
	}
}

func defineMetalLbPodAffinity(component string) *corev1.Affinity {
	return &corev1.Affinity{PodAffinity: &corev1.PodAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
			{
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"component": component},
				},
				TopologyKey: corev1.LabelHostname,
			},
		},
	}}
}

func verifyMetalLbDeploymentsEventually() {
	GinkgoHelper()

	Eventually(func(gomega Gomega) {
		controlDep, err := deployment.Pull(
			APIClient, tsparams.MetalLbControllerName, NetConfig.MlbOperatorNamespace)
		gomega.Expect(err).NotTo(HaveOccurred(), "Unable to get MetalLB controller deployment")
		verifyMetalLbPodTemplateSpec(controlDep.Object.Spec.Template,
			controllerPriorityClass, controllerAnnotationKey, controllerAnnotationValue, controllerAffinityLabel)(gomega)

		speakerDs, err := daemonset.Pull(APIClient, tsparams.MetalLbDsName, NetConfig.MlbOperatorNamespace)
		gomega.Expect(err).NotTo(HaveOccurred(), "Unable to get MetalLB speaker daemonSet")
		verifyMetalLbPodTemplateSpec(speakerDs.Object.Spec.Template,
			speakerPriorityClass, speakerAnnotationKey, speakerAnnotationValue, speakerAffinityLabel)(gomega)
	}).WithTimeout(tsparams.DefaultTimeout).WithPolling(tsparams.DefaultRetryInterval).Should(Succeed(),
		"MetalLB deployment parameters were not applied to controller and speaker")
}

func verifyMetalLbPodTemplateSpec(
	podTemplate corev1.PodTemplateSpec,
	priorityClass, annotationKey, annotationValue, affinityLabel string,
) func(Gomega) {
	return func(gomega Gomega) {
		podSpec := podTemplate.Spec

		gomega.Expect(podSpec.PriorityClassName).To(Equal(priorityClass))
		gomega.Expect(podSpec.RuntimeClassName).NotTo(BeNil())
		gomega.Expect(*podSpec.RuntimeClassName).To(Equal(runtimeClassName))
		gomega.Expect(podTemplate.Annotations).To(HaveKeyWithValue(annotationKey, annotationValue))
		gomega.Expect(podSpec.Affinity).NotTo(BeNil())
		gomega.Expect(podSpec.Affinity.PodAffinity).NotTo(BeNil())
		gomega.Expect(podSpec.Affinity.PodAffinity.RequiredDuringSchedulingIgnoredDuringExecution).NotTo(BeEmpty())
		term := podSpec.Affinity.PodAffinity.RequiredDuringSchedulingIgnoredDuringExecution[0]
		gomega.Expect(term.LabelSelector).NotTo(BeNil())
		gomega.Expect(term.LabelSelector.MatchLabels).To(HaveKeyWithValue("component", affinityLabel))
		verifyMetalLbToleration(podSpec.Tolerations)(gomega)
	}
}

func verifyMetalLbToleration(tolerations []corev1.Toleration) func(Gomega) {
	return func(gomega Gomega) {
		expected := expectedMetalLbToleration()
		matched := false

		for _, toleration := range tolerations {
			if toleration.Key == expected.Key &&
				toleration.Operator == expected.Operator &&
				toleration.Effect == expected.Effect {
				matched = true

				break
			}
		}

		gomega.Expect(matched).To(BeTrue(), "expected MetalLB toleration was not found in pod spec")
	}
}
