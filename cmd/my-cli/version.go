package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// Build information, overridable at link time:
//
//	go build -ldflags "-X main.version=v1.2.3 -X main.commit=abc1234 -X main.date=2026-01-02T15:04:05Z" ./cmd/my-cli
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// versionOutput is the machine-readable payload of `my-cli version --output json`.
type versionOutput struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

// newVersionCmd returns the `version` subcommand.
func newVersionCmd() *cobra.Command {
	var outputFormat string

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version information",
		Long: `Print the version, commit, and build date of this binary.

Use --output json for machine-readable output.`,
		Args: wrapUsage(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			info := versionOutput{
				Version: version,
				Commit:  commit,
				Date:    date,
			}

			switch outputFormat {
			case "json":
				data, err := json.MarshalIndent(info, "", "  ")
				if err != nil {
					return fmt.Errorf("marshal version info: %w", err)
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
			case "table":
				out := cmd.OutOrStdout()
				fmt.Fprintf(out, "version: %s\n", info.Version)
				fmt.Fprintf(out, "commit:  %s\n", info.Commit)
				fmt.Fprintf(out, "date:    %s\n", info.Date)
			default:
				// Guarded by MarkFlagCustom below, but keep a defensive
				// branch so the switch stays exhaustive.
				return fmt.Errorf("unsupported output format %q", outputFormat)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(
		&outputFormat, "output", "o", "table",
		"output format (table|json)",
	)
	_ = cmd.RegisterFlagCompletionFunc("output", func(
		cmd *cobra.Command, args []string, toComplete string,
	) ([]string, cobra.ShellCompDirective) {
		return []string{"table", "json"}, cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}
