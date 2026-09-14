package main

import (
	"encoding/json"
	"fmt"
	"html"
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
	assert.Equal(t, testSourceText+"\n你好，世界！\n\n", out)
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

	var pairs []translatePair
	require.NoError(t, json.Unmarshal([]byte(out), &pairs))
	require.Len(t, pairs, 1)
	assert.Equal(t, testSourceText, pairs[0].Source)
	assert.Equal(t, "你好，世界！", pairs[0].Translation)
}

func TestTranslateFileGoogleProvider(t *testing.T) {
	t.Parallel()

	inputPath := writeTranslateTestFile(t, testSourceText)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, http.MethodPost, request.Method)
		assert.Equal(t, "/v1/translateHtml", request.URL.Path)
		assert.Equal(t, "application/json+protobuf", request.Header.Get("Content-Type"))
		assert.Equal(t, googleTranslateAPIKey, request.Header.Get("X-Goog-API-Key"))

		var payload []any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode Google request: %v", err)
		}

		assert.Equal(t, []any{[]any{[]any{testSourceText}, "auto", "zh-TW"}, "wt_lib"}, payload)

		writer.Header().Set("Content-Type", "application/json")

		if _, err := writer.Write([]byte(`[["你好，&#34;世界&#34; &amp; more"]]`)); err != nil {
			t.Errorf("write Google response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	out, err := executeCommand(t,
		argTranslateFile, inputPath,
		"--provider", translateProviderGoogle,
		"--endpoint", server.URL+"/v1/translateHtml",
	)
	require.NoError(t, err)
	assert.Contains(t, out, "你好，\"世界\" & more")
}

func TestTranslateFileGoogleEscapesHTML(t *testing.T) {
	t.Parallel()

	inputPath := writeTranslateTestFile(t, `a < b & "c" > d`)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var payload []any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode Google request: %v", err)
		}

		requestText, ok := payload[0].([]any)[0].([]any)[0].(string)
		if !ok {
			t.Errorf("unexpected Google request payload shape: %v", payload)
			return
		}

		assert.Equal(t, html.EscapeString(`a < b & "c" > d`), requestText)

		writer.Header().Set("Content-Type", "application/json")

		if _, err := writer.Write([]byte(`[["ok"]]`)); err != nil {
			t.Errorf("write Google response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	_, err := executeCommand(t,
		argTranslateFile, inputPath,
		"--provider", translateProviderGoogle,
		"--endpoint", server.URL+"/v1/translateHtml",
	)
	require.NoError(t, err)
}

func TestTranslateFileMicrosoftProvider(t *testing.T) {
	t.Parallel()

	inputPath := writeTranslateTestFile(t, testSourceText)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, http.MethodPost, request.Method)
		assert.Equal(t, "/translate/translatetext", request.URL.Path)
		assert.Empty(t, request.URL.Query().Get("from"))
		assert.Equal(t, "zh-Hant", request.URL.Query().Get("to"))
		assert.Equal(t, "false", request.URL.Query().Get("isEnterpriseClient"))
		assert.Equal(t, "application/json", request.Header.Get("Content-Type"))
		assert.Empty(t, request.Header.Get("Ocp-Apim-Subscription-Key"))

		var payload []string
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode Microsoft request: %v", err)
		}

		if len(payload) != 1 {
			t.Errorf("Microsoft request item count = %d, want 1", len(payload))
			return
		}

		assert.Equal(t, testSourceText, payload[0])

		writer.Header().Set("Content-Type", "application/json")

		if _, err := writer.Write([]byte(`[{"translations":[{"text":"你好，&#34;世界&#34; &amp; more"}]}]`)); err != nil {
			t.Errorf("write Microsoft response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	out, err := executeCommand(t,
		argTranslateFile, inputPath,
		"--provider", translateProviderMicrosoft,
		"--endpoint", server.URL+"/translate/translatetext",
	)
	require.NoError(t, err)
	assert.Contains(t, out, "你好，\"世界\" & more")
}

func TestTranslateFileMicrosoftEscapesHTML(t *testing.T) {
	t.Parallel()

	inputPath := writeTranslateTestFile(t, `a < b & "c" > d`)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var payload []string
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode Microsoft request: %v", err)
		}

		if len(payload) != 1 {
			t.Errorf("Microsoft request item count = %d, want 1", len(payload))
			return
		}

		assert.Equal(t, html.EscapeString(`a < b & "c" > d`), payload[0])

		writer.Header().Set("Content-Type", "application/json")

		if _, err := writer.Write([]byte(`[{"translations":[{"text":"ok"}]}]`)); err != nil {
			t.Errorf("write Microsoft response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	_, err := executeCommand(t,
		argTranslateFile, inputPath,
		"--provider", translateProviderMicrosoft,
		"--endpoint", server.URL+"/translate/translatetext",
	)
	require.NoError(t, err)
}

func TestTranslateFileParagraphPairs(t *testing.T) {
	t.Parallel()

	inputPath := writeTranslateTestFile(t, "First paragraph.\n\nSecond paragraph.\n\n\nThird paragraph.\n")

	var requests int

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, requestIn *http.Request) {
		requests++

		var payload translateRequest
		if err := json.NewDecoder(requestIn.Body).Decode(&payload); err != nil {
			t.Errorf("decode translation request: %v", err)
		}

		writer.Header().Set("Content-Type", "application/json")

		if _, err := fmt.Fprintf(writer, `{"data":"翻譯 %d"}`, requests); err != nil {
			t.Errorf("write translation response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	out, err := executeCommand(t, argTranslateFile, inputPath, "--endpoint", server.URL)
	require.NoError(t, err)
	assert.Equal(t, 3, requests)
	assert.Equal(t,
		"First paragraph.\n翻譯 1\n\nSecond paragraph.\n翻譯 2\n\nThird paragraph.\n翻譯 3\n\n",
		out,
	)
}

func TestTranslateFileParagraphsKeepInnerLines(t *testing.T) {
	t.Parallel()

	inputPath := writeTranslateTestFile(t, "line one\nline two\n\nnext paragraph")

	var seen []string

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, requestIn *http.Request) {
		var payload translateRequest
		if err := json.NewDecoder(requestIn.Body).Decode(&payload); err != nil {
			t.Errorf("decode translation request: %v", err)
		}

		seen = append(seen, payload.Text)

		writer.Header().Set("Content-Type", "application/json")

		if _, err := writer.Write([]byte(`{"data":"ok"}`)); err != nil {
			t.Errorf("write translation response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	_, err := executeCommand(t, argTranslateFile, inputPath, "--endpoint", server.URL)
	require.NoError(t, err)
	assert.Equal(t, []string{"line one\nline two", "next paragraph"}, seen)
}

func TestTranslateFileEmptySource(t *testing.T) {
	t.Parallel()

	inputPath := writeTranslateTestFile(t, "\n\n  \n")

	_, err := executeCommand(t, argTranslateFile, inputPath, "--endpoint", "http://localhost:1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "source file contains no translatable text")
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
