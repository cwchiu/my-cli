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
