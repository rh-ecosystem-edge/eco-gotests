/*
SPDX-FileCopyrightText: Red Hat

SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GeoLocation contains geographic coordinates for a location.
// Based on O-RAN.WG6.TS.O2IMS-INTERFACE-R005-v11.00 section 3.2.6.2.16
//
// +kubebuilder:validation:XValidation:rule="double(self.latitude) >= -90.0 && double(self.latitude) <= 90.0",message="latitude must be between -90.0 and 90.0"
// +kubebuilder:validation:XValidation:rule="double(self.longitude) >= -180.0 && double(self.longitude) <= 180.0",message="longitude must be between -180.0 and 180.0"
type GeoLocation struct {
	// Latitude is the latitude coordinate in decimal degrees.
	// Valid range: -90.0 to 90.0. Must be a valid decimal number string.
	// +kubebuilder:validation:Pattern=`^-?[0-9]+(\.[0-9]+)?$`
	Latitude string `json:"latitude"`

	// Longitude is the longitude coordinate in decimal degrees.
	// Valid range: -180.0 to 180.0. Must be a valid decimal number string.
	// +kubebuilder:validation:Pattern=`^-?[0-9]+(\.[0-9]+)?$`
	Longitude string `json:"longitude"`

	// Altitude is the altitude in meters above sea level.
	// Must be a valid decimal number string.
	// +kubebuilder:validation:Pattern=`^-?[0-9]+(\.[0-9]+)?$`
	// +optional
	Altitude *string `json:"altitude,omitempty"`
}

// CivicAddressElement represents a single civic address element per RFC 4776.
// The caType field identifies the type of element (e.g., country=0, state=1, city=3, etc.)
type CivicAddressElement struct {
	// CaType is the civic address type as defined in RFC 4776
	// Common values: 0=country, 1=state/province, 3=city, 6=street, 19=building, 26=unit
	CaType int `json:"caType"`

	// CaValue is the value for this civic address element
	// +kubebuilder:validation:MinLength=1
	CaValue string `json:"caValue"`
}

// LocationSpec defines the desired state of Location.
// Represents a physical or logical location where O-Cloud Sites can be deployed.
// Based on O-RAN.WG6.TS.O2IMS-INTERFACE-R005-v11.00 section 3.2.6.2.16
//
// Note: The globalLocationId for API responses is derived from metadata.name.
// Use metadata.name as the SMO-defined identifier when creating Location CRs.
//
// +kubebuilder:validation:XValidation:rule="has(self.coordinate) || (has(self.civicAddress) && size(self.civicAddress) > 0) || (has(self.address) && size(self.address) > 0)",message="at least one of coordinate, non-empty civicAddress, or non-empty address must be specified"
type LocationSpec struct {
	// Description provides additional details about the location
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Description",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	Description string `json:"description"`

	// Coordinate contains the geographic coordinates (latitude, longitude, altitude)
	// +optional
	Coordinate *GeoLocation `json:"coordinate,omitempty"`

	// CivicAddress contains RFC 4776 civic address elements.
	// Each element has a type (caType) and value (caValue).
	// +optional
	CivicAddress []CivicAddressElement `json:"civicAddress,omitempty"`

	// Address is a human-readable address string
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Address",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	Address *string `json:"address,omitempty"`

	// Extensions contains additional custom attributes
	// +optional
	Extensions map[string]string `json:"extensions,omitempty"`
}

// LocationStatus defines the observed state of Location
type LocationStatus struct {
	// Conditions represent the latest available observations of the Location's state
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="Conditions"
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=locations,shortName=loc
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Location is the Schema for the locations API.
// Represents a physical or logical location where O-Cloud Sites can be deployed.
// +operator-sdk:csv:customresourcedefinitions:displayName="Location",resources={{Location,v1alpha1}}
type Location struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LocationSpec   `json:"spec,omitempty"`
	Status LocationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// LocationList contains a list of Location
type LocationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Location `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Location{}, &LocationList{})
}
