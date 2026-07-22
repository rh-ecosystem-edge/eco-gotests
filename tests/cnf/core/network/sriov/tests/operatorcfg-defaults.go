package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/daemonset"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/tsparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

const sriovOperatorConfigResourceName = "default"

var sriovOperatorConfigGVR = schema.GroupVersionResource{
	Group:    "sriovnetwork.openshift.io",
	Version:  "v1",
	Resource: "sriovoperatorconfigs",
}

var specFieldsToVerify = []string{
	"enableInjector", "enableOperatorWebhook", "disableDrain", "logLevel",
}

var _ = Describe("SriovOperatorConfigDefaults", Ordered, Label(tsparams.LabelOperatorConfigDefaultsTestCases),
	ContinueOnFailure, func() {
		var (
			originalSpec            map[string]interface{}
			configSavedSuccessfully bool
		)

		BeforeAll(func() {
			By("Verifying SriovOperatorConfig exists and saving original raw spec")

			rawObj, err := APIClient.Resource(sriovOperatorConfigGVR).
				Namespace(NetConfig.SriovOperatorNamespace).
				Get(context.TODO(), sriovOperatorConfigResourceName, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred(), "Failed to get raw SriovOperatorConfig")

			spec, ok := rawObj.Object["spec"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "spec field not found or not a map in SriovOperatorConfig")

			originalSpec = make(map[string]interface{})

			for _, key := range specFieldsToVerify {
				if val, exists := spec[key]; exists {
					originalSpec[key] = val
				}
			}

			configSavedSuccessfully = true
		})

		AfterAll(func() {
			if !configSavedSuccessfully {
				return
			}

			By("Restoring original SriovOperatorConfig values")

			restoreSpec := make(map[string]interface{})

			for _, key := range specFieldsToVerify {
				if val, exists := originalSpec[key]; exists {
					restoreSpec[key] = val
				} else {
					restoreSpec[key] = nil
				}
			}

			patchObj := map[string]interface{}{"spec": restoreSpec}
			restorePatch, err := json.Marshal(patchObj)
			Expect(err).ToNot(HaveOccurred(), "Failed to marshal restore patch")

			Eventually(func() error {
				_, patchErr := APIClient.Resource(sriovOperatorConfigGVR).
					Namespace(NetConfig.SriovOperatorNamespace).
					Patch(context.TODO(), sriovOperatorConfigResourceName, types.MergePatchType,
						restorePatch, metav1.PatchOptions{})

				return patchErr
			}, 2*time.Minute, 5*time.Second).Should(Succeed(),
				"Failed to restore SriovOperatorConfig")
		})

		It("Verifies spec fields with zero values are preserved when explicitly set",
			reportxml.ID("80401"), func() {
				By("Setting all spec fields to their zero values via merge patch")

				patchData := `{"spec":{"enableInjector":false,"enableOperatorWebhook":false,` +
					`"disableDrain":false,"logLevel":0}}`

				_, err := APIClient.Resource(sriovOperatorConfigGVR).
					Namespace(NetConfig.SriovOperatorNamespace).
					Patch(context.TODO(), sriovOperatorConfigResourceName, types.MergePatchType,
						[]byte(patchData), metav1.PatchOptions{})
				Expect(err).ToNot(HaveOccurred(), "Failed to patch SriovOperatorConfig with zero values")

				By("Verifying fields are preserved in the raw spec after operator reconciliation")

				expectedFields := map[string]interface{}{
					"enableInjector":        false,
					"enableOperatorWebhook": false,
					"disableDrain":          false,
					"logLevel":              float64(0),
				}

				Consistently(func() error {
					return verifyOperatorConfigFields(expectedFields)
				}, 2*time.Minute, 10*time.Second).Should(Succeed(),
					"Spec fields with zero values were stripped from SriovOperatorConfig")
			})

		It("Verifies spec fields are preserved after toggling from non-zero to zero values",
			reportxml.ID("80402"), func() {
				By("Setting all spec fields to non-zero values via merge patch")

				truePatch := `{"spec":{"enableInjector":true,"enableOperatorWebhook":true,` +
					`"disableDrain":true,"logLevel":2}}`

				Eventually(func() error {
					_, patchErr := APIClient.Resource(sriovOperatorConfigGVR).
						Namespace(NetConfig.SriovOperatorNamespace).
						Patch(context.TODO(), sriovOperatorConfigResourceName, types.MergePatchType,
							[]byte(truePatch), metav1.PatchOptions{})

					return patchErr
				}, time.Minute, 5*time.Second).Should(Succeed(),
					"Failed to patch SriovOperatorConfig with non-zero values")

				By("Waiting for operator to reconcile with non-zero values")

				nonZeroFields := map[string]interface{}{
					"enableInjector":        true,
					"enableOperatorWebhook": true,
					"disableDrain":          true,
					"logLevel":              float64(2),
				}

				Eventually(func() error {
					return verifyOperatorConfigFields(nonZeroFields)
				}, time.Minute, 5*time.Second).Should(Succeed(),
					"Spec fields should be set to non-zero values")

				By("Waiting for operator webhook daemonset to become ready")

				Eventually(func() bool {
					webhookDS, dsErr := daemonset.Pull(
						APIClient, tsparams.OperatorWebhook, NetConfig.SriovOperatorNamespace)
					if dsErr != nil {
						return false
					}

					return webhookDS.IsReady(5 * time.Second)
				}, 2*time.Minute, 5*time.Second).Should(BeTrue(),
					"Operator webhook daemonset did not become ready")

				By("Setting all spec fields to zero values via merge patch")

				falsePatch := `{"spec":{"enableInjector":false,"enableOperatorWebhook":false,` +
					`"disableDrain":false,"logLevel":0}}`

				Eventually(func() error {
					_, patchErr := APIClient.Resource(sriovOperatorConfigGVR).
						Namespace(NetConfig.SriovOperatorNamespace).
						Patch(context.TODO(), sriovOperatorConfigResourceName, types.MergePatchType,
							[]byte(falsePatch), metav1.PatchOptions{})

					return patchErr
				}, time.Minute, 5*time.Second).Should(Succeed(),
					"Failed to patch SriovOperatorConfig with zero values")

				By("Verifying fields are preserved in the raw spec after operator reconciliation")

				zeroFields := map[string]interface{}{
					"enableInjector":        false,
					"enableOperatorWebhook": false,
					"disableDrain":          false,
					"logLevel":              float64(0),
				}

				Consistently(func() error {
					return verifyOperatorConfigFields(zeroFields)
				}, 2*time.Minute, 10*time.Second).Should(Succeed(),
					"Spec fields with zero values were stripped after toggling from non-zero to zero")
			})
	})

// verifyOperatorConfigFields fetches the SriovOperatorConfig using the dynamic client
// and verifies that the specified fields exist in the spec with the expected values.
// The dynamic client is used because the typed Go struct cannot distinguish between
// a missing field and a field explicitly set to its zero value.
func verifyOperatorConfigFields(expectedFields map[string]interface{}) error {
	rawObj, err := APIClient.Resource(sriovOperatorConfigGVR).
		Namespace(NetConfig.SriovOperatorNamespace).
		Get(context.TODO(), sriovOperatorConfigResourceName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get raw SriovOperatorConfig: %w", err)
	}

	spec, ok := rawObj.Object["spec"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("spec field not found or not a map in SriovOperatorConfig")
	}

	for field, expected := range expectedFields {
		actual, exists := spec[field]
		if !exists {
			return fmt.Errorf("%s field is missing from SriovOperatorConfig spec", field)
		}

		if fmt.Sprintf("%v", actual) != fmt.Sprintf("%v", expected) {
			return fmt.Errorf("%s: expected %v (%T), got %v (%T)", field, expected, expected, actual, actual)
		}
	}

	return nil
}
