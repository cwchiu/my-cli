package main

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nexusTestServerPayload is a small representative GET /v1/repositories
// response used by the command-level tests.
const nexusTestServerPayload = `[
  {
    "name": "npm-proxy",
    "format": "npm",
    "type": "proxy",
    "url": "http://nexus.example.com/repository/npm-proxy",
    "version": null,
    "attributes": {
      "storage": {"blobStoreName": "default", "strictContentTypeValidation": true},
      "proxy": {"remoteUrl": "https://registry.npmjs.org", "contentMaxAge": 1440},
      "negativeCache": {"enabled": true, "timeToLive": 1440},
      "httpclient": {"blocked": false, "autoBlock": true, "retries": 2}
    }
  },
  {
    "name": "docker-hosted",
    "format": "docker",
    "type": "hosted",
    "url": "http://nexus.example.com/repository/docker-hosted",
    "version": null,
    "attributes": {
      "storage": {"blobStoreName": "docker-blob", "writePolicy": "ALLOW"},
      "docker": {"httpPort": 8082, "v1Enabled": false}
    }
  }
]`

// startNexusTestServer starts an httptest server serving a fixed repositories
// payload, recording the Authorization header of each request.
func startNexusTestServer(t *testing.T, status int, body string) (string, *[]string) {
	t.Helper()

	var gotAuth []string

	ts := newNexusTestHandler(status, body, &gotAuth)
	t.Cleanup(ts.Close)

	return ts.URL, &gotAuth
}

// newNexusTestHandler builds the httptest server backing startNexusTestServer.
// argNexusExport is the subcommand name used across nexus tests; goconst
// flags the repeated literal otherwise.
const argNexusExport = "nexus-repo-export"

// flag names repeated across nexus test cases (goconst).
const (
	argFlagBaseURL = "--base-url"
	argFlagOutput  = "--output"
)

func newNexusTestHandler(status int, body string, gotAuth *[]string) *httptest.Server {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotAuth = append(*gotAuth, r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))

	return ts
}

func TestNexusRepoExportCommand(t *testing.T) {
	t.Parallel()

	baseURL, _ := startNexusTestServer(t, 200, nexusTestServerPayload)

	tests := []struct {
		name     string
		args     []string
		checkOut func(t *testing.T, out string)
	}{
		{
			name: "table output lists repositories sorted",
			args: []string{argNexusExport, argFlagBaseURL, baseURL},
			checkOut: func(t *testing.T, out string) {
				t.Helper()

				is := assert.New(t)
				is.Contains(out, "NAME")
				is.Contains(out, "docker-hosted")
				is.Contains(out, "npm-proxy")
				is.Less(strings.Index(out, "docker-hosted"), strings.Index(out, "npm-proxy"))
			},
		},
		{
			name: "json output is lossless",
			args: []string{argNexusExport, argFlagBaseURL, baseURL, argFlagOutput, "json"},
			checkOut: func(t *testing.T, out string) {
				t.Helper()

				is := assert.New(t)
				// Fields the Repository struct does not model must survive.
				is.Contains(out, `"httpPort": 8082`)
				is.Contains(out, `"remoteUrl": "https://registry.npmjs.org"`)

				var decoded []map[string]any
				require.NoError(t, json.Unmarshal([]byte(out), &decoded))
				is.Len(decoded, 2)
			},
		},
		{
			name: "csv output has 24-column header and rows",
			args: []string{argNexusExport, argFlagBaseURL, baseURL, argFlagOutput, "csv"},
			checkOut: func(t *testing.T, out string) {
				t.Helper()

				is := assert.New(t)

				reader := csv.NewReader(strings.NewReader(out))
				records, err := reader.ReadAll()
				require.NoError(t, err)
				is.Len(records, 3) // header + 2 repos
				is.Equal(nexusCSVHeader, records[0])

				// docker-hosted row: writePolicy from storage section.
				is.Equal("docker-hosted", records[1][0])
				is.Equal("docker", records[1][1])
				is.Equal("hosted", records[1][2])
				is.Equal("ALLOW", records[1][8])

				// npm-proxy row: proxy remoteUrl flattened.
				is.Equal("npm-proxy", records[2][0])
				is.Equal("https://registry.npmjs.org", records[2][17])
			},
		},
		{
			name: "format filter keeps only matching",
			args: []string{argNexusExport, argFlagBaseURL, baseURL, "--format", "docker"},
			checkOut: func(t *testing.T, out string) {
				t.Helper()

				is := assert.New(t)
				is.Contains(out, "docker-hosted")
				is.NotContains(out, "npm-proxy")
			},
		},
		{
			name: "type filter keeps only matching",
			args: []string{argNexusExport, argFlagBaseURL, baseURL, "--type", "proxy"},
			checkOut: func(t *testing.T, out string) {
				t.Helper()

				is := assert.New(t)
				is.Contains(out, "npm-proxy")
				is.NotContains(out, "docker-hosted")
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

func TestNexusRepoExportOutFile(t *testing.T) {
	t.Parallel()

	assert := assert.New(t)

	baseURL, _ := startNexusTestServer(t, 200, nexusTestServerPayload)
	outPath := filepath.Join(t.TempDir(), "repos.csv")

	_, err := executeCommand(t, argNexusExport,
		argFlagBaseURL, baseURL, argFlagOutput, "csv", "--out", outPath)
	require.NoError(t, err)

	// #nosec G304 -- path comes from t.TempDir(), not user input.
	data, err := os.ReadFile(outPath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	assert.Len(lines, 3)
	assert.True(strings.HasPrefix(lines[0], "Name,Format,Type,URL"))
}

func TestNexusRepoExportErrors(t *testing.T) {
	t.Parallel()

	baseURL, _ := startNexusTestServer(t, 401, `{"message":"bad credentials"}`)

	tests := []struct {
		name    string
		args    []string
		wantErr bool
		isUsage bool
	}{
		{
			name:    "missing base-url is a usage error",
			args:    []string{argNexusExport},
			wantErr: true,
			isUsage: true,
		},
		{
			name:    "invalid output format is a usage error",
			args:    []string{argNexusExport, argFlagBaseURL, baseURL, argFlagOutput, "xml"},
			wantErr: true,
			isUsage: true,
		},
		{
			name:    "invalid type filter is a usage error",
			args:    []string{argNexusExport, argFlagBaseURL, baseURL, "--type", "mirror"},
			wantErr: true,
			isUsage: true,
		},
		{
			name:    "username without password env fails fast",
			args:    []string{argNexusExport, argFlagBaseURL, baseURL, "--username", "admin"},
			wantErr: true,
		},
		{
			name:    "401 from server is a general error",
			args:    []string{argNexusExport, argFlagBaseURL, baseURL},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := executeCommand(t, tt.args...)
			require.Error(t, err)

			if tt.isUsage {
				assert.ErrorIs(t, err, errUsage)
			}
		})
	}
}

func TestNexusRepoExportBasicAuthFromEnv(t *testing.T) {
	// t.Setenv cannot be used with t.Parallel.
	assert := assert.New(t)

	baseURL, gotAuth := startNexusTestServer(t, 200, nexusTestServerPayload)
	t.Setenv("NEXUS_USERNAME", "admin")
	t.Setenv("NEXUS_PASSWORD", "secret123")

	_, err := executeCommand(t, argNexusExport, argFlagBaseURL, baseURL)
	require.NoError(t, err)

	assert.Len(*gotAuth, 1)
	assert.True(strings.HasPrefix((*gotAuth)[0], "Basic "))
}
