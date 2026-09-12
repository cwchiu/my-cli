package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/cwchiu/my-cli/internal/nexus"
	"github.com/spf13/cobra"
)

// Environment variables consulted by the nexus-repo-export command.
//
// #nosec G101 -- these are environment variable names, not credentials.
const (
	envNexusPassword = "NEXUS_PASSWORD"
	envNexusUsername = "NEXUS_USERNAME"
	envNexusBaseURL  = "NEXUS_BASE_URL"
)

// defaultNexusTimeout bounds one export run.
const defaultNexusTimeout = 2 * time.Minute

// nexusRepoType enumerates the NXRM3 repository types accepted by --type.
const (
	nexusTypeHosted = "hosted"
	nexusTypeProxy  = "proxy"
	nexusTypeGroup  = "group"
)

// nexusConfig holds the resolved settings for one nexus-repo-export run.
type nexusConfig struct {
	baseURL      string
	username     string
	password     string
	formatFilter string
	typeFilter   string
	timeout      time.Duration
	outputFormat string
	outFile      string
}

// newNexusExportCmd returns the `nexus-repo-export` subcommand, which
// exports repository settings from a Sonatype Nexus Repository 3 server.
func newNexusExportCmd() *cobra.Command {
	var raw nexusConfig

	cmd := &cobra.Command{
		Use:   "nexus-repo-export",
		Short: "Export repository settings from a Sonatype Nexus Repository 3 server",
		Long: `Export repository settings from a Sonatype Nexus Repository 3 server
via the REST v1 API (GET /service/rest/v1/repositories).

Authentication uses HTTP Basic credentials: --username (falling back to
NEXUS_USERNAME) and the NEXUS_PASSWORD environment variable only (never a
flag, so the password cannot leak via shell history or process listings).
Anonymous access is used when no username is configured.

Output formats:
  table  human-readable summary (default)
  json   the untouched API response, preserving every field so the export
         can be used to rebuild the repositories
  csv    a flat 24-column table of the common settings`,
		Example: `  my-cli nexus-repo-export --base-url https://nexus.example.com
  my-cli nexus-repo-export --base-url https://nexus.example.com --output csv -O repos.csv
  my-cli nexus-repo-export --base-url https://nexus.example.com --format docker --type proxy`,
		Args: wrapUsage(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Flag/env validation failures are usage errors (exit code 2).
			cfg, err := resolveNexusConfig(raw)
			if err != nil {
				return fmt.Errorf("%w: %w", errUsage, err)
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), cfg.timeout)
			defer cancel()

			client := nexus.NewClient(cfg.baseURL, cfg.username, cfg.password, nil)

			// JSON output must be lossless (usable to rebuild the
			// repositories), so it bypasses the Repository struct entirely;
			// decoding into a struct would silently drop unknown fields.
			if cfg.outputFormat == outputFormatJSON {
				raw, err := client.ListRepositoriesRaw(ctx)
				if err != nil {
					return err
				}

				out := cmd.OutOrStdout()

				if cfg.outFile != "" {
					f, ferr := os.Create(cfg.outFile)
					if ferr != nil {
						return fmt.Errorf("create output file: %w", ferr)
					}

					defer func() {
						_ = f.Close()
					}()

					out = f
				}

				return renderNexusRawJSON(out, raw)
			}

			repos, err := client.ListRepositories(ctx)
			if err != nil {
				return err
			}

			repos = filterNexusRepositories(repos, cfg.formatFilter, cfg.typeFilter)

			out := cmd.OutOrStdout()

			if cfg.outFile != "" {
				f, ferr := os.Create(cfg.outFile)
				if ferr != nil {
					return fmt.Errorf("create output file: %w", ferr)
				}

				defer func() {
					_ = f.Close()
				}()

				out = f
			}

			return renderNexusRepositories(out, cfg.outputFormat, repos)
		},
	}

	cmd.Flags().StringVar(&raw.baseURL, "base-url", "",
		"Nexus server base URL, required (falls back to NEXUS_BASE_URL)")
	cmd.Flags().StringVar(&raw.username, "username", "",
		"Nexus user for Basic auth (falls back to NEXUS_USERNAME; anonymous when empty)")
	cmd.Flags().StringVar(&raw.formatFilter, "format", "",
		"only export repositories with this format (e.g. maven, npm, docker)")
	cmd.Flags().StringVar(&raw.typeFilter, "type", "",
		"only export repositories with this type (hosted|proxy|group)")
	cmd.Flags().DurationVar(&raw.timeout, "timeout", defaultNexusTimeout,
		"total run timeout")
	cmd.Flags().StringVarP(&raw.outputFormat, "output", "o", outputFormatTable,
		"output format (table|json|csv)")
	cmd.Flags().StringVarP(&raw.outFile, "out", "O", "",
		"write the output to this file instead of stdout")

	// base-url is validated in resolveNexusConfig (not MarkFlagRequired) so
	// the failure is wrapped as errUsage and maps to exit code 2.
	_ = cmd.RegisterFlagCompletionFunc("output", func(
		_ *cobra.Command, _ []string, _ string,
	) ([]string, cobra.ShellCompDirective) {
		return []string{outputFormatTable, outputFormatJSON, outputFormatCSV}, cobra.ShellCompDirectiveNoFileComp
	})

	_ = cmd.RegisterFlagCompletionFunc("type", func(
		_ *cobra.Command, _ []string, _ string,
	) ([]string, cobra.ShellCompDirective) {
		return []string{nexusTypeHosted, nexusTypeProxy, nexusTypeGroup}, cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

// resolveNexusConfig validates the raw flag values and fills in the settings
// derived from the environment, failing fast before any network I/O.
func resolveNexusConfig(raw nexusConfig) (nexusConfig, error) {
	if raw.timeout <= 0 {
		return nexusConfig{}, fmt.Errorf("timeout must be positive, got %s", raw.timeout)
	}

	switch raw.outputFormat {
	case outputFormatTable, outputFormatJSON, outputFormatCSV:
	default:
		return nexusConfig{}, fmt.Errorf("unsupported output format %q", raw.outputFormat)
	}

	switch raw.typeFilter {
	case "", nexusTypeHosted, nexusTypeProxy, nexusTypeGroup:
	default:
		return nexusConfig{}, fmt.Errorf(
			"unsupported repository type %q (want hosted, proxy, or group)", raw.typeFilter)
	}

	baseURL := raw.baseURL
	if baseURL == "" {
		baseURL = os.Getenv(envNexusBaseURL)
	}

	if baseURL == "" {
		return nexusConfig{}, fmt.Errorf(
			"missing Nexus base URL: set --base-url or %s", envNexusBaseURL)
	}

	username := raw.username
	if username == "" {
		username = os.Getenv(envNexusUsername)
	}

	// The password is read from the environment only, never a flag.
	password := os.Getenv(envNexusPassword)

	if username != "" && password == "" {
		return nexusConfig{}, fmt.Errorf(
			"missing Nexus password: set %s (never a flag)", envNexusPassword)
	}

	return nexusConfig{
		baseURL:      baseURL,
		username:     username,
		password:     password,
		formatFilter: raw.formatFilter,
		typeFilter:   raw.typeFilter,
		timeout:      raw.timeout,
		outputFormat: raw.outputFormat,
		outFile:      raw.outFile,
	}, nil
}

// filterNexusRepositories keeps only the repositories matching the optional
// format and type filters (both are client-side; the API has no server-side
// filter for this endpoint).
func filterNexusRepositories(repos []nexus.Repository, formatFilter, typeFilter string) []nexus.Repository {
	if formatFilter == "" && typeFilter == "" {
		return repos
	}

	filtered := make([]nexus.Repository, 0, len(repos))
	for _, repo := range repos {
		if formatFilter != "" && !strings.EqualFold(repo.Format, formatFilter) {
			continue
		}

		if typeFilter != "" && !strings.EqualFold(repo.Type, typeFilter) {
			continue
		}

		filtered = append(filtered, repo)
	}

	return filtered
}

// renderNexusRepositories writes the repositories in the requested format.
// JSON is handled earlier in RunE via the lossless raw path.
func renderNexusRepositories(out io.Writer, format string, repos []nexus.Repository) error {
	switch format {
	case outputFormatTable:
		return renderNexusTable(out, repos)
	case outputFormatCSV:
		return renderNexusCSV(out, repos)
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

// renderNexusRawJSON writes the untouched API response as indented JSON.
// Re-indenting only reformats whitespace; no field is added or dropped.
func renderNexusRawJSON(out io.Writer, raw json.RawMessage) error {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return fmt.Errorf("indent json: %w", err)
	}

	if _, err := fmt.Fprintln(out, buf.String()); err != nil {
		return fmt.Errorf("print json: %w", err)
	}

	return nil
}

// renderNexusTable writes a human-readable summary of the repositories.
func renderNexusTable(out io.Writer, repos []nexus.Repository) error {
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)

	if _, err := fmt.Fprintf(w, "NAME\tFORMAT\tTYPE\tURL\n"); err != nil {
		return fmt.Errorf("print table header: %w", err)
	}

	for _, repo := range repos {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			repo.Name, repo.Format, repo.Type, repo.URL,
		); err != nil {
			return fmt.Errorf("print table row: %w", err)
		}
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush table: %w", err)
	}

	return nil
}

// nexusCSVHeader is the fixed 24-column flat schema of the CSV export. It
// flattens the common attributes sections (storage, httpclient, proxy,
// negativeCache, positiveCache, cleanup); fields a repository does not have
// are written as empty strings.
var nexusCSVHeader = []string{
	"Name",
	"Format",
	"Type",
	"URL",
	"Version",
	"ContainsComponent",
	"Exposed",
	"Online",
	"WritePolicy",
	"StorageBlobStoreName",
	"StorageStrictContentTypeValidation",
	"StorageQuotaType",
	"StorageQuotaLimit",
	"CleanupPolicyNames",
	"HTTPClientBlocked",
	"HTTPClientAutoBlock",
	"HTTPClientRetries",
	"ProxyRemoteURL",
	"ProxyContentMaxAge",
	"ProxyMetadataMaxAge",
	"NegativeCacheEnabled",
	"NegativeCacheTimeToLive",
	"PositiveCacheEnabled",
	"PositiveCacheTimeToLive",
}

// renderNexusCSV writes the flat 24-column CSV export.
func renderNexusCSV(out io.Writer, repos []nexus.Repository) error {
	w := csv.NewWriter(out)

	if err := w.Write(nexusCSVHeader); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}

	for _, repo := range repos {
		if err := w.Write(nexusCSVRow(repo)); err != nil {
			return fmt.Errorf("write csv row: %w", err)
		}
	}

	w.Flush()

	if err := w.Error(); err != nil {
		return fmt.Errorf("flush csv: %w", err)
	}

	return nil
}

// nexusCSVRow flattens one repository into the fixed CSV column order.
func nexusCSVRow(repo nexus.Repository) []string {
	return []string{
		repo.Name,
		repo.Format,
		repo.Type,
		repo.URL,
		repo.Version,
		nexusAttributeValue(repo, "core", "containsComponent"),
		nexusAttributeValue(repo, "core", "exposed"),
		nexusAttributeValue(repo, "storage", "online"),
		nexusAttributeValue(repo, "storage", "writePolicy"),
		nexusAttributeValue(repo, "storage", "blobStoreName"),
		nexusAttributeValue(repo, "storage", "strictContentTypeValidation"),
		nexusAttributeValue(repo, "storage", "quotaType"),
		nexusAttributeValue(repo, "storage", "quotaLimit"),
		nexusAttributeValue(repo, "cleanup", "policyNames"),
		nexusAttributeValue(repo, "httpclient", "blocked"),
		nexusAttributeValue(repo, "httpclient", "autoBlock"),
		nexusAttributeValue(repo, "httpclient", "retries"),
		nexusAttributeValue(repo, "proxy", "remoteUrl"),
		nexusAttributeValue(repo, "proxy", "contentMaxAge"),
		nexusAttributeValue(repo, "proxy", "metadataMaxAge"),
		nexusAttributeValue(repo, "negativeCache", "enabled"),
		nexusAttributeValue(repo, "negativeCache", "timeToLive"),
		nexusAttributeValue(repo, "positiveCache", "enabled"),
		nexusAttributeValue(repo, "positiveCache", "timeToLive"),
	}
}

// nexusAttributeValue renders one attribute field for CSV output. Missing
// fields become empty strings; numbers render without a trailing ".0".
func nexusAttributeValue(repo nexus.Repository, section, key string) string {
	fields, ok := repo.Attributes[section]
	if !ok {
		return ""
	}

	v, ok := fields[key]
	if !ok || v == nil {
		return ""
	}

	switch val := v.(type) {
	case string:
		return val
	case bool:
		return strconv.FormatBool(val)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case []any:
		parts := make([]string, 0, len(val))
		for _, item := range val {
			parts = append(parts, fmt.Sprintf("%v", item))
		}

		return strings.Join(parts, ";")
	default:
		return fmt.Sprintf("%v", val)
	}
}
