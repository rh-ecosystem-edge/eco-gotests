package helpers

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ibipreinstall/internal/tsparams"
)

const httpFetchTimeout = 30 * time.Second

// IBIConfigTemplateData holds the values resolved at runtime to fill the
// image-based-installation-config.yaml template stored in the config repo.
type IBIConfigTemplateData struct {
	SeedImage          string
	SeedVersion        string
	PullSecret         string
	SSHKey             string
	CACert             string
	NetworkConfig      string
	PreinstallRegistry string
	MCEMirror          string
	ACMMirror          string
}

// ResolveIBIConfigTemplate fetches the template from templateURL, executes it
// with the provided data, and writes the result as image-based-installation-config.yaml
// in workDir (the directory openshift-install reads from).
func ResolveIBIConfigTemplate(templateURL string, data *IBIConfigTemplateData, workDir string, skipTLS bool) error {
	klog.V(tsparams.LogLevel).Infof("Fetching IBI config template from %s", templateURL)

	raw, err := FetchYAMLFromURL(templateURL, skipTLS)
	if err != nil {
		return fmt.Errorf("fetch IBI config template: %w", err)
	}

	data.CACert = indentBlock(data.CACert, 2)
	data.NetworkConfig = indentBlock(data.NetworkConfig, 2)

	tmpl, err := template.New("ibi-config").Option("missingkey=error").Parse(string(raw))
	if err != nil {
		return fmt.Errorf("parse IBI config template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("execute IBI config template: %w", err)
	}

	outPath := filepath.Join(workDir, "image-based-installation-config.yaml")
	if err := os.WriteFile(outPath, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", outPath, err)
	}

	klog.V(tsparams.LogLevel).Infof("Wrote resolved IBI config to %s (%d bytes)", outPath, buf.Len())

	saveRedactedCopy(tmpl, data, workDir)

	return nil
}

// ResolveIBIConfigTemplateURL returns the explicit override URL if set, otherwise
// derives it from the ClusterInstance URL using the convention:
//
//	<same dir>/preinstall-test/image-based-installation-config.yaml
//
// Example:
//
//	ClusterInstance: https://gitlab.example.com/.../siteconfig/helix54/helix54.yaml
//	Derives: <same base>/.../preinstall-test/image-based-installation-config.yaml
func ResolveIBIConfigTemplateURL(explicitURL, clusterInstanceURL string) (string, error) {
	if strings.TrimSpace(explicitURL) != "" {
		return explicitURL, nil
	}

	parsed, err := url.Parse(clusterInstanceURL)
	if err != nil {
		return "", fmt.Errorf("parse ClusterInstance URL %q: %w", clusterInstanceURL, err)
	}

	dir := path.Dir(parsed.Path)
	parsed.Path = path.Join(dir, "preinstall-test", "image-based-installation-config.yaml")

	result := parsed.String()
	klog.V(tsparams.LogLevel).Infof("Derived IBI config template URL: %s", result)

	return result, nil
}

// ResolveSeedVersion determines the seed version from explicit override or seed image tag.
// Priority: explicit override > tag parsed from seedImage reference.
// Returns an error if the version cannot be determined (e.g., digest-pinned without override).
func ResolveSeedVersion(explicitOverride, seedImage string) (string, error) {
	if v := strings.TrimSpace(explicitOverride); v != "" {
		return v, nil
	}

	tag := tagFromImageRef(seedImage)
	if tag == "" {
		return "", fmt.Errorf(
			"cannot derive seedVersion: image %q has no tag (digest-pinned?); "+
				"set ECO_CNF_RAN_IBI_SEED_VERSION explicitly", seedImage)
	}

	return tag, nil
}

// RegistryHostFromMirror extracts the host:port portion from a mirror URL like
// "registry.example.com:5000/multicluster-engine". Returns everything before the
// first "/" path component, or an error if the mirror string is empty.
func RegistryHostFromMirror(mirror string) (string, error) {
	if mirror == "" {
		return "", fmt.Errorf("cannot derive registry host from empty mirror")
	}

	if idx := strings.Index(mirror, "/"); idx > 0 {
		return mirror[:idx], nil
	}

	return mirror, nil
}

// CopyISOToHTTPDir copies the ISO file to the local HTTP serving directory so the
// BMH can download it via PreinstallHTTPBaseURL. This replaces the previous SCP-based
// approach and works because the test executor is co-located with the HTTP server.
func CopyISOToHTTPDir(isoPath, httpDir string) error {
	destPath := filepath.Join(httpDir, filepath.Base(isoPath))
	tmpPath := destPath + ".tmp"

	klog.V(tsparams.LogLevel).Infof("Copying ISO %s -> %s", isoPath, destPath)

	src, err := os.Open(isoPath)
	if err != nil {
		return fmt.Errorf("open ISO %s: %w", isoPath, err)
	}
	defer src.Close()

	dst, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create destination %s: %w", tmpPath, err)
	}

	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()

		return fmt.Errorf("copy ISO to %s: %w", tmpPath, err)
	}

	if err := dst.Close(); err != nil {
		return fmt.Errorf("close destination %s: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", tmpPath, destPath, err)
	}

	klog.V(tsparams.LogLevel).Infof("Successfully copied ISO to HTTP directory")

	return nil
}

// CreateIBIISO runs openshift-install to create the image-based installation ISO.
func CreateIBIISO(ctx context.Context, openshiftInstallPath, workDir, isoFilename string) (string, error) {
	klog.V(tsparams.LogLevel).Infof("Creating IBI ISO using %s in %s", openshiftInstallPath, workDir)

	cmd := exec.CommandContext(ctx, openshiftInstallPath, "image-based", "create", "image", "--dir", workDir)

	output, err := cmd.CombinedOutput()
	if err != nil {
		const maxOutputLen = 2000

		sanitized := string(output)
		if len(sanitized) > maxOutputLen {
			sanitized = sanitized[:maxOutputLen] + "... (truncated)"
		}

		return "", fmt.Errorf("failed to create IBI ISO: %w, output: %s", err, sanitized)
	}

	isoPath := filepath.Join(workDir, isoFilename)

	if _, statErr := os.Stat(isoPath); statErr != nil {
		return "", fmt.Errorf("ISO output path %s: %w", isoPath, statErr)
	}

	klog.V(tsparams.LogLevel).Infof("Successfully created IBI ISO at %s", isoPath)

	return isoPath, nil
}

// VerifyNmstatectlAvailable checks that nmstatectl is in $PATH. openshift-install
// requires it at runtime when networkConfig is specified in the IBI config.
func VerifyNmstatectlAvailable() error {
	nmstatePath, err := exec.LookPath("nmstatectl")
	if err != nil {
		return fmt.Errorf("nmstatectl not found in $PATH: %w — "+
			"install nmstate (dnf install -y nmstate) or remove networkConfig from the IBI config template", err)
	}

	klog.V(tsparams.LogLevel).Infof("nmstatectl found at %s", nmstatePath)

	return nil
}

// FetchYAMLFromURL fetches raw YAML content from a URL. When skipTLS is true,
// TLS certificate verification is skipped.
func FetchYAMLFromURL(rawURL string, skipTLS bool) ([]byte, error) {
	klog.V(tsparams.LogLevel).Infof("Fetching YAML from %s (skipTLS=%v)", rawURL, skipTLS)

	client := httpClient(skipTLS)

	resp, err := client.Get(rawURL)
	if err != nil {
		return nil, fmt.Errorf("HTTP fetch %s: %w", rawURL, err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP fetch %s: status %d", rawURL, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response from %s: %w", rawURL, err)
	}

	return data, nil
}

// VerifyHTTPAccessible sends a HEAD request to the given URL and returns an error
// if the server does not respond with HTTP 200. Use this to confirm that an artifact
// (e.g., the IBI ISO) is reachable before handing the URL to an external consumer
// like a BareMetalHost.
func VerifyHTTPAccessible(rawURL string) error {
	client := &http.Client{Timeout: httpFetchTimeout}

	resp, err := client.Head(rawURL)
	if err != nil {
		return fmt.Errorf("HEAD %s: %w", rawURL, err)
	}

	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HEAD %s: expected 200, got %d", rawURL, resp.StatusCode)
	}

	klog.V(tsparams.LogLevel).Infof("Verified HTTP accessible: %s (%d)", rawURL, resp.StatusCode)

	return nil
}

// httpClient returns an HTTP client, optionally configured to skip TLS verification.
func httpClient(skipTLS bool) *http.Client {
	client := &http.Client{Timeout: httpFetchTimeout}

	if skipTLS {
		client.Transport = &http.Transport{
			//nolint:gosec // user-requested via ECO_CNF_RAN_SKIP_TLS_VERIFY
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				MinVersion:         tls.VersionTLS12,
			},
		}
	}

	return client
}

// RedactedIBIConfigFilename is the name of the redacted IBI config copy that
// survives openshift-install's "purge asset" step.  The diagnostics collector
// looks for this file instead of the original (which is deleted).
const RedactedIBIConfigFilename = "image-based-installation-config.redacted.yaml"

// saveRedactedCopy writes a copy of the rendered IBI config with the pull secret replaced,
// so the file survives openshift-install's asset purge and can be used for diagnostics.
func saveRedactedCopy(tmpl *template.Template, original *IBIConfigTemplateData, workDir string) {
	redacted := *original
	redacted.PullSecret = "<REDACTED>"

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, &redacted); err != nil {
		klog.V(tsparams.LogLevel).Infof("Failed to render redacted IBI config copy: %v", err)

		return
	}

	dest := filepath.Join(workDir, RedactedIBIConfigFilename)
	if err := os.WriteFile(dest, buf.Bytes(), 0o644); err != nil {
		klog.V(tsparams.LogLevel).Infof("Failed to write redacted IBI config to %s: %v", dest, err)

		return
	}

	klog.V(tsparams.LogLevel).Infof("Saved redacted IBI config to %s", dest)
}

// indentBlock prepends each non-empty line of s with the given number of spaces.
func indentBlock(s string, spaces int) string {
	prefix := strings.Repeat(" ", spaces)
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")

	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + line
		}
	}

	return strings.Join(lines, "\n")
}

// tagFromImageRef extracts the tag portion (after ":") from a container image reference.
func tagFromImageRef(imageRef string) string {
	ref := strings.TrimSpace(imageRef)
	if ref == "" {
		return ""
	}

	if atIdx := strings.LastIndex(ref, "@"); atIdx >= 0 {
		ref = ref[:atIdx]
	}

	lastSlash := strings.LastIndex(ref, "/")
	lastColon := strings.LastIndex(ref, ":")

	if lastColon > lastSlash && lastColon < len(ref)-1 {
		return ref[lastColon+1:]
	}

	return ""
}
