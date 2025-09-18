/*
The content of this file is taken from 'tests/hw-accel/nvidiagpu/internal/nvidiagpuconfig/device-config.go'. The full

	content should be moved to 'tests/hw-accel/internal'
	LabelPresentOnAllNodes
*/
package labels

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	amdparams "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/amdgpu/params"
)

// LabelPresentOnAllNodes checks if label is present on all nodes matching nodeSelector.
func LabelPresentOnAllNodes(apiClient *clients.Settings, nodeLabel, nodeLabelValue string,
	nodeSelector map[string]string) (bool, error) {
	nodeBuilder, err := nodes.List(apiClient, metav1.ListOptions{LabelSelector: labels.Set(nodeSelector).String()})

	// in all the nodes that match the nodeSelectors, look for specific label
	// For example, look in all the worker nodes for a specific label with specific value
	if err != nil {
		glog.V(amdparams.AMDGPULogLevel).Infof("could not discover %v nodes, error encountered: '%v'",
			nodeSelector, err)

		return false, err
	}

	// Sample label: feature.node.kubernetes.io/system-os_release.ID=rhcos.
	foundLabels := 0

	for _, node := range nodeBuilder {
		labelValue := node.Object.Labels[nodeLabel]

		if labelValue == nodeLabelValue {
			glog.V(amdparams.AMDGPULogLevel).Infof("Found label %v that contains %v with label value %s on "+
				"node %v", nodeLabel, nodeLabel, nodeLabelValue, node.Object.Name)

			foundLabels++
			// if all nodes matching nodeSelector have this label with label value.
			if foundLabels == len(nodeBuilder) {
				return true, nil
			}
		}
	}

	err = fmt.Errorf("not all (%v) nodes have the label '%s' with value '%s'", len(nodeBuilder),
		nodeLabel, nodeLabelValue)

	return false, err
}

// LabelPresentOnAtLeastOneNode checks if label is present on at least one node matching nodeSelector.
func LabelPresentOnAtLeastOneNode(apiClient *clients.Settings,
	nodeLabel string, nodeSelector map[string]string) (bool, error) {
	nodeBuilder, err := nodes.List(apiClient, metav1.ListOptions{LabelSelector: labels.Set(nodeSelector).String()})

	// Check if at least one node matching the nodeSelector has the specific nodeLabel label set to true
	// For example, look in all the worker nodes for specific label
	if err != nil {
		glog.V(amdparams.AMDGPULogLevel).Infof("could not discover %v nodes", nodeSelector)

		return false, err
	}

	for _, node := range nodeBuilder {
		labelValue, ok := node.Object.Labels[nodeLabel]

		if ok {
			glog.V(amdparams.AMDGPULogLevel).Infof("Found label '%v' with label value '%v' on node '%v'",
				nodeLabel, labelValue, node.Object.Name)

			return true, nil
		}
	}

	err = fmt.Errorf("could not find one node with label '%s' set to true", nodeLabel)

	return false, err
}

// LabelsExistOnAllNodes - Check if labels exist on all given nodes.
func LabelsExistOnAllNodes(labelNodes []*nodes.Builder,
	labels []string, timeout time.Duration, interval time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	errCh := make(chan error, len(labelNodes))

	var waitGroup sync.WaitGroup

	for _, labelNode := range labelNodes {
		waitGroup.Add(1)

		go LabelsExistOnNode(ctx, labelNode, labels, timeout, interval, &waitGroup, errCh, true)
	}

	waitGroup.Wait()

	if len(errCh) > 0 {
		return fmt.Errorf("errors encountered during labels exist on all nodes: %w", <-errCh)
	}

	return nil
}

// LabelsExistOnNode - Make sure that all given labels exist on a Node. An Error is sent it an Error Channel if not.
//
//gocognit:ignore
func LabelsExistOnNode(parentCtx context.Context, labelNode *nodes.Builder, labels []string,
	timeout time.Duration, interval time.Duration, waitGroup *sync.WaitGroup, errCh chan error, checkAMDDeviceID bool) {
	defer waitGroup.Done()

	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	var foundLabels []string
	// foundLabelsIter - Last complete iteration of labels check
	// Needed for more informative error message
	var foundLabelsIter []string

	for {
		select {
		case <-ctx.Done():
			var missingLabels []string

			for _, label := range labels {
				if !slices.Contains(foundLabelsIter, label) {
					missingLabels = append(missingLabels, label)
				}
			}
			errCh <- fmt.Errorf("timeout exceeded while checking "+
				"labels exist on node %v. Missing labels are: %v", labelNode.Object.Name, missingLabels)

			return
		case <-time.After(interval):
			foundLabels = nil

			labelNode.Exists()

			for _, label := range labels {
				glog.V(amdparams.AMDGPULogLevel).Infof("Checking label '%v' on node '%s'\n", label, labelNode.Object.Name)
				labelVal, labelFound := labelNode.Object.Labels[label]

				if labelFound {
					foundLabels = append(foundLabels, label)

					if checkAMDDeviceID && strings.HasSuffix(label, "device-id") {
						deviceName, deviceFound := amdparams.DeviceIDsMap[labelVal]
						if !deviceFound {
							errCh <- fmt.Errorf("the device '%v' ('%v') isn't"+
								" found in the list of supported devices", labelVal, deviceName)

							return
						}
					}
				}
			}

			foundLabelsIter = foundLabels

			if len(foundLabels) == len(labels) {
				return
			}
		}
	}
}

// NodeLabellersLabelsMissingOnAllAMDGPUNode - Make sure Node Labeller Labels don't exit on all AMD GPU Worker Nodes.
func NodeLabellersLabelsMissingOnAllAMDGPUNode(amdGpuNodes []*nodes.Builder) error {
	return LabelsMissingOnAllNode(amdGpuNodes, amdparams.NodeLabellerLabels,
		amdparams.DefaultTimeout, amdparams.DefaultSleepInterval)
}

// LabelsMissingOnAllNode - Make sure the given labels don't exist on all given nodes.
func LabelsMissingOnAllNode(labelNodes []*nodes.Builder, labels []string,
	timeout time.Duration, interval time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	errCh := make(chan error, len(labelNodes))
	defer close(errCh)

	var waitGroup sync.WaitGroup

	for _, labelNode := range labelNodes {
		waitGroup.Add(1)

		go LabelsMissingOnNode(ctx, labelNode, labels, timeout, interval, &waitGroup, errCh)
	}

	waitGroup.Wait()

	if len(errCh) > 0 {
		var allErrors []error
		for err := range errCh {
			allErrors = append(allErrors, err)
		}

		return fmt.Errorf("errors encountered during missing labels check: %w", errors.Join(allErrors...))
	}

	return nil
}

// LabelsMissingOnNode - Make sure that all given label don't exist on the given node.
// In case of error or if some label still exist after the given timeout.
// Error will be sent to the given error channel errCh.
func LabelsMissingOnNode(parentCtx context.Context, labelNode *nodes.Builder, labels []string,
	timeout time.Duration, interval time.Duration, waitGroup *sync.WaitGroup, errCh chan error) {
	defer waitGroup.Done()

	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	var existingLabels []string

	var existingLabelsIter []string

	for {
		select {
		case <-ctx.Done():
			errCh <- fmt.Errorf("some labels are still exist on node '%s': %v", labelNode.Object.Name, existingLabelsIter)

			return
		case <-time.After(interval):
			existingLabels = nil

			// Making sure the nodeBuilder object is updated
			labelNode.Exists()

			for _, label := range labels {
				_, labelFound := labelNode.Object.Labels[label]

				if labelFound {
					existingLabels = append(existingLabels, label)
				}
			}

			existingLabelsIter = existingLabels

			if len(existingLabels) == 0 {
				return
			}
		}
	}
}
