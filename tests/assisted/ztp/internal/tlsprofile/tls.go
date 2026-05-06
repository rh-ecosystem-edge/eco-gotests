package tlsprofile

import (
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
)

// AssertTLSConnects verifies that a TLS handshake succeeds with the given version and cipher constraints.
func AssertTLSConnects(client *clients.Settings, component *Component,
	endpoint Endpoint, minVersion, maxVersion uint16, cipherSuites []uint16) {
	addr := StartPortForward(client, component, endpoint)

	defer StopPortForward(endpoint)

	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec
		MinVersion:         minVersion,
		MaxVersion:         maxVersion,
	}

	if len(cipherSuites) > 0 {
		tlsConfig.CipherSuites = cipherSuites
	}

	Eventually(func() error {
		dialer := &net.Dialer{Timeout: 10 * time.Second}

		conn, dialErr := tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
		if dialErr != nil {
			return dialErr
		}

		defer conn.Close()

		state := conn.ConnectionState()
		if !state.HandshakeComplete {
			return fmt.Errorf("TLS handshake not complete")
		}

		return nil
	}).WithTimeout(30*time.Second).WithPolling(2*time.Second).
		Should(Succeed(), "TLS connection to %s (%s) should succeed",
			endpoint.ServiceName, component.Name)
}

// AssertTLSRejectedVersion verifies that a TLS handshake is rejected for the given TLS version.
func AssertTLSRejectedVersion(client *clients.Settings, component *Component,
	endpoint Endpoint, version uint16) {
	AssertTLSRejectedWith(client, component, endpoint, version, version, nil)
}

// AssertTLSRejected verifies that a TLS 1.2 handshake is rejected with the given cipher suites.
func AssertTLSRejected(client *clients.Settings, component *Component,
	endpoint Endpoint, cipherSuites []uint16) {
	AssertTLSRejectedWith(client, component, endpoint, tls.VersionTLS12, tls.VersionTLS12, cipherSuites)
}

// AssertTLSRejectedWith verifies that a TLS handshake is rejected with the given version and cipher constraints.
func AssertTLSRejectedWith(client *clients.Settings, component *Component,
	endpoint Endpoint, minVersion, maxVersion uint16, cipherSuites []uint16) {
	addr := StartPortForward(client, component, endpoint)

	defer StopPortForward(endpoint)

	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec
		MinVersion:         minVersion,
		MaxVersion:         maxVersion,
	}

	if len(cipherSuites) > 0 {
		tlsConfig.CipherSuites = cipherSuites
	}

	Eventually(func() string {
		dialer := &net.Dialer{Timeout: 10 * time.Second}
		conn, dialErr := tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)

		if conn != nil {
			conn.Close()
		}

		if dialErr == nil {
			return "connected"
		}

		errMsg := dialErr.Error()
		if strings.Contains(errMsg, "connection refused") || errMsg == "EOF" {
			return "not-ready"
		}

		return errMsg
	}).WithTimeout(30*time.Second).WithPolling(2*time.Second).
		Should(ContainSubstring("tls:"),
			"TLS connection to %s (%s) should be rejected with TLS error",
			endpoint.ServiceName, component.Name)
}
