package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// executeCommand builds a fresh command tree (cobra accumulates flag state
// on a shared tree) and runs it with the given arguments.
func executeCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()

	rootCmd := newRootCmd()
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs(args)

	err := rootCmd.Execute()

	return buf.String(), err
}

// argVersion is the subcommand name used across tests; goconst flags the
// repeated literal otherwise.
const argVersion = "version"

func TestVersionCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     []string
		wantErr  bool
		checkOut func(t *testing.T, out string)
	}{
		{
			name: "table output by default",
			args: []string{argVersion},
			checkOut: func(t *testing.T, out string) {
				t.Helper()

				is := assert.New(t)
				is.Contains(out, "version: dev")
				is.Contains(out, "commit:  none")
				is.Contains(out, "date:    unknown")
			},
		},
		{
			name: "json output",
			args: []string{argVersion, "--output", outputFormatJSON},
			checkOut: func(t *testing.T, out string) {
				t.Helper()

				var info versionOutput
				require.NoError(t, json.Unmarshal([]byte(out), &info))
				assert.Equal(t, "dev", info.Version)
				assert.Equal(t, "none", info.Commit)
				assert.Equal(t, "unknown", info.Date)
			},
		},
		{
			name: "json output via shorthand",
			args: []string{argVersion, "-o", outputFormatJSON},
			checkOut: func(t *testing.T, out string) {
				t.Helper()

				assert.True(t, strings.HasPrefix(strings.TrimSpace(out), "{"))
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

func TestVersionCommandUsageErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		wantMsg string
	}{
		{
			name:    "unexpected positional argument",
			args:    []string{argVersion, "extra"},
			wantMsg: "unknown command",
		},
		{
			name:    "unknown flag",
			args:    []string{argVersion, "--nope"},
			wantMsg: "unknown flag",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := executeCommand(t, tt.args...)
			require.Error(t, err)
			require.ErrorIs(t, err, errUsage)
			assert.Contains(t, err.Error(), tt.wantMsg)
		})
	}
}

func TestRootCommandHelp(t *testing.T) {
	t.Parallel()

	out, err := executeCommand(t)
	require.NoError(t, err)
	assert.Contains(t, out, "Available Commands:")
	assert.Contains(t, out, argVersion)
}
