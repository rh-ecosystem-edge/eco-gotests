package helper

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
)

const httpFetchTimeout = 30 * time.Second

// fetchYAMLFromURL fetches raw YAML content from an HTTPS URL. When skipTLS is true, TLS certificate verification is
// skipped. Cleartext HTTP URLs and HTTPS-to-HTTP redirects are rejected.
func fetchYAMLFromURL(rawURL string, skipTLS bool) ([]byte, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse URL %s: %w", rawURL, err)
	}

	if parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("URL must use https scheme, got %q", parsedURL.Scheme)
	}

	klog.V(tsparams.LogLevel).Infof("Fetching YAML from %s (skipTLS=%v)", rawURL, skipTLS)

	client := &http.Client{
		Timeout: httpFetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "https" {
				return fmt.Errorf("refusing redirect to non-https URL %s", req.URL.String())
			}

			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}

			return nil
		},
	}

	if skipTLS {
		client.Transport = &http.Transport{
			//nolint:gosec // user-requested via ECO_CNF_RAN_SKIP_TLS_VERIFY
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				MinVersion:         tls.VersionTLS12,
			},
		}
	}

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
