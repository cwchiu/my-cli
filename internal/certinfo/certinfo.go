// Package certinfo fetches and describes the TLS certificate chain served
// by a remote endpoint, for the `cert-info` subcommand.
//
// The package performs a TLS handshake against host:port and returns the
// peer certificate chain plus connection metadata. It never validates
// anything beyond what the caller asks for: verification is controlled by
// the Insecure flag, mirroring `openssl s_client -verify 0`.
package certinfo

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// defaultPort is used when the target has no explicit port.
const defaultPort = "443"

// defaultTimeout bounds the dial plus handshake when the caller passes no
// deadline through ctx.
const defaultTimeout = 10 * time.Second

// errEmptyChain is returned when a server completes the handshake without
// presenting any certificate.
var errEmptyChain = errors.New("server presented no certificate")

// Info is the human-relevant summary of one TLS connection's certificates.
type Info struct {
	// Host is the target as given by the caller (host[:port]).
	Host string `json:"host"`
	// Addr is the resolved dial address (host:port).
	Addr string `json:"addr"`
	// TLSVersion is the negotiated version, e.g. "TLS 1.3".
	TLSVersion string `json:"tls_version"`
	// CipherSuite is the negotiated cipher, e.g. "TLS_AES_128_GCM_SHA256".
	CipherSuite string `json:"cipher_suite"`
	// ServerName is the SNI value sent to the server.
	ServerName string `json:"server_name"`
	// Chain holds the peer certificates, leaf first.
	Chain []Certificate `json:"chain"`
}

// Certificate is the human-relevant summary of one X.509 certificate.
type Certificate struct {
	Subject           string    `json:"subject"`
	Issuer            string    `json:"issuer"`
	SerialNumber      string    `json:"serial_number"`
	NotBefore         time.Time `json:"not_before"`
	NotAfter          time.Time `json:"not_after"`
	DNSNames          []string  `json:"dns_names,omitempty"`
	IPAddresses       []string  `json:"ip_addresses,omitempty"`
	SignatureAlgo     string    `json:"signature_algorithm"`
	PublicKeyAlgo     string    `json:"public_key_algorithm"`
	PublicKeyBits     int       `json:"public_key_bits"`
	SHA256Fingerprint string    `json:"sha256_fingerprint"`
	IsCA              bool      `json:"is_ca"`
}

// Options controls one Fetch call.
type Options struct {
	// Insecure skips certificate verification (like curl -k / openssl
	// -verify 0) so self-signed or internal-CA chains can still be
	// inspected. The chain is collected either way.
	Insecure bool
	// Timeout bounds dial plus handshake. Zero means defaultTimeout.
	Timeout time.Duration
}

// Fetch connects to target (host or host:port), performs a TLS handshake,
// and returns the certificate chain and connection metadata.
func Fetch(ctx context.Context, target string, opts Options) (*Info, error) {
	host, port, err := splitTarget(target)
	if err != nil {
		return nil, err
	}

	addr := net.JoinHostPort(host, port)

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	dialer := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: timeout},
		Config: &tls.Config{
			ServerName: host,
			MinVersion: tls.VersionTLS12,
			// When verification is on, Go fills PeerCertificates with the
			// verified chain. When off, the raw peer chain is kept as-is.
			InsecureSkipVerify: opts.Insecure, //nolint:gosec // opt-in via --insecure to inspect untrusted chains
		},
	}

	conn, err := dialer.DialContext(dialCtx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", addr, err)
	}

	defer func() {
		_ = conn.Close()
	}()

	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return nil, fmt.Errorf("connection to %s is not TLS", addr)
	}

	// ConnectionState is only meaningful after the handshake completes;
	// DialContext returns before that, so finish it explicitly.
	if err := tlsConn.HandshakeContext(dialCtx); err != nil {
		return nil, fmt.Errorf("handshake %s: %w", addr, err)
	}

	state := tlsConn.ConnectionState()

	certs := state.PeerCertificates
	if len(certs) == 0 {
		return nil, errEmptyChain
	}

	return &Info{
		Host:        target,
		Addr:        addr,
		TLSVersion:  tlsVersionName(state.Version),
		CipherSuite: tls.CipherSuiteName(state.CipherSuite),
		// ConnectionState.ServerName is only populated server-side; the SNI
		// we actually sent is the host from splitTarget.
		ServerName: host,
		Chain:      describeChain(certs),
	}, nil
}

// splitTarget splits "host" or "host:port" into its parts, defaulting the
// port to 443. Bare IPv6 literals must be bracketed, matching net.Dial.
func splitTarget(target string) (host, port string, err error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", "", errors.New("empty target")
	}

	// Bracketed IPv6 literal.
	if strings.HasPrefix(target, "[") {
		host, port, err = net.SplitHostPort(target)
		if err != nil {
			return "", "", fmt.Errorf("parse target %q: %w", target, err)
		}

		return host, port, nil
	}

	if host, port, err = net.SplitHostPort(target); err == nil {
		if _, convErr := strconv.ParseUint(port, 10, 16); convErr != nil {
			return "", "", fmt.Errorf("parse target %q: invalid port %q", target, port)
		}

		return host, port, nil
	}

	// No port: treat the whole string as a host. Reject bare IPv6 literals
	// (ambiguous without brackets) but allow IPv4 and DNS names.
	if strings.Contains(target, ":") {
		return "", "", fmt.Errorf(
			"parse target %q: bare IPv6 address must be written as [addr]:port", target)
	}

	return target, defaultPort, nil
}

// describeChain converts x509 certificates into the human-relevant summary.
func describeChain(certs []*x509.Certificate) []Certificate {
	out := make([]Certificate, 0, len(certs))

	for _, cert := range certs {
		ips := make([]string, 0, len(cert.IPAddresses))
		for _, ip := range cert.IPAddresses {
			ips = append(ips, ip.String())
		}

		out = append(out, Certificate{
			Subject:           cert.Subject.String(),
			Issuer:            cert.Issuer.String(),
			SerialNumber:      cert.SerialNumber.Text(16),
			NotBefore:         cert.NotBefore,
			NotAfter:          cert.NotAfter,
			DNSNames:          cert.DNSNames,
			IPAddresses:       ips,
			SignatureAlgo:     cert.SignatureAlgorithm.String(),
			PublicKeyAlgo:     cert.PublicKeyAlgorithm.String(),
			PublicKeyBits:     publicKeyBits(cert),
			SHA256Fingerprint: fingerprint(cert),
			IsCA:              cert.IsCA,
		})
	}

	return out
}

// publicKeyBits returns the public key size in bits, or 0 when the key type
// is unsupported (the algorithm name is still shown).
func publicKeyBits(cert *x509.Certificate) int {
	switch key := cert.PublicKey.(type) {
	case interface{ Size() int }:
		return key.Size() * 8
	default:
		return 0
	}
}

// fingerprint returns the colon-separated SHA-256 fingerprint of the
// certificate's DER encoding.
func fingerprint(cert *x509.Certificate) string {
	sum := sha256Sum(cert.Raw)

	parts := make([]string, len(sum))
	for i, b := range sum {
		parts[i] = strconv.FormatUint(uint64(b), 16)
	}

	return strings.Join(parts, ":")
}
