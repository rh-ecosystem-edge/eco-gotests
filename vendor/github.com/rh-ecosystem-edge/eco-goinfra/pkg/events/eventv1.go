package events

import (
	"context"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	eventsv1 "k8s.io/api/events/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// EventV1Builder provides a struct for the events.k8s.io/v1 Event resource containing a connection to the cluster and
// the Event definition.
type EventV1Builder struct {
	common.EmbeddableBuilder[eventsv1.Event, *eventsv1.Event]
	common.EmbeddableCreator[eventsv1.Event, EventV1Builder, *eventsv1.Event, *EventV1Builder]
	common.EmbeddableDeleter[eventsv1.Event, *eventsv1.Event]
}

// AttachMixins wires the embedded CRUD mixins to this builder instance.
func (builder *EventV1Builder) AttachMixins() {
	builder.EmbeddableCreator.SetBase(builder)
	builder.EmbeddableDeleter.SetBase(builder)
}

// GetGVK returns the events.k8s.io/v1 Event GVK for this builder.
func (builder *EventV1Builder) GetGVK() schema.GroupVersionKind {
	return eventsv1.SchemeGroupVersion.WithKind("Event")
}

// NewEventV1Builder creates a new instance of EventV1Builder.
func NewEventV1Builder(apiClient *clients.Settings, name, nsname string) *EventV1Builder {
	return common.NewNamespacedBuilder[eventsv1.Event, EventV1Builder](
		apiClient, eventsv1.AddToScheme, name, nsname)
}

// PullEventV1 fetches an existing events.k8s.io/v1 Event from the cluster by name and namespace.
func PullEventV1(apiClient *clients.Settings, name, nsname string) (*EventV1Builder, error) {
	return common.PullNamespacedBuilder[eventsv1.Event, EventV1Builder](
		context.TODO(), apiClient, eventsv1.AddToScheme, name, nsname)
}

// ListEventV1s returns all events.k8s.io/v1 with the provided options.
func ListEventV1s(
	apiClient *clients.Settings, options ...runtimeclient.ListOption) ([]*EventV1Builder, error) {
	return common.List[eventsv1.Event, eventsv1.EventList, EventV1Builder](
		context.TODO(), apiClient, eventsv1.AddToScheme, options...)
}
