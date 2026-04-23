package helpers

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/openshift/installer/pkg/types"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

const (
	idmsName                  = "image-digest-mirror"
	icspName                  = "redhat-internal-icsp"
	mirrorRegistryCAName      = "mirror-registry-ca"
	mirrorRegistryCANamespace = "multicluster-engine"

	registryConfBlockMarker    = "[[registry]]"
	registryConfSearchMaxBytes = 4000
)

// BuildImageDigestSourcesFromHub discovers mirror hosts from the hub (IDMS, ICSP, optional mirror-registry-ca)
// and returns imageDigestSources for image-based-installation-config.yaml, aligned with
// ocp-edge ibi_clusterinstance_preinstall.yaml.
func BuildImageDigestSourcesFromHub(ctx context.Context, apiClient *clients.Settings) (
	[]types.ImageDigestSource, error) {
	if apiClient == nil {
		return nil, fmt.Errorf("hub api client is nil")
	}

	mirrorHost, err := primaryMirrorHostFromHub(ctx, apiClient)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(mirrorHost) == "" {
		return nil, fmt.Errorf("primary mirror host is empty")
	}

	mceMirror, acmMirror := mceAndACMMirrorsFromConfigMap(ctx, apiClient, mirrorHost)

	if strings.TrimSpace(mceMirror) == "" || strings.TrimSpace(acmMirror) == "" {
		return nil, fmt.Errorf(
			"MCE/ACM mirror locations are required for disconnected IBI config: mce=%q acm=%q (mirror host %q)",
			mceMirror, acmMirror, mirrorHost)
	}

	return ibiDisconnectedImageDigestSources(mirrorHost, mceMirror, acmMirror), nil
}

func primaryMirrorHostFromHub(ctx context.Context, apiClient *clients.Settings) (string, error) {
	idms, err := apiClient.ImageDigestMirrorSets().Get(ctx, idmsName, metav1.GetOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return "", fmt.Errorf("get ImageDigestMirrorSet %q: %w", idmsName, err)
		}
	}

	idmsHadEntries := idms != nil && len(idms.Spec.ImageDigestMirrors) > 0

	if idmsHadEntries {
		for _, entry := range idms.Spec.ImageDigestMirrors {
			if len(entry.Mirrors) == 0 {
				continue
			}

			full := string(entry.Mirrors[0])
			host := strings.Split(full, "/")[0]

			if host != "" {
				klog.Infof("Using mirror host from ImageDigestMirrorSet %q: %s", idmsName, host)

				return host, nil
			}
		}
	}

	icsp, err := apiClient.ImageContentSourcePolicies().Get(ctx, icspName, metav1.GetOptions{})
	if err != nil {
		if idmsHadEntries {
			return "", fmt.Errorf(
				"ImageDigestMirrorSet %q had no usable mirrors in entry.Mirrors and ICSP %q fallback failed: %w",
				idmsName, icspName, err)
		}

		return "", fmt.Errorf("no usable mirror host from ImageDigestMirrorSet %q and get ICSP %q failed: %w",
			idmsName, icspName, err)
	}

	if icsp == nil || len(icsp.Spec.RepositoryDigestMirrors) == 0 ||
		len(icsp.Spec.RepositoryDigestMirrors[0].Mirrors) == 0 {
		if idmsHadEntries {
			return "", fmt.Errorf(
				"ImageDigestMirrorSet %q had no usable mirrors in entry.Mirrors and ICSP %q has no usable repositoryDigestMirrors",
				idmsName, icspName)
		}

		return "", fmt.Errorf("ICSP %q has no repositoryDigestMirrors", icspName)
	}

	full := icsp.Spec.RepositoryDigestMirrors[0].Mirrors[0]
	host := strings.Split(full, "/")[0]
	klog.Infof("Using mirror host from ImageContentSourcePolicy %q: %s", icspName, host)

	return host, nil
}

func mceAndACMMirrorsFromConfigMap(
	ctx context.Context,
	apiClient *clients.Settings,
	mirrorHostFallback string,
) (mce, acm string) {
	registryCAConfigMap, err := apiClient.ConfigMaps(mirrorRegistryCANamespace).Get(
		ctx, mirrorRegistryCAName, metav1.GetOptions{})
	if err != nil || registryCAConfigMap == nil {
		klog.V(1).Infof("mirror-registry-ca configmap unavailable: %v", err)

		return defaultMCEACMMirrors(mirrorHostFallback)
	}

	raw, ok := registryCAConfigMap.Data["registries.conf"]
	if !ok || raw == "" {
		return defaultMCEACMMirrors(mirrorHostFallback)
	}

	mce = registryLocationFromConf(raw, "registry.redhat.io/multicluster-engine")
	acm = registryLocationFromConf(raw, "registry.redhat.io/rhacm2")

	if mce == "" || acm == "" {
		dMCE, dACM := defaultMCEACMMirrors(mirrorHostFallback)

		if mce == "" {
			mce = dMCE
		}

		if acm == "" {
			acm = dACM
		}
	}

	return mce, acm
}

func defaultMCEACMMirrors(mirrorHost string) (string, string) {
	if mirrorHost == "" {
		return "", ""
	}

	return mirrorHost + "/multicluster-engine", mirrorHost + "/rhacm2"
}

// registryLocationFromConf approximates Ansible grep/sed on registries.conf for a registry prefix block.
// Some blocks list the upstream registry first and the mirror second; this returns the first location value
// that is not registryPrefix (the mirror). If every location equals registryPrefix, it falls back to the first match.
func registryLocationFromConf(registriesConf, registryPrefix string) string {
	idx := strings.Index(registriesConf, registryPrefix)
	if idx < 0 {
		return ""
	}

	window := registryConfBlockWindow(registriesConf[idx:])

	re := regexp.MustCompile(`location\s*=\s*"([^"]+)"`)

	all := re.FindAllStringSubmatch(window, -1)
	if len(all) == 0 {
		return ""
	}

	prefix := strings.TrimSpace(registryPrefix)

	var first string

	for _, m := range all {
		if len(m) < 2 {
			continue
		}

		loc := strings.TrimSpace(m[1])
		if first == "" {
			first = loc
		}

		if loc != prefix {
			return loc
		}
	}

	return first
}

// registryConfBlockWindow limits the slice to the same [[registry]] block as the match: from the
// prefix hit up to (but not including) the next registryConfBlockMarker. If no marker appears,
// the search is capped at registryConfSearchMaxBytes (legacy bound).
func registryConfBlockWindow(fromPrefix string) string {
	if fromPrefix == "" {
		return ""
	}

	i := strings.Index(fromPrefix, registryConfBlockMarker)
	switch {
	case i > 0:
		return fromPrefix[:i]
	case i == 0:
		rest := fromPrefix[len(registryConfBlockMarker):]

		j := strings.Index(rest, registryConfBlockMarker)
		if j >= 0 {
			return fromPrefix[:len(registryConfBlockMarker)+j]
		}

		if len(fromPrefix) > registryConfSearchMaxBytes {
			return fromPrefix[:registryConfSearchMaxBytes]
		}

		return fromPrefix
	default:
		if len(fromPrefix) > registryConfSearchMaxBytes {
			return fromPrefix[:registryConfSearchMaxBytes]
		}

		return fromPrefix
	}
}

// ibiDisconnectedImageDigestSources mirrors the Jinja template in ibi_clusterinstance_preinstall.yaml.
// release-images covers nightly multi + x86 nightlies and ec/rc/prod; ocp-v4.0-art-dev still uses
// openshift-release-dev/ocp-release fallback.
func ibiDisconnectedImageDigestSources(mirrorHost, mceMirror, acmMirror string) []types.ImageDigestSource {
	openshiftRelease := mirrorHost + "/openshift/release"
	openshiftReleaseImages := mirrorHost + "/openshift/release-images"
	ocpReleasePath := mirrorHost + "/openshift-release-dev/ocp-release"
	artDevMirrors := []string{openshiftRelease, ocpReleasePath}
	nightlyMirrors := []string{openshiftReleaseImages, openshiftRelease}
	ocpReleaseMirrors := []string{openshiftRelease, openshiftReleaseImages}

	return []types.ImageDigestSource{
		{Mirrors: []string{mirrorHost}, Source: "brew.registry.redhat.io"},
		{Mirrors: nightlyMirrors, Source: "quay.io/openshift-release-dev/ocp-release-nightly"},
		{Mirrors: ocpReleaseMirrors, Source: "quay.io/openshift-release-dev/ocp-release"},
		{Mirrors: artDevMirrors, Source: "quay.io/openshift-release-dev/ocp-v4.0-art-dev"},
		{Mirrors: []string{mirrorHost}, Source: "quay.io"},
		{Mirrors: []string{mirrorHost}, Source: "registry-proxy.engineering.redhat.com"},
		{Mirrors: []string{mirrorHost + "/ocp"}, Source: "registry.ci.openshift.org/ocp"},
		{
			Mirrors: []string{
				mirrorHost + "/ocp/release",
				openshiftReleaseImages,
			},
			Source: "registry.ci.openshift.org/ocp/release",
		},
		{
			Mirrors: []string{mirrorHost + "/ocp/4-dev-preview"},
			Source:  "registry.ci.openshift.org/ocp/4-dev-preview",
		},
		{Mirrors: []string{mirrorHost}, Source: "registry.connect.redhat.com"},
		{Mirrors: []string{mirrorHost}, Source: "registry.redhat.io"},
		{Mirrors: []string{mirrorHost}, Source: "registry.stage.redhat.io"},
		{Mirrors: []string{mceMirror}, Source: "registry.redhat.io/multicluster-engine"},
		{Mirrors: []string{acmMirror}, Source: "registry.redhat.io/rhacm2"},
	}
}
