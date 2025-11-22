package tests

import (
	"strings"

	"github.com/hashicorp/go-version"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/olm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/get"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmminittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/mcm/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
)

var _ = Describe("KMM-Hub", Ordered, Label(tsparams.LabelSuite), func() {
	Context("MCM", Label("hub-check-install"), func() {

		It("Operator should be properly installed", reportxml.ID("56674"), func() {
			if ModulesConfig.SubscriptionName == "" {
				Skip("No subscription name defined. Skipping test")
			}

			By("Checking subscription exists")
			sub, err := olm.PullSubscription(APIClient, ModulesConfig.SubscriptionName, kmmparams.KmmHubOperatorNamespace)
			Expect(err).NotTo(HaveOccurred(), "error getting subscription")
			Expect(string(sub.Object.Status.State)).To(Equal("AtLatestKnown"))

			By("Checking operator namespace exists")
			exists := namespace.NewBuilder(APIClient, kmmparams.KmmHubOperatorNamespace).Exists()
			Expect(exists).To(Equal(true))

			By("Listing deployment in operator namespace")
			deploymentList, err := deployment.List(APIClient, kmmparams.KmmHubOperatorNamespace)
			Expect(err).NotTo(HaveOccurred(), "error getting deployment list")

			By("Checking KMM-HUB deployment")
			for _, ds := range deploymentList {
				if strings.Contains(ds.Object.Name, kmmparams.HubDeploymentName) {
					Expect(ds.Object.Status.ReadyReplicas).To(Equal(int32(1)))
					klog.V(kmmparams.KmmLogLevel).Infof("Successfully found deployment '%s'"+
						" with ReadyReplicas %d", ds.Object.Name, ds.Object.Status.ReadyReplicas)
				}
			}
		})

		It("HUB Webhook server be properly installed", reportxml.ID("72718"), func() {
			By("Checking if version is greater than 2.1.0")
			currentVersion, err := get.KmmHubOperatorVersion(APIClient)
			Expect(err).ToNot(HaveOccurred(), "failed to get current KMM version")

			featureFromVersion, _ := version.NewVersion("2.1.0")
			if currentVersion.LessThan(featureFromVersion) {
				Skip("Test not supported for versions lower than 2.1.0")
			}

			By("Listing deployments in KMM-HUB operator namespace")
			deploymentList, err := deployment.List(APIClient, kmmparams.KmmHubOperatorNamespace)
			Expect(err).NotTo(HaveOccurred(), "error getting deployment list")

			By("Checking KMM deployment")
			for _, ds := range deploymentList {
				if strings.Contains(ds.Object.Name, kmmparams.HubWebhookDeploymentName) {
					Expect(ds.Object.Status.ReadyReplicas).To(Equal(int32(1)))
					klog.V(kmmparams.KmmLogLevel).Infof("Successfully found deployment '%s'"+
						" with ReadyReplicas %d", ds.Object.Name, ds.Object.Status.ReadyReplicas)
				}
			}
		})

	})
})
