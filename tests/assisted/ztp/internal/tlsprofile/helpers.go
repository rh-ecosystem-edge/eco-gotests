package tlsprofile

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// PatchAPIServerTLSProfile applies the given TLS security profile to the cluster APIServer.
func PatchAPIServerTLSProfile(client *clients.Settings, profile configv1.TLSSecurityProfile) {
	patchMap := map[string]interface{}{
		"spec": map[string]interface{}{
			"tlsSecurityProfile": profile,
		},
	}

	patchBytes, err := json.Marshal(patchMap)
	Expect(err).ToNot(HaveOccurred(), "failed to marshal TLS profile patch")

	apiserver := &configv1.APIServer{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}}
	err = client.Patch(context.TODO(), apiserver, runtimeclient.RawPatch(types.MergePatchType, patchBytes))
	Expect(err).ToNot(HaveOccurred(), "failed to patch apiserver TLS profile")
}

// PatchTLSAdherence sets the tlsAdherence policy on the cluster APIServer.
// The policy parameter is a string because TLSAdherencePolicy is behind a
// feature gate and not available in the vendored configv1 types.
func PatchTLSAdherence(client *clients.Settings, policy string) {
	patchMap := map[string]interface{}{
		"spec": map[string]interface{}{
			"tlsAdherence": policy,
		},
	}

	patchBytes, err := json.Marshal(patchMap)
	Expect(err).ToNot(HaveOccurred(), "failed to marshal tlsAdherence patch")

	apiserver := &configv1.APIServer{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}}
	err = client.Patch(context.TODO(), apiserver, runtimeclient.RawPatch(types.MergePatchType, patchBytes))
	Expect(err).ToNot(HaveOccurred(), "failed to patch tlsAdherence to %s", policy)
}

// RemoveAPIServerTLSProfile removes the tlsSecurityProfile from the cluster APIServer.
func RemoveAPIServerTLSProfile(client *clients.Settings) {
	patchBytes := []byte(`{"spec":{"tlsSecurityProfile":null}}`)
	apiserver := &configv1.APIServer{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}}

	err := client.Patch(context.TODO(), apiserver,
		runtimeclient.RawPatch(types.MergePatchType, patchBytes))
	Expect(err).ToNot(HaveOccurred(), "failed to remove apiserver TLS profile")
}

// WaitForClusterStability waits until all cluster operators are Available and not
// Progressing or Degraded. This prevents test cases from running while the control
// plane is mid-rollout from a previous TLS profile change.
func WaitForClusterStability(client *clients.Settings, timeout time.Duration) {
	By("Waiting for cluster operators to stabilize")

	Eventually(func() string {
		coList := &configv1.ClusterOperatorList{}

		err := client.List(context.TODO(), coList)
		if err != nil {
			return fmt.Sprintf("failed to list cluster operators: %v", err)
		}

		for i := range coList.Items {
			operator := &coList.Items[i]
			available := false
			progressing := true
			degraded := true

			for _, cond := range operator.Status.Conditions {
				switch cond.Type { //nolint:exhaustive
				case configv1.OperatorAvailable:
					available = cond.Status == configv1.ConditionTrue
				case configv1.OperatorProgressing:
					progressing = cond.Status == configv1.ConditionTrue
				case configv1.OperatorDegraded:
					degraded = cond.Status == configv1.ConditionTrue
				}
			}

			if !available || progressing || degraded {
				return fmt.Sprintf("operator %s: available=%v progressing=%v degraded=%v",
					operator.Name, available, progressing, degraded)
			}
		}

		return ""
	}).WithTimeout(timeout).WithPolling(15*time.Second).
		Should(BeEmpty(), "cluster operators should be stable")
}
