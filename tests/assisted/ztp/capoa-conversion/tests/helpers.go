package capoa_conversion_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/capoa-conversion/internal/inittools"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// readResource fetches a cluster resource as the specified GVK and fails the test if not found.
func readResource(gvk schema.GroupVersionKind, name, namespace string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)

	err := HubAPIClient.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, obj)
	ExpectWithOffset(1, err).ToNot(HaveOccurred(),
		"failed to read %s/%s as %s", namespace, name, gvk.Version)

	return obj
}

// assertCRDServesVersion fails the test if the given CRD does not serve the specified API version.
func assertCRDServesVersion(crdName, version string) {
	crd := &unstructured.Unstructured{}
	crd.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Version: "v1",
		Kind:    "CustomResourceDefinition",
	})

	err := HubAPIClient.Get(context.TODO(), types.NamespacedName{Name: crdName}, crd)
	ExpectWithOffset(1, err).ToNot(HaveOccurred(), "CRD %s not found", crdName)

	versions, found, _ := unstructured.NestedSlice(crd.Object, "spec", "versions")
	ExpectWithOffset(1, found).To(BeTrue(), "CRD %s has no spec.versions", crdName)

	served := false

	for _, v := range versions {
		vMap, ok := v.(map[string]interface{})
		if !ok {
			continue
		}

		vName, _, _ := unstructured.NestedString(vMap, "name")
		vServed, _, _ := unstructured.NestedBool(vMap, "served")

		if vName == version && vServed {
			served = true

			break
		}
	}

	ExpectWithOffset(1, served).To(BeTrue(),
		"CRD %s does not serve version %s", crdName, version)
}

// nestedNumber extracts a numeric field from an unstructured object, handling both int64 and float64 JSON representations.
func nestedNumber(obj map[string]interface{}, fields ...string) int64 {
	val, found, err := unstructured.NestedFieldNoCopy(obj, fields...)
	ExpectWithOffset(1, err).ToNot(HaveOccurred(), "error reading %v", fields)
	ExpectWithOffset(1, found).To(BeTrue(), "%v should exist", fields)

	switch v := val.(type) {
	case int64:
		return v
	case float64:
		return int64(v)
	default:
		Fail(fmt.Sprintf("%v has unexpected type %T (value: %v)", fields, val, val))

		return 0
	}
}
