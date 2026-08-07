package tsparams

import (
	bmhv1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/openshift-kni/k8sreporter"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = []string{ranparam.Label, LabelSuite}

	// ReporterNamespacesToDump tells the reporter from where to collect logs on failure.
	ReporterNamespacesToDump = map[string]string{
		"openshift-machine-api": "machine-api",
		"openshift-config":      "openshift-config",
	}

	// ReporterCRDsToDump lists additional custom resources to dump on failure.
	ReporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &bmhv1alpha1.BareMetalHostList{}},
		{Cr: &bmhv1alpha1.PreprovisioningImageList{}},
		{Cr: &configv1.ImageDigestMirrorSetList{}},
	}
)
