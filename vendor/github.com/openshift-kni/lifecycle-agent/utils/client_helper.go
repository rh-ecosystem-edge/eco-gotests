package utils

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"strings"

	"github.com/samber/lo"
	log "github.com/sirupsen/logrus"

	"github.com/openshift-kni/lifecycle-agent/api/seedreconfig"
	"github.com/openshift-kni/lifecycle-agent/internal/common"
	ocp_config_v1 "github.com/openshift/api/config/v1"
	mcv1 "github.com/openshift/api/machineconfiguration/v1"
	operatorv1alpha1 "github.com/openshift/api/operator/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func GetSecretData(ctx context.Context, name, namespace, key string, client runtimeclient.Client) (string, error) {
	secret := &corev1.Secret{}
	if err := client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, secret); err != nil {
		// NOTE: The error is intentionally left unwrapped here, so the caller
		// can check client.IgnoreNotFound on it
		return "", err //nolint:wrapcheck
	}

	data, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("did not find key %s in Secret %s/%s", key, name, namespace)
	}

	return string(data), nil
}

func GetConfigMapData(ctx context.Context, name, namespace, key string, client runtimeclient.Client) (string, error) {
	cm := &corev1.ConfigMap{}
	if err := client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, cm); err != nil {
		return "", fmt.Errorf("failed to get get configMap: %w", err)
	}

	data, ok := cm.Data[key]
	if !ok {
		return "", fmt.Errorf("did not find key %s in ConfigMap", key)
	}

	return data, nil
}

func GetClusterName(ctx context.Context, client runtimeclient.Client) (string, error) {
	installConfig, err := getInstallConfig(ctx, client)
	if err != nil {
		return "", fmt.Errorf("failed to get install config: %w", err)
	}
	return installConfig.Metadata.Name, nil
}

func GetClusterBaseDomain(ctx context.Context, client runtimeclient.Client) (string, error) {
	installConfig, err := getInstallConfig(ctx, client)
	if err != nil {
		return "", fmt.Errorf("failed to get install config: %w", err)
	}
	return installConfig.BaseDomain, nil
}

type ClusterInfo struct {
	OCPVersion               string
	BaseDomain               string
	ClusterName              string
	ClusterID                string
	NodeIP                   string
	ReleaseRegistry          string
	Hostname                 string
	MirrorRegistryConfigured bool
	ClusterNetworks          []string
	ServiceNetworks          []string
	MachineNetwork           string
	NodeLabels               map[string]string
	IngressCertificateCN     string
}

func GetClusterInfo(ctx context.Context, client runtimeclient.Client) (*ClusterInfo, error) {
	clusterVersion := &ocp_config_v1.ClusterVersion{}
	if err := client.Get(ctx, types.NamespacedName{Name: "version"}, clusterVersion); err != nil {
		return nil, fmt.Errorf("failed to get clusterversion: %w", err)
	}

	clusterName, err := GetClusterName(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to get clusterName: %w", err)
	}

	clusterBaseDomain, err := GetClusterBaseDomain(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to get clusterBaseDomain: %w", err)
	}

	node, err := GetSNOMasterNode(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to get SNOMasterNode: %w", err)
	}
	ip, err := getNodeInternalIP(*node)
	if err != nil {
		return nil, err
	}
	machineNetwork, err := getMachineNetwork(ctx, client, ip)
	if err != nil {
		return nil, err
	}

	hostname, err := getNodeHostname(*node)
	if err != nil {
		return nil, err
	}

	nodeLabels := node.GetLabels()

	releaseRegistry, err := GetReleaseRegistry(ctx, client)
	if err != nil {
		return nil, err
	}

	mirrorRegistrySources, err := GetMirrorRegistrySourceRegistries(ctx, client)
	if err != nil {
		return nil, err
	}

	clusterNetworks, serviceNetworks, err := getClusterNetworks(ctx, client)
	if err != nil {
		return nil, err
	}

	ingressCN, err := GetIngressCertificateCN(ctx, client)
	if err != nil {
		return nil, err
	}

	return &ClusterInfo{
		ClusterName:              clusterName,
		BaseDomain:               clusterBaseDomain,
		OCPVersion:               clusterVersion.Status.Desired.Version,
		ClusterID:                string(clusterVersion.Spec.ClusterID),
		NodeIP:                   ip,
		ReleaseRegistry:          releaseRegistry,
		Hostname:                 hostname,
		MirrorRegistryConfigured: len(mirrorRegistrySources) > 0,
		ClusterNetworks:          clusterNetworks,
		ServiceNetworks:          serviceNetworks,
		MachineNetwork:           machineNetwork,
		NodeLabels:               nodeLabels,
		IngressCertificateCN:     ingressCN,
	}, nil
}

// TODO: add dual stuck support
func getNodeInternalIP(node corev1.Node) (string, error) {
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address, nil
		}
	}
	return "", fmt.Errorf("failed to find node internal ip address")
}

func getMachineNetwork(ctx context.Context, client runtimeclient.Client, nodeIp string) (string, error) {
	installConfig, err := getInstallConfig(ctx, client)
	if err != nil {
		return "", fmt.Errorf("failed to get install config: %w", err)
	}

	for _, mn := range installConfig.Networking.MachineNetwork {
		if _, _, err := net.ParseCIDR(mn.CIDR); err != nil {
			return "", fmt.Errorf("machineNetwork has an invalid CIDR: %s", mn.CIDR)
		}
		if _, ipnet, err := net.ParseCIDR(mn.CIDR); err == nil && ipnet.Contains(net.ParseIP(nodeIp)) {
			return mn.CIDR, nil
		}
	}

	log.Warnf("failed to find machine network in <%s> for node ip: %s. Returning empty string",
		installConfig.Networking.MachineNetwork, nodeIp)

	// retuning empty string if no match found in order to be backward compatible as there is a possibility that
	// that we upgrade cluster that was previously upgraded with lca-agent version that didn't set new machineNetwork
	// in install config (or full install config)
	// in all other cases we should find the match
	return "", nil
}

func getNodeHostname(node corev1.Node) (string, error) {
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeHostName {
			return addr.Address, nil
		}
	}
	return "", fmt.Errorf("failed to find node hostname")
}

type installConfigMetadata struct {
	Name string `json:"name"`
}

type machineNetworkEntry struct {
	// CIDR is the IP block address pool for machines within the cluster.
	CIDR string `json:"cidr"`
}

type basicInstallConfig struct {
	BaseDomain string                `json:"baseDomain"`
	Metadata   installConfigMetadata `json:"metadata"`
	Networking struct {
		MachineCIDR    string                `json:"machineCIDR"`
		MachineNetwork []machineNetworkEntry `json:"machineNetwork,omitempty"`
	} `json:"networking"`
}

func getInstallConfig(ctx context.Context, client runtimeclient.Client) (*basicInstallConfig, error) {
	cm := &corev1.ConfigMap{}
	err := client.Get(ctx, types.NamespacedName{Name: common.InstallConfigCM, Namespace: common.InstallConfigCMNamespace}, cm)
	if err != nil {
		return nil, fmt.Errorf("could not get configMap: %w", err)
	}

	data, ok := cm.Data["install-config"]
	if !ok {
		return nil, fmt.Errorf("did not find key install-config in configmap")
	}

	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(data)), 4096)
	instConf := &basicInstallConfig{}
	if err := decoder.Decode(instConf); err != nil {
		return nil, fmt.Errorf("failed to decode install config, err: %w", err)
	}
	return instConf, nil
}

func GetCSVDeployment(ctx context.Context, client runtimeclient.Client) (*appsv1.Deployment, error) {
	deployment := &appsv1.Deployment{}
	if err := client.Get(ctx,
		types.NamespacedName{
			Name:      common.CsvDeploymentName,
			Namespace: common.CsvDeploymentNamespace},
		deployment); err != nil {
		return nil, fmt.Errorf("failed to get cluster version deployment, err: %w", err)
	}

	return deployment, nil
}

func GetInfrastructure(ctx context.Context, client runtimeclient.Client) (*ocp_config_v1.Infrastructure, error) {
	infrastructure := &ocp_config_v1.Infrastructure{}
	if err := client.Get(ctx,
		types.NamespacedName{
			Name: common.OpenshiftInfraCRName},
		infrastructure); err != nil {
		return nil, fmt.Errorf("failed to get infra CR: %w", err)
	}

	return infrastructure, nil
}

func GetReleaseRegistry(ctx context.Context, client runtimeclient.Client) (string, error) {
	deployment, err := GetCSVDeployment(ctx, client)
	if err != nil {
		return "", err
	}

	return strings.Split(deployment.Spec.Template.Spec.Containers[0].Image, "/")[0], nil
}
func ReadSeedReconfigurationFromFile(path string) (*seedreconfig.SeedReconfiguration, error) {
	data := &seedreconfig.SeedReconfiguration{}
	err := ReadYamlOrJSONFile(path, data)
	return data, err
}

func ExtractRegistryFromImage(image string) string {
	return strings.Split(image, "/")[0]
}

func GetMirrorRegistrySourceRegistries(ctx context.Context, client runtimeclient.Client) ([]string, error) {
	var sourceRegistries []string
	allNamespaces := runtimeclient.ListOptions{Namespace: metav1.NamespaceAll}
	currentIcps := &operatorv1alpha1.ImageContentSourcePolicyList{}
	if err := client.List(ctx, currentIcps, &allNamespaces); err != nil {
		return nil, fmt.Errorf("failed to list ImageContentSourcePolicy: %w", err)
	}
	for _, icsp := range currentIcps.Items {
		for _, rdp := range icsp.Spec.RepositoryDigestMirrors {
			sourceRegistries = append(sourceRegistries, ExtractRegistryFromImage(rdp.Source))
		}
	}
	currentIdms := ocp_config_v1.ImageDigestMirrorSetList{}
	if err := client.List(ctx, &currentIdms, &allNamespaces); err != nil {
		return nil, fmt.Errorf("failed to list ImageDigestMirrorSet: %w", err)
	}

	for _, idms := range currentIdms.Items {
		for _, idm := range idms.Spec.ImageDigestMirrors {
			sourceRegistries = append(sourceRegistries, ExtractRegistryFromImage(idm.Source))
		}
	}
	return sourceRegistries, nil
}

func HasProxy(ctx context.Context, client runtimeclient.Client) (bool, error) {
	proxy := &ocp_config_v1.Proxy{}
	if err := client.Get(ctx, types.NamespacedName{Name: common.OpenshiftProxyCRName}, proxy); err != nil {
		return false, fmt.Errorf("failed to get proxy CR: %w", err)
	}

	if proxy.Spec.HTTPProxy == "" && proxy.Spec.HTTPSProxy == "" && proxy.Spec.NoProxy == "" {
		return false, nil
	}

	return true, nil
}

func HasFIPS(ctx context.Context, client runtimeclient.Client) (bool, error) {
	nodes := &corev1.NodeList{}
	if err := client.List(ctx, nodes); err != nil {
		return false, fmt.Errorf("failed to list nodes: %w", err)
	}

	if len(nodes.Items) != 1 {
		return false, fmt.Errorf("expected exactly one node, got %d", len(nodes.Items))
	}
	node := nodes.Items[0]

	currentConfig := node.Annotations["machineconfiguration.openshift.io/currentConfig"]
	if currentConfig == "" {
		return false, fmt.Errorf("failed to get currentConfig annotation")
	}

	machineConfig := &mcv1.MachineConfig{}
	if err := client.Get(ctx, types.NamespacedName{Name: currentConfig}, machineConfig); err != nil {
		return false, fmt.Errorf("failed to get machineConfig %s: %w", currentConfig, err)
	}

	return machineConfig.Spec.FIPS, nil
}

func GetAdditionalTrustBundleFromConfigmap(ctx context.Context, client client.Client, configmapName string) (string, error) {
	userCaBundleConfigmap := corev1.ConfigMap{}
	if err := client.Get(ctx, types.NamespacedName{Name: configmapName,
		Namespace: common.OpenshiftConfigNamespace}, &userCaBundleConfigmap); err != nil {
		if errors.IsNotFound(err) {
			return "", nil
		}

		return "", fmt.Errorf("failed to get %s/%s configmap: %w", common.OpenshiftConfigNamespace, configmapName, err)
	}

	if userCaBundleConfigmap.Data == nil {
		return "", nil
	}

	if userCaBundleConfigmap.Data[common.CaBundleDataKey] == "" {
		return "", nil
	}

	return userCaBundleConfigmap.Data[common.CaBundleDataKey], nil
}

func GetClusterAdditionalTrustBundleState(ctx context.Context, client client.Client) (bool, string, error) {
	clusterAdditionalTrustBundle, err := GetAdditionalTrustBundleFromConfigmap(ctx, client, common.ClusterAdditionalTrustBundleName)
	if err != nil {
		return false, "", fmt.Errorf("failed to get additional trust bundle from configmap: %w", err)
	}

	hasUserCaBundle := clusterAdditionalTrustBundle != ""

	proxy := ocp_config_v1.Proxy{}
	if err := client.Get(ctx, types.NamespacedName{Name: common.OpenshiftProxyCRName}, &proxy); err != nil {
		return false, "", fmt.Errorf("failed to get proxy: %w", err)
	}

	proxyCaBundle := ""
	switch proxy.Spec.TrustedCA.Name {
	case common.ClusterAdditionalTrustBundleName:
		proxyCaBundle = clusterAdditionalTrustBundle
	case "":
		// No proxy trustedCA configmap is set, do nothing
	default:
		proxyCaBundle, err = GetAdditionalTrustBundleFromConfigmap(ctx, client, proxy.Spec.TrustedCA.Name)
		if err != nil {
			return false, "", fmt.Errorf("failed to get additional trust bundle from configmap: %w", err)
		}

		if proxyCaBundle == "" {
			// This is a very weird but probably valid OCP configuration that we prefer to not support in LCA
			return false, "", fmt.Errorf("proxy trustedCA configmap %s/%s exists but is empty", common.OpenshiftConfigNamespace, proxy.Spec.TrustedCA.Name)
		}
	}

	proxyConfigmapName := ""
	if proxyCaBundle != "" {
		proxyConfigmapName = proxy.Spec.TrustedCA.Name
	}

	return hasUserCaBundle, proxyConfigmapName, nil
}

func ShouldOverrideSeedRegistry(seedMirrorRegistryConfigured bool, seedReleaseRegistry string, mirrorRegistrySources []string) (bool, error) {
	mirrorRegistryConfigured := len(mirrorRegistrySources) > 0
	// if the SNO doesn't have mirror registries configured:
	//   - and seed SNO has mirror registries configured, then we should try to override the registry
	//   - and seed SNO has no mirror registries configured, then we should not try to override the registry
	if !mirrorRegistryConfigured {
		return seedMirrorRegistryConfigured, nil
	}
	return !lo.Contains(mirrorRegistrySources, seedReleaseRegistry), nil
}

func GetIngressCertificateCN(ctx context.Context, client runtimeclient.Client) (string, error) {
	ingressOperatorCrt, err := GetSecretData(ctx, "router-ca", "openshift-ingress-operator", "tls.crt", client)
	if err != nil {
		return "", err
	}
	return getCommonNameFromCertificate([]byte(ingressOperatorCrt))
}

func getCommonNameFromCertificate(certPEM []byte) (string, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return "", fmt.Errorf("failed to decode PEM block")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("failed to parse certificate: %w", err)
	}

	return cert.Subject.CommonName, nil
}

func getClusterNetworks(ctx context.Context, client runtimeclient.Client) ([]string, []string, error) {
	// oc get network cluster -o yaml
	network := &ocp_config_v1.Network{}
	if err := client.Get(ctx,
		types.NamespacedName{
			Name: common.OpenshiftInfraCRName},
		network); err != nil {
		return nil, nil, fmt.Errorf("failed to get network CR: %w", err)
	}

	var clusterNetworks []string
	for _, cNet := range network.Status.ClusterNetwork {
		clusterNetworks = append(clusterNetworks, cNet.CIDR)
	}

	return clusterNetworks, network.Status.ServiceNetwork, nil
}
