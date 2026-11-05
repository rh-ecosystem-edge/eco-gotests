/*
SPDX-FileCopyrightText: Red Hat

SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FirmwareImage defines a firmware image entry in the catalog.
type FirmwareImage struct {
	// Name is a unique identifier for this firmware image within the catalog.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Component identifies the hardware component type.
	// +kubebuilder:validation:Enum=bios;bmc;nic
	Component string `json:"component"`

	// URL points to the firmware image file.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^(http|https)://.*$`
	URL string `json:"url"`

	// Version is the firmware version string.
	// +kubebuilder:validation:MinLength=1
	Version string `json:"version"`

	// Vendor identifies the firmware vendor or manufacturer.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Vendor string `json:"vendor,omitempty"`

	// Description is an optional human-readable description of the firmware image.
	// +optional
	Description string `json:"description,omitempty"`
}

// FirmwareCatalogSpec defines the desired state of FirmwareCatalog.
type FirmwareCatalogSpec struct {
	// Images is the set of firmware images available in this catalog.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=1024
	Images []FirmwareImage `json:"images,omitempty"`
}

// ImageValidationStatus reports the validation result for a single image entry.
type ImageValidationStatus struct {
	// Name is the name of the firmware image entry.
	Name string `json:"name"`

	// Valid indicates whether the image entry passed validation.
	Valid bool `json:"valid"`

	// Reason provides a machine-readable reason for the validation result.
	// +optional
	Reason string `json:"reason,omitempty"`

	// Message provides a human-readable description of the validation result.
	// +optional
	Message string `json:"message,omitempty"`
}

// FirmwareCatalogStatus defines the observed state of FirmwareCatalog.
type FirmwareCatalogStatus struct {
	// +operator-sdk:csv:customresourcedefinitions:type=status
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ImageStatuses reports the validation result for each image in the catalog.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +operator-sdk:csv:customresourcedefinitions:type=status
	ImageStatuses []ImageValidationStatus `json:"imageStatuses,omitempty"`

	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=status
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +operator-sdk:csv:customresourcedefinitions:displayName="Firmware Catalog",resources={{ConfigMap, v1}}
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=firmwarecatalogs,scope=Namespaced
// +kubebuilder:resource:shortName=fwcatalog;fwcatalogs
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="The age of the FirmwareCatalog resource."
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=".status.conditions[-1:].reason"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[-1:].status"
// +kubebuilder:printcolumn:name="Details",type="string",JSONPath=".status.conditions[-1:].message"

// FirmwareCatalog is the Schema for the firmwarecatalogs API
type FirmwareCatalog struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FirmwareCatalogSpec   `json:"spec,omitempty"`
	Status FirmwareCatalogStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FirmwareCatalogList contains a list of FirmwareCatalog
type FirmwareCatalogList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FirmwareCatalog `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FirmwareCatalog{}, &FirmwareCatalogList{})
}
