package spoke_test

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmh"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/ztpinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/spoke/internal/tsparams"
	"k8s.io/klog/v2"
)

const (
	bmhString = "bmac.agent-install.openshift.io.node-label.node-role.kubernetes.io"
)

var (
	spokeClusterNameSpace string
	agentLabelsList       map[string]map[string]string
	bmhLabelsList         map[string]map[string]string
)

var _ = Describe(
	"NodeLabelling",
	Ordered,
	ContinueOnFailure,
	Label(tsparams.LabelNodeLabellingAtInstallTestCases), func() {
		BeforeAll(func() {
			By("Get the spoke cluster name")
			spokeClusterNameSpace = ZTPConfig.SpokeClusterName

			By("Get list of agents by using infraenv")
			agentsBuilderList, err := ZTPConfig.SpokeInfraEnv.GetAllAgents()
			Expect(err).ToNot(HaveOccurred(), "error getting list of agents")

			By("Populating agents, bmh lists")
			agentLabelsList = make(map[string]map[string]string)
			bmhLabelsList = make(map[string]map[string]string)

			for _, agent := range agentsBuilderList {
				if len(agent.Object.Spec.NodeLabels) != 0 {
					hostName := agent.Object.Spec.Hostname
					if hostName == "" {
						hostName = agent.Object.Status.Inventory.Hostname
					}
					agentLabelsList[hostName] = make(map[string]string)
					agentLabelsList[hostName] = agent.Object.Spec.NodeLabels
					if bmhName, ok := agent.Object.Labels["agent-install.openshift.io/bmh"]; ok {
						bmhObject, err := bmh.Pull(HubAPIClient, bmhName, spokeClusterNameSpace)
						Expect(err).ToNot(HaveOccurred(), "error getting bmh")
						bmhLabelsList[hostName] = make(map[string]string)
						for annotation := range bmhObject.Object.Annotations {
							if strings.Contains(annotation, bmhString) {
								bmhLabelsList[hostName][strings.TrimPrefix(annotation, "bmac.agent-install.openshift.io.node-label.")] = ""
							}
						}
					}
				}
			}
			if len(agentLabelsList) == 0 && len(bmhLabelsList) == 0 {
				Skip("skipping tests as no labels were set for the nodes")
			}
		})

		It("Node label appears on agents and spoke nodes with ZTP flow", reportxml.ID("60399"), func() {
			if len(bmhLabelsList) == 0 {
				Skip("no BMHs were used")
			}
			if len(bmhLabelsList) > 0 {
				numNodes := 0
				for agent := range agentLabelsList {
					if labelList, exist := bmhLabelsList[agent]; exist {
						passedCheck, err := checkNodeHasAllLabels(ZTPConfig.SpokeAPIClient, agent, labelList)
						Expect(err).ToNot(HaveOccurred(), "error in retrieving nodes and node labels")
						if passedCheck {
							numNodes++
						}
					}
				}
				Expect(numNodes).To(Equal(len(bmhLabelsList)), "not all nodes have all the labels")
			}
		})

		It("Node label appears on agents and spoke nodes with boot-it-yourself flow", reportxml.ID("60498"), func() {
			numNodes := 0
			expectedNumNodes := len(agentLabelsList) - len(bmhLabelsList)
			if expectedNumNodes == 0 {
				Skip("no agents were booted with boot-it-yourself")
			}
			for agent, labelList := range agentLabelsList {
				if _, exist := bmhLabelsList[agent]; !exist {
					passedCheck, err := checkNodeHasAllLabels(ZTPConfig.SpokeAPIClient, agent, labelList)
					Expect(err).ToNot(HaveOccurred(), "error in retrieving nodes and node labels")
					if passedCheck {
						numNodes++
					}
				}
			}
			Expect(numNodes).To(Equal(expectedNumNodes), "not all nodes have all of the labels")
		})
	})

// checkNodeHasAllLabels checks if all labels are present on a specific node.
func checkNodeHasAllLabels(
	apiClient *clients.Settings,
	nodeName string,
	nodeLabels map[string]string) (bool, error) {
	myNode, err := nodes.Pull(apiClient, nodeName)
	if err != nil {
		klog.V(100).Infof("could not discover nodes, error encountered: '%v'", err)

		return false, err
	}

	for label := range nodeLabels {
		if _, ok := myNode.Object.Labels[label]; !ok {
			return false, fmt.Errorf("node '%v' does not have the label '%v'", nodeName, label)
		}
	}

	return true, nil
}
