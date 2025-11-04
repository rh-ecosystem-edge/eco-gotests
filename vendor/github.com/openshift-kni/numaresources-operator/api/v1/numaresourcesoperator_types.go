/*
 * Copyright 2023 Red Hat, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package v1

import (
	"fmt"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	configv1 "github.com/openshift/api/config/v1"
	mcov1 "github.com/openshift/api/machineconfiguration/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
)

// NUMAResourcesOperatorSpec defines the desired state of NUMAResourcesOperator
type NUMAResourcesOperatorSpec struct {
	// Group of Nodes to enable RTE on
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Group of nodes to enable RTE on"
	NodeGroups []NodeGroup `json:"nodeGroups,omitempty"`
	// Optional Resource Topology Exporter image URL
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Optional RTE image URL",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	ExporterImage string `json:"imageSpec,omitempty"`
	// Valid values are: "Normal", "Debug", "Trace", "TraceAll".
	// Defaults to "Normal".
	// +optional
	// +kubebuilder:default=Normal
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="RTE log verbosity"
	LogLevel operatorv1.LogLevel `json:"logLevel,omitempty"`
	// Optional Namespace/Name glob patterns of pod to ignore at node level
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Optional ignore pod namespace/name glob patterns"
	PodExcludes []NamespacedName `json:"podExcludes,omitempty"`
}

// +kubebuilder:validation:Enum=Disabled;Enabled;EnabledExclusiveResources
type PodsFingerprintingMode string

const (
	// PodsFingerprintingDisabled disables the pod fingerprinting reporting.
	PodsFingerprintingDisabled PodsFingerprintingMode = "Disabled"

	// PodsFingerprintingEnabled enables the pod fingerprint considering all the pods running on nodes. It is the default.
	PodsFingerprintingEnabled PodsFingerprintingMode = "Enabled"

	// PodsFingerprintingEnabledExclusiveResources enables the pod fingerprint considering only pods which have exclusive resources assigned.
	PodsFingerprintingEnabledExclusiveResources PodsFingerprintingMode = "EnabledExclusiveResources"
)

// +kubebuilder:validation:Enum=Disabled;Enabled
type InfoRefreshPauseMode string

const (
	// InfoRefreshPauseDisabled enables RTE and NRT sync
	InfoRefreshPauseDisabled InfoRefreshPauseMode = "Disabled"

	// InfoRefreshPauseEnabled pauses RTE and disables the NRT sync
	InfoRefreshPauseEnabled InfoRefreshPauseMode = "Enabled"
)

// +kubebuilder:validation:Enum=Periodic;Events;PeriodicAndEvents
type InfoRefreshMode string

const (
	// InfoRefreshPeriodic is the default. Periodically polls the state and reports it.
	InfoRefreshPeriodic InfoRefreshMode = "Periodic"

	// InfoRefreshEvents reports a new state each time a pod lifecycle event is received.
	InfoRefreshEvents InfoRefreshMode = "Events"

	// InfoRefreshPeriodicAndEvents enables both periodic and event-based reporting.
	InfoRefreshPeriodicAndEvents InfoRefreshMode = "PeriodicAndEvents"
)

// NodeGroupConfig exposes topology info reporting setting per node group
type NodeGroupConfig struct {
	// PodsFingerprinting defines if pod fingerprint should be reported for the machines belonging to this group
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Enable or disable the pods fingerprinting setting",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	PodsFingerprinting *PodsFingerprintingMode `json:"podsFingerprinting,omitempty"`
	// InfoRefreshMode sets the mechanism which will be used to refresh the topology info.
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Topology info mechanism setting",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	InfoRefreshMode *InfoRefreshMode `json:"infoRefreshMode,omitempty"`
	// InfoRefreshPeriod sets the topology info refresh period. Use explicit 0 to disable.
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Topology info refresh period setting",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	InfoRefreshPeriod *metav1.Duration `json:"infoRefreshPeriod,omitempty"`
	// InfoRefreshPause defines if updates to NRTs are paused for the machines belonging to this group
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Enable or disable the RTE pause setting",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	InfoRefreshPause *InfoRefreshPauseMode `json:"infoRefreshPause,omitempty"`
	// Tolerations overrides tolerations to be set into RTE daemonsets for this NodeGroup. If not empty, the tolerations will be the one set here.
	// Leave empty to make the system use the default tolerations.
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Extra tolerations for the topology updater daemonset",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
}

const (
	// NodeGroupMaxAnnotations limits the number of annotations a node group can have.
	// These annotations differ from regular kubernetes object annotations in key ways:
	// - User-provided generic annotations are not supported and can be removed any time by the functioning of the operator.
	// - Reserved for internal usage of the NUMA resources operator to fine-tune and detail the behavior of node groups.
	// - Based on projected usage, there is a fixed cap to the maximum amount of annotations a NodeGroup can hold
	// Besides this funtamental key differences, per-nodegroup annotations will work like regular kubernetes objects annotations.
	NodeGroupMaxAnnotations = 8
)

// NodeGroup defines group of nodes that will run resource topology exporter daemon set
// You can choose the group of node by MachineConfigPoolSelector or by PoolName
type NodeGroup struct {
	// MachineConfigPoolSelector defines label selector for the machine config pool
	// +optional
	MachineConfigPoolSelector *metav1.LabelSelector `json:"machineConfigPoolSelector,omitempty"`
	// Config defines the RTE behavior for this NodeGroup
	// +optional
	Config *NodeGroupConfig `json:"config,omitempty"`
	// PoolName defines the pool name to which the nodes belong that the config of this node group will be applied to
	// +optional
	PoolName *string `json:"poolName,omitempty"`
	// Annotations is the per-nodegroup equivalent of the per-object annotations.
	// Unlike the regular annotations, there is a hard limit of supported annotations, dependent on the operator version.
	// In general, prefer to use per-object standard annotations.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// NodeGroupStatus reports the status of a NodeGroup once matches an actual set of nodes and it is correctly processed
// by the system. In other words, is not possible to have a NodeGroupStatus which does not represent a valid NodeGroup
// which in turn correctly references unambiguously a set of nodes in the cluster.
// Hence, if a NodeGroupStatus is published, its `Name` must be present, because it refers back to a NodeGroup whose
// config was correctly processed in the Spec. And its DaemonSet will be nonempty, because matches correctly a set
// of nodes in the cluster. The Config is best-effort always represented, possibly reflecting the system defaults.
// If the system cannot process a NodeGroup correctly from the Spec, it will report Degraded state in the top-level
// condition, and will provide details using the aforementioned conditions.
type NodeGroupStatus struct {
	// DaemonSet of the configured RTEs, for this node group
	//+operator-sdk:csv:customresourcedefinitions:type=status,displayName="RTE DaemonSets"
	DaemonSet NamespacedName `json:"daemonsets"`
	// NodeGroupConfig represents the latest available configuration applied to this NodeGroup
	//+operator-sdk:csv:customresourcedefinitions:type=status,displayName="Optional configuration enforced on this NodeGroup"
	Config NodeGroupConfig `json:"config"`
	// PoolName represents the pool name to which the nodes belong that the config of this node group is be applied to
	//+operator-sdk:csv:customresourcedefinitions:type=status,displayName="Pool name of nodes in this node group"
	PoolName string `json:"selector"`
}

// NUMAResourcesOperatorStatus defines the observed state of NUMAResourcesOperator
type NUMAResourcesOperatorStatus struct {
	// DaemonSets of the configured RTEs, one per node group
	//+operator-sdk:csv:customresourcedefinitions:type=status,displayName="RTE DaemonSets"
	DaemonSets []NamespacedName `json:"daemonsets,omitempty"`
	// MachineConfigPools resolved from configured node groups
	//+operator-sdk:csv:customresourcedefinitions:type=status,displayName="RTE MCPs from node groups"
	MachineConfigPools []MachineConfigPool `json:"machineconfigpools,omitempty"`
	// NodeGroups report the observed status of the configured NodeGroups, matching by their name
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=status,displayName="Node groups observed status"
	NodeGroups []NodeGroupStatus `json:"nodeGroups,omitempty"`
	// Conditions show the current state of the NUMAResourcesOperator Operator
	//+operator-sdk:csv:customresourcedefinitions:type=status,displayName="Condition reported"
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// RelatedObjects list of objects of interest for this operator
	//+operator-sdk:csv:customresourcedefinitions:type=status,displayName="Related Objects"
	RelatedObjects []configv1.ObjectReference `json:"relatedObjects,omitempty"`
}

// MachineConfigPool defines the observed state of each MachineConfigPool selected by node groups
type MachineConfigPool struct {
	// Name the name of the machine config pool
	Name string `json:"name"`
	// Conditions represents the latest available observations of MachineConfigPool current state.
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=status,displayName="Optional conditions reported for this NodeGroup"
	Conditions []mcov1.MachineConfigPoolCondition `json:"conditions,omitempty"`
	// NodeGroupConfig represents the latest available configuration applied to this MachineConfigPool
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=status,displayName="Optional configuration enforced on this NodeGroup"
	Config *NodeGroupConfig `json:"config,omitempty"`
}

//+genclient
//+genclient:nonNamespaced
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName=numaresop,path=numaresourcesoperators,scope=Cluster
//+kubebuilder:storageversion

// NUMAResourcesOperator is the Schema for the numaresourcesoperators API
// +operator-sdk:csv:customresourcedefinitions:displayName="NUMA Resources Operator",resources={{DaemonSet,v1,rte-daemonset,ConfigMap,v1,rte-configmap}}
type NUMAResourcesOperator struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NUMAResourcesOperatorSpec   `json:"spec,omitempty"`
	Status NUMAResourcesOperatorStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NUMAResourcesOperatorList contains a list of NUMAResourcesOperator
type NUMAResourcesOperatorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NUMAResourcesOperator `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NUMAResourcesOperator{}, &NUMAResourcesOperatorList{})
}

func (ngc *NodeGroupConfig) ToString() string {
	if ngc == nil {
		return "nil"
	}
	ngc.SetDefaults()
	return fmt.Sprintf("PodsFingerprinting mode: %s InfoRefreshMode: %s InfoRefreshPeriod: %s InfoRefreshPause: %s Tolerations: %+v", *ngc.PodsFingerprinting, *ngc.InfoRefreshMode, *ngc.InfoRefreshPeriod, *ngc.InfoRefreshPause, ngc.Tolerations)
}

func (ng *NodeGroup) GetName() string {
	if ng == nil {
		return "nil"
	}

	if ng.PoolName != nil {
		return *ng.PoolName
	}

	if ng.MachineConfigPoolSelector != nil {
		return ng.MachineConfigPoolSelector.String()
	}

	return "<missingName>"
}

func (ng *NodeGroup) ToString() string {
	if ng == nil {
		return "nil"
	}
	return fmt.Sprintf("PoolName: %s Config: %s Annotations: %s", ng.GetName(), ng.Config.ToString(), annsToString(ng.Annotations))
}

func annsToString(anns map[string]string) string {
	items := make([]string, 0, len(anns))
	keys := make([]string, 0, len(anns)) // TODO: use `keys := slices.Collect(maps.Keys(anns))` when we move to 1.23
	for key := range anns {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		items = append(items, key+"="+anns[key])
	}
	return strings.Join(items, ",")
}
