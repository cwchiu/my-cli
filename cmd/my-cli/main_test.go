package main

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// run() is the real entry point: it builds its own command tree, reads
// os.Args, and writes to the process stdout/stderr. These tests therefore
// swap os.Args and redirect both streams via os.Pipe — process-global
// mutations that forbid t.Parallel.

// captureStdio runs fn with os.Stdout and os.Stderr redirected to pipes and
// returns the exit code plus everything the function wrote to each stream.
func captureStdio(t *testing.T, fn func() int) (code int, stdout, stderr string) {
	t.Helper()

	origStdout, origStderr := os.Stdout, os.Stderr

	outR, outW, err := os.Pipe()
	require.NoError(t, err)

	errR, errW, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout, os.Stderr = outW, errW
	code = fn()

	// Restore the real streams and close the writers before draining, so
	// ReadAll sees EOF.
	require.NoError(t, outW.Close())
	require.NoError(t, errW.Close())

	os.Stdout, os.Stderr = origStdout, origStderr

	outBytes, err := io.ReadAll(outR)
	require.NoError(t, err)

	errBytes, err := io.ReadAll(errR)
	require.NoError(t, err)

	return code, string(outBytes), string(errBytes)
}

//nolint:paralleltest // TestRunExitCodes swaps process-global os.Args and stdio.
func TestRunExitCodes(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{
			name:       "success exits 0",
			args:       []string{"my-cli", argVersion, "-o", "json"},
			wantCode:   exitCodeOK,
			wantStdout: `"version": "dev"`,
		},
		{
			name:       "usage error exits 2",
			args:       []string{"my-cli", argVersion, "--nope"},
			wantCode:   exitCodeUsageErr,
			wantStderr: "unknown flag",
		},
		{
			name:       "general error exits 1",
			args:       []string{"my-cli", argVersion, "-o", "xml"},
			wantCode:   exitCodeGeneralErr,
			wantStderr: "unsupported output format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origArgs := os.Args
			os.Args = tt.args

			t.Cleanup(func() { os.Args = origArgs })

			code, stdout, stderr := captureStdio(t, run)

			is := assert.New(t)
			is.Equal(tt.wantCode, code)

			if tt.wantStdout != "" {
				is.Contains(stdout, tt.wantStdout)
			}

			if tt.wantStderr != "" {
				is.Contains(stderr, tt.wantStderr)
			}
		})
	}
}
