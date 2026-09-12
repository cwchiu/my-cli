package certinfo

import (
	"crypto/sha256"
	"crypto/tls"
)

// sha256Sum computes the SHA-256 digest of data.
func sha256Sum(data []byte) []byte {
	sum := sha256.Sum256(data)

	return sum[:]
}

// tlsVersionName maps a tls version constant to its well-known name.
func tlsVersionName(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return "unknown"
	}
}
