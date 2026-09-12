package nexus

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// repositoriesFixture is a representative NXRM3 GET /v1/repositories body:
// two repositories with different formats and attribute sections, listed in
// non-alphabetical order to exercise the sort.
const repositoriesFixture = `[
  {
    "name": "npm-proxy",
    "format": "npm",
    "type": "proxy",
    "url": "http://nexus.example.com/repository/npm-proxy",
    "version": null,
    "attributes": {
      "storage": {
        "blobStoreName": "default",
        "strictContentTypeValidation": true,
        "quotaType": null
      },
      "proxy": {
        "remoteUrl": "https://registry.npmjs.org",
        "contentMaxAge": 1440,
        "metadataMaxAge": 1440
      },
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
      "storage": {
        "blobStoreName": "docker-blob",
        "strictContentTypeValidation": false,
        "writePolicy": "ALLOW",
        "quotaType": "count",
        "quotaLimit": 100
      },
      "docker": {"httpPort": 8082, "v1Enabled": false},
      "cleanup": {"policyNames": ["weekly-cleanup", "old-artifacts"]}
    }
  }
]`

// newTestServer starts an httptest server serving the given status and body,
// recording the received Authorization header.
func newTestServer(t *testing.T, status int, body string) (*httptest.Server, *[]string) {
	t.Helper()

	var gotAuth []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = append(gotAuth, r.Header.Get("Authorization"))

		if r.URL.Path != pathRepositories {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(ts.Close)

	return ts, &gotAuth
}

func TestListRepositories(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		status    int
		body      string
		username  string
		password  string
		wantNames []string
		wantErr   bool
	}{
		"success sorts by name": {
			status:    http.StatusOK,
			body:      repositoriesFixture,
			wantNames: []string{"docker-hosted", "npm-proxy"},
		},
		"unauthorized is a clear error": {
			status:  http.StatusUnauthorized,
			body:    `{"message":"bad credentials"}`,
			wantErr: true,
		},
		"server error includes truncated body": {
			status:  http.StatusInternalServerError,
			body:    "boom",
			wantErr: true,
		},
		"invalid json is an error": {
			status:  http.StatusOK,
			body:    "not json",
			wantErr: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert := assert.New(t)

			ts, _ := newTestServer(t, tt.status, tt.body)
			client := NewClient(ts.URL, tt.username, tt.password, nil)

			repos, err := client.ListRepositories(t.Context())

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(repos)

				return
			}

			require.NoError(t, err)
			assert.Len(repos, len(tt.wantNames))

			for i, want := range tt.wantNames {
				assert.Equal(want, repos[i].Name)
			}
		})
	}
}

func TestListRepositoriesSendsBasicAuth(t *testing.T) {
	t.Parallel()

	assert := assert.New(t)

	ts, gotAuth := newTestServer(t, http.StatusOK, repositoriesFixture)
	client := NewClient(ts.URL, "admin", "secret123", nil)

	_, err := client.ListRepositories(t.Context())
	require.NoError(t, err)
	assert.Len(*gotAuth, 1)
	assert.Equal("Basic YWRtaW46c2VjcmV0MTIz", (*gotAuth)[0])
}

func TestListRepositoriesAnonymous(t *testing.T) {
	t.Parallel()

	assert := assert.New(t)

	ts, gotAuth := newTestServer(t, http.StatusOK, repositoriesFixture)
	client := NewClient(ts.URL, "", "", nil)

	_, err := client.ListRepositories(t.Context())
	require.NoError(t, err)
	assert.Len(*gotAuth, 1)
	assert.Empty((*gotAuth)[0])
}

func TestListRepositoriesRawIsLossless(t *testing.T) {
	t.Parallel()

	assert := assert.New(t)

	ts, _ := newTestServer(t, http.StatusOK, repositoriesFixture)
	client := NewClient(ts.URL, "", "", nil)

	raw, err := client.ListRepositoriesRaw(t.Context())
	require.NoError(t, err)

	// The raw body must still contain fields the Repository struct does not
	// model (docker.httpPort, cleanup.policyNames), proving nothing was
	// dropped by decoding.
	assert.Contains(string(raw), `"httpPort": 8082`)
	assert.Contains(string(raw), `"policyNames"`)

	var decoded []map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Len(decoded, 2)
}

func TestAttributeValue(t *testing.T) {
	t.Parallel()

	// sectionStorage is repeated in the fixture table below (goconst).
	const sectionStorage = "storage"

	repo := Repository{
		Name: "docker-hosted",
		Attributes: map[string]map[string]any{
			sectionStorage: {
				"writePolicy":     "ALLOW",
				"quotaLimit":      float64(100),
				"strictCT":        true,
				"policyNamesList": []any{"a", "b"},
			},
			"empty": {"nilValue": nil},
		},
	}

	tests := map[string]struct {
		section string
		key     string
		want    string
	}{
		"string":          {sectionStorage, "writePolicy", "ALLOW"},
		"number no .0":    {sectionStorage, "quotaLimit", "100"},
		"bool":            {sectionStorage, "strictCT", "true"},
		"array joined":    {sectionStorage, "policyNamesList", "a;b"},
		"nil value":       {"empty", "nilValue", ""},
		"missing section": {"proxy", "remoteUrl", ""},
		"missing key":     {sectionStorage, "blobStoreName", ""},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, attributeValue(repo, tt.section, tt.key))
		})
	}
}
