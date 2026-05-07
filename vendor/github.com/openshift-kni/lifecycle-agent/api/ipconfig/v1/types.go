/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=ipconfigs,scope=Cluster,shortName=ipc
// +operator-sdk:csv:customresourcedefinitions:displayName="IP Configuration",resources={{Namespace, v1}}
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="Desired Stage",type="string",JSONPath=".spec.stage"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.conditions[-1:].reason"
// +kubebuilder:printcolumn:name="Details",type="string",JSONPath=".status.conditions[-1:].message"
// +kubebuilder:printcolumn:name="Current IPv4",type="string",JSONPath=".status.ipv4.address",priority=1
// +kubebuilder:printcolumn:name="Desired IPv4",type="string",JSONPath=".spec.ipv4.address",priority=1
// +kubebuilder:printcolumn:name="Current IPv6",type="string",JSONPath=".status.ipv6.address",priority=1
// +kubebuilder:printcolumn:name="Desired IPv6",type="string",JSONPath=".spec.ipv6.address",priority=1
// +kubebuilder:validation:XValidation:message="ipconfig is a singleton, metadata.name must be 'ipconfig'", rule="self.metadata.name == 'ipconfig'"
// +kubebuilder:validation:XValidation:message="can not change spec.ipv4 while ipconfig is not idle",rule="!has(oldSelf.status) || oldSelf.status.conditions.exists(c, c.type=='Idle' && c.status=='True') || has(oldSelf.spec.ipv4) && has(self.spec.ipv4) && oldSelf.spec.ipv4==self.spec.ipv4 || !has(self.spec.ipv4) && !has(oldSelf.spec.ipv4)"
// +kubebuilder:validation:XValidation:message="can not change spec.ipv6 while ipconfig is not idle",rule="!has(oldSelf.status) || oldSelf.status.conditions.exists(c, c.type=='Idle' && c.status=='True') || has(oldSelf.spec.ipv6) && has(self.spec.ipv6) && oldSelf.spec.ipv6==self.spec.ipv6 || !has(self.spec.ipv6) && !has(oldSelf.spec.ipv6)"
// +kubebuilder:validation:XValidation:message="can not change spec.dnsServers while ipconfig is not idle",rule="!has(oldSelf.status) || oldSelf.status.conditions.exists(c, c.type=='Idle' && c.status=='True') || has(oldSelf.spec.dnsServers) && has(self.spec.dnsServers) && oldSelf.spec.dnsServers==self.spec.dnsServers || !has(self.spec.dnsServers) && !has(oldSelf.spec.dnsServers)"
// +kubebuilder:validation:XValidation:message="can not change spec.dnsFilterOutFamily while ipconfig is not idle",rule="!has(oldSelf.status) || oldSelf.status.conditions.exists(c, c.type=='Idle' && c.status=='True') || has(oldSelf.spec.dnsFilterOutFamily) && has(self.spec.dnsFilterOutFamily) && oldSelf.spec.dnsFilterOutFamily==self.spec.dnsFilterOutFamily || !has(self.spec.dnsFilterOutFamily) && !has(oldSelf.spec.dnsFilterOutFamily)"
// +kubebuilder:validation:XValidation:message="can not change spec.autoRollbackOnFailure while ipconfig is not idle",rule="!has(oldSelf.status) || oldSelf.status.conditions.exists(c, c.type=='Idle' && c.status=='True') || has(oldSelf.spec.autoRollbackOnFailure) && has(self.spec.autoRollbackOnFailure) && oldSelf.spec.autoRollbackOnFailure==self.spec.autoRollbackOnFailure || !has(self.spec.autoRollbackOnFailure) && !has(oldSelf.spec.autoRollbackOnFailure)"
// +kubebuilder:validation:XValidation:message="can not change spec.vlanID while ipconfig is not idle",rule="!has(oldSelf.status) || oldSelf.status.conditions.exists(c, c.type=='Idle' && c.status=='True') || has(oldSelf.spec.vlanID) && has(self.spec.vlanID) && oldSelf.spec.vlanID==self.spec.vlanID || !has(self.spec.vlanID) && !has(oldSelf.spec.vlanID)"
// +kubebuilder:validation:XValidation:message="can not assign spec.ipv4.address for cluster without current ipv4 address",rule="(!has(self.spec.ipv4) || !has(self.spec.ipv4.address) || self.spec.ipv4.address=='') || !has(oldSelf.status) || !has(oldSelf.status.ipv4) && !has(oldSelf.status.ipv6) || (has(oldSelf.status) && has(oldSelf.status.ipv4))"
// +kubebuilder:validation:XValidation:message="can not assign spec.ipv6.address for cluster without current ipv6 address",rule="(!has(self.spec.ipv6) || !has(self.spec.ipv6.address) || self.spec.ipv6.address=='') || !has(oldSelf.status) || !has(oldSelf.status.ipv4) && !has(oldSelf.status.ipv6) || (has(oldSelf.status) && has(oldSelf.status.ipv6))"
// +kubebuilder:validation:XValidation:message="can not assign spec.vlanID for a cluster without current vlan ID",rule="(!has(self.spec.vlanID) || self.spec.vlanID==0) || !has(oldSelf.status) || (!has(oldSelf.status.ipv4) && !has(oldSelf.status.ipv6)) || (has(oldSelf.status.vlanID) && oldSelf.status.vlanID > 0)"
// +kubebuilder:validation:XValidation:message="can not assign spec.dnsFilterOutFamily to 'ipv4' or 'ipv6' for a non-dual-stack cluster",rule="!has(self.spec.dnsFilterOutFamily) || self.spec.dnsFilterOutFamily=='' || self.spec.dnsFilterOutFamily=='none' || !has(oldSelf.status) || !has(oldSelf.status.ipv4) && !has(oldSelf.status.ipv6) || has(oldSelf.status.ipv4) && oldSelf.status.ipv4.address!='' && has(oldSelf.status.ipv6) && oldSelf.status.ipv6.address!=''"
// +kubebuilder:validation:XValidation:message="the stage transition is not permitted. Please refer to status.validNextStages for valid transitions. If status.validNextStages is not present, it indicates that no transitions are currently allowed", rule="!has(oldSelf.status) || has(oldSelf.status.validNextStages) && self.spec.stage in oldSelf.status.validNextStages || has(oldSelf.spec.stage) && has(self.spec.stage) && oldSelf.spec.stage==self.spec.stage"

// IPConfig is the Schema for IPConfigs API
type IPConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IPConfigSpec   `json:"spec,omitempty"`
	Status IPConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// IPConfigList contains a list of IPConfig
type IPConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IPConfig `json:"items"`
}

// IPConfigStage defines the type for the stage field
type IPConfigStage string

var IPStages = struct {
	Idle     IPConfigStage
	Config   IPConfigStage
	Rollback IPConfigStage
}{
	Idle:     "Idle",
	Config:   "Config",
	Rollback: "Rollback",
}

// IPAddress represents an IP address (IPv4 or IPv6).
// IPv6 max textual length is 39 chars; we allow some headroom.
// +kubebuilder:validation:MaxLength=45
// +kubebuilder:validation:XValidation:rule="isIP(self)",message="must be a valid IP address"
type IPAddress string

// IPv4Config represents a single IPv4 stack configuration
// +kubebuilder:validation:XValidation:message="IPv4 address must be within its machineNetwork CIDR",rule="!has(self.address) || self.address == \"\" || !has(self.machineNetwork) || self.machineNetwork == \"\" || cidr(self.machineNetwork).containsIP(self.address)"
// +kubebuilder:validation:XValidation:message="IPv4 gateway must be within its machineNetwork CIDR",rule="!has(self.gateway) || self.gateway == \"\" || !has(self.machineNetwork) || self.machineNetwork == \"\" || cidr(self.machineNetwork).containsIP(self.gateway)"
type IPv4Config struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Format=ipv4
	// Address is the full IPv4 address without prefix length (e.g., 192.0.2.10)
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Address",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text","urn:alm:descriptor:com.tectonic.ui:fieldGroup:IPv4"}
	Address string `json:"address,omitempty"`
	// +kubebuilder:validation:Format=cidr
	// MachineNetwork is the IPv4 machine network CIDR (e.g., 192.0.2.0/24)
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Machine Network",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text","urn:alm:descriptor:com.tectonic.ui:fieldGroup:IPv4"}
	MachineNetwork string `json:"machineNetwork,omitempty"`
	// +kubebuilder:validation:Format=ipv4
	// Gateway is the default IPv4 gateway address
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Gateway",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text","urn:alm:descriptor:com.tectonic.ui:fieldGroup:IPv4"}
	Gateway string `json:"gateway,omitempty"`
}

// IPv6Config represents a single IPv6 stack configuration
// +kubebuilder:validation:XValidation:message="IPv6 address must be within its machineNetwork CIDR",rule="!has(self.address) || self.address == \"\" || !has(self.machineNetwork) || self.machineNetwork == \"\" || cidr(self.machineNetwork).containsIP(self.address)"
// +kubebuilder:validation:XValidation:message="IPv6 gateway must be within its machineNetwork CIDR unless it is link-local",rule="!has(self.gateway) || self.gateway == \"\" || !has(self.machineNetwork) || self.machineNetwork == \"\" || cidr(self.machineNetwork).containsIP(self.gateway) || cidr(\"fe80::/10\").containsIP(self.gateway)"
type IPv6Config struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Format=ipv6
	// Address is the full IPv6 address without prefix length (e.g., 2001:db8::1)
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Address",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text","urn:alm:descriptor:com.tectonic.ui:fieldGroup:IPv6"}
	Address string `json:"address,omitempty"`
	// +kubebuilder:validation:Format=cidr
	// MachineNetwork is the IPv6 machine network CIDR (e.g., 2001:db8::/64)
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Machine Network",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text","urn:alm:descriptor:com.tectonic.ui:fieldGroup:IPv6"}
	MachineNetwork string `json:"machineNetwork,omitempty"`
	// +kubebuilder:validation:Format=ipv6
	// Gateway is the default IPv6 gateway address
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Gateway",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text","urn:alm:descriptor:com.tectonic.ui:fieldGroup:IPv6"}
	Gateway string `json:"gateway,omitempty"`
}

// IPv4Status represents observed IPv4 configuration.
type IPv4Status struct {
	// Address is the node internal IPv4 address (plain address, no prefix).
	//+operator-sdk:csv:customresourcedefinitions:type=status,displayName="IPv4 Address",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	Address string `json:"address,omitempty"`
	// MachineNetwork is the machine network CIDR inferred for the node IP.
	//+operator-sdk:csv:customresourcedefinitions:type=status,displayName="IPv4 Machine Network",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	MachineNetwork string `json:"machineNetwork,omitempty"`
	// Gateway is the default IPv4 gateway address.
	//+operator-sdk:csv:customresourcedefinitions:type=status,displayName="IPv4 Gateway",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	Gateway string `json:"gateway,omitempty"`
}

// IPv6Status represents observed IPv6 configuration.
type IPv6Status struct {
	// Address is the node internal IPv6 address (plain address, no prefix).
	//+operator-sdk:csv:customresourcedefinitions:type=status,displayName="IPv6 Address",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	Address string `json:"address,omitempty"`
	// MachineNetwork is the machine network CIDR inferred for the node IP.
	//+operator-sdk:csv:customresourcedefinitions:type=status,displayName="IPv6 Machine Network",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	MachineNetwork string `json:"machineNetwork,omitempty"`
	// Gateway is the default IPv6 gateway address.
	//+operator-sdk:csv:customresourcedefinitions:type=status,displayName="IPv6 Gateway",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	Gateway string `json:"gateway,omitempty"`
}

// IPConfigSpec defines the desired state of IPConfig
type IPConfigSpec struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=Idle;Config;Rollback
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Stage",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:select"}
	Stage IPConfigStage `json:"stage,omitempty"`

	// IPv4 stack (omit for IPv6-only)
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="IPv4",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:fieldGroup:IPv4"}
	IPv4 *IPv4Config `json:"ipv4,omitempty"`

	// IPv6 stack (omit for IPv4-only)
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="IPv6",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:fieldGroup:IPv6"}
	IPv6 *IPv6Config `json:"ipv6,omitempty"`

	// DNSServers is the complete ordered list of DNS server IPs (IPv4 and/or IPv6)
	// to configure on the node. Up to 2 DNS servers are supported.
	// +kubebuilder:validation:MaxItems=2
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="DNS Servers"
	DNSServers []IPAddress `json:"dnsServers,omitempty"`

	// Optional VLAN applied to br-ex path
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=4095
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="VLAN ID",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:number"}
	VLANID int `json:"vlanID,omitempty"`

	// DNSFilterOutFamily selects the IP family to filter out from DNS responses.
	// This setting is only meaningful on dual-stack clusters:
	// - "ipv4" => filter out A records, leaving AAAA (IPv6)
	// - "ipv6" => filter out AAAA records, leaving A (IPv4)
	// - "none" => explicitly disable DNS response filtering
	// +kubebuilder:validation:Enum=ipv4;ipv6;none
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="DNS Filter-Out Family",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:select"}
	DNSFilterOutFamily string `json:"dnsFilterOutFamily,omitempty"`

	// AutoRollbackOnFailure defines automatic rollback settings for IPConfig if the configuration
	// does not complete within the specified time limit.
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Auto Rollback On Failure",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:fieldGroup:Auto Rollback On Failure"}
	AutoRollbackOnFailure *AutoRollbackOnFailure `json:"autoRollbackOnFailure,omitempty"`
}

// AutoRollbackOnFailure defines automatic rollback settings if the IP configuration does not
// complete within the specified time limit.
type AutoRollbackOnFailure struct {
	// InitMonitorTimeoutSeconds defines the time frame in seconds. If not defined or set to 0,
	// the default value of 1800 seconds (30 minutes) is used.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=0
	//+operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:number","urn:alm:descriptor:com.tectonic.ui:fieldGroup:Auto Rollback On Failure"}
	InitMonitorTimeoutSeconds int `json:"initMonitorTimeoutSeconds,omitempty"`
}

// IPConfigStatus defines the observed state of IPConfig
type IPConfigStatus struct {
	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="Observed Generation"
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="Conditions",xDescriptors={"urn:alm:descriptor:io.kubernetes.conditions"}
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ValidNextStages enumerates allowed next transitions from current stage
	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="Valid Next Stage"
	ValidNextStages []IPConfigStage `json:"validNextStages,omitempty"`

	// RollbackAvailabilityExpiration reflects the point at which rolling back may require manual recovery from expired control plane certificates.
	RollbackAvailabilityExpiration metav1.Time `json:"rollbackAvailabilityExpiration,omitempty"`

	// IPv4 reports the currently detected IPv4 configuration (if any)
	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="IPv4"
	IPv4 *IPv4Status `json:"ipv4,omitempty"`

	// IPv6 reports the currently detected IPv6 configuration (if any)
	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="IPv6"
	IPv6 *IPv6Status `json:"ipv6,omitempty"`

	// DNSServers reports the currently detected ordered list of DNS server IPs (IPv4 and/or IPv6).
	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="DNS Servers"
	DNSServers []string `json:"dnsServers,omitempty"`

	// VLANID reports the currently detected VLAN ID on the br-ex uplink path (if any)
	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="VLAN ID"
	VLANID int `json:"vlanID,omitempty"`

	// DNSFilterOutFamily reports the active DNS response filtering family (the IP family being filtered out):
	// "ipv4", "ipv6" or "none".
	// +kubebuilder:validation:Enum=ipv4;ipv6;none
	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="DNS Filter-Out Family"
	DNSFilterOutFamily string `json:"dnsFilterOutFamily,omitempty"`

	// History stores timing info of different IPConfig stages and their important phases
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="History"
	History []*IPHistory `json:"history,omitempty"`
}

// history for IPConfig stages
type IPHistory struct {
	// Stage The desired stage name read from spec
	Stage IPConfigStage `json:"stage,omitempty"`
	// Phases allows a granular view of important tasks within a Stage
	Phases []*IPPhase `json:"phases,omitempty"`
	// StartTime A timestamp to indicate the Stage has started
	StartTime metav1.Time `json:"startTime,omitempty"`
	// CompletionTime A timestamp indicating the Stage completed successfully
	CompletionTime metav1.Time `json:"completionTime,omitempty"`
}

// IPPhase represents a sub-step within a stage
type IPPhase struct {
	// Phase current phase within a Stage
	Phase string `json:"phase,omitempty"`
	// StartTime A timestamp indicating the Phase has started
	StartTime metav1.Time `json:"startTime,omitempty"`
	// CompletionTime A timestamp indicating the phase completed successfully
	CompletionTime metav1.Time `json:"completionTime,omitempty"`
}

func init() {
	SchemeBuilder.Register(&IPConfig{}, &IPConfigList{})
}
