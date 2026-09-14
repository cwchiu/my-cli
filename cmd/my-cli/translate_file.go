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
	defaultTranslateTimeout    = 30 * time.Second
	maxTranslateFileSize       = 10 << 20
	maxTranslateResponseSize   = 10 << 20
	outputFormatErrorPrefix    = "unsupported output" + " format"
	translateProviderDeepLX    = "deeplx"
	translateProviderGoogle    = "google"
	translateProviderMicrosoft = "microsoft"
	defaultDeepLXEndpoint      = "http://localhost:1188/translate"
	defaultGoogleEndpoint      = "https://translate.googleapis.com/translate_a/single"
	defaultMicrosoftEndpoint   = "https://api.cognitive.microsofttranslator.com/translate"
	googleMaxChunkRunes        = 4_500
	microsoftMaxChunkRunes     = 45_000
)

// envMicrosoftTranslateKey is the Microsoft Translator subscription-key environment variable.
//
// #nosec G101 -- this is an environment variable name, not a credential.
const envMicrosoftTranslateKey = "MICROSOFT_TRANSLATE_KEY"

// envMicrosoftTranslateRegion is the Microsoft Translator region environment variable.

const envMicrosoftTranslateRegion = "MICROSOFT_TRANSLATE_REGION"

// translateConfig holds the command settings resolved from flags.
type translateConfig struct {
	endpoint        string
	provider        string
	microsoftKey    string
	microsoftRegion string
	timeout         time.Duration
	outputFormat    string
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

Use --provider to choose deeplx, google, or microsoft. DeepLX must be running
separately. Microsoft requires the MICROSOFT_TRANSLATE_KEY and
MICROSOFT_TRANSLATE_REGION environment variables.

Use --output json for machine-readable bilingual output.`,
		Example: `  my-cli translate-file article.txt
	  my-cli translate-file article.txt --provider google
	  MICROSOFT_TRANSLATE_KEY=... MICROSOFT_TRANSLATE_REGION=westus my-cli translate-file article.txt --provider microsoft
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

	cmd.Flags().StringVar(&raw.provider, "provider", translateProviderDeepLX,
		"translation provider (deeplx|google|microsoft)")
	cmd.Flags().StringVar(&raw.endpoint, "endpoint", "", "override the selected provider endpoint")
	cmd.Flags().DurationVar(&raw.timeout, "timeout", defaultTranslateTimeout, "request timeout")
	cmd.Flags().StringVarP(&raw.outputFormat, "output", "o", outputFormatTable,
		"output format (table|json)")
	_ = cmd.RegisterFlagCompletionFunc("output", func(
		_ *cobra.Command, _ []string, _ string,
	) ([]string, cobra.ShellCompDirective) {
		return []string{outputFormatTable, outputFormatJSON}, cobra.ShellCompDirectiveNoFileComp
	})
	_ = cmd.RegisterFlagCompletionFunc("provider", func(
		_ *cobra.Command, _ []string, _ string,
	) ([]string, cobra.ShellCompDirective) {
		return []string{translateProviderDeepLX, translateProviderGoogle, translateProviderMicrosoft},
			cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

// resolveTranslateConfig validates the request configuration before file or
// network I/O begins.
func resolveTranslateConfig(raw translateConfig) (translateConfig, error) {
	if raw.timeout <= 0 {
		return translateConfig{}, fmt.Errorf("timeout must be positive, got %s", raw.timeout)
	}

	endpointValue, err := defaultProviderEndpoint(raw.provider, raw.endpoint)
	if err != nil {
		return translateConfig{}, err
	}

	endpoint, err := url.ParseRequestURI(endpointValue)
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" {
		return translateConfig{}, fmt.Errorf("invalid translation endpoint %q", endpointValue)
	}

	if endpoint.Scheme != "http" && endpoint.Scheme != "https" {
		return translateConfig{}, errors.New("translation endpoint must use http or https")
	}

	switch raw.outputFormat {
	case outputFormatTable, outputFormatJSON:
	default:
		return translateConfig{}, fmt.Errorf("%s %q", outputFormatErrorPrefix, raw.outputFormat)
	}

	microsoftKey := os.Getenv(envMicrosoftTranslateKey)
	microsoftRegion := os.Getenv(envMicrosoftTranslateRegion)

	if raw.provider == translateProviderMicrosoft && (microsoftKey == "" || microsoftRegion == "") {
		return translateConfig{}, errors.New("microsoft provider requires MICROSOFT_TRANSLATE_KEY and MICROSOFT_TRANSLATE_REGION")
	}

	return translateConfig{
		endpoint:        strings.TrimRight(endpointValue, "/"),
		provider:        raw.provider,
		microsoftKey:    microsoftKey,
		microsoftRegion: microsoftRegion,
		timeout:         raw.timeout,
		outputFormat:    raw.outputFormat,
	}, nil
}

// defaultProviderEndpoint selects a provider's public endpoint unless --endpoint overrides it.
func defaultProviderEndpoint(provider, endpoint string) (string, error) {
	if endpoint != "" {
		return endpoint, nil
	}

	switch provider {
	case translateProviderDeepLX:
		return defaultDeepLXEndpoint, nil
	case translateProviderGoogle:
		return defaultGoogleEndpoint, nil
	case translateProviderMicrosoft:
		return defaultMicrosoftEndpoint, nil
	default:
		return "", fmt.Errorf("unsupported translation provider %q", provider)
	}
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

// requestTranslation sends source text through the selected translation provider.
func requestTranslation(ctx context.Context, source string, cfg translateConfig) (*translateResult, error) {
	var (
		translation string
		err         error
	)

	switch cfg.provider {
	case translateProviderDeepLX:
		translation, err = translateDeepLX(ctx, source, cfg)
	case translateProviderGoogle:
		translation, err = translateGoogle(ctx, source, cfg)
	case translateProviderMicrosoft:
		translation, err = translateMicrosoft(ctx, source, cfg)
	default:
		return nil, fmt.Errorf("unsupported translation provider %q", cfg.provider)
	}

	if err != nil {
		return nil, err
	}

	return &translateResult{Source: source, Translation: translation}, nil
}

// translateDeepLX posts source text to a DeepLX-compatible endpoint.
func translateDeepLX(ctx context.Context, source string, cfg translateConfig) (string, error) {
	payload, err := json.Marshal(translateRequest{
		Text:       source,
		SourceLang: "auto",
		TargetLang: "ZH",
	})
	if err != nil {
		return "", fmt.Errorf("marshal translation request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("build translation request: %w", err)
	}

	request.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: cfg.timeout}

	body, err := sendTranslationRequest(client, request)
	if err != nil {
		return "", err
	}

	var parsed translateResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("parse translation response: %w", err)
	}

	if parsed.Data == "" {
		return "", errors.New("translation response contains no data")
	}

	return parsed.Data, nil
}

// translateGoogle calls the Google endpoint used by the referenced read-frog implementation.
func translateGoogle(ctx context.Context, source string, cfg translateConfig) (string, error) {
	return translateChunks(source, googleMaxChunkRunes, func(chunk string) (string, error) {
		endpoint, err := url.Parse(cfg.endpoint)
		if err != nil {
			return "", fmt.Errorf("parse Google endpoint: %w", err)
		}

		query := endpoint.Query()
		query.Set("client", "gtx")
		query.Set("sl", "auto")
		query.Set("tl", "zh-TW")
		query.Set("dt", "t")
		query.Set("dj", "1")
		query.Set("ie", "UTF-8")
		query.Set("oe", "UTF-8")
		query.Set("q", chunk)

		endpoint.RawQuery = query.Encode()

		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			return "", fmt.Errorf("build Google translation request: %w", err)
		}

		request.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")

		body, err := sendTranslationRequest(&http.Client{Timeout: cfg.timeout}, request)
		if err != nil {
			return "", err
		}

		var response struct {
			Sentences []struct {
				Translation string `json:"trans"`
			} `json:"sentences"`
		}

		if err := json.Unmarshal(body, &response); err != nil {
			return "", fmt.Errorf("parse Google translation response: %w", err)
		}

		var builder strings.Builder
		for _, sentence := range response.Sentences {
			builder.WriteString(sentence.Translation)
		}

		if builder.Len() == 0 {
			return "", errors.New("google translation response contains no data")
		}

		return builder.String(), nil
	})
}

// translateMicrosoft calls the Azure Translator endpoint used by the referenced read-frog implementation.
func translateMicrosoft(ctx context.Context, source string, cfg translateConfig) (string, error) {
	return translateChunks(source, microsoftMaxChunkRunes, func(chunk string) (string, error) {
		endpoint, err := url.Parse(cfg.endpoint)
		if err != nil {
			return "", fmt.Errorf("parse Microsoft endpoint: %w", err)
		}

		query := endpoint.Query()
		query.Set("api-version", "3.0")
		query.Set("from", "auto-detect")
		query.Set("to", "zh-Hant")

		endpoint.RawQuery = query.Encode()

		payload, err := json.Marshal([]struct {
			Text string `json:"Text"`
		}{{Text: chunk}})
		if err != nil {
			return "", fmt.Errorf("marshal Microsoft translation request: %w", err)
		}

		request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
		if err != nil {
			return "", fmt.Errorf("build Microsoft translation request: %w", err)
		}

		request.Header.Set("Content-Type", "application/json; charset=UTF-8")
		request.Header.Set("Ocp-Apim-Subscription-Key", cfg.microsoftKey)
		request.Header.Set("Ocp-Apim-Subscription-Region", cfg.microsoftRegion)

		body, err := sendTranslationRequest(&http.Client{Timeout: cfg.timeout}, request)
		if err != nil {
			return "", err
		}

		var response []struct {
			Translations []struct {
				Text string `json:"text"`
			} `json:"translations"`
		}

		if err := json.Unmarshal(body, &response); err != nil {
			return "", fmt.Errorf("parse Microsoft translation response: %w", err)
		}

		if len(response) == 0 || len(response[0].Translations) == 0 || response[0].Translations[0].Text == "" {
			return "", errors.New("microsoft translation response contains no data")
		}

		return response[0].Translations[0].Text, nil
	})
}

// sendTranslationRequest executes one bounded translation request and validates its HTTP status.
func sendTranslationRequest(client *http.Client, request *http.Request) ([]byte, error) {
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

	return body, nil
}

// translateChunks keeps individual provider requests within conservative character limits.
func translateChunks(source string, maxRunes int, translate func(string) (string, error)) (string, error) {
	runes := []rune(source)

	var builder strings.Builder

	for start := 0; start < len(runes); start += maxRunes {
		end := min(start+maxRunes, len(runes))
		if err := appendTranslatedChunk(&builder, translate, string(runes[start:end])); err != nil {
			return "", err
		}
	}

	return builder.String(), nil
}

// appendTranslatedChunk translates source then appends it to the completed translation.
func appendTranslatedChunk(builder *strings.Builder, translate func(string) (string, error), source string) error {
	translation, err := translate(source)
	if err != nil {
		return err
	}

	builder.WriteString(translation)

	return nil
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
