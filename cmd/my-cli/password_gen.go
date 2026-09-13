package main

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"

	"github.com/spf13/cobra"
)

// Character classes used to build passwords. The symbol set deliberately
// omits characters that are hard to paste into shells (backtick, quotes,
// backslash, pipe, dollar sign).
const (
	passwordDigits  = "0123456789"
	passwordUpper   = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	passwordLower   = "abcdefghijklmnopqrstuvwxyz"
	passwordSymbols = "!@#$%^&*()-_=+[]{};:,.<>?/"
)

// passwordGenLimits bounds the --length flag.
const (
	minPasswordLength = 4
	maxPasswordLength = 256
)

// defaultPasswordLength is the password length used when --length is not
// given (issue #15: default 16).
const defaultPasswordLength = 16

// passwordGenConfig holds the resolved settings for one password-gen run.
type passwordGenConfig struct {
	length   int
	noDigits bool
	noUpper  bool
	noLower  bool
	symbols  bool
	// output holds "table" or "json" (see outputFormat* constants).
	output string
}

// passwordGenOutput is the machine-readable payload of
// `password-gen --output json`.
type passwordGenOutput struct {
	Password string `json:"password"`
	Length   int    `json:"length"`
}

// newPasswordGenCmd returns the `password-gen` subcommand, which generates
// one random password locally from the enabled character classes.
func newPasswordGenCmd() *cobra.Command {
	var raw passwordGenConfig

	cmd := &cobra.Command{
		Use:   "password-gen",
		Short: "Generate one random password locally",
		Long: `Generate one cryptographically random password locally (crypto/rand,
no network access) and print it to stdout.

Character classes are on by default for digits, uppercase, and lowercase;
special symbols are off by default. Every enabled class is guaranteed to
appear at least once, and the result is shuffled so the guaranteed
characters are not predictable by position.

The symbol set omits characters that are awkward to paste into shells
(backtick, quotes, backslash, pipe, dollar sign).

Use --output json for machine-readable output.`,
		Example: `  my-cli password-gen
  my-cli password-gen --length 32 --symbols
  my-cli password-gen --no-digits --no-upper --output json`,
		Args: wrapUsage(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Flag validation failures are usage errors (exit 2).
			cfg, err := resolvePasswordGenConfig(raw)
			if err != nil {
				return fmt.Errorf("%w: %w", errUsage, err)
			}

			password, err := generatePassword(cfg)
			if err != nil {
				return err
			}

			return renderPasswordGen(cmd.OutOrStdout(), cfg, password)
		},
	}

	cmd.Flags().IntVar(&raw.length, "length", defaultPasswordLength,
		"password length (minimum is the number of enabled classes)")
	cmd.Flags().BoolVar(&raw.noDigits, "no-digits", false,
		"exclude digits 0-9")
	cmd.Flags().BoolVar(&raw.noUpper, "no-upper", false,
		"exclude uppercase letters A-Z")
	cmd.Flags().BoolVar(&raw.noLower, "no-lower", false,
		"exclude lowercase letters a-z")
	cmd.Flags().BoolVar(&raw.symbols, "symbols", false,
		"include special symbols "+passwordSymbols)
	cmd.Flags().StringVarP(&raw.output, "output", "o", outputFormatTable,
		"output format (table|json)")

	_ = cmd.RegisterFlagCompletionFunc("output", func(
		_ *cobra.Command, _ []string, _ string,
	) ([]string, cobra.ShellCompDirective) {
		return []string{outputFormatTable, outputFormatJSON}, cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

// resolvePasswordGenConfig validates the raw flag values, failing fast
// before any randomness is consumed.
func resolvePasswordGenConfig(raw passwordGenConfig) (passwordGenConfig, error) {
	if raw.length < minPasswordLength || raw.length > maxPasswordLength {
		return passwordGenConfig{}, fmt.Errorf(
			"length must be between %d and %d, got %d",
			minPasswordLength, maxPasswordLength, raw.length)
	}

	switch raw.output {
	case outputFormatTable, outputFormatJSON:
	default:
		return passwordGenConfig{}, fmt.Errorf("unsupported output format %q", raw.output)
	}

	enabled := enabledPasswordClasses(raw)
	if enabled == 0 {
		return passwordGenConfig{}, errors.New(
			"at least one character class must be enabled: digits, upper, lower, or symbols")
	}

	if raw.length < enabled {
		return passwordGenConfig{}, fmt.Errorf(
			"length %d is below the %d enabled character classes (each must appear at least once)",
			raw.length, enabled)
	}

	return raw, nil
}

// enabledPasswordClasses counts how many character classes are switched on.
func enabledPasswordClasses(raw passwordGenConfig) int {
	count := 0

	if !raw.noDigits {
		count++
	}

	if !raw.noUpper {
		count++
	}

	if !raw.noLower {
		count++
	}

	if raw.symbols {
		count++
	}

	return count
}

// generatePassword samples length characters from the union of the enabled
// classes, then guarantees one character per enabled class and shuffles the
// result so the guaranteed positions are unpredictable.
func generatePassword(cfg passwordGenConfig) (string, error) {
	charset := passwordCharset(cfg)

	letters := make([]byte, 0, cfg.length)
	for range cfg.length {
		ch, err := randomChoice(charset)
		if err != nil {
			return "", err
		}

		letters = append(letters, ch)
	}

	// Overwrite the first k positions with one random character from each
	// enabled class, then shuffle so the guarantee leaves no positional
	// pattern.
	pos := 0

	for _, class := range passwordClasses(cfg) {
		ch, err := randomChoice(class)
		if err != nil {
			return "", err
		}

		letters[pos] = ch
		pos++
	}

	if err := shuffleBytes(letters); err != nil {
		return "", err
	}

	return string(letters), nil
}

// passwordCharset concatenates the enabled classes into the sampling pool.
func passwordCharset(cfg passwordGenConfig) string {
	var builder strings.Builder
	for _, class := range passwordClasses(cfg) {
		builder.WriteString(class)
	}

	return builder.String()
}

// passwordClasses returns the enabled character classes in a fixed order.
func passwordClasses(cfg passwordGenConfig) []string {
	classes := make([]string, 0, 4)

	if !cfg.noDigits {
		classes = append(classes, passwordDigits)
	}

	if !cfg.noUpper {
		classes = append(classes, passwordUpper)
	}

	if !cfg.noLower {
		classes = append(classes, passwordLower)
	}

	if cfg.symbols {
		classes = append(classes, passwordSymbols)
	}

	return classes
}

// randomChoice picks one uniformly random byte from s using crypto/rand
// with rejection sampling (no modulo bias).
func randomChoice(s string) (byte, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(s))))
	if err != nil {
		return 0, fmt.Errorf("read random byte: %w", err)
	}

	return s[n.Int64()], nil
}

// shuffleBytes randomises the order of s in place with a Fisher-Yates
// shuffle driven by crypto/rand.
func shuffleBytes(s []byte) error {
	for i := len(s) - 1; i > 0; i-- {
		j, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return fmt.Errorf("read random index: %w", err)
		}

		s[i], s[j.Int64()] = s[j.Int64()], s[i]
	}

	return nil
}

// renderPasswordGen writes the password in the requested output format.
// The password goes to stdout only; it is never logged.
func renderPasswordGen(out io.Writer, cfg passwordGenConfig, password string) error {
	if cfg.output == outputFormatJSON {
		data, err := jsonMarshalIndent(passwordGenOutput{
			Password: password,
			Length:   cfg.length,
		})
		if err != nil {
			return err
		}

		if _, err := fmt.Fprintln(out, string(data)); err != nil {
			return fmt.Errorf("print password json: %w", err)
		}

		return nil
	}

	if _, err := fmt.Fprintln(out, password); err != nil {
		return fmt.Errorf("print password: %w", err)
	}

	return nil
}
