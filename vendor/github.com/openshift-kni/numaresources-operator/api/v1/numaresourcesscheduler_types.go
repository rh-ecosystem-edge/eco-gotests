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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
)

// +kubebuilder:validation:Enum=Disabled;DumpJSONFile
type CacheResyncDebugMode string

const (
	// CacheResyncDisabled disables additional report of the scheduler cache state.
	CacheResyncDebugDisabled CacheResyncDebugMode = "Disabled"

	// CacheResyncDumpJSONFile makes the scheduler cache dump its internal state as JSON at each failed resync. Default.
	CacheResyncDebugDumpJSONFile CacheResyncDebugMode = "DumpJSONFile"
)

// +kubebuilder:validation:Enum=Shared;Dedicated
type SchedulerInformerMode string

const (
	// SchedulerInformerShared makes the NodeResourceTopologyMatch plugin use the default framework informer.
	SchedulerInformerShared SchedulerInformerMode = "Shared"

	// SchedulerInformerDedicated sets an additional separate informer just for the NodeResourceTopologyMatch plugin. Default.
	SchedulerInformerDedicated SchedulerInformerMode = "Dedicated"
)

// +kubebuilder:validation:Enum=MostAllocated;BalancedAllocation;LeastAllocated
type ScoringStrategyType string

const (
	// MostAllocated strategy favors node with the least amount of available resource
	MostAllocated ScoringStrategyType = "MostAllocated"
	// BalancedAllocation strategy favors nodes with balanced resource usage rate
	BalancedAllocation ScoringStrategyType = "BalancedAllocation"
	// LeastAllocated strategy favors node with the most amount of available resource
	LeastAllocated ScoringStrategyType = "LeastAllocated"
)

type ResourceSpecParams struct {
	// Name of the resource.
	Name string `json:"name"`
	// Weight of the resource.
	Weight int64 `json:"weight,omitempty"`
}

type ScoringStrategyParams struct {
	Type      ScoringStrategyType  `json:"type,omitempty"`
	Resources []ResourceSpecParams `json:"resources,omitempty"`
}

// +kubebuilder:validation:Enum=Relaxed;Aggressive
type CacheResyncDetectionMode string

const (
	// CacheResyncDetectionRelaxed makes the NodeResourceTopologyMatch plugin reconcile the node state only when detecting guaranteed QoS pods request NUMA-specific resources.
	CacheResyncDetectionRelaxed CacheResyncDetectionMode = "Relaxed"

	// CacheResyncDetectionRelaxed makes the NodeResourceTopologyMatch plugin reconcile the node state when detecting any guaranteed QoS pods.
	CacheResyncDetectionAggressive CacheResyncDetectionMode = "Aggressive"
)

// NUMAResourcesSchedulerSpec defines the desired state of NUMAResourcesScheduler
type NUMAResourcesSchedulerSpec struct {
	// Scheduler container image URL
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Scheduler container image URL",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	SchedulerImage string `json:"imageSpec"`
	// Scheduler name to be used in pod templates
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Scheduler name",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	SchedulerName string `json:"schedulerName,omitempty"`
	// Valid values are: "Normal", "Debug", "Trace", "TraceAll".
	// Defaults to "Normal".
	// +optional
	// +kubebuilder:default=Normal
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Scheduler log verbosity"
	LogLevel operatorv1.LogLevel `json:"logLevel,omitempty"`
	// Set the cache resync period. Use explicit 0 to disable.
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Scheduler cache resync period setting",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	CacheResyncPeriod *metav1.Duration `json:"cacheResyncPeriod,omitempty"`
	// Set the cache resync debug options. Defaults to disable.
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Scheduler cache resync debug setting",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	CacheResyncDebug *CacheResyncDebugMode `json:"cacheResyncDebug,omitempty"`
	// Set the informer type to be used by the scheduler to connect to the apiserver. Defaults to dedicated.
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Scheduler cache apiserver informer setting",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	SchedulerInformer *SchedulerInformerMode `json:"schedulerInformer,omitempty"`
	// Set the cache resync detection mode. Default is to trigger resyncs only when detected guaranteed QoS pods which require NUMA-specific resources.
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Scheduler cache resync detection setting",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	CacheResyncDetection *CacheResyncDetectionMode `json:"cacheResyncDetection,omitempty"`
	// ScoringStrategy a scoring model that determine how the plugin will score the nodes. Defaults to LeastAllocated.
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Scheduler scoring strategy setting",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
	ScoringStrategy *ScoringStrategyParams `json:"scoringStrategy,omitempty"`
	// Replicas control how many scheduler pods must be configured for High Availability (HA)
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Scheduler replicas",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:int"}
	Replicas *int32 `json:"replicas,omitempty"`
}

// NUMAResourcesSchedulerStatus defines the observed state of NUMAResourcesScheduler
type NUMAResourcesSchedulerStatus struct {
	// Deployment of the secondary scheduler, namespaced name
	//+operator-sdk:csv:customresourcedefinitions:type=status,displayName="Scheduler deployment"
	Deployment NamespacedName `json:"deployment,omitempty"`
	// Scheduler name to be used in pod templates
	//+operator-sdk:csv:customresourcedefinitions:type=status,displayName="Scheduler name"
	SchedulerName string `json:"schedulerName,omitempty"`
	// CacheResyncPeriod shows the current cache resync period
	// +optional
	//+operator-sdk:csv:customresourcedefinitions:type=status,displayName="Scheduler cache resync period"
	CacheResyncPeriod *metav1.Duration `json:"cacheResyncPeriod,omitempty"`
	// Conditions show the current state of the NUMAResourcesOperator Operator
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// RelatedObjects list of objects of interest for this operator
	//+operator-sdk:csv:customresourcedefinitions:type=status,displayName="Related Objects"
	RelatedObjects []configv1.ObjectReference `json:"relatedObjects,omitempty"`
}

//+genclient
//+genclient:nonNamespaced
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName=numaressched,path=numaresourcesschedulers,scope=Cluster
//+kubebuilder:storageversion

// NUMAResourcesScheduler is the Schema for the numaresourcesschedulers API
// +operator-sdk:csv:customresourcedefinitions:displayName="NUMA Aware Scheduler",resources={{Deployment,v1,secondary-scheduler-deployment}}
type NUMAResourcesScheduler struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NUMAResourcesSchedulerSpec   `json:"spec,omitempty"`
	Status NUMAResourcesSchedulerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NUMAResourcesSchedulerList contains a list of NUMAResourcesScheduler
type NUMAResourcesSchedulerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NUMAResourcesScheduler `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NUMAResourcesScheduler{}, &NUMAResourcesSchedulerList{})
}
