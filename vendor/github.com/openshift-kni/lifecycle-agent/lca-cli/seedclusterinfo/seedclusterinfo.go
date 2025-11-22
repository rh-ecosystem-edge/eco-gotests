package seedclusterinfo

import (
	"fmt"

	"github.com/openshift-kni/lifecycle-agent/utils"
)

// SeedClusterInfo is a struct that contains information about the seed cluster
// that was used to create the seed image. It is meant to be serialized to a
// file on the seed image. It has multiple purposes, see the documentation of
// each field for more information.
//
// Changes to this struct should not be made lightly, as it will break
// backwards compatibilitiy with existing seed images. If you've made a
// breaking change, you will need to increment the [SeedFormatVersion] constant
// to avoid silently breakage and allow for backwards compatibility code.
type SeedClusterInfo struct {
	// The OCP version of the seed cluster that was used to create this seed
	// image. During an IBU, lifecycle-agent will compare the user's desired
	// version with the seed cluster's version to ensure the image the user is
	// using is actually using the version they expect. During an IBI, this
	// parameter is ignored.
	SeedClusterOCPVersion string `json:"seed_cluster_ocp_version,omitempty"`

	// The base domain of the seed cluster that was used to create this seed
	// image. This in combination with the cluster name is used to construct
	// the cluster's full domain name. That domain name is required when we ask
	// recert to replace the domain in certificates, as recert needs both the
	// original domain (the seed's) and the new domain to perform the replace.
	BaseDomain string `json:"base_domain,omitempty"`

	// See BaseDomain documentation above.
	ClusterName string `json:"cluster_name,omitempty"`

	// The IP address of the seed cluster's SNO node.
	// Deprecated: Use NodeIPs instead.
	NodeIP string `json:"node_ip,omitempty"`

	// The IP addresses of the seed cluster's SNO node. One for single stack
	// clusters, and two for dual-stack clusters.
	NodeIPs []string `json:"node_ips,omitempty"`

	// The container registry used to host the release image of the seed cluster.
	// TODO: Document what this is for
	// TODO: Is this really necessary? Find a way to get rid of this
	ReleaseRegistry string `json:"release_registry,omitempty"`

	// Whether or not the seed cluster was configured to use a mirror registry or not.
	// TODO: Document what this is for
	// TODO: Is this really necessary? Find a way to get rid of this
	MirrorRegistryConfigured bool `json:"mirror_registry_configured,omitempty"`

	// The hostname of the seed cluster's SNO node. This hostname is required
	// when we ask recert to replace the original hostname in certificates, as
	// recert needs both the original hostname and the new hostname to perform
	// the replace.
	SNOHostname string `json:"sno_hostname,omitempty"`

	// The recert image pull-spec that was used by the seed cluster. This is
	// used to run recert using the same version of recert that was used to
	// create the seed image (the seed cluster runs recert to expire the
	// certificates, so it has already proven to run successfully on the seed
	// data).
	RecertImagePullSpec string `json:"recert_image_pull_spec,omitempty"`

	// Whether the seed has a proxy configured or not. Seed clusters without a
	// proxy cannot be used to upgrade or install clusters with a proxy. So
	// whether or not the seed has a proxy configured is important for LCA to
	// know so it can optionally deny the upgrade or installation of clusters
	// with a proxy from seeds that don't have one. Similarly, installing a
	// cluster without a proxy from a seed with a proxy is also not supported.
	HasProxy bool `json:"has_proxy"`

	// The cluster network CIDR of the seed cluster that was used to create this seed
	ClusterNetworks []string `json:"cluster_network_cidr,omitempty"`

	// The service network CIDR of the seed cluster that was used to create this seed
	ServiceNetworks []string `json:"service_network_cidr,omitempty"`

	// Whether the seed has a proxy configured or not. Seed clusters without
	// FIPS cannot be used to upgrade or install clusters with FIPS. So whether
	// or not the seed has a proxy configured is important for LCA to know so
	// it can optionally deny the upgrade or installation of clusters with FIPS
	// from seeds that don't have one. Similarly, installing a cluster without
	// FIPS from a seed with FIPS is also not supported.
	HasFIPS bool `json:"has_fips"`

	AdditionalTrustBundle *AdditionalTrustBundle `json:"additionalTrustBundle"`

	// The target for the /var/lib/containers mountpoint, if setup.
	ContainerStorageMountpointTarget string `json:"container_storage_mountpoint_target,omitempty"`

	// IngressCertificateCN is the Subject.CN of the ingress router-ca signer
	// certificate, which is of the form `ingress-operator@<unix-timestampt>`.
	// Without this recert will not be able to sign the ingress certificate with
	// the correct ingress private key, because it looks for strict equality in
	// the certificate's Subject.CN.
	IngressCertificateCN string `json:"ingress_certificate_cn,omitempty"`

	// The list of subnets of the seed cluster.
	// For single stack ocp clusters, this will be a single subnet.
	// For dual-stack ocp clusters, this will be a list of two subnets.
	MachineNetworks []string `json:"machine_networks,omitempty"`
}

type AdditionalTrustBundle struct {
	// Whether the "user-ca-bundle" configmap in the "openshift-config"
	// namespace has a value or not
	HasUserCaBundle bool `json:"hasUserCaBundle"`

	// The Proxy CR trustedCA configmap name
	ProxyConfigmapName string `json:"proxyConfigmapName"`
}

func NewFromClusterInfo(clusterInfo *utils.ClusterInfo,
	seedImagePullSpec string,
	hasProxy,
	hasFIPS bool,
	additionalTrustBundle *AdditionalTrustBundle,
	containerStorageMountpointTarget string,
	ingressCertificateCN string,
) *SeedClusterInfo {
	return &SeedClusterInfo{
		SeedClusterOCPVersion:    clusterInfo.OCPVersion,
		BaseDomain:               clusterInfo.BaseDomain,
		ClusterName:              clusterInfo.ClusterName,
		NodeIPs:                  clusterInfo.NodeIPs,
		ReleaseRegistry:          clusterInfo.ReleaseRegistry,
		SNOHostname:              clusterInfo.Hostname,
		MirrorRegistryConfigured: clusterInfo.MirrorRegistryConfigured,
		RecertImagePullSpec:      seedImagePullSpec,
		HasProxy:                 hasProxy,
		HasFIPS:                  hasFIPS,
		AdditionalTrustBundle:    additionalTrustBundle,
		MachineNetworks:          clusterInfo.MachineNetworks,

		ContainerStorageMountpointTarget: containerStorageMountpointTarget,

		IngressCertificateCN: ingressCertificateCN,
	}
}

func ReadSeedClusterInfoFromFile(path string) (*SeedClusterInfo, error) {
	data := &SeedClusterInfo{}
	err := utils.ReadYamlOrJSONFile(path, data)
	if err != nil {
		return nil, fmt.Errorf("failed to create seed cluster info from file %w", err)
	}
	return data, nil
}
