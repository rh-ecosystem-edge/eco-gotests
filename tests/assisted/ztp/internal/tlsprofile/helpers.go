package tlsprofile

import (
	"context"
	"encoding/json"

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

// RemoveAPIServerTLSProfile removes the tlsSecurityProfile from the cluster APIServer.
func RemoveAPIServerTLSProfile(client *clients.Settings) {
	patchBytes := []byte(`{"spec":{"tlsSecurityProfile":null}}`)
	apiserver := &configv1.APIServer{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}}

	err := client.Patch(context.TODO(), apiserver,
		runtimeclient.RawPatch(types.MergePatchType, patchBytes))
	Expect(err).ToNot(HaveOccurred(), "failed to remove apiserver TLS profile")
}
