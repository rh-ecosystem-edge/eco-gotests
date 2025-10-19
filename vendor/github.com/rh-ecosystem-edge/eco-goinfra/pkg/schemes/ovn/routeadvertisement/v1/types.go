package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ovn/types"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:path=routeadvertisements,scope=Cluster,shortName=ra,singular=routeadvertisement
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=".status.status"
// RouteAdvertisements is the Schema for the routeadvertisements API
type RouteAdvertisements struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RouteAdvertisementsSpec   `json:"spec,omitempty"`
	Status RouteAdvertisementsStatus `json:"status,omitempty"`
}

// RouteAdvertisementsSpec defines the desired state of RouteAdvertisements
// +kubebuilder:validation:XValidation:rule="(!has(self.nodeSelector.matchLabels) && !has(self.nodeSelector.matchExpressions)) || !('PodNetwork' in self.advertisements)",message="If 'PodNetwork' is selected for advertisement, a 'nodeSelector' can't be specified as it needs to be advertised on all nodes"
// +kubebuilder:validation:XValidation:rule="!self.networkSelectors.exists(i, i.networkSelectionType != 'DefaultNetwork' && i.networkSelectionType != 'ClusterUserDefinedNetworks')",message="Only DefaultNetwork or ClusterUserDefinedNetworks can be selected"
type RouteAdvertisementsSpec struct {
	// TargetVRF controls in which VRF the pods advertised routes will be installed.
	// Leave empty for the default VRF.
	// +optional
	TargetVRF string `json:"targetVRF,omitempty"`

	// NetworkSelectors determines which networks the router should advertise.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=1
	NetworkSelectors types.NetworkSelectors `json:"networkSelectors"`

	// NodeSelector selects the nodes that should advertise the selected networks.
	// When empty, all nodes are selected.
	// +optional
	NodeSelector metav1.LabelSelector `json:"nodeSelector,omitempty"`

	// FrrConfigurationSelector selects the FRRConfigurations that should be used for advertising.
	// When empty, all FRRConfigurations will be used.
	// +optional
	FrrConfigurationSelector metav1.LabelSelector `json:"frrConfigurationSelector,omitempty"`

	// Advertisements determines the type of network announcements.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=1
	Advertisements []AdvertisementType `json:"advertisements"`
}

// RouteAdvertisementsStatus defines the observed state of RouteAdvertisements
type RouteAdvertisementsStatus struct {
	// Status reports the state of the RouteAdvertisements.
	// +optional
	Status string `json:"status,omitempty"`

	// Conditions contains the different condition statuses for the RouteAdvertisements.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true

// RouteAdvertisementsList contains a list of RouteAdvertisements
type RouteAdvertisementsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RouteAdvertisements `json:"items"`
}

// AdvertisementType defines the type of network advertisement
// +kubebuilder:validation:Enum=PodNetwork
type AdvertisementType string

const (
	// PodNetwork advertises pod network routes
	PodNetwork AdvertisementType = "PodNetwork"
)
