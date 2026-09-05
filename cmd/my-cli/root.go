package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// errUsage is re-declared here so validators can wrap it; main.go owns the
// canonical definition. See main.go for the exit-code mapping.
//
//nolint:unused // referenced by subcommand validators in later increments
var errUsage = errors.New("usage error")

// newRootCmd builds the root command with all persistent flags and
// subcommands attached. A fresh tree is created on every call, which keeps
// tests isolated (cobra accumulates flag state on a shared tree).
func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "my-cli",
		Short: "A multi-subcommand CLI tool",
		Long: `my-cli is a multi-subcommand CLI tool.

Global flags and configuration are shared by all subcommands.
Run "my-cli <command> --help" for details on a specific command.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		// The root command itself takes no arguments; subcommands are
		// invoked explicitly. Unknown subcommands fall through to help.
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	// Flag-parse failures anywhere in the tree are usage errors.
	rootCmd.SetFlagErrorFunc(markFlagErrors)

	rootCmd.PersistentFlags().StringP(
		"config", "c", "",
		"config file (default: ./my-cli.yaml then $HOME/.my-cli.yaml)",
	)

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		return initConfig(cmd)
	}

	rootCmd.AddCommand(newVersionCmd())

	return rootCmd
}

// initConfig sets up viper with the documented precedence:
// Set > flag > env > file > default.
//
// The config file is optional: a missing file is not an error, but a file
// that exists and fails to parse is.
func initConfig(cmd *cobra.Command) error {
	v := viper.New()

	v.SetEnvPrefix("MYCLI")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	cfgFile, err := cmd.Flags().GetString("config")
	if err != nil {
		return fmt.Errorf("read --config flag: %w", err)
	}

	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.SetConfigName("my-cli")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home directory: %w", err)
		}
		v.AddConfigPath(home)
	}

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return fmt.Errorf("read config: %w", err)
		}
		// No config file found — that is fine, defaults and env still apply.
	}

	cmd.SetContext(context.WithValue(cmd.Context(), viperKey{}, v))
	return nil
}

// viperKey is an unexported type used as the context key for the viper
// instance, per AGENTS.md §4 (unexported key types for context values).
type viperKey struct{}
