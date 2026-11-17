/*
Copyright 2026.

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

package utils

import (
	"context"
	"fmt"
	"strconv"

	"github.com/go-logr/logr"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterclaimv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	ibuv1 "github.com/openshift-kni/lifecycle-agent/api/imagebasedupgrade/v1"
	"github.com/openshift-kni/lifecycle-agent/controllers/utils"
)

// PublishSeedFormatVersion writes the seed format version this LCA expects into
// the IBU singleton's status.seedFormatVersion field. It must be called on every
// LCA start, after InitIBU: the post-pivot restore path in InitIBU restores a
// saved IBU together with its saved status, which carries the previous LCA's
// value, so this rewrites it with the running LCA's value.
//
// It is called before mgr.Start, when the manager cache is not running, so the
// caller must pass an uncached, direct client (the one created in main, or
// mgr.GetAPIReader-backed), not mgr.GetClient().
func PublishSeedFormatVersion(ctx context.Context, c runtimeclient.Client, log *logr.Logger) error {
	ibu := &ibuv1.ImageBasedUpgrade{}
	if err := c.Get(ctx, types.NamespacedName{Name: utils.IBUName}, ibu); err != nil {
		return fmt.Errorf("failed to get IBU to publish seed format version: %w", err)
	}

	if ibu.Status.SeedFormatVersion == ibuv1.SeedFormatVersion {
		return nil
	}

	patch := runtimeclient.MergeFrom(ibu.DeepCopy())
	ibu.Status.SeedFormatVersion = ibuv1.SeedFormatVersion
	if err := c.Status().Patch(ctx, ibu, patch); err != nil {
		return fmt.Errorf("failed to update IBU status with seed format version: %w", err)
	}

	log.Info("Published seed format version to IBU status", "seedFormatVersion", ibuv1.SeedFormatVersion)
	return nil
}

// EnsureSeedFormatClaim creates, or updates the value of, the ClusterClaim that
// publishes the seed format version LCA expects, so a hub consumer (TALM) can
// read a spoke's capability directly. It is idempotent and deliberately does NOT
// watch the ClusterClaim kind.
//
// The ClusterClaim CRD belongs to the klusterlet, not to LCA, and is absent on
// seed clusters (which must not be ACM-managed), on standalone/not-yet-imported
// SNOs, and in the post-pivot window before LCA restores ACM. A watch on a kind
// whose CRD is missing would fail the manager start and crash-loop LCA in exactly
// those cases; reading uncached and tolerating the missing CRD avoids that. When
// the CRD is absent the cluster is simply not ACM-managed and there is nothing to
// publish, so the function is a no-op. The hub treats a missing claim as unknown
// and falls back to the IBU status field.
//
// reader must be an uncached reader: at startup the manager cache is not running,
// and even when it is, LCA does not (and must not) cache/watch ClusterClaim, so
// a cached Get would try to start an informer on the possibly-absent CRD. c is
// used only for writes, which go straight to the API server.
func EnsureSeedFormatClaim(ctx context.Context, reader runtimeclient.Reader, c runtimeclient.Client, log *logr.Logger) error {
	if reader == nil || c == nil {
		// In UT code
		return nil
	}

	value := strconv.Itoa(ibuv1.SeedFormatVersion)

	claim := &clusterclaimv1alpha1.ClusterClaim{}
	err := reader.Get(ctx, types.NamespacedName{Name: ibuv1.SeedFormatClusterClaimName}, claim)
	switch {
	case err == nil:
		if claim.Spec.Value == value {
			return nil
		}
		patch := runtimeclient.MergeFrom(claim.DeepCopy())
		claim.Spec.Value = value
		if err := c.Patch(ctx, claim, patch); err != nil {
			return fmt.Errorf("failed to patch %s ClusterClaim: %w", ibuv1.SeedFormatClusterClaimName, err)
		}
		log.Info("Updated seed format ClusterClaim", "name", ibuv1.SeedFormatClusterClaimName, "value", value)
		return nil
	case k8serrors.IsNotFound(err):
		claim = &clusterclaimv1alpha1.ClusterClaim{
			ObjectMeta: metav1.ObjectMeta{Name: ibuv1.SeedFormatClusterClaimName},
			Spec:       clusterclaimv1alpha1.ClusterClaimSpec{Value: value},
		}
		if err := c.Create(ctx, claim); err != nil {
			if k8serrors.IsAlreadyExists(err) {
				return nil
			}
			return fmt.Errorf("failed to create %s ClusterClaim: %w", ibuv1.SeedFormatClusterClaimName, err)
		}
		log.Info("Created seed format ClusterClaim", "name", ibuv1.SeedFormatClusterClaimName, "value", value)
		return nil
	case meta.IsNoMatchError(err):
		// The ClusterClaim CRD is not installed: this cluster is not ACM-managed
		// (seed cluster, standalone/not-yet-imported SNO, or the post-pivot window
		// before ACM is restored). Nothing to publish.
		log.V(1).Info("ClusterClaim CRD not installed, skipping seed format claim",
			"name", ibuv1.SeedFormatClusterClaimName)
		return nil
	default:
		return fmt.Errorf("failed to get %s ClusterClaim: %w", ibuv1.SeedFormatClusterClaimName, err)
	}
}
