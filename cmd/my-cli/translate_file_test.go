package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	argTranslateFile = "translate-file"
	testSourceText   = "Hello, world!"
)

func TestTranslateFileCommand(t *testing.T) {
	t.Parallel()

	inputPath := writeTranslateTestFile(t, testSourceText)

	var request translateRequest

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, requestIn *http.Request) {
		if requestIn.Method != http.MethodPost {
			t.Errorf("request method = %q, want %q", requestIn.Method, http.MethodPost)
		}

		if err := json.NewDecoder(requestIn.Body).Decode(&request); err != nil {
			t.Errorf("decode translation request: %v", err)
		}

		writer.Header().Set("Content-Type", "application/json")

		if _, err := writer.Write([]byte(`{"data":"你好，世界！"}`)); err != nil {
			t.Errorf("write translation response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	out, err := executeCommand(t, argTranslateFile, inputPath, "--endpoint", server.URL)
	require.NoError(t, err)
	assert.Equal(t, testSourceText, request.Text)
	assert.Equal(t, "auto", request.SourceLang)
	assert.Equal(t, "ZH", request.TargetLang)
	assert.Contains(t, out, "Source:\n"+testSourceText)
	assert.Contains(t, out, "Chinese:\n你好，世界！")
}

func TestTranslateFileJSONOutput(t *testing.T) {
	t.Parallel()

	inputPath := writeTranslateTestFile(t, testSourceText)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")

		if _, err := writer.Write([]byte(`{"data":"你好，世界！"}`)); err != nil {
			t.Errorf("write translation response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	out, err := executeCommand(t,
		argTranslateFile, inputPath,
		"--endpoint", server.URL,
		"--output", outputFormatJSON,
	)
	require.NoError(t, err)

	var result translateResult
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.Equal(t, testSourceText, result.Source)
	assert.Equal(t, "你好，世界！", result.Translation)
}

func TestTranslateFileGoogleProvider(t *testing.T) {
	t.Parallel()

	inputPath := writeTranslateTestFile(t, testSourceText)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, http.MethodGet, request.Method)
		assert.Equal(t, "/translate_a/single", request.URL.Path)
		assert.Equal(t, "gtx", request.URL.Query().Get("client"))
		assert.Equal(t, "auto", request.URL.Query().Get("sl"))
		assert.Equal(t, "zh-TW", request.URL.Query().Get("tl"))
		assert.Equal(t, testSourceText, request.URL.Query().Get("q"))

		writer.Header().Set("Content-Type", "application/json")

		if _, err := writer.Write([]byte(`{"sentences":[{"trans":"你好，"},{"trans":"世界！"}]}`)); err != nil {
			t.Errorf("write Google response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	out, err := executeCommand(t,
		argTranslateFile, inputPath,
		"--provider", translateProviderGoogle,
		"--endpoint", server.URL+"/translate_a/single",
	)
	require.NoError(t, err)
	assert.Contains(t, out, "Chinese:\n你好，世界！")
}

// TestTranslateFileMicrosoftProvider uses process-wide environment variables
// for credentials and therefore cannot run in parallel.
func TestTranslateFileMicrosoftProvider(t *testing.T) {
	inputPath := writeTranslateTestFile(t, testSourceText)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, http.MethodPost, request.Method)
		assert.Equal(t, "/translate", request.URL.Path)
		assert.Equal(t, "3.0", request.URL.Query().Get("api-version"))
		assert.Equal(t, "auto-detect", request.URL.Query().Get("from"))
		assert.Equal(t, "zh-Hant", request.URL.Query().Get("to"))
		assert.Equal(t, "test-key", request.Header.Get("Ocp-Apim-Subscription-Key"))
		assert.Equal(t, "westus", request.Header.Get("Ocp-Apim-Subscription-Region"))

		var payload []struct {
			Text string `json:"Text"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode Microsoft request: %v", err)
		}

		if len(payload) != 1 {
			t.Errorf("Microsoft request item count = %d, want 1", len(payload))
			return
		}

		assert.Equal(t, testSourceText, payload[0].Text)

		writer.Header().Set("Content-Type", "application/json")

		if _, err := writer.Write([]byte(`[{"translations":[{"text":"你好，世界！"}]}]`)); err != nil {
			t.Errorf("write Microsoft response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv(envMicrosoftTranslateKey, "test-key")
	t.Setenv(envMicrosoftTranslateRegion, "westus")

	out, err := executeCommand(t,
		argTranslateFile, inputPath,
		"--provider", translateProviderMicrosoft,
		"--endpoint", server.URL+"/translate",
	)
	require.NoError(t, err)
	assert.Contains(t, out, "Chinese:\n你好，世界！")
}

func TestTranslateFileUsageErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		wantMsg string
	}{
		{
			name:    "missing file",
			args:    []string{argTranslateFile},
			wantMsg: "accepts 1 arg",
		},
		{
			name:    "invalid output format",
			args:    []string{argTranslateFile, "file.txt", argFlagOutput, argOutputXML},
			wantMsg: wantBadFormat,
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

func TestTranslateFileRequestError(t *testing.T) {
	t.Parallel()

	inputPath := writeTranslateTestFile(t, testSourceText)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(server.Close)

	_, err := executeCommand(t, argTranslateFile, inputPath, "--endpoint", server.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "translation request failed: status 502")
}

func TestTranslateFileTimeout(t *testing.T) {
	t.Parallel()

	inputPath := writeTranslateTestFile(t, testSourceText)
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
		<-request.Context().Done()
	}))
	t.Cleanup(server.Close)

	_, err := executeCommand(t,
		argTranslateFile, inputPath,
		"--endpoint", server.URL,
		"--timeout", "50ms",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "send translation request")
}

func TestTranslateFileMissingSource(t *testing.T) {
	t.Parallel()

	_, err := executeCommand(t, argTranslateFile, "missing-source.txt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "open source file")
}

func TestTranslateFileRejectsInvalidUTF8(t *testing.T) {
	t.Parallel()

	inputPath := writeTranslateTestFile(t, string([]byte{0xff}))

	_, err := executeCommand(t, argTranslateFile, inputPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "source file is not valid UTF-8")
}

func writeTranslateTestFile(t *testing.T, content string) string {
	t.Helper()

	file, err := os.CreateTemp(t.TempDir(), "source-*.txt")
	require.NoError(t, err)
	require.NoError(t, file.Close())
	require.NoError(t, os.WriteFile(file.Name(), []byte(content), 0o600))

	return file.Name()
}
