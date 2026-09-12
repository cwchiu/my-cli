package certinfo

import (
	"context"
	"crypto/tls"
	"net"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startTLSServer starts an httptest TLS server that keeps the connection
// open long enough for the handshake, and returns its host:port.
func startTLSServer(t *testing.T) string {
	t.Helper()

	ts := httptest.NewTLSServer(nil)
	t.Cleanup(ts.Close)

	addr := strings.TrimPrefix(ts.URL, "https://")

	return addr
}

func TestSplitTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		target  string
		host    string
		port    string
		wantErr bool
	}{
		{name: "bare host defaults to 443", target: "example.com", host: "example.com", port: "443"},
		{name: "host with port", target: "example.com:8443", host: "example.com", port: "8443"},
		{name: "bracketed IPv6 with port", target: "[::1]:443", host: "::1", port: "443"},
		{name: "IPv4 host", target: "127.0.0.1", host: "127.0.0.1", port: "443"},
		{name: "empty target", target: "", wantErr: true},
		{name: "invalid port", target: "example.com:notaport", wantErr: true},
		{name: "bare IPv6 is ambiguous", target: "::1", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			host, port, err := splitTarget(tt.target)
			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.host, host)
			assert.Equal(t, tt.port, port)
		})
	}
}

func TestFetchAgainstTestServer(t *testing.T) {
	t.Parallel()

	addr := startTLSServer(t)
	host, _, err := net.SplitHostPort(addr)
	require.NoError(t, err)

	info, err := Fetch(t.Context(), addr, Options{Insecure: true, Timeout: 5 * time.Second})
	require.NoError(t, err)

	assert.Equal(t, addr, info.Addr)
	assert.Equal(t, host, info.ServerName)
	assert.Contains(t, []string{"TLS 1.2", "TLS 1.3"}, info.TLSVersion)
	assert.NotEmpty(t, info.CipherSuite)
	require.NotEmpty(t, info.Chain)

	leaf := info.Chain[0]
	assert.NotEmpty(t, leaf.Subject)
	assert.NotEmpty(t, leaf.Issuer)
	assert.NotEmpty(t, leaf.SerialNumber)
	assert.False(t, leaf.NotAfter.IsZero())
	assert.NotEmpty(t, leaf.SHA256Fingerprint)
	assert.Contains(t, leaf.SHA256Fingerprint, ":")
	assert.Positive(t, leaf.PublicKeyBits)
	assert.NotEmpty(t, leaf.SignatureAlgo)
}

func TestFetchVerificationFailureWithoutInsecure(t *testing.T) {
	t.Parallel()

	// httptest uses a self-signed cert, so strict verification must fail.
	addr := startTLSServer(t)

	_, err := Fetch(t.Context(), addr, Options{Timeout: 5 * time.Second})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "certificate")
}

func TestFetchConnectionRefused(t *testing.T) {
	t.Parallel()

	// Port 1 on localhost is refused in practice; the error must mention
	// the address.
	_, err := Fetch(t.Context(), "127.0.0.1:1", Options{Timeout: 5 * time.Second})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "127.0.0.1:1")
}

func TestFetchTimeout(t *testing.T) {
	t.Parallel()

	// A non-routable address makes the dial hang until the timeout fires.
	_, err := Fetch(t.Context(), "10.255.255.1:443", Options{Timeout: 200 * time.Millisecond})
	require.Error(t, err)
}

func TestFetchEmptyTarget(t *testing.T) {
	t.Parallel()

	_, err := Fetch(t.Context(), "", Options{})
	require.Error(t, err)
}

func TestTLSVersionName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		version uint16
		want    string
	}{
		{tls.VersionTLS10, "TLS 1.0"},
		{tls.VersionTLS11, "TLS 1.1"},
		{tls.VersionTLS12, "TLS 1.2"},
		{tls.VersionTLS13, "TLS 1.3"},
		{0x0000, "unknown"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, tlsVersionName(tt.version))
	}
}

func TestFetchContextCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := Fetch(ctx, "example.com:443", Options{})
	require.Error(t, err)
}
