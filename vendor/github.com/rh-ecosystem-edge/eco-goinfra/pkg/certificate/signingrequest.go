package certificate

import (
	"context"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	certificatesv1 "k8s.io/api/certificates/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// SigningRequestBuilder provides a struct for CertificateSigningRequest resource containing a connection to the cluster
// and the CertificateSigningRequest definition.
type SigningRequestBuilder struct {
	common.EmbeddableBuilder[certificatesv1.CertificateSigningRequest, *certificatesv1.CertificateSigningRequest]
	common.EmbeddableCreator[
		certificatesv1.CertificateSigningRequest, SigningRequestBuilder,
		*certificatesv1.CertificateSigningRequest, *SigningRequestBuilder,
	]
	common.EmbeddableDeleter[certificatesv1.CertificateSigningRequest, *certificatesv1.CertificateSigningRequest]
}

// AttachMixins wires the embedded CRUD mixins to this builder instance.
func (builder *SigningRequestBuilder) AttachMixins() {
	builder.EmbeddableCreator.SetBase(builder)
	builder.EmbeddableDeleter.SetBase(builder)
}

// GetGVK returns the CertificateSigningRequest GVK for this builder.
func (builder *SigningRequestBuilder) GetGVK() schema.GroupVersionKind {
	return certificatesv1.SchemeGroupVersion.WithKind("CertificateSigningRequest")
}

// NewSigningRequestBuilder creates a new instance of SigningRequestBuilder.
func NewSigningRequestBuilder(apiClient *clients.Settings, name string) *SigningRequestBuilder {
	return common.NewClusterScopedBuilder[certificatesv1.CertificateSigningRequest, SigningRequestBuilder](
		apiClient, certificatesv1.AddToScheme, name)
}

// PullSigningRequest loads an existing signing request into SigningRequestBuilder struct.
func PullSigningRequest(apiClient *clients.Settings, name string) (*SigningRequestBuilder, error) {
	return common.PullClusterScopedBuilder[certificatesv1.CertificateSigningRequest, SigningRequestBuilder](
		context.TODO(), apiClient, certificatesv1.AddToScheme, name)
}
