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
	"sort"

	corev1 "k8s.io/api/core/v1"
)

func (nodeGroup NodeGroup) NormalizeConfig() NodeGroupConfig {
	conf := DefaultNodeGroupConfig()
	if nodeGroup.Config == nil {
		// nothing to do
		return conf
	}
	// always pass through tolerations
	conf.Tolerations = CloneTolerations(nodeGroup.Config.Tolerations)
	return conf.Merge(*nodeGroup.Config)
}

func (current NodeGroupConfig) Merge(updated NodeGroupConfig) NodeGroupConfig {
	conf := NodeGroupConfig{}
	current.DeepCopyInto(&conf)

	if updated.PodsFingerprinting != nil {
		conf.PodsFingerprinting = updated.PodsFingerprinting
	}
	if updated.InfoRefreshPeriod != nil {
		conf.InfoRefreshPeriod = updated.InfoRefreshPeriod
	}
	if updated.InfoRefreshMode != nil {
		conf.InfoRefreshMode = updated.InfoRefreshMode
	}
	if updated.InfoRefreshPause != nil {
		conf.InfoRefreshPause = updated.InfoRefreshPause
	}
	return conf
}

func CloneTolerations(tols []corev1.Toleration) []corev1.Toleration {
	ret := make([]corev1.Toleration, 0, len(tols))
	for _, tol := range tols {
		ret = append(ret, *tol.DeepCopy())
	}
	return ret
}

// SortedTolerations return a sorted clone of the provided toleration slice
func SortedTolerations(tols []corev1.Toleration) []corev1.Toleration {
	ret := CloneTolerations(tols)
	sort.SliceStable(ret, func(i, j int) bool {
		if ret[i].Key != ret[j].Key {
			return ret[i].Key < ret[j].Key
		}
		if ret[i].Operator != ret[j].Operator {
			return ret[i].Operator < ret[j].Operator
		}
		if ret[i].Value != ret[j].Value {
			return ret[i].Value < ret[j].Value
		}
		return ret[i].Effect < ret[j].Effect
	})
	return ret
}
