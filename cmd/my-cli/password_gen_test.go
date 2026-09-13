package main

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// argPasswordGen and the flag literals are extracted for goconst; they are
// reused across the password-gen test tables. argFlagOutput (nexus_export_test.go),
// argOutputXML and wantBadFormat (ip_lookup_test.go) are shared from there.
const (
	argPasswordGen   = "password-gen"
	argFlagLength    = "--length"
	argFlagSymbols   = "--symbols"
	argFlagNoDigits  = "--no-digits"
	argFlagNoUpper   = "--no-upper"
	argFlagNoLower   = "--no-lower"
	argPasswordExtra = "extra"
	wantLengthRange  = "length must be between"
)

// passwordGenRandomRuns is how many passwords the randomness test draws
// before requiring at least two distinct values.
const passwordGenRandomRuns = 5

// passwordContainsClass reports whether pw contains at least one character
// from the given class.
func passwordContainsClass(pw, class string) bool {
	return strings.ContainsAny(pw, class)
}

// passwordUsesOnlyClasses reports whether every character of pw belongs to
// one of the given classes.
func passwordUsesOnlyClasses(pw string, classes ...string) bool {
	for _, ch := range pw {
		found := slices.ContainsFunc(classes, func(class string) bool {
			return strings.ContainsRune(class, ch)
		})
		if !found {
			return false
		}
	}

	return true
}

// checkPasswordGenOutput runs password-gen with args and asserts the printed
// password has the wanted length, uses only the wanted classes, and contains
// every wanted class at least once (issue #15 guarantee).
func checkPasswordGenOutput(t *testing.T, args []string, wantLen int, wantClasses []string) {
	t.Helper()

	out, err := executeCommand(t, args...)
	require.NoError(t, err)

	pw := strings.TrimSpace(out)
	is := assert.New(t)
	is.Len(pw, wantLen)
	is.True(passwordUsesOnlyClasses(pw, wantClasses...),
		"password %q must only use the enabled classes %v", pw, wantClasses)

	for _, class := range wantClasses {
		is.True(passwordContainsClass(pw, class),
			"password %q must contain class %q at least once", pw, class)
	}
}

func TestPasswordGenCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		args        []string
		wantLen     int
		wantClasses []string
	}{
		{
			name:        "default is 16 chars from digits, upper and lower",
			args:        []string{argPasswordGen},
			wantLen:     defaultPasswordLength,
			wantClasses: []string{passwordDigits, passwordUpper, passwordLower},
		},
		{
			name:        "symbols adds the fourth class",
			args:        []string{argPasswordGen, argFlagSymbols},
			wantLen:     defaultPasswordLength,
			wantClasses: []string{passwordDigits, passwordUpper, passwordLower, passwordSymbols},
		},
		{
			name:        "no-digits drops digits only",
			args:        []string{argPasswordGen, argFlagNoDigits},
			wantLen:     defaultPasswordLength,
			wantClasses: []string{passwordUpper, passwordLower},
		},
		{
			name:        "no-upper drops uppercase only",
			args:        []string{argPasswordGen, argFlagNoUpper},
			wantLen:     defaultPasswordLength,
			wantClasses: []string{passwordDigits, passwordLower},
		},
		{
			name:        "no-lower drops lowercase only",
			args:        []string{argPasswordGen, argFlagNoLower},
			wantLen:     defaultPasswordLength,
			wantClasses: []string{passwordDigits, passwordUpper},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			checkPasswordGenOutput(t, tt.args, tt.wantLen, tt.wantClasses)
		})
	}
}

func TestPasswordGenLengths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		args        []string
		wantLen     int
		wantClasses []string
	}{
		{
			name:        "minimum length honours the guarantee",
			args:        []string{argPasswordGen, argFlagLength, "4"},
			wantLen:     4,
			wantClasses: []string{passwordDigits, passwordUpper, passwordLower},
		},
		{
			name:        "minimum length fits all four classes exactly once",
			args:        []string{argPasswordGen, argFlagLength, "4", argFlagSymbols},
			wantLen:     4,
			wantClasses: []string{passwordDigits, passwordUpper, passwordLower, passwordSymbols},
		},
		{
			name:        "longer passwords honour --length",
			args:        []string{argPasswordGen, argFlagLength, "32", argFlagSymbols},
			wantLen:     32,
			wantClasses: []string{passwordDigits, passwordUpper, passwordLower, passwordSymbols},
		},
		{
			name:        "maximum length is accepted",
			args:        []string{argPasswordGen, argFlagLength, "256"},
			wantLen:     256,
			wantClasses: []string{passwordDigits, passwordUpper, passwordLower},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			checkPasswordGenOutput(t, tt.args, tt.wantLen, tt.wantClasses)
		})
	}
}

func TestPasswordGenJSONOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		wantLen int
	}{
		{
			name:    "default json payload",
			args:    []string{argPasswordGen, argFlagOutput, outputFormatJSON},
			wantLen: defaultPasswordLength,
		},
		{
			name:    "json honours custom length and the -o shorthand",
			args:    []string{argPasswordGen, argFlagLength, "32", argFlagSymbols, "-o", outputFormatJSON},
			wantLen: 32,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out, err := executeCommand(t, tt.args...)
			require.NoError(t, err)

			var got passwordGenOutput
			require.NoError(t, json.Unmarshal([]byte(out), &got))

			is := assert.New(t)
			is.Equal(tt.wantLen, got.Length)
			is.Len(got.Password, tt.wantLen)
			is.True(passwordUsesOnlyClasses(got.Password,
				passwordDigits, passwordUpper, passwordLower, passwordSymbols))
		})
	}
}

func TestPasswordGenErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "length below minimum is a usage error",
			args:    []string{argPasswordGen, argFlagLength, "3"},
			wantErr: wantLengthRange,
		},
		{
			name:    "zero length is a usage error",
			args:    []string{argPasswordGen, argFlagLength, "0"},
			wantErr: wantLengthRange,
		},
		{
			name:    "negative length is a usage error",
			args:    []string{argPasswordGen, argFlagLength, "-1"},
			wantErr: wantLengthRange,
		},
		{
			name:    "length above maximum is a usage error",
			args:    []string{argPasswordGen, argFlagLength, "257"},
			wantErr: wantLengthRange,
		},
		{
			name:    "disabling every class is a usage error",
			args:    []string{argPasswordGen, argFlagNoDigits, argFlagNoUpper, argFlagNoLower},
			wantErr: "at least one character class",
		},
		{
			name:    "unsupported output format is a usage error",
			args:    []string{argPasswordGen, argFlagOutput, argOutputXML},
			wantErr: wantBadFormat,
		},
		{
			name:    "positional arguments are rejected",
			args:    []string{argPasswordGen, argPasswordExtra},
			wantErr: "unknown command",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := executeCommand(t, tt.args...)
			require.ErrorContains(t, err, tt.wantErr)
			assert.ErrorIs(t, err, errUsage)
		})
	}
}

func TestPasswordGenRandomness(t *testing.T) {
	t.Parallel()

	seen := make(map[string]struct{}, passwordGenRandomRuns)

	for range passwordGenRandomRuns {
		out, err := executeCommand(t, argPasswordGen)
		require.NoError(t, err)

		seen[strings.TrimSpace(out)] = struct{}{}
	}

	assert.Greater(t, len(seen), 1,
		"repeated runs must produce different passwords")
}
