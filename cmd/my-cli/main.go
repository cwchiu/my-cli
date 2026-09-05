// Command my-cli is a multi-subcommand CLI tool.
//
// main.go is intentionally thin: it only wires up the root command and
// translates a returned error into an exit code. All command definitions
// live in sibling files (root.go, version.go, ...).
package main

import (
	"errors"
	"fmt"
	"os"
)

// Exit codes follow the convention documented in AGENTS.md §3:
// 0 success, 1 general error, 2 usage error.
const (
	exitCodeOK         = 0
	exitCodeGeneralErr = 1
	exitCodeUsageErr   = 2
)

func main() {
	os.Exit(run())
}

func run() int {
	rootCmd := newRootCmd()

	// SilenceUsage/SilenceErrors are set on the root command, so cobra
	// prints nothing itself; we own both the message and the exit code.
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if errors.Is(err, errUsage) {
			return exitCodeUsageErr
		}
		return exitCodeGeneralErr
	}
	return exitCodeOK
}
