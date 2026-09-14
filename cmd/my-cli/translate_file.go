package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"
)

const (
	defaultTranslateEndpoint = "http://localhost:1188/translate"
	defaultTranslateTimeout  = 30 * time.Second
	maxTranslateFileSize     = 10 << 20
	maxTranslateResponseSize = 10 << 20
	outputFormatErrorPrefix  = "unsupported output" + " format"
)

// translateConfig holds the command settings resolved from flags.
type translateConfig struct {
	endpoint     string
	timeout      time.Duration
	outputFormat string
}

// translateRequest is the JSON payload accepted by DeepLX-compatible APIs.
type translateRequest struct {
	Text       string `json:"text"`
	SourceLang string `json:"source_lang"`
	TargetLang string `json:"target_lang"`
}

// translateResponse is the successful response from a DeepLX-compatible API.
type translateResponse struct {
	Data string `json:"data"`
}

// translateResult is the bilingual output of one translation request.
type translateResult struct {
	Source      string `json:"source"`
	Translation string `json:"translation"`
}

// newTranslateFileCmd returns the `translate-file` subcommand, which sends a
// UTF-8 text file to a DeepLX-compatible endpoint and prints the source with
// its Traditional Chinese translation.
func newTranslateFileCmd() *cobra.Command {
	var raw translateConfig

	cmd := &cobra.Command{
		Use:   "translate-file FILE",
		Short: "Translate a text file into Traditional Chinese",
		Long: `Read a UTF-8 plain-text file, translate it with a DeepLX-compatible API,
and print the source and Traditional Chinese translation together.

DeepLX must be running separately. Use --endpoint to select its translation
endpoint; the default is http://localhost:1188/translate.

Use --output json for machine-readable bilingual output.`,
		Example: `  my-cli translate-file article.txt
  my-cli translate-file article.txt --endpoint http://localhost:1188/translate
  my-cli translate-file article.txt --output json`,
		Args: wrapUsage(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := resolveTranslateConfig(raw)
			if err != nil {
				return err
			}

			result, err := translateFile(cmd.Context(), args[0], cfg)
			if err != nil {
				return err
			}

			return renderTranslateResult(cmd.OutOrStdout(), cfg.outputFormat, result)
		},
	}

	cmd.Flags().StringVar(&raw.endpoint, "endpoint", defaultTranslateEndpoint,
		"DeepLX-compatible translation endpoint")
	cmd.Flags().DurationVar(&raw.timeout, "timeout", defaultTranslateTimeout, "request timeout")
	cmd.Flags().StringVarP(&raw.outputFormat, "output", "o", outputFormatTable,
		"output format (table|json)")
	_ = cmd.RegisterFlagCompletionFunc("output", func(
		_ *cobra.Command, _ []string, _ string,
	) ([]string, cobra.ShellCompDirective) {
		return []string{outputFormatTable, outputFormatJSON}, cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

// resolveTranslateConfig validates the request configuration before file or
// network I/O begins.
func resolveTranslateConfig(raw translateConfig) (translateConfig, error) {
	if raw.timeout <= 0 {
		return translateConfig{}, fmt.Errorf("timeout must be positive, got %s", raw.timeout)
	}

	endpoint, err := url.ParseRequestURI(raw.endpoint)
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" {
		return translateConfig{}, fmt.Errorf("invalid translation endpoint %q", raw.endpoint)
	}

	if endpoint.Scheme != "http" && endpoint.Scheme != "https" {
		return translateConfig{}, errors.New("translation endpoint must use http or https")
	}

	switch raw.outputFormat {
	case outputFormatTable, outputFormatJSON:
	default:
		return translateConfig{}, fmt.Errorf("%s %q", outputFormatErrorPrefix, raw.outputFormat)
	}

	return translateConfig{
		endpoint:     strings.TrimRight(raw.endpoint, "/"),
		timeout:      raw.timeout,
		outputFormat: raw.outputFormat,
	}, nil
}

// translateFile reads one bounded text file and sends it to the configured
// DeepLX-compatible endpoint.
func translateFile(ctx context.Context, filename string, cfg translateConfig) (*translateResult, error) {
	source, err := readTranslateFile(filename)
	if err != nil {
		return nil, err
	}

	return requestTranslation(ctx, source, cfg)
}

// readTranslateFile limits input size before reading to prevent an accidental
// large file from exhausting memory or exceeding typical translation limits.
func readTranslateFile(filename string) (string, error) {
	// #nosec G304 -- the user explicitly selects the source file supplied to this CLI command.
	file, err := os.Open(filename)
	if err != nil {
		return "", fmt.Errorf("open source file: %w", err)
	}

	limited := io.LimitReader(file, maxTranslateFileSize+1)
	contents, err := io.ReadAll(limited)
	closeErr := file.Close()

	if err != nil {
		return "", fmt.Errorf("read source file: %w", err)
	}

	if closeErr != nil {
		return "", fmt.Errorf("close source file: %w", closeErr)
	}

	if len(contents) > maxTranslateFileSize {
		return "", fmt.Errorf("source file exceeds %d-byte limit", maxTranslateFileSize)
	}

	if !utf8.Valid(contents) {
		return "", errors.New("source file is not valid UTF-8")
	}

	return string(contents), nil
}

// requestTranslation posts the source text to a DeepLX-compatible endpoint.
func requestTranslation(ctx context.Context, source string, cfg translateConfig) (*translateResult, error) {
	payload, err := json.Marshal(translateRequest{
		Text:       source,
		SourceLang: "auto",
		TargetLang: "ZH",
	})
	if err != nil {
		return nil, fmt.Errorf("marshal translation request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build translation request: %w", err)
	}

	request.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: cfg.timeout}

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("send translation request: %w", err)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxTranslateResponseSize+1))
	closeErr := response.Body.Close()

	if err != nil {
		return nil, fmt.Errorf("read translation response: %w", err)
	}

	if closeErr != nil {
		return nil, fmt.Errorf("close translation response: %w", closeErr)
	}

	if len(body) > maxTranslateResponseSize {
		return nil, fmt.Errorf("translation response exceeds %d-byte limit", maxTranslateResponseSize)
	}

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("translation request failed: status %d", response.StatusCode)
	}

	var parsed translateResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse translation response: %w", err)
	}

	if parsed.Data == "" {
		return nil, errors.New("translation response contains no data")
	}

	return &translateResult{Source: source, Translation: parsed.Data}, nil
}

// renderTranslateResult writes a bilingual result in the requested format.
func renderTranslateResult(out io.Writer, format string, result *translateResult) error {
	switch format {
	case outputFormatJSON:
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal translation result: %w", err)
		}

		if _, err := fmt.Fprintln(out, string(data)); err != nil {
			return fmt.Errorf("print translation result: %w", err)
		}
	case outputFormatTable:
		if _, err := fmt.Fprintf(out, "Source:\n%s\n\nChinese:\n%s\n", result.Source, result.Translation); err != nil {
			return fmt.Errorf("print translation result: %w", err)
		}
	default:
		return fmt.Errorf("%s %q", outputFormatErrorPrefix, format)
	}

	return nil
}
