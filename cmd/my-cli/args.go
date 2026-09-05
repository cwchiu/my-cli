package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// wrapUsage wraps a cobra positional-args validator so validation failures
// are marked with errUsage. main.go maps errUsage to exit code 2 (usage
// error) instead of 1 (general error), per AGENTS.md §3.
func wrapUsage(validate cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := validate(cmd, args); err != nil {
			return fmt.Errorf("%w: %w", errUsage, err)
		}
		return nil
	}
}

// markFlagErrors wraps flag-parse failures with errUsage so they also map
// to exit code 2. It is installed once on the root command via
// SetFlagErrorFunc and applies to the whole command tree.
func markFlagErrors(_ *cobra.Command, err error) error {
	return fmt.Errorf("%w: %w", errUsage, err)
}
