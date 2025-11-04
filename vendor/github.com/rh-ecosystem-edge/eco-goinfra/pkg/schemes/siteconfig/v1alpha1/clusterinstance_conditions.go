/*
Copyright 2024.

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

package v1alpha1

// ClusterInstanceConditionType is a string representing the condition's type
type ClusterInstanceConditionType string

// String satisfies the conditions.ConditionType interface
func (t ClusterInstanceConditionType) String() string {
	return string(t)
}

// ClusterInstanceConditionReason is a string representing the ClusterInstanceConditionType's reason
type ClusterInstanceConditionReason string

// String returns the ClusterInstanceConditionReason as a string
func (r ClusterInstanceConditionReason) String() string {
	return string(r)
}

// The following constants define the different types of conditions that will be set
const (
	ClusterInstanceValidated   ClusterInstanceConditionType = "ClusterInstanceValidated"
	RenderedTemplates          ClusterInstanceConditionType = "RenderedTemplates"
	RenderedTemplatesValidated ClusterInstanceConditionType = "RenderedTemplatesValidated"
	RenderedTemplatesApplied   ClusterInstanceConditionType = "RenderedTemplatesApplied"
	RenderedTemplatesDeleted   ClusterInstanceConditionType = "RenderedTemplatesDeleted"
	ClusterProvisioned         ClusterInstanceConditionType = "Provisioned"
)

// The following constants define the different reasons that conditions will be set for
const (
	Initialized     ClusterInstanceConditionReason = "Initialized"
	Completed       ClusterInstanceConditionReason = "Completed"
	Failed          ClusterInstanceConditionReason = "Failed"
	TimedOut        ClusterInstanceConditionReason = "TimedOut"
	InProgress      ClusterInstanceConditionReason = "InProgress"
	Unknown         ClusterInstanceConditionReason = "Unknown"
	StaleConditions ClusterInstanceConditionReason = "StaleConditions"
)

// The following constants define the different reinstall condition types
const (
	ReinstallRequestValidated            ClusterInstanceConditionType = "ReinstallRequestValidated"
	ReinstallRequestProcessed            ClusterInstanceConditionType = "ReinstallRequestProcessed"
	ReinstallPreservationDataBackedup    ClusterInstanceConditionType = "ReinstallPreservationDataBackedup"
	ReinstallPreservationDataRestored    ClusterInstanceConditionType = "ReinstallPreservationDataRestored"
	ReinstallClusterIdentityDataDetected ClusterInstanceConditionType = "ReinstallClusterIdentityDataDetected"
	ReinstallRenderedManifestsDeleted    ClusterInstanceConditionType = "ReinstallRenderedManifestsDeleted"
)

// The following constants define additional reinstall condition reasons
const (
	PreservationNotRequired ClusterInstanceConditionReason = "PreservationNotRequired"
	DataUnavailable         ClusterInstanceConditionReason = "DataUnavailable"
	DataAvailable           ClusterInstanceConditionReason = "DataAvailable"
)
