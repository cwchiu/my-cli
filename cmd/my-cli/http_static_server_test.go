package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// argHTTPStaticServer is the subcommand name used across http-static-server
// tests; goconst flags the repeated literal otherwise.
const (
	argHTTPStaticServer = "http-static-server"
	argFlagFolder       = "--folder"
)

// writeHTTPStaticFixture creates a temporary folder with one file and one
// subfolder, returning the folder path.
func writeHTTPStaticFixture(t *testing.T) string {
	t.Helper()

	folder := t.TempDir()

	require.NoError(t, os.WriteFile(
		filepath.Join(folder, "hello.txt"), []byte("hello world\n"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(folder, "sub"), 0o750))

	return folder
}

// executeHTTPStaticServer runs the command with a cancellable context so a
// live server can be stopped from the test without signals.
func executeHTTPStaticServer(ctx context.Context, args ...string) error {
	rootCmd := newRootCmd()
	rootCmd.SetOut(io.Discard)
	rootCmd.SetErr(io.Discard)
	rootCmd.SetArgs(args)
	rootCmd.SetContext(ctx)

	return rootCmd.Execute()
}

func TestHTTPStaticServerResolveErrors(t *testing.T) {
	t.Parallel()

	folder := writeHTTPStaticFixture(t)
	filePath := filepath.Join(folder, "hello.txt")

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "missing folder flag and env is a usage error",
			args: []string{argHTTPStaticServer},
		},
		{
			name: "nonexistent folder is a usage error",
			args: []string{argHTTPStaticServer, argFlagFolder, filepath.Join(folder, "nope")},
		},
		{
			name: "folder pointing at a file is a usage error",
			args: []string{argHTTPStaticServer, argFlagFolder, filePath},
		},
		{
			name: "port out of range is a usage error",
			args: []string{argHTTPStaticServer, argFlagFolder, folder, "--port", "70000"},
		},
		{
			name: "port not a number is a usage error",
			args: []string{argHTTPStaticServer, argFlagFolder, folder, "--port", "abc"},
		},
		{
			name: "listen not an IP is a usage error",
			args: []string{argHTTPStaticServer, argFlagFolder, folder, "--listen", "localhost"},
		},
		{
			name: "extra positional arg is a usage error",
			args: []string{argHTTPStaticServer, argFlagFolder, folder, "extra"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := executeCommand(t, tt.args...)
			require.Error(t, err)
			assert.ErrorIs(t, err, errUsage)
		})
	}
}

func TestHTTPStaticServerConfigFromEnv(t *testing.T) {
	// t.Setenv cannot be used with t.Parallel.
	folder := writeHTTPStaticFixture(t)

	// The env-provided port must be free before the server starts.
	port := reserveFreePort(t)
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	t.Setenv(envHTTPStaticFolder, folder)
	t.Setenv(envHTTPStaticPort, strconv.Itoa(port))
	t.Setenv(envHTTPStaticListen, "127.0.0.1")

	ctx, cancel := context.WithCancel(t.Context())

	cmdErr := make(chan error, 1)

	go func() {
		cmdErr <- executeHTTPStaticServer(ctx, argHTTPStaticServer)
	}()

	waitForHTTPStaticServer(t, baseURL)

	cancel()
	require.NoError(t, <-cmdErr)
}

func TestHTTPStaticServerServesFiles(t *testing.T) {
	t.Parallel()

	folder := writeHTTPStaticFixture(t)

	// Reserve a free port, then hand it to the server so tests can dial it.
	port := reserveFreePort(t)
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	ctx, cancel := context.WithCancel(t.Context())

	cmdErr := make(chan error, 1)

	go func() {
		cmdErr <- executeHTTPStaticServer(ctx,
			argHTTPStaticServer, argFlagFolder, folder, "--port", strconv.Itoa(port))
	}()

	waitForHTTPStaticServer(t, baseURL)

	// Parallel subtests resume only after this test function returns, so the
	// server must be stopped from Cleanup: it runs after all subtests have
	// completed, whereas cancelling below the subtests would run too early.
	t.Cleanup(func() {
		cancel()
		require.NoError(t, <-cmdErr)
	})

	t.Run("serves file content", func(t *testing.T) {
		t.Parallel()

		resp, err := httpGet(t, baseURL+"/hello.txt")
		require.NoError(t, err)

		defer func() { _ = resp.Body.Close() }()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "hello world\n", string(body))
	})

	t.Run("missing file returns 404", func(t *testing.T) {
		t.Parallel()

		resp, err := httpGet(t, baseURL+"/missing.txt")
		require.NoError(t, err)

		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("directory listing contains entries", func(t *testing.T) {
		t.Parallel()

		resp, err := httpGet(t, baseURL+"/")
		require.NoError(t, err)

		defer func() { _ = resp.Body.Close() }()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, string(body), "hello.txt")
		assert.Contains(t, string(body), "sub/")
	})

	t.Run("path traversal is rejected", func(t *testing.T) {
		t.Parallel()

		resp, err := httpGet(t, baseURL+"/../http_static_server.go")
		require.NoError(t, err)

		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

// reserveFreePort binds a loopback listener on port 0, closes it, and
// returns the port for the server under test to reuse.
func reserveFreePort(t *testing.T) int {
	t.Helper()

	var lc net.ListenConfig

	listener, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok)

	port := addr.Port

	require.NoError(t, listener.Close())

	return port
}

// httpGet issues a GET with the test context, satisfying the noctx linter.
func httpGet(t *testing.T, url string) (*http.Response, error) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)

	return http.DefaultClient.Do(req)
}

// waitForHTTPStaticServer polls the base URL until the server accepts
// connections or the test deadline expires.
func waitForHTTPStaticServer(t *testing.T, baseURL string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		resp, err := httpGet(t, baseURL+"/")
		if err == nil {
			_ = resp.Body.Close()

			return
		}

		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("http server did not start within deadline at %s", baseURL)
}
