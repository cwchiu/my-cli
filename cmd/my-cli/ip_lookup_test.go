package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ipInfoTestPayload is a representative ipinfo.io /json response used by the
// command-level tests. It mirrors the real API (Source of Truth, see spec).
const ipInfoTestPayload = `{
  "ip": "8.8.8.8",
  "hostname": "dns.google",
  "city": "Mountain View",
  "region": "California",
  "country": "US",
  "loc": "37.4056,-122.0775",
  "org": "AS15169 Google LLC",
  "postal": "94043",
  "timezone": "America/Los_Angeles",
  "readme": "https://ipinfo.io/missingauth",
  "anycast": true
}`

// ipInfoMinimalPayload omits every optional field, exercising the skip-empty
// path of the table renderer.
const ipInfoMinimalPayload = `{"ip": "36.239.107.27"}`

// argIPLookup is the subcommand name used across ip tests; goconst flags the
// repeated literal otherwise. argFlagBaseURL is declared in
// nexus_export_test.go; these are new literals shared across test files.
const (
	argIPLookup   = "ip-lookup"
	argOutputXML  = "xml"
	wantBadFormat = "unsupported output format"
)

// startIPInfoTestServer starts an httptest server serving a fixed payload
// with the given status, and records the request paths it received.
func startIPInfoTestServer(t *testing.T, status int, body string) (string, *[]string) {
	t.Helper()

	paths := &[]string{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*paths = append(*paths, r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(ts.Close)

	return ts.URL, paths
}

func TestIPLookupCommand(t *testing.T) {
	t.Parallel()

	baseURL, _ := startIPInfoTestServer(t, http.StatusOK, ipInfoTestPayload)

	tests := []struct {
		name     string
		args     []string
		checkOut func(t *testing.T, out string)
	}{
		{
			name: "table output shows key-value fields in order",
			args: []string{argIPLookup, argFlagBaseURL, baseURL},
			checkOut: func(t *testing.T, out string) {
				t.Helper()

				is := assert.New(t)
				// tabwriter aligns to the widest key ("hostname:") plus padding.
				is.Contains(out, "ip:        8.8.8.8")
				is.Contains(out, "hostname:  dns.google")
				is.Contains(out, "city:      Mountain View")
				is.Contains(out, "org:       AS15169 Google LLC")
				is.Contains(out, "anycast:   true")
				// readme is never shown.
				is.NotContains(out, "readme")
			},
		},
		{
			name: "json output is lossless",
			args: []string{argIPLookup, argFlagBaseURL, baseURL, argFlagOutput, outputFormatJSON},
			checkOut: func(t *testing.T, out string) {
				t.Helper()

				is := assert.New(t)
				is.Contains(out, `"anycast": true`)
				is.Contains(out, `"readme": "https://ipinfo.io/missingauth"`)

				var decoded map[string]any
				require.NoError(t, json.Unmarshal([]byte(out), &decoded))
				is.Len(decoded, 11)
			},
		},
		{
			name: "positional IP targets that address",
			args: []string{argIPLookup, argFlagBaseURL, baseURL, "8.8.8.8"},
			checkOut: func(t *testing.T, out string) {
				t.Helper()

				assert.Contains(t, out, "ip:        8.8.8.8")
			},
		},
		{
			name: "ipv6 positional address is accepted",
			args: []string{argIPLookup, argFlagBaseURL, baseURL, "2001:4860:4860::8888"},
			checkOut: func(t *testing.T, out string) {
				t.Helper()

				assert.Contains(t, out, "ip:        8.8.8.8")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out, err := executeCommand(t, tt.args...)
			require.NoError(t, err)
			tt.checkOut(t, out)
		})
	}
}

func TestIPLookupRequestPath(t *testing.T) {
	t.Parallel()

	assert := assert.New(t)

	baseURL, paths := startIPInfoTestServer(t, http.StatusOK, ipInfoTestPayload)

	_, err := executeCommand(t, argIPLookup, argFlagBaseURL, baseURL, "8.8.8.8")
	require.NoError(t, err)
	assert.Equal([]string{"/8.8.8.8/json"}, *paths)

	_, err = executeCommand(t, argIPLookup, argFlagBaseURL, baseURL)
	require.NoError(t, err)
	assert.Equal([]string{"/8.8.8.8/json", "/json"}, *paths)
}

func TestIPLookupSkipsEmptyFields(t *testing.T) {
	t.Parallel()

	assert := assert.New(t)

	baseURL, _ := startIPInfoTestServer(t, http.StatusOK, ipInfoMinimalPayload)

	out, err := executeCommand(t, argIPLookup, argFlagBaseURL, baseURL)
	require.NoError(t, err)

	assert.Contains(out, "ip:  36.239.107.27")
	// Only the ip line is rendered; no empty placeholders.
	assert.Equal(1, strings.Count(out, "\n"))
}

func TestIPLookupErrors(t *testing.T) {
	t.Parallel()

	okURL, _ := startIPInfoTestServer(t, http.StatusOK, ipInfoTestPayload)
	rateURL, _ := startIPInfoTestServer(t, http.StatusTooManyRequests,
		`{"status":429,"error":{"title":"Rate limited","message":"Quota exceeded"}}`)
	plainURL, _ := startIPInfoTestServer(t, http.StatusInternalServerError, "boom")
	badJSONURL, _ := startIPInfoTestServer(t, http.StatusOK, "{not json")

	tests := []struct {
		name    string
		args    []string
		wantErr string
		isUsage bool
	}{
		{
			name:    "invalid positional IP is a usage error",
			args:    []string{argIPLookup, argFlagBaseURL, okURL, "not-an-ip"},
			wantErr: "invalid IP address",
			isUsage: true,
		},
		{
			name:    "two positional args are a usage error",
			args:    []string{argIPLookup, argFlagBaseURL, okURL, "8.8.8.8", "1.1.1.1"},
			wantErr: "accepts at most 1 arg",
			isUsage: true,
		},
		{
			name:    "invalid output format is a usage error",
			args:    []string{argIPLookup, argFlagBaseURL, okURL, argFlagOutput, argOutputXML},
			wantErr: wantBadFormat,
			isUsage: true,
		},
		{
			name:    "non-positive timeout is a usage error",
			args:    []string{argIPLookup, argFlagBaseURL, okURL, "--timeout", "0s"},
			wantErr: "timeout must be positive",
			isUsage: true,
		},
		{
			name:    "429 with envelope names the quota",
			args:    []string{argIPLookup, argFlagBaseURL, rateURL},
			wantErr: "free anonymous quota exhausted",
		},
		{
			name:    "500 with plain body is truncated",
			args:    []string{argIPLookup, argFlagBaseURL, plainURL},
			wantErr: "status 500: boom",
		},
		{
			name:    "invalid json from server is an error",
			args:    []string{argIPLookup, argFlagBaseURL, badJSONURL},
			wantErr: "parse ipinfo response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := executeCommand(t, tt.args...)
			require.ErrorContains(t, err, tt.wantErr)

			if tt.isUsage {
				assert.ErrorIs(t, err, errUsage)
			}
		})
	}
}
