package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

// defaultFreeGamesBaseURL is the GamerPower API base used when --base-url is
// not given. The giveaways endpoint requires no API token.
const defaultFreeGamesBaseURL = "https://www.gamerpower.com"

// defaultFreeGamesTimeout bounds one collection run.
const defaultFreeGamesTimeout = 30 * time.Second

// freeGameTypeGame is the GamerPower giveaway type that represents a full
// game, as opposed to DLC, loot, or early access builds.
const freeGameTypeGame = "Game"

// freeGameDateLayout is the fixed timestamp format the GamerPower API uses
// for published_date and end_date.
const freeGameDateLayout = "2006-01-02 15:04:05"

// freeGameNoEndDate marks giveaways without a known end date.
const freeGameNoEndDate = "N/A"

// freeGameUserAgent identifies the CLI to the GamerPower API; the site sits
// behind bot protection that rejects requests without a User-Agent.
const freeGameUserAgent = "my-cli (+https://github.com/cwchiu/my-cli)"

// Platform selectors accepted by --platform.
const (
	platformSteam   = "steam"
	platformEpic    = "epic"
	platformAndroid = "android"
)

// freeGamePlatformOrder is the deterministic order of platform selectors,
// used for the default (all platforms) and shell completion.
var freeGamePlatformOrder = []string{platformSteam, platformEpic, platformAndroid}

// freeGamePlatformLabels maps a --platform selector to the exact platform
// token matched (as a substring) inside the API's platforms field.
var freeGamePlatformLabels = map[string]string{
	platformSteam:   "Steam",
	platformEpic:    "Epic Games Store",
	platformAndroid: "Android",
}

// freeGamesConfig holds the resolved settings for one free-games run.
type freeGamesConfig struct {
	baseURL      string
	platforms    []string
	timeout      time.Duration
	outputFormat string
	outFile      string
}

// newFreeGamesCmd returns the `free-games` subcommand, which collects
// limited-time free game giveaways from the GamerPower API.
func newFreeGamesCmd() *cobra.Command {
	var raw freeGamesConfig

	cmd := &cobra.Command{
		Use:   "free-games",
		Short: "Collect limited-time free game giveaways (Steam, Epic, Android)",
		Long: `Collect limited-time free game giveaways from the GamerPower API
(https://www.gamerpower.com, no API token required).

Giveaways are filtered to full games (type "Game") available on the
selected platforms; giveaways whose end date has already passed are
dropped. Results are sorted by end time, soonest first; giveaways
without an end date sort last.

A single request covers every platform: the API's comma-separated
platform parameter is broken (it returns 404), so platform filtering
happens client-side.

Output formats:
  table  human-readable summary (default)
  json   the untouched API entries, preserving every field
  csv    a flat Title,Platforms,URL,Start,End table`,
		Example: `  my-cli free-games
  my-cli free-games --platform steam,epic
  my-cli free-games -p android --output csv -O free.csv`,
		Args: wrapUsage(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Flag validation failures are usage errors (exit code 2).
			cfg, err := resolveFreeGamesConfig(raw)
			if err != nil {
				return fmt.Errorf("%w: %w", errUsage, err)
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), cfg.timeout)
			defer cancel()

			entries, err := fetchFreeGameEntries(ctx, cfg.baseURL)
			if err != nil {
				return err
			}

			games := collectFreeGames(entries, cfg.platforms, time.Now())

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

			return renderFreeGames(out, cfg.outputFormat, games)
		},
	}

	cmd.Flags().StringVar(&raw.baseURL, "base-url", defaultFreeGamesBaseURL,
		"GamerPower API base URL (override for testing)")
	cmd.Flags().StringSliceVarP(&raw.platforms, "platform", "p", nil,
		"platforms to include: steam|epic|android, repeatable or comma-separated (default: all)")
	cmd.Flags().DurationVar(&raw.timeout, "timeout", defaultFreeGamesTimeout,
		"request timeout")
	cmd.Flags().StringVarP(&raw.outputFormat, "output", "o", outputFormatTable,
		"output format (table|json|csv)")
	cmd.Flags().StringVarP(&raw.outFile, "out", "O", "",
		"write the output to this file instead of stdout")

	_ = cmd.RegisterFlagCompletionFunc("output", func(
		_ *cobra.Command, _ []string, _ string,
	) ([]string, cobra.ShellCompDirective) {
		return []string{outputFormatTable, outputFormatJSON, outputFormatCSV}, cobra.ShellCompDirectiveNoFileComp
	})

	_ = cmd.RegisterFlagCompletionFunc("platform", func(
		_ *cobra.Command, _ []string, _ string,
	) ([]string, cobra.ShellCompDirective) {
		return freeGamePlatformOrder, cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

// resolveFreeGamesConfig validates the raw flag values, failing fast before
// any network I/O.
func resolveFreeGamesConfig(raw freeGamesConfig) (freeGamesConfig, error) {
	if raw.timeout <= 0 {
		return freeGamesConfig{}, fmt.Errorf("timeout must be positive, got %s", raw.timeout)
	}

	switch raw.outputFormat {
	case outputFormatTable, outputFormatJSON, outputFormatCSV:
	default:
		return freeGamesConfig{}, fmt.Errorf("unsupported output format %q", raw.outputFormat)
	}

	platforms, err := normalizeFreeGamePlatforms(raw.platforms)
	if err != nil {
		return freeGamesConfig{}, err
	}

	return freeGamesConfig{
		baseURL:      strings.TrimRight(raw.baseURL, "/"),
		platforms:    platforms,
		timeout:      raw.timeout,
		outputFormat: raw.outputFormat,
		outFile:      raw.outFile,
	}, nil
}

// normalizeFreeGamePlatforms lowercases the selected platforms, drops
// duplicates, and rejects unknown selectors. An empty selection means all
// platforms.
func normalizeFreeGamePlatforms(selectors []string) ([]string, error) {
	if len(selectors) == 0 {
		return slices.Clone(freeGamePlatformOrder), nil
	}

	seen := make(map[string]struct{}, len(selectors))
	normalized := make([]string, 0, len(selectors))

	for _, selector := range selectors {
		key := strings.ToLower(strings.TrimSpace(selector))

		if _, ok := freeGamePlatformLabels[key]; !ok {
			return nil, fmt.Errorf(
				"unsupported platform %q (want %s)",
				selector, strings.Join(freeGamePlatformOrder, ", "),
			)
		}

		if _, dup := seen[key]; dup {
			continue
		}

		seen[key] = struct{}{}
		normalized = append(normalized, key)
	}

	return normalized, nil
}

// gamerPowerErrorBody mirrors the error envelope returned by the GamerPower
// API on failures such as an unknown platform parameter:
// {"status":0,"status_message":"No category found, ..."}.
type gamerPowerErrorBody struct {
	Status        int    `json:"status"`
	StatusMessage string `json:"status_message"`
}

// freeGameEntry mirrors the fields of a GamerPower giveaway entry used for
// filtering and rendering. The untouched entry is kept separately as
// json.RawMessage for lossless JSON output.
type freeGameEntry struct {
	Title           string `json:"title"`
	Type            string `json:"type"`
	Platforms       string `json:"platforms"`
	OpenGiveawayURL string `json:"open_giveaway_url"`
	PublishedDate   string `json:"published_date"`
	EndDate         string `json:"end_date"`
}

// freeGame is one rendered giveaway row. raw preserves the untouched API
// entry for lossless JSON output.
type freeGame struct {
	raw       json.RawMessage
	title     string
	platforms string
	url       string
	start     string
	end       string
	endTime   time.Time
	hasEnd    bool
}

// fetchFreeGameEntries performs the GET against the GamerPower giveaways
// endpoint and returns the untouched JSON entries. A single request covers
// every platform; see the command help for why filtering is client-side.
func fetchFreeGameEntries(ctx context.Context, baseURL string) ([]json.RawMessage, error) {
	endpoint := baseURL + "/api/giveaways"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build request %s: %w", endpoint, err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", freeGameUserAgent)

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
		return nil, freeGamesHTTPError(endpoint, resp.StatusCode, body)
	}

	var entries []json.RawMessage
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("parse giveaways response: %w", err)
	}

	return entries, nil
}

// freeGamesHTTPError builds the error for a non-2xx response, preferring the
// structured GamerPower error envelope and falling back to a truncated body.
func freeGamesHTTPError(endpoint string, status int, body []byte) error {
	var envelope gamerPowerErrorBody
	if err := json.Unmarshal(body, &envelope); err == nil && envelope.StatusMessage != "" {
		return fmt.Errorf("request %s failed: status %d: %s",
			endpoint, status, envelope.StatusMessage)
	}

	return fmt.Errorf("request %s failed: status %d: %s",
		endpoint, status, truncateDetail(body))
}

// collectFreeGames filters the raw API entries to full games on the selected
// platforms that have not expired yet, sorted by end time (soonest first, no
// end date last, then title). Entries that do not decode are skipped: the
// surrounding array is valid JSON, so a malformed member is API noise.
func collectFreeGames(entries []json.RawMessage, platforms []string, now time.Time) []freeGame {
	games := make([]freeGame, 0, len(entries))

	for _, raw := range entries {
		var entry freeGameEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			continue
		}

		if entry.Type != freeGameTypeGame || !matchesFreeGamePlatforms(entry.Platforms, platforms) {
			continue
		}

		game := freeGame{
			raw:       raw,
			title:     entry.Title,
			platforms: entry.Platforms,
			url:       entry.OpenGiveawayURL,
			start:     entry.PublishedDate,
			end:       entry.EndDate,
		}

		// The API keeps expired giveaways listed as Active (some for
		// years), so the expiry check happens here, client-side. An end
		// date of N/A means no known end; such giveaways are kept.
		if entry.EndDate != freeGameNoEndDate && entry.EndDate != "" {
			end, err := time.Parse(freeGameDateLayout, entry.EndDate)
			if err != nil {
				continue
			}

			if end.Before(now) {
				continue
			}

			game.endTime = end
			game.hasEnd = true
		}

		games = append(games, game)
	}

	slices.SortFunc(games, compareFreeGames)

	return games
}

// matchesFreeGamePlatforms reports whether the API's comma-separated
// platforms field contains any of the selected platforms. Substring matching
// is safe: no other platform name contains "Steam", "Epic Games Store", or
// "Android" as a substring (verified against the live API, see spec).
func matchesFreeGamePlatforms(platforms string, selected []string) bool {
	for _, platform := range selected {
		if strings.Contains(platforms, freeGamePlatformLabels[platform]) {
			return true
		}
	}

	return false
}

// compareFreeGames orders giveaways by end time ascending; giveaways without
// an end date sort last, and ties break by title.
func compareFreeGames(a, b freeGame) int {
	switch {
	case a.hasEnd && b.hasEnd:
		if c := a.endTime.Compare(b.endTime); c != 0 {
			return c
		}
	case a.hasEnd:
		return -1
	case b.hasEnd:
		return 1
	}

	return strings.Compare(a.title, b.title)
}

// renderFreeGames writes the giveaways in the requested format.
func renderFreeGames(out io.Writer, format string, games []freeGame) error {
	switch format {
	case outputFormatTable:
		return renderFreeGamesTable(out, games)
	case outputFormatJSON:
		return renderFreeGamesJSON(out, games)
	case outputFormatCSV:
		return renderFreeGamesCSV(out, games)
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

// renderFreeGamesTable writes a human-readable summary of the giveaways.
func renderFreeGamesTable(out io.Writer, games []freeGame) error {
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)

	if _, err := fmt.Fprintf(w, "TITLE\tPLATFORMS\tSTART\tEND\tURL\n"); err != nil {
		return fmt.Errorf("print table header: %w", err)
	}

	for _, game := range games {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			game.title, game.platforms, game.start, game.end, game.url,
		); err != nil {
			return fmt.Errorf("print table row: %w", err)
		}
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush table: %w", err)
	}

	return nil
}

// freeGamesCSVHeader is the fixed CSV schema of the free-games export.
var freeGamesCSVHeader = []string{"Title", "Platforms", "URL", "Start", "End"}

// renderFreeGamesCSV writes the flat CSV export (RFC 4180 via encoding/csv).
func renderFreeGamesCSV(out io.Writer, games []freeGame) error {
	w := csv.NewWriter(out)

	if err := w.Write(freeGamesCSVHeader); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}

	for _, game := range games {
		row := []string{game.title, game.platforms, game.url, game.start, game.end}

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

// renderFreeGamesJSON writes the filtered giveaways as an indented JSON
// array of the untouched API entries; no field is added or dropped.
func renderFreeGamesJSON(out io.Writer, games []freeGame) error {
	var compact bytes.Buffer
	compact.WriteString("[")

	for i, game := range games {
		if i > 0 {
			compact.WriteString(",")
		}

		compact.Write(game.raw)
	}

	compact.WriteString("]")

	var buf bytes.Buffer
	if err := json.Indent(&buf, compact.Bytes(), "", "  "); err != nil {
		return fmt.Errorf("indent json: %w", err)
	}

	if _, err := fmt.Fprintln(out, buf.String()); err != nil {
		return fmt.Errorf("print json: %w", err)
	}

	return nil
}
