/*
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
 *
 * Copyright 2023 Red Hat, Inc.
 */

package v1

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultCacheResyncPeriod    = 5 * time.Second
	defaultCacheResyncDebug     = CacheResyncDebugDumpJSONFile
	defaultSchedulerInformer    = SchedulerInformerDedicated
	defaultCacheResyncDetection = CacheResyncDetectionRelaxed
	defaultScoringStrategy      = LeastAllocated
)

func SetDefaults_NUMAResourcesSchedulerSpec(spec *NUMAResourcesSchedulerSpec) {
	if spec.CacheResyncPeriod == nil {
		spec.CacheResyncPeriod = &metav1.Duration{
			Duration: defaultCacheResyncPeriod,
		}
	}
	if spec.CacheResyncDebug == nil {
		resyncDebug := defaultCacheResyncDebug
		spec.CacheResyncDebug = &resyncDebug
	}
	if spec.SchedulerInformer == nil {
		infMode := defaultSchedulerInformer
		spec.SchedulerInformer = &infMode
	}
	if spec.CacheResyncDetection == nil {
		resyncDetection := defaultCacheResyncDetection
		spec.CacheResyncDetection = &resyncDetection
	}
	if spec.ScoringStrategy == nil {
		spec.ScoringStrategy = &ScoringStrategyParams{
			Type: defaultScoringStrategy,
		}
	}
	// do not set replicas default value
	// it will be computed later
}
