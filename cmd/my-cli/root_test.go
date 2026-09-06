package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeConfig writes content to a fresh my-cli.yaml in a temp dir and
// returns its path, for use as the --config value.
func writeConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "my-cli.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

// viperFromContext extracts the viper instance initConfig stores on the
// command context.
func viperFromContext(t *testing.T, cmd *cobra.Command) *viper.Viper {
	t.Helper()

	v, ok := cmd.Context().Value(viperKey{}).(*viper.Viper)
	require.True(t, ok, "initConfig must store *viper.Viper on the command context")

	return v
}

func TestInitConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cfgValue func(t *testing.T) string
		wantErr  string
		check    func(t *testing.T, v *viper.Viper)
	}{
		{
			name: "valid config file is loaded",
			cfgValue: func(t *testing.T) string {
				t.Helper()

				return writeConfig(t, "log:\n  level: info\n")
			},
			check: func(t *testing.T, v *viper.Viper) {
				t.Helper()

				assert.Equal(t, "info", v.GetString("log.level"))
			},
		},
		{
			name: "missing explicit config file is an error",
			cfgValue: func(t *testing.T) string {
				t.Helper()

				return filepath.Join(t.TempDir(), "absent.yaml")
			},
			wantErr: "read config",
		},
		{
			name: "unparseable config file is an error",
			cfgValue: func(t *testing.T) string {
				t.Helper()

				return writeConfig(t, "not: [valid: yaml")
			},
			wantErr: "read config",
		},
		{
			// No --config: viper searches . and $HOME; a missing file
			// there is tolerated (ConfigFileNotFoundError).
			name:     "no config file found is not an error",
			cfgValue: func(*testing.T) string { return "" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd := newRootCmd()
			// cobra.Execute() sets a context before running hooks;
			// initConfig calls context.WithValue(cmd.Context(), ...),
			// so replicate that here for direct invocation.
			cmd.SetContext(t.Context())
			require.NoError(t, cmd.ParseFlags([]string{"--config", tt.cfgValue(t)}))

			err := initConfig(cmd)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)

			if tt.check != nil {
				tt.check(t, viperFromContext(t, cmd))
			}
		})
	}
}

func TestInitConfigEnvOverride(t *testing.T) {
	t.Setenv("MYCLI_LOG_LEVEL", "debug")

	cmd := newRootCmd()
	cmd.SetContext(t.Context())
	require.NoError(t, cmd.ParseFlags(nil))
	require.NoError(t, initConfig(cmd))

	// MYCLI_LOG_LEVEL maps to the key "log.level" via the env prefix and
	// the "." -> "_" key replacer configured in initConfig.
	assert.Equal(t, "debug", viperFromContext(t, cmd).GetString("log.level"))
}
