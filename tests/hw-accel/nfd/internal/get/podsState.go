package get

import (
	"strings"

	"github.com/golang/glog"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/nfdhelpersparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/nfdparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodState an object that describe the name and state of a pod.
type PodState struct {
	Name  string
	State string
}

// PodStatus return a list pod and state.
func PodStatus(apiClient *clients.Settings, nsname string) ([]PodState, error) {
	podList, err := pod.List(apiClient, nsname, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	nfdResources := NfdResourceCount(apiClient)
	podStateList := make([]PodState, 0)

	for _, x := range podList {
		glog.V(nfdparams.LogLevel).Infof("%v", x.Object.Name)
	}

	for _, onePod := range podList {
		state := onePod.Object.Status.Phase

		glog.V(nfdparams.LogLevel).Infof("%s is in %s status", onePod.Object.Name, state)
		glog.V(nfdparams.LogLevel).Infof("%v", nfdResources)

		for _, nfdPodName := range nfdhelpersparams.ValidPodNameList {
			if strings.Contains(onePod.Object.Name, nfdPodName) {
				nfdResources[nfdPodName]--
				podStateList = append(podStateList, PodState{Name: onePod.Object.Name, State: string(state)})
			}
		}
	}

	return podStateList, nil
}
