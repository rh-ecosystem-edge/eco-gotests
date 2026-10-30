package certificate

import (
	"context"
	"slices"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	certificatesv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ListSigningRequests returns a list of all CertificateSigningRequest objects in the cluster with the provided options.
func ListSigningRequests(
	apiClient *clients.Settings, options ...runtimeclient.ListOptions) ([]*SigningRequestBuilder, error) {
	return common.List[
		certificatesv1.CertificateSigningRequest,
		certificatesv1.CertificateSigningRequestList,
		SigningRequestBuilder,
	](context.TODO(), apiClient, certificatesv1.AddToScheme, common.ConvertListOptionsToOptions(options)...)
}

// WaitUntilSigningRequestsApproved polls the cluster for all CertificateSigningRequests with the provided options every
// 3 seconds for up to the timeout duration or until all CertificateSigningRequests are approved.
func WaitUntilSigningRequestsApproved(
	apiClient *clients.Settings, timeout time.Duration, options ...runtimeclient.ListOptions) error {
	klog.V(100).Info("Waiting for all CertificateSigningRequests to be approved")

	return wait.PollUntilContextTimeout(
		context.TODO(), 3*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			signingRequests, err := ListSigningRequests(apiClient, options...)
			if err != nil {
				klog.V(100).Infof("Failed to list CertificateSigningRequests: %v", err)

				return false, nil
			}

			for _, signingRequest := range signingRequests {
				if !slices.ContainsFunc(signingRequest.Object.Status.Conditions, approvedCondition) {
					klog.V(100).Infof("CertificateSigningRequest %s is not approved yet", signingRequest.Object.Name)

					return false, nil
				}
			}

			return true, nil
		})
}

func approvedCondition(cond certificatesv1.CertificateSigningRequestCondition) bool {
	return cond.Type == certificatesv1.CertificateApproved && cond.Status == corev1.ConditionTrue
}
