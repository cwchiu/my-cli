package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

// defaultIPInfoBaseURL is the ipinfo.io endpoint used when --base-url is not
// given. The legacy /json endpoint works without a token.
const defaultIPInfoBaseURL = "https://ipinfo.io"

// defaultIPLookupTimeout bounds one lookup.
const defaultIPLookupTimeout = 30 * time.Second

// ipInfoErrorBody mirrors the error envelope returned by ipinfo.io on
// failures such as 403 (bad token) or 429 (quota exhausted):
// {"status":403,"error":{"title":"...","message":"..."}}.
type ipInfoErrorBody struct {
	Status int `json:"status"`
	Error  struct {
		Title   string `json:"title"`
		Message string `json:"message"`
	} `json:"error"`
}

// ipLookupConfig holds the resolved settings for one ip-lookup run.
type ipLookupConfig struct {
	baseURL      string
	targetIP     string
	timeout      time.Duration
	outputFormat string
}

// newIPLookupCmd returns the `ip-lookup` subcommand, which fetches external
// IP information from ipinfo.io. Without arguments it reports the calling
// host's own external IP; with one argument it looks up that IP.
func newIPLookupCmd() *cobra.Command {
	var raw ipLookupConfig

	cmd := &cobra.Command{
		Use:   "ip-lookup [ip]",
		Short: "Look up external IP information via ipinfo.io",
		Long: `Look up external IP information via ipinfo.io.

Without arguments, the calling host's own external IP is reported.
With one argument, that IPv4 or IPv6 address is looked up instead.

The ipinfo.io /json endpoint is used, which requires no API token.
Free anonymous access is rate limited per day; exceeding it returns 429.

Output formats:
  table  human-readable key-value summary (default)
  json   the untouched API response, preserving every field`,
		Example: `  my-cli ip-lookup
  my-cli ip-lookup 8.8.8.8
  my-cli ip-lookup 2001:4860:4860::8888 --output json`,
		Args: wrapUsage(cobra.MaximumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Flag/argument validation failures are usage errors (exit 2).
			cfg, err := resolveIPLookupConfig(raw, args)
			if err != nil {
				return fmt.Errorf("%w: %w", errUsage, err)
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), cfg.timeout)
			defer cancel()

			body, err := fetchIPInfo(ctx, cfg.baseURL, cfg.targetIP)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()

			if cfg.outputFormat == outputFormatJSON {
				return renderNexusRawJSON(out, body)
			}

			return renderIPInfoTable(out, body)
		},
	}

	cmd.Flags().StringVar(&raw.baseURL, "base-url", defaultIPInfoBaseURL,
		"ipinfo.io base URL (override for testing)")
	cmd.Flags().DurationVar(&raw.timeout, "timeout", defaultIPLookupTimeout,
		"request timeout")
	cmd.Flags().StringVarP(&raw.outputFormat, "output", "o", outputFormatTable,
		"output format (table|json)")

	_ = cmd.RegisterFlagCompletionFunc("output", func(
		_ *cobra.Command, _ []string, _ string,
	) ([]string, cobra.ShellCompDirective) {
		return []string{outputFormatTable, outputFormatJSON}, cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

// resolveIPLookupConfig validates the raw flag values and the optional
// positional IP argument, failing fast before any network I/O.
func resolveIPLookupConfig(raw ipLookupConfig, args []string) (ipLookupConfig, error) {
	if raw.timeout <= 0 {
		return ipLookupConfig{}, fmt.Errorf("timeout must be positive, got %s", raw.timeout)
	}

	switch raw.outputFormat {
	case outputFormatTable, outputFormatJSON:
	default:
		return ipLookupConfig{}, fmt.Errorf("unsupported output format %q", raw.outputFormat)
	}

	targetIP := ""

	if len(args) == 1 {
		if net.ParseIP(args[0]) == nil {
			return ipLookupConfig{}, fmt.Errorf("invalid IP address %q", args[0])
		}

		targetIP = args[0]
	}

	return ipLookupConfig{
		baseURL:      strings.TrimRight(raw.baseURL, "/"),
		targetIP:     targetIP,
		timeout:      raw.timeout,
		outputFormat: raw.outputFormat,
	}, nil
}

// fetchIPInfo performs the GET against the ipinfo.io /json endpoint and
// returns the raw response body. Non-2xx responses become errors built from
// the structured error envelope when present, falling back to a truncated
// body.
func fetchIPInfo(ctx context.Context, baseURL, targetIP string) (json.RawMessage, error) {
	endpoint := baseURL + "/json"
	if targetIP != "" {
		endpoint = baseURL + "/" + targetIP + "/json"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build request %s: %w", endpoint, err)
	}

	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 0} // deadline comes from ctx.

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request %s: %w", endpoint, err)
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response %s: %w", endpoint, err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, ipInfoHTTPError(endpoint, resp.StatusCode, body)
	}

	return json.RawMessage(body), nil
}

// ipInfoHTTPError builds the error for a non-2xx response, preferring the
// structured error envelope and falling back to a truncated raw body.
func ipInfoHTTPError(endpoint string, status int, body []byte) error {
	detail := truncateDetail(body)

	var envelope ipInfoErrorBody
	if err := json.Unmarshal(body, &envelope); err == nil && envelope.Error.Title != "" {
		if status == http.StatusTooManyRequests {
			return fmt.Errorf("request %s failed: status 429: %s: %s (free anonymous quota exhausted)",
				endpoint, envelope.Error.Title, envelope.Error.Message)
		}

		return fmt.Errorf("request %s failed: status %d: %s: %s",
			endpoint, status, envelope.Error.Title, envelope.Error.Message)
	}

	if status == http.StatusTooManyRequests {
		return fmt.Errorf("request %s failed: status 429: free anonymous quota exhausted: %s",
			endpoint, detail)
	}

	return fmt.Errorf("request %s failed: status %d: %s",
		endpoint, status, detail)
}

// truncateDetail shortens an error body for safe display, mirroring the
// truncation used by the nexus client.
func truncateDetail(body []byte) string {
	detail := strings.TrimSpace(string(body))
	if len(detail) > maxErrorBodyLen {
		return detail[:maxErrorBodyLen] + "..."
	}

	return detail
}

// ipInfoTableOrder is the fixed field order of the table output. Fields the
// API omitted are skipped; readme is never shown.
var ipInfoTableOrder = []string{
	"ip", "hostname", "city", "region", "country",
	"loc", "org", "postal", "timezone",
}

// renderIPInfoTable writes a key-value summary of the API response. Unknown
// fields other than anycast and readme are ignored; anycast is shown only
// when true.
func renderIPInfoTable(out io.Writer, raw json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return fmt.Errorf("parse ipinfo response: %w", err)
	}

	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)

	for _, key := range ipInfoTableOrder {
		value, ok := fields[key]
		if !ok || string(value) == "null" || string(value) == `""` {
			continue
		}

		var text string
		if err := json.Unmarshal(value, &text); err != nil {
			return fmt.Errorf("parse field %s: %w", key, err)
		}

		if _, err := fmt.Fprintf(w, "%s:\t%s\n", key, text); err != nil {
			return fmt.Errorf("print table row: %w", err)
		}
	}

	if anycast, ok := fields["anycast"]; ok && string(anycast) == "true" {
		if _, err := fmt.Fprintf(w, "anycast:\ttrue\n"); err != nil {
			return fmt.Errorf("print table row: %w", err)
		}
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush table: %w", err)
	}

	return nil
}
