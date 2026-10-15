//go:build unit_test

package await

import (
	"testing"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/events"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCountModuleEvents(t *testing.T) {
	const (
		moduleNamespace = "simple-kmod"
		moduleName      = "simple-kmod"
		moduleRef       = moduleNamespace + "/" + moduleName
	)

	loadedMessage := "Module simple-kmod/simple-kmod loaded into the kernel"
	unloadedMessage := "Module simple-kmod/simple-kmod unloaded from the kernel"

	tests := map[string]struct {
		events []*events.Builder
		reason string
		want   int
	}{
		"counts event and series occurrences": {
			events: []*events.Builder{
				newNodeEvent(corev1.Event{
					Reason:  "ModuleLoaded",
					Message: loadedMessage,
					Count:   2,
					ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
						moduleEventAnnotation: moduleRef,
					}},
				}),
				newNodeEvent(corev1.Event{
					Reason:  "ModuleLoaded",
					Message: loadedMessage,
					Count:   1,
					Series:  &corev1.EventSeries{Count: 3},
					ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
						moduleEventAnnotation: moduleRef,
					}},
				}),
			},
			reason: "ModuleLoaded",
			want:   5,
		},
		"ignores unrelated pod events": {
			events: []*events.Builder{
				newEvent(corev1.Event{
					InvolvedObject: corev1.ObjectReference{Kind: "Pod"},
					Reason:         "ModuleLoaded",
					Message:        loadedMessage,
					Count:          10,
				}),
			},
			reason: "ModuleLoaded",
			want:   0,
		},
		"uses annotation to reject a different module": {
			events: []*events.Builder{
				newNodeEvent(corev1.Event{
					Reason:  "ModuleLoaded",
					Message: loadedMessage,
					Count:   3,
					ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
						moduleEventAnnotation: "other-namespace/other-module",
					}},
				}),
			},
			reason: "ModuleLoaded",
			want:   0,
		},
		"supports legacy events without annotation": {
			events: []*events.Builder{
				newNodeEvent(corev1.Event{
					Reason:  "ModuleUnloaded",
					Message: unloadedMessage,
				}),
			},
			reason: "ModuleUnloaded",
			want:   1,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.want,
				countModuleEvents(test.events, test.reason, moduleNamespace, moduleName))
		})
	}
}

func TestCountAggregatedModuleEvents(t *testing.T) {
	const (
		moduleNamespace = "simple-kmod"
		moduleName      = "simple-kmod"
		moduleRef       = moduleNamespace + "/" + moduleName
	)

	loadedMessage := "Module simple-kmod/simple-kmod loaded into the kernel"
	unloadedMessage := "Module simple-kmod/simple-kmod unloaded from the kernel"

	tests := map[string]struct {
		event  corev1.Event
		reason string
		want   int
	}{
		"supports legacy event aggregation without annotation": {
			event: corev1.Event{
				Reason:  "ModuleLoaded",
				Message: "(combined from similar events): " + loadedMessage,
				Count:   2,
			},
			reason: "ModuleLoaded",
			want:   2,
		},
		"uses annotation for aggregated events": {
			event: corev1.Event{
				Reason:  "ModuleUnloaded",
				Message: "(combined from similar events): " + unloadedMessage,
				Count:   2,
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
					moduleEventAnnotation: moduleRef,
				}},
			},
			reason: "ModuleUnloaded",
			want:   2,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.want,
				countModuleEvents([]*events.Builder{newNodeEvent(test.event)},
					test.reason, moduleNamespace, moduleName))
		})
	}
}

func newNodeEvent(event corev1.Event) *events.Builder {
	event.InvolvedObject.Kind = "Node"

	return newEvent(event)
}

func newEvent(event corev1.Event) *events.Builder {
	return &events.Builder{Object: &event}
}
