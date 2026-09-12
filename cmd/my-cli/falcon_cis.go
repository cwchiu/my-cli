package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cwchiu/my-cli/internal/falconcis"
	"github.com/spf13/cobra"
)

// Environment variables consulted by the falcon-cis-export command.
//
// #nosec G101 -- these are environment variable names, not credentials.
const (
	envFalconClientSecret = "FALCON_CLIENT_SECRET"
	envFalconClientID     = "FALCON_CLIENT_ID"
	envFalconBaseURL      = "FALCON_BASE_URL"
	outputFormatCSV       = "csv"
)

// defaultFalconBaseURL is the Falcon cloud used when neither --base-url nor
// FALCON_BASE_URL is set.
const defaultFalconBaseURL = "https://api.crowdstrike.com"

// defaultFalconTimeout bounds the whole probe run.
const defaultFalconTimeout = 10 * time.Minute

// falconCisConfig holds the resolved settings for one falcon-cis-export run.
type falconCisConfig struct {
	baseURL         string
	clientID        string
	clientSecret    string
	frameworkFilter string
	timeout         time.Duration
	outputFormat    string
	outFile         string
	probe           bool
}

// newFalconCisExportCmd returns the `falcon-cis-export` subcommand, which
// talks to the CrowdStrike Falcon Kubernetes Container Compliance API.
// This work item implements --probe only; full CSV export follows.
func newFalconCisExportCmd() *cobra.Command {
	var raw falconCisConfig

	cmd := &cobra.Command{
		Use:   "falcon-cis-export",
		Short: "Export K8s CIS violations from the Falcon container compliance API",
		Long: `Export Kubernetes CIS benchmark violations from the CrowdStrike
Falcon container compliance API.

The API client credentials are taken from --client-id, falling back to the
FALCON_CLIENT_ID environment variable; the secret is read from the
FALCON_CLIENT_SECRET environment variable only (never a flag, so it cannot
leak via shell history or process listings).

Use --probe to validate credentials and API access with minimal traffic:
it lists assessed frameworks, fetches the first page of failed rules, and
retrieves metadata for up to five rules.`,
		Example: `  my-cli falcon-cis-export --probe
  my-cli falcon-cis-export --probe --framework CIS --output json
  FALCON_BASE_URL=https://api.us-2.crowdstrike.com my-cli falcon-cis-export --probe`,
		Args: wrapUsage(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := resolveFalconCisConfig(raw)
			if err != nil {
				return err
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), cfg.timeout)
			defer cancel()

			client := falconcis.NewClient(cfg.baseURL, cfg.clientID, cfg.clientSecret, nil)

			result, err := client.Probe(ctx, cfg.frameworkFilter)
			if err != nil {
				if cfg.outputFormat == outputFormatCSV {
					return fmt.Errorf("falcon API rejected the credentials or scope: %w", err)
				}

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

			return renderFalconProbe(out, cfg.outputFormat, result)
		},
	}

	cmd.Flags().StringVar(&raw.baseURL, "base-url", "",
		"Falcon cloud base URL (falls back to FALCON_BASE_URL, default "+defaultFalconBaseURL+")")
	cmd.Flags().StringVar(&raw.clientID, "client-id", "",
		"Falcon API client ID (falls back to FALCON_CLIENT_ID)")
	cmd.Flags().StringVar(&raw.frameworkFilter, "framework", "",
		"only consider frameworks whose name contains this text (e.g. CIS)")
	cmd.Flags().DurationVar(&raw.timeout, "timeout", defaultFalconTimeout,
		"total run timeout")
	cmd.Flags().StringVarP(&raw.outputFormat, "output", "o", outputFormatTable,
		"output format (table|json|csv)")
	cmd.Flags().StringVarP(&raw.outFile, "out", "O", "",
		"write the output to this file instead of stdout")
	cmd.Flags().BoolVar(&raw.probe, "probe", false,
		"validate credentials and API access with minimal traffic")

	_ = cmd.RegisterFlagCompletionFunc("output", func(
		_ *cobra.Command, _ []string, _ string,
	) ([]string, cobra.ShellCompDirective) {
		return []string{outputFormatTable, outputFormatJSON, outputFormatCSV}, cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

// resolveFalconCisConfig validates the raw flag values and fills in the
// settings derived from the environment, failing fast before any network
// I/O.
func resolveFalconCisConfig(raw falconCisConfig) (falconCisConfig, error) {
	if raw.timeout <= 0 {
		return falconCisConfig{}, fmt.Errorf("timeout must be positive, got %s", raw.timeout)
	}

	switch raw.outputFormat {
	case outputFormatTable, outputFormatJSON, outputFormatCSV:
	default:
		return falconCisConfig{}, fmt.Errorf("unsupported output format %q", raw.outputFormat)
	}

	clientID := raw.clientID
	if clientID == "" {
		clientID = os.Getenv(envFalconClientID)
	}

	clientSecret := os.Getenv(envFalconClientSecret)

	baseURL := raw.baseURL
	if baseURL == "" {
		baseURL = os.Getenv(envFalconBaseURL)
	}

	if baseURL == "" {
		baseURL = defaultFalconBaseURL
	}

	if clientID == "" {
		return falconCisConfig{}, fmt.Errorf(
			"missing Falcon API client ID: set --client-id or %s", envFalconClientID)
	}

	if clientSecret == "" {
		return falconCisConfig{}, fmt.Errorf(
			"missing Falcon API client secret: set %s", envFalconClientSecret)
	}

	return falconCisConfig{
		baseURL:         strings.TrimRight(baseURL, "/"),
		clientID:        clientID,
		clientSecret:    clientSecret,
		frameworkFilter: raw.frameworkFilter,
		timeout:         raw.timeout,
		outputFormat:    raw.outputFormat,
		outFile:         raw.outFile,
		probe:           raw.probe,
	}, nil
}

// renderFalconProbe writes the probe result in the requested output format.
func renderFalconProbe(out io.Writer, format string, result *falconcis.ProbeResult) error {
	switch format {
	case outputFormatJSON:
		return renderFalconProbeJSON(out, result)
	case outputFormatTable:
		return renderFalconProbeTable(out, result)
	case outputFormatCSV:
		return renderFalconProbeCSV(out, result)
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

// renderFalconProbeJSON writes the probe result as indented JSON.
func renderFalconProbeJSON(out io.Writer, result *falconcis.ProbeResult) error {
	data, err := jsonMarshalIndent(result)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintln(out, string(data)); err != nil {
		return fmt.Errorf("print probe result: %w", err)
	}

	return nil
}

// renderFalconProbeCSV writes a flat CSV matching the Web export format of ComplianceByRules.
func renderFalconProbeCSV(out io.Writer, result *falconcis.ProbeResult) error {
	w := csv.NewWriter(out)

	header := []string{
		"ID",
		"Framework Name Version",
		"Framework Name",
		"Framework Version",
		"Name",
		"Recommendation ID",
		"Severity",
		"Asset Type",
		"Passed Assets Count",
		"Failed Assets Count",
		"Total Assets Count",
		"Percentage of Passed Assets",
	}

	if err := w.Write(header); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}

	for _, rule := range result.Rules {
		row := []string{
			rule.ID,
			rule.FrameworkNameVersion,
			rule.FrameworkName,
			rule.FrameworkVersion,
			rule.Name,
			rule.RecommendationID,
			strconv.Itoa(rule.Severity),
			rule.AssetType,
			strconv.Itoa(rule.PassedAssessmentCount),
			strconv.Itoa(rule.FailedAssessmentCount),
			strconv.Itoa(rule.TotalAssessmentCount),
			strconv.FormatFloat(rule.PercentageOfPassedAssessments, 'f', -1, 64),
		}

		if err := w.Write(row); err != nil {
			return fmt.Errorf("write csv row: %w", err)
		}
	}

	w.Flush()

	if err := w.Error(); err != nil {
		return fmt.Errorf("flush csv: %w", err)
	}

	return nil
}

// renderFalconProbeTable writes the probe result as human-readable text.
func renderFalconProbeTable(out io.Writer, result *falconcis.ProbeResult) error {
	if _, err := fmt.Fprintf(out, "frameworks: %d assessed\n", len(result.Frameworks)); err != nil {
		return fmt.Errorf("print framework count: %w", err)
	}

	for _, fw := range result.Frameworks {
		if _, err := fmt.Fprintf(out, "  %-40s %-10s pass=%d fail=%d\n",
			fw.FrameworkName, fw.FrameworkVersion, fw.PassedCount, fw.FailedCount,
		); err != nil {
			return fmt.Errorf("print framework: %w", err)
		}
	}

	if _, err := fmt.Fprintf(out, "rules: %d assessed\n", len(result.Rules)); err != nil {
		return fmt.Errorf("print rule count: %w", err)
	}

	for _, rule := range result.Rules {
		if err := writeProbeRule(out, rule); err != nil {
			return err
		}
	}

	return nil
}

// writeProbeRule writes one compliance rule summary.
func writeProbeRule(out io.Writer, rule falconcis.RuleCompliance) error {
	if _, err := fmt.Fprintf(out, "  %s  [%s]  severity=%d  passed=%d  failed=%d\n",
		rule.ID, rule.RecommendationID, rule.Severity, rule.PassedAssessmentCount, rule.FailedAssessmentCount,
	); err != nil {
		return fmt.Errorf("print rule: %w", err)
	}

	if rule.Name != "" {
		if _, err := fmt.Fprintf(out, "    name: %s\n", firstLine(rule.Name)); err != nil {
			return fmt.Errorf("print name: %w", err)
		}
	}

	return nil
}

// firstLine returns the first non-empty line of a multi-line API text field.
func firstLine(text string) string {
	for line := range strings.SplitSeq(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}

	return ""
}
