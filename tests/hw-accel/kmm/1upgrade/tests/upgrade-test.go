package tests

import (
	"strings"
	"time"

	"github.com/golang/glog"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/olm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/1upgrade/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/1upgrade/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmminittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
)

var _ = Describe("KMM", Ordered, Label(tsparams.LabelSuite), func() {
	Context("Operator", Label("upgrade"), func() {
		It("should upgrade successfully", reportxml.ID("53609"), func() {

			if ModulesConfig.CatalogSourceName == "" {
				Skip("No CatalogSourceName defined. Skipping test")
			}

			if ModulesConfig.UpgradeTargetVersion == "" {
				Skip("No UpgradeTargetVersion defined. Skipping test ")
			}

			opNamespace := kmmparams.KmmOperatorNamespace
			if strings.Contains(ModulesConfig.SubscriptionName, "hub") {
				opNamespace = kmmparams.KmmHubOperatorNamespace
			}
			By("Getting KMM subscription")
			sub, err := olm.PullSubscription(APIClient, ModulesConfig.SubscriptionName, opNamespace)
			Expect(err).ToNot(HaveOccurred(), "failed getting subscription")

			By("Update subscription to use new channel, if defined")
			if ModulesConfig.CatalogSourceChannel != "" {
				glog.V(90).Infof("setting subscription channel to: %s", ModulesConfig.CatalogSourceChannel)
				sub.Definition.Spec.Channel = ModulesConfig.CatalogSourceChannel
			}

			By("Update subscription to use new catalog source")
			glog.V(90).Infof("Subscription's catalog source: %s", sub.Object.Spec.CatalogSource)
			sub.Definition.Spec.CatalogSource = ModulesConfig.CatalogSourceName
			_, err = sub.Update()
			Expect(err).ToNot(HaveOccurred(), "failed updating subscription")

			By("Await operator to be upgraded")
			err = await.OperatorUpgrade(APIClient, ModulesConfig.UpgradeTargetVersion, 2*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "failed awaiting subscription upgrade")
		})
	})
})
