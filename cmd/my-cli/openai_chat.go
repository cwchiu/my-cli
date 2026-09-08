package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// testPrompt is the fixed message sent to the model under test.
const testPrompt = "Say 'OK'."

// envAPIKey is the environment variable consulted when --key is not given.
//
// #nosec G101 -- this is an environment variable name, not a credential.
const envAPIKey = "OPENAI_API_KEY"

// Output format values shared by commands that support --output.
const (
	outputFormatTable = "table"
	outputFormatJSON  = "json"
)

// maxErrorBodyLen caps how much of an error response body is echoed back,
// so a misbehaving server cannot flood the terminal.
const maxErrorBodyLen = 512

// chatTestConfig holds the resolved settings for one chat test run.
type chatTestConfig struct {
	baseAPI      string
	model        string
	key          string
	timeout      time.Duration
	outputFormat string
}

// chatMessage is one message in a chat completion request or response.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatRequest is the request body for POST {base-api}/chat/completions.
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

// chatChoice is one completion choice returned by the API.
type chatChoice struct {
	Message chatMessage `json:"message"`
}

// chatUsage reports token accounting for the completion.
type chatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// chatResponse is the success payload of the chat completions endpoint.
type chatResponse struct {
	Choices []chatChoice `json:"choices"`
	Usage   chatUsage    `json:"usage"`
}

// chatTestResult is the outcome of one chat test run, in both output
// formats.
type chatTestResult struct {
	Model     string    `json:"model"`
	Content   string    `json:"content"`
	LatencyMS float64   `json:"latency_ms"`
	Usage     chatUsage `json:"usage"`
}

// newOpenaiChatTestCmd returns the `openai-chat-test` subcommand, which
// sends one fixed prompt to an OpenAI-compatible chat API and reports the
// reply, latency, and token usage.
func newOpenaiChatTestCmd() *cobra.Command {
	var raw chatTestConfig

	cmd := &cobra.Command{
		Use:   "openai-chat-test",
		Short: "Test an OpenAI-compatible chat API with a fixed prompt",
		Long: `Send one fixed prompt ("Say 'OK'.") to an OpenAI-compatible chat API
and print the model's reply, latency, and token usage.

The request goes to {base-api}/chat/completions. The API key is taken
from --key, falling back to the OPENAI_API_KEY environment variable;
when neither is set the request is sent without an Authorization
header, which suits keyless servers such as ollama or vLLM.

Use --output json for machine-readable output.`,
		Example: `  my-cli openai-chat-test --base-api https://api.openai.com/v1 --model gpt-4o-mini
  my-cli openai-chat-test --base-api http://localhost:11434/v1 --model llama3.2 --output json`,
		Args: wrapUsage(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := resolveChatTestConfig(raw)
			if err != nil {
				return err
			}

			result, err := runChatTest(cmd.Context(), cfg)
			if err != nil {
				return err
			}

			return renderChatResult(cmd.OutOrStdout(), cfg.outputFormat, result)
		},
	}

	cmd.Flags().StringVar(&raw.baseAPI, "base-api", "",
		"base URL of the OpenAI-compatible API (e.g. https://api.openai.com/v1)")
	cmd.Flags().StringVar(&raw.model, "model", "",
		"model name to test (e.g. gpt-4o-mini)")
	cmd.Flags().StringVar(&raw.key, "key", "",
		"API key (falls back to the OPENAI_API_KEY environment variable)")
	cmd.Flags().DurationVar(&raw.timeout, "timeout", 30*time.Second,
		"request timeout")
	cmd.Flags().StringVarP(&raw.outputFormat, "output", "o", outputFormatTable,
		"output format (table|json)")

	_ = cmd.MarkFlagRequired("base-api")
	_ = cmd.MarkFlagRequired("model")
	_ = cmd.RegisterFlagCompletionFunc("output", func(
		_ *cobra.Command, _ []string, _ string,
	) ([]string, cobra.ShellCompDirective) {
		return []string{outputFormatTable, outputFormatJSON}, cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

// resolveChatTestConfig validates the raw flag values and fills in derived
// settings: a trimmed base URL, the resolved API key, and an early output
// format check so bad input fails before any network I/O.
func resolveChatTestConfig(raw chatTestConfig) (chatTestConfig, error) {
	if raw.timeout <= 0 {
		return chatTestConfig{}, fmt.Errorf("timeout must be positive, got %s", raw.timeout)
	}

	switch raw.outputFormat {
	case outputFormatTable, outputFormatJSON:
	default:
		return chatTestConfig{}, fmt.Errorf("unsupported output format %q", raw.outputFormat)
	}

	return chatTestConfig{
		baseAPI:      strings.TrimRight(raw.baseAPI, "/"),
		model:        raw.model,
		key:          resolveAPIKey(raw.key),
		timeout:      raw.timeout,
		outputFormat: raw.outputFormat,
	}, nil
}

// resolveAPIKey picks the API key: the --key flag wins, then the
// OPENAI_API_KEY environment variable. An empty result means the request
// is sent without an Authorization header.
func resolveAPIKey(flagKey string) string {
	if flagKey != "" {
		return flagKey
	}

	return os.Getenv(envAPIKey)
}

// runChatTest sends the fixed prompt to the chat completions endpoint and
// returns the measured result. The API key never appears in returned
// errors.
func runChatTest(ctx context.Context, cfg chatTestConfig) (*chatTestResult, error) {
	payload, err := json.Marshal(chatRequest{
		Model:    cfg.model,
		Messages: []chatMessage{{Role: "user", Content: testPrompt}},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal chat request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, cfg.baseAPI+"/chat/completions", bytes.NewReader(payload),
	)
	if err != nil {
		return nil, fmt.Errorf("build chat request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	if cfg.key != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.key)
	}

	client := &http.Client{Timeout: cfg.timeout}

	start := time.Now()

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send chat request: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			// Nothing useful can be done when closing a response body that
			// has already been fully read; the error is intentionally
			// discarded rather than logged.
			_ = cerr
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read chat response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, chatHTTPError(resp.StatusCode, body, cfg.key)
	}

	return parseChatResponse(cfg.model, body, time.Since(start))
}

// parseChatResponse decodes a successful chat completions body.
func parseChatResponse(model string, body []byte, latency time.Duration) (*chatTestResult, error) {
	var parsed chatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse chat response: %w", err)
	}

	if len(parsed.Choices) == 0 {
		return nil, errors.New("chat response contains no choices")
	}

	return &chatTestResult{
		Model:     model,
		Content:   parsed.Choices[0].Message.Content,
		LatencyMS: float64(latency) / float64(time.Millisecond),
		Usage:     parsed.Usage,
	}, nil
}

// chatAPIErrorBody mirrors the error envelope used by OpenAI-compatible
// APIs: {"error":{"message":"..."}}.
type chatAPIErrorBody struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// chatHTTPError builds the error for a non-2xx response, preferring the
// API's error message and falling back to a truncated raw body. The API
// key is scrubbed so it can never leak into output.
func chatHTTPError(status int, body []byte, key string) error {
	detail := strings.TrimSpace(string(body))

	var apiErr chatAPIErrorBody
	if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Error.Message != "" {
		detail = apiErr.Error.Message
	}

	if key != "" {
		detail = strings.ReplaceAll(detail, key, "[redacted]")
	}

	if len(detail) > maxErrorBodyLen {
		detail = detail[:maxErrorBodyLen] + "..."
	}

	return fmt.Errorf("chat completions request failed: status %d: %s", status, detail)
}

// renderChatResult writes the result in the requested output format.
func renderChatResult(out io.Writer, format string, result *chatTestResult) error {
	switch format {
	case outputFormatJSON:
		return renderChatJSON(out, result)
	case outputFormatTable:
		return renderChatTable(out, result)
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

// renderChatJSON writes the result as indented JSON.
func renderChatJSON(out io.Writer, result *chatTestResult) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal chat result: %w", err)
	}

	if _, err := fmt.Fprintln(out, string(data)); err != nil {
		return fmt.Errorf("print chat result: %w", err)
	}

	return nil
}

// renderChatTable writes the result as human-readable text.
func renderChatTable(out io.Writer, result *chatTestResult) error {
	if _, err := fmt.Fprintf(out, "model:    %s\n", result.Model); err != nil {
		return fmt.Errorf("print model: %w", err)
	}

	if _, err := fmt.Fprintf(out, "response: %s\n", result.Content); err != nil {
		return fmt.Errorf("print response: %w", err)
	}

	if _, err := fmt.Fprintf(out, "latency:  %.1fms\n", result.LatencyMS); err != nil {
		return fmt.Errorf("print latency: %w", err)
	}

	if _, err := fmt.Fprintf(out, "usage:    prompt=%d completion=%d total=%d\n",
		result.Usage.PromptTokens, result.Usage.CompletionTokens, result.Usage.TotalTokens,
	); err != nil {
		return fmt.Errorf("print usage: %w", err)
	}

	return nil
}
