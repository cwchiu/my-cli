package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/cwchiu/my-cli/internal/certinfo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// test constants to keep goconst quiet about repeated literals.
const (
	testCertTarget         = "example.com"
	testCertBadFmt         = "yaml"
	testCertUnsupportedFmt = "unsupported output format"
)

func TestResolveCertConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     certConfig
		args    []string
		wantErr string
	}{
		{
			name: "valid config",
			raw: certConfig{
				timeout:      time.Second,
				outputFormat: outputFormatTable,
			},
			args: []string{testCertTarget},
		},
		{
			name: "missing target",
			raw: certConfig{
				timeout:      time.Second,
				outputFormat: outputFormatTable,
			},
			args:    []string{},
			wantErr: "expected exactly one target",
		},
		{
			name: "blank target",
			raw: certConfig{
				timeout:      time.Second,
				outputFormat: outputFormatTable,
			},
			args:    []string{"   "},
			wantErr: "target must not be empty",
		},
		{
			name: "non-positive timeout",
			raw: certConfig{
				timeout:      0,
				outputFormat: outputFormatTable,
			},
			args:    []string{testCertTarget},
			wantErr: "timeout must be positive",
		},
		{
			name: testCertUnsupportedFmt,
			raw: certConfig{
				timeout:      time.Second,
				outputFormat: testCertBadFmt,
			},
			args:    []string{testCertTarget},
			wantErr: testCertUnsupportedFmt,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := resolveCertConfig(tt.raw, tt.args)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, strings.TrimSpace(tt.args[0]), cfg.target)
		})
	}
}

func TestExpiryWarning(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		notAfter time.Time
		want     string
	}{
		{
			name:     "expired",
			notAfter: now.Add(-48 * time.Hour),
			want:     "EXPIRED 2 days ago",
		},
		{
			name:     "expiring soon",
			notAfter: now.Add(10 * 24 * time.Hour),
			want:     "WARNING: expires in 11 days",
		},
		{
			name:     "far future",
			notAfter: now.Add(365 * 24 * time.Hour),
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, expiryWarning(now, tt.notAfter))
		})
	}
}

func TestRenderCertTable(t *testing.T) {
	t.Parallel()

	info := &certinfo.Info{
		Host:        testCertTarget,
		Addr:        testCertTarget + ":443",
		TLSVersion:  "TLS 1.3",
		CipherSuite: "TLS_AES_128_GCM_SHA256",
		ServerName:  testCertTarget,
		Chain: []certinfo.Certificate{
			{
				Subject:           "CN=" + testCertTarget,
				Issuer:            "CN=Test CA",
				SerialNumber:      "0a0b0c",
				NotBefore:         time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				NotAfter:          time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
				DNSNames:          []string{testCertTarget, "www." + testCertTarget},
				SignatureAlgo:     "SHA256-RSA",
				PublicKeyAlgo:     "RSA",
				PublicKeyBits:     2048,
				SHA256Fingerprint: "aa:bb:cc",
			},
		},
	}

	var buf bytes.Buffer
	require.NoError(t, renderCertTable(&buf, info))

	out := buf.String()
	assert.Contains(t, out, "Host:")
	assert.Contains(t, out, "example.com:443")
	assert.Contains(t, out, "TLS 1.3")
	assert.Contains(t, out, "TLS_AES_128_GCM_SHA256")
	assert.Contains(t, out, "CN=example.com")
	assert.Contains(t, out, "CN=Test CA")
	assert.Contains(t, out, "example.com, www.example.com")
	assert.Contains(t, out, "SHA256-RSA")
	assert.Contains(t, out, "RSA (2048 bits)")
	assert.Contains(t, out, "aa:bb:cc")
	assert.Contains(t, out, "Chain depth:")
}

func TestRenderCertJSON(t *testing.T) {
	t.Parallel()

	info := &certinfo.Info{
		Host:        testCertTarget,
		Addr:        testCertTarget + ":443",
		TLSVersion:  "TLS 1.3",
		CipherSuite: "TLS_AES_128_GCM_SHA256",
		ServerName:  testCertTarget,
		Chain: []certinfo.Certificate{
			{
				Subject:           "CN=" + testCertTarget,
				Issuer:            "CN=Test CA",
				SHA256Fingerprint: "aa:bb:cc",
				PublicKeyBits:     2048,
			},
		},
	}

	var buf bytes.Buffer
	require.NoError(t, renderCertInfo(&buf, outputFormatJSON, info))

	var decoded certinfo.Info
	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
	assert.Equal(t, *info, decoded)
}

func TestRenderCertInfoUnsupportedFormat(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	err := renderCertInfo(&buf, testCertBadFmt, &certinfo.Info{})
	require.ErrorContains(t, err, testCertUnsupportedFmt)
}
