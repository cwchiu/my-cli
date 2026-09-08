package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// argOpenaiChatTest is the subcommand name used across tests; goconst flags
// the repeated literal otherwise.
const argOpenaiChatTest = "openai-chat-test"

// flagBaseAPI and flagModel are repeated flag names in test tables.
const (
	flagBaseAPI = "--base-api"
	flagModel   = "--model"
)

// urlPlaceholder is substituted with the httptest server URL at runtime.
const urlPlaceholder = "{URL}"

// testModel is the model name used across test tables.
const testModel = "gpt-4o-mini"

// chatTestServer is a fake OpenAI-compatible chat completions endpoint.
type chatTestServer struct {
	status      int
	body        string
	gotAuth     string
	gotBody     chatRequest
	requestSeen bool
}

// handler returns an http.HandlerFunc that records the request and replies
// with the canned status and body.
func (s *chatTestServer) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.gotAuth = r.Header.Get("Authorization")
		s.requestSeen = true

		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			s.gotBody = req
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(s.status)
		_, _ = w.Write([]byte(s.body))
	}
}

// successBody is a minimal valid chat completions response.
const successBody = `{
  "choices": [
    {"message": {"role": "assistant", "content": "OK"}}
  ],
  "usage": {"prompt_tokens": 5, "completion_tokens": 1, "total_tokens": 6}
}`

// newChatTestServer starts an httptest server answering with the given
// status and body, and returns its base URL.
func newChatTestServer(t *testing.T, status int, body string) (string, *chatTestServer) {
	t.Helper()

	srv := &chatTestServer{status: status, body: body}
	ts := httptest.NewServer(srv.handler())
	t.Cleanup(ts.Close)

	return ts.URL, srv
}

func TestOpenaiChatTestCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T, out string, srv *chatTestServer)
	}{
		{
			name: "success with key flag sends bearer token",
			args: []string{
				argOpenaiChatTest,
				flagBaseAPI, urlPlaceholder,
				flagModel, testModel,
				"--key", "sk-test-123",
			},
			check: func(t *testing.T, out string, srv *chatTestServer) {
				t.Helper()

				is := assert.New(t)
				is.True(srv.requestSeen)
				is.Equal("Bearer sk-test-123", srv.gotAuth)
				is.Equal(testModel, srv.gotBody.Model)
				is.Len(srv.gotBody.Messages, 1)
				is.Equal("user", srv.gotBody.Messages[0].Role)
				is.Equal(testPrompt, srv.gotBody.Messages[0].Content)
				is.Contains(out, "model:    "+testModel)
				is.Contains(out, "response: OK")
				is.Contains(out, "usage:    prompt=5 completion=1 total=6")
			},
		},
		{
			name: "no key sends no authorization header",
			args: []string{
				argOpenaiChatTest,
				flagBaseAPI, urlPlaceholder,
				flagModel, "llama3.2",
			},
			check: func(t *testing.T, out string, srv *chatTestServer) {
				t.Helper()

				is := assert.New(t)
				is.Empty(srv.gotAuth)
				is.Contains(out, "response: OK")
			},
		},
		{
			name: "json output is machine readable",
			args: []string{
				argOpenaiChatTest,
				flagBaseAPI, urlPlaceholder,
				flagModel, testModel,
				"--output", outputFormatJSON,
			},
			check: func(t *testing.T, out string, _ *chatTestServer) {
				t.Helper()

				var result chatTestResult
				require.NoError(t, json.Unmarshal([]byte(out), &result))
				assert.Equal(t, testModel, result.Model)
				assert.Equal(t, "OK", result.Content)
				assert.Equal(t, 6, result.Usage.TotalTokens)
				assert.GreaterOrEqual(t, result.LatencyMS, 0.0)
			},
		},
		{
			name: "base api trailing slash is trimmed",
			args: []string{
				argOpenaiChatTest,
				flagBaseAPI, urlPlaceholder + "/",
				flagModel, "gpt-4o-mini",
			},
			check: func(t *testing.T, out string, srv *chatTestServer) {
				t.Helper()

				assert.True(t, srv.requestSeen)
				assert.Contains(t, out, "response: OK")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			url, srv := newChatTestServer(t, http.StatusOK, successBody)

			args := make([]string, 0, len(tt.args))
			for _, a := range tt.args {
				args = append(args, strings.ReplaceAll(a, urlPlaceholder, url))
			}

			out, err := executeCommand(t, args...)
			require.NoError(t, err)
			tt.check(t, out, srv)
		})
	}
}

// TestOpenaiChatTestKeyFromEnv covers the OPENAI_API_KEY fallback. It uses
// t.Setenv, which mutates process state and therefore cannot run in
// parallel with the table above.
func TestOpenaiChatTestKeyFromEnv(t *testing.T) {
	url, srv := newChatTestServer(t, http.StatusOK, successBody)

	t.Setenv(envAPIKey, "sk-from-env")

	out, err := executeCommand(t,
		argOpenaiChatTest,
		flagBaseAPI, url,
		flagModel, testModel,
	)
	require.NoError(t, err)
	assert.Equal(t, "Bearer sk-from-env", srv.gotAuth)
	assert.Contains(t, out, "response: OK")
}

func TestOpenaiChatTestHTTPError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		status  int
		body    string
		wantMsg string
	}{
		{
			name:    "401 with api error envelope",
			status:  http.StatusUnauthorized,
			body:    `{"error":{"message":"Incorrect API key provided","type":"invalid_request_error"}}`,
			wantMsg: "Incorrect API key provided",
		},
		{
			name:    "500 with plain body",
			status:  http.StatusInternalServerError,
			body:    "internal server error",
			wantMsg: "internal server error",
		},
		{
			name:    "404 with empty body",
			status:  http.StatusNotFound,
			body:    "",
			wantMsg: "status 404",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			url, _ := newChatTestServer(t, tt.status, tt.body)

			_, err := executeCommand(t,
				argOpenaiChatTest,
				flagBaseAPI, url,
				flagModel, testModel,
				"--key", "sk-secret",
			)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantMsg)
			assert.Contains(t, err.Error(), "chat completions request failed")
		})
	}
}

func TestOpenaiChatTestKeyNeverLeaks(t *testing.T) {
	t.Parallel()

	url, _ := newChatTestServer(t, http.StatusUnauthorized,
		`{"error":{"message":"bad key sk-leaky-key here","type":"invalid_request_error"}}`)

	out, err := executeCommand(t,
		argOpenaiChatTest,
		flagBaseAPI, url,
		flagModel, testModel,
		"--key", "sk-leaky-key",
	)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "sk-leaky-key")
	assert.Contains(t, err.Error(), "[redacted]")
	assert.Empty(t, out)
}

func TestOpenaiChatTestUsageErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		wantMsg string
	}{
		{
			name:    "missing required base-api flag",
			args:    []string{argOpenaiChatTest, flagModel, testModel},
			wantMsg: "required flag(s)",
		},
		{
			name:    "missing required model flag",
			args:    []string{argOpenaiChatTest, flagBaseAPI, "http://localhost/v1"},
			wantMsg: "required flag(s)",
		},
		{
			name:    "unexpected positional argument",
			args:    []string{argOpenaiChatTest, flagBaseAPI, "http://localhost/v1", flagModel, "m", "extra"},
			wantMsg: "unknown command",
		},
		{
			name:    "unsupported output format",
			args:    []string{argOpenaiChatTest, flagBaseAPI, "http://localhost/v1", flagModel, "m", "--output", "xml"},
			wantMsg: "unsupported output format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := executeCommand(t, tt.args...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantMsg)
		})
	}
}

func TestOpenaiChatTestTimeout(t *testing.T) {
	t.Parallel()

	// A server that never answers: the client timeout must surface as an
	// error instead of hanging the test. The handler first drains the
	// request body to EOF, which arms the server's background read on the
	// connection; only then can the server notice the client disconnect
	// and cancel the request context the handler waits on. Without the
	// drain, the context is never cancelled and ts.Close() hangs forever.
	ts := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	}))
	t.Cleanup(ts.Close)

	_, err := executeCommand(t,
		argOpenaiChatTest,
		flagBaseAPI, ts.URL,
		flagModel, testModel,
		"--timeout", "50ms",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "send chat request")
}
