package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strconv"
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
	platformGOG     = "gog"
	platformItch    = "itch"
	platformDRMFree = "drm-free"
)

// freeGamePlatformOrder is the deterministic order of platform selectors,
// used for the default (all platforms) and shell completion.
var freeGamePlatformOrder = []string{
	platformSteam, platformEpic, platformGOG, platformItch, platformDRMFree, platformAndroid,
}

// freeGamePlatformLabels maps a --platform selector to the exact platform
// token matched (as a substring) inside the API's platforms field. The tokens
// are taken verbatim from the live API (see spec); none is a substring of
// another, so substring matching cannot produce false positives.
var freeGamePlatformLabels = map[string]string{
	platformSteam:   "Steam",
	platformEpic:    "Epic Games Store",
	platformAndroid: "Android",
	platformGOG:     "GOG",
	platformItch:    "Itch.io",
	platformDRMFree: "DRM-Free",
}

// Giveaway type selectors accepted by --type, mapped to the exact type token
// the API uses. freeGameTypeAll disables type filtering.
const (
	freeGameTypeAll         = "all"
	freeGameTypeDLC         = "dlc"
	freeGameTypeEarlyAccess = "early-access"
	freeGameTypeOther       = "other"
)

// freeGameTypeOrder is the deterministic order of type selectors, used for
// shell completion.
var freeGameTypeOrder = []string{
	freeGameTypeGame, freeGameTypeDLC, freeGameTypeEarlyAccess, freeGameTypeOther, freeGameTypeAll,
}

// freeGameTypeLabels maps a --type selector to the exact type token in the
// API response.
var freeGameTypeLabels = map[string]string{
	freeGameTypeGame:        "Game",
	freeGameTypeDLC:         "DLC",
	freeGameTypeEarlyAccess: "Early Access",
	freeGameTypeOther:       "Other",
}

// Sort keys accepted by --sort.
const (
	freeGameSortEnd       = "end"
	freeGameSortWorth     = "worth"
	freeGameSortPublished = "published"
	freeGameSortTitle     = "title"
)

// freeGameSortOrder is the deterministic order of sort keys, used for shell
// completion.
var freeGameSortOrder = []string{
	freeGameSortEnd, freeGameSortWorth, freeGameSortPublished, freeGameSortTitle,
}

// freeGameDescriptionWidth caps the description column in table output so a
// long blurb cannot blow up the layout; --output json keeps the full text.
const freeGameDescriptionWidth = 80

// freeGamesConfig holds the resolved settings for one free-games run.
type freeGamesConfig struct {
	baseURL      string
	platforms    []string
	types        []string
	minWorth     float64
	sortKey      string
	details      bool
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
		Short: "Collect limited-time free game giveaways (Steam, Epic, GOG, itch.io)",
		Long: `Collect limited-time free game giveaways from the GamerPower API
(https://www.gamerpower.com, no API token required).

Giveaways are filtered by type (default: full games) and by platform;
giveaways whose end date has already passed are dropped. Results are
sorted by end time, soonest first; giveaways without an end date sort
last.

A single request covers every platform: the API's comma-separated
platform parameter is broken (it returns 404), so platform filtering
happens client-side.

Output formats:
  table  human-readable summary (default)
  json   the untouched API entries, preserving every field
  csv    a flat Title,Platforms,Type,Worth,Users,URL,Start,End table

Use --details to add worth, users, and a truncated description to the
table output; --output json always carries the full text.`,
		Example: `  my-cli free-games
  my-cli free-games --platform steam,epic
  my-cli free-games -p gog --details
  my-cli free-games --min-worth 10 --sort worth
  my-cli free-games --type all -p android --output csv -O free.csv`,
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

			games := collectFreeGames(entries, cfg, time.Now())

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

			return renderFreeGames(out, cfg, games)
		},
	}

	cmd.Flags().StringVar(&raw.baseURL, "base-url", defaultFreeGamesBaseURL,
		"GamerPower API base URL (override for testing)")
	cmd.Flags().StringSliceVarP(&raw.platforms, "platform", "p", nil,
		"platforms to include: steam|epic|gog|itch|drm-free|android, "+
			"repeatable or comma-separated (default: all)")
	cmd.Flags().StringSliceVarP(&raw.types, "type", "t", nil,
		"giveaway types to include: game|dlc|early-access|other|all, "+
			"repeatable or comma-separated (default: game)")
	cmd.Flags().Float64Var(&raw.minWorth, "min-worth", 0,
		"only keep giveaways worth at least this many USD (N/A counts as 0)")
	cmd.Flags().StringVar(&raw.sortKey, "sort", freeGameSortEnd,
		"sort order (end|worth|published|title)")
	cmd.Flags().BoolVar(&raw.details, "details", false,
		"add worth, users, and description columns to the table output")
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

	_ = cmd.RegisterFlagCompletionFunc("type", func(
		_ *cobra.Command, _ []string, _ string,
	) ([]string, cobra.ShellCompDirective) {
		return freeGameTypeOrder, cobra.ShellCompDirectiveNoFileComp
	})

	_ = cmd.RegisterFlagCompletionFunc("sort", func(
		_ *cobra.Command, _ []string, _ string,
	) ([]string, cobra.ShellCompDirective) {
		return freeGameSortOrder, cobra.ShellCompDirectiveNoFileComp
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

	if raw.minWorth < 0 {
		return freeGamesConfig{}, fmt.Errorf("min-worth must not be negative, got %g", raw.minWorth)
	}

	if !slices.Contains(freeGameSortOrder, raw.sortKey) {
		return freeGamesConfig{}, fmt.Errorf(
			"unsupported sort %q (want %s)",
			raw.sortKey, strings.Join(freeGameSortOrder, ", "),
		)
	}

	platforms, err := normalizeFreeGamePlatforms(raw.platforms)
	if err != nil {
		return freeGamesConfig{}, err
	}

	types, err := normalizeFreeGameTypes(raw.types)
	if err != nil {
		return freeGamesConfig{}, err
	}

	return freeGamesConfig{
		baseURL:      strings.TrimRight(raw.baseURL, "/"),
		platforms:    platforms,
		types:        types,
		minWorth:     raw.minWorth,
		sortKey:      raw.sortKey,
		details:      raw.details,
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

// normalizeFreeGameTypes lowercases the selected types, drops duplicates, and
// rejects unknown selectors. An empty selection means full games only, which
// is the historical behaviour of the command. Selecting "all" disables type
// filtering entirely.
func normalizeFreeGameTypes(selectors []string) ([]string, error) {
	if len(selectors) == 0 {
		return []string{freeGameTypeGame}, nil
	}

	seen := make(map[string]struct{}, len(selectors))
	normalized := make([]string, 0, len(selectors))

	for _, selector := range selectors {
		key := strings.ToLower(strings.TrimSpace(selector))

		if key == freeGameTypeAll {
			return []string{freeGameTypeAll}, nil
		}

		if _, ok := freeGameTypeLabels[key]; !ok {
			return nil, fmt.Errorf(
				"unsupported type %q (want %s)",
				selector, strings.Join(freeGameTypeOrder, ", "),
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
	Worth           string `json:"worth"`
	Description     string `json:"description"`
	Instructions    string `json:"instructions"`
	Thumbnail       string `json:"thumbnail"`
	Image           string `json:"image"`
	Users           int    `json:"users"`
	OpenGiveawayURL string `json:"open_giveaway_url"`
	GamerPowerURL   string `json:"gamerpower_url"`
	PublishedDate   string `json:"published_date"`
	EndDate         string `json:"end_date"`
}

// freeGame is one rendered giveaway row. raw preserves the untouched API
// entry for lossless JSON output.
type freeGame struct {
	raw         json.RawMessage
	title       string
	gameType    string
	platforms   string
	worth       string
	worthValue  float64
	hasWorth    bool
	description string
	users       int
	url         string
	gamerPower  string
	start       string
	end         string
	startTime   time.Time
	hasStart    bool
	endTime     time.Time
	hasEnd      bool
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

// collectFreeGames filters the raw API entries to the selected types and
// platforms that have not expired yet, then sorts them with the configured
// key. Entries that do not decode are skipped: the surrounding array is valid
// JSON, so a malformed member is API noise.
func collectFreeGames(entries []json.RawMessage, cfg freeGamesConfig, now time.Time) []freeGame {
	games := make([]freeGame, 0, len(entries))

	for _, raw := range entries {
		var entry freeGameEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			continue
		}

		if !matchesFreeGameTypes(entry.Type, cfg.types) ||
			!matchesFreeGamePlatforms(entry.Platforms, cfg.platforms) {
			continue
		}

		game := freeGame{
			raw:         raw,
			title:       entry.Title,
			gameType:    entry.Type,
			platforms:   entry.Platforms,
			worth:       entry.Worth,
			description: entry.Description,
			users:       entry.Users,
			url:         entry.OpenGiveawayURL,
			gamerPower:  entry.GamerPowerURL,
			start:       entry.PublishedDate,
			end:         entry.EndDate,
		}

		game.worthValue, game.hasWorth = parseFreeGameWorth(entry.Worth)

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

		if entry.PublishedDate != "" {
			if start, err := time.Parse(freeGameDateLayout, entry.PublishedDate); err == nil {
				game.startTime = start
				game.hasStart = true
			}
		}

		if game.worthValue < cfg.minWorth {
			continue
		}

		games = append(games, game)
	}

	slices.SortFunc(games, freeGameComparator(cfg.sortKey))

	return games
}

// parseFreeGameWorth converts the API's worth string ("$6.99", "$384.00", or
// "N/A") into a comparable amount. The second result reports whether the
// entry carries a real price; "N/A" and unparsable values report false and a
// zero amount.
func parseFreeGameWorth(worth string) (float64, bool) {
	trimmed := strings.TrimSpace(worth)
	if trimmed == "" || trimmed == freeGameNoEndDate {
		return 0, false
	}

	trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "$"))

	value, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0, false
	}

	return value, true
}

// matchesFreeGameTypes reports whether the entry's type is among the selected
// types. Selecting "all" matches everything.
func matchesFreeGameTypes(gameType string, selected []string) bool {
	for _, key := range selected {
		if key == freeGameTypeAll {
			return true
		}

		if freeGameTypeLabels[key] == gameType {
			return true
		}
	}

	return false
}

// matchesFreeGamePlatforms reports whether the API's comma-separated
// platforms field contains any of the selected platforms. Substring matching
// is safe: no platform token is a substring of another (verified against the
// live API, see spec).
func matchesFreeGamePlatforms(platforms string, selected []string) bool {
	for _, platform := range selected {
		if strings.Contains(platforms, freeGamePlatformLabels[platform]) {
			return true
		}
	}

	return false
}

// freeGameComparator returns the ordering for the given --sort key. Every
// ordering falls back to the title so the output is stable across runs.
func freeGameComparator(sortKey string) func(a, b freeGame) int {
	switch sortKey {
	case freeGameSortWorth:
		return func(a, b freeGame) int {
			if c := cmp.Compare(b.worthValue, a.worthValue); c != 0 {
				return c
			}

			return strings.Compare(a.title, b.title)
		}
	case freeGameSortPublished:
		return func(a, b freeGame) int {
			switch {
			case a.hasStart && b.hasStart:
				if c := b.startTime.Compare(a.startTime); c != 0 {
					return c
				}
			case a.hasStart:
				return -1
			case b.hasStart:
				return 1
			}

			return strings.Compare(a.title, b.title)
		}
	case freeGameSortTitle:
		return func(a, b freeGame) int {
			return strings.Compare(a.title, b.title)
		}
	case freeGameSortEnd:
		return compareFreeGames
	default:
		return compareFreeGames
	}
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
func renderFreeGames(out io.Writer, cfg freeGamesConfig, games []freeGame) error {
	switch cfg.outputFormat {
	case outputFormatTable:
		return renderFreeGamesTable(out, games, cfg.details)
	case outputFormatJSON:
		return renderFreeGamesJSON(out, games)
	case outputFormatCSV:
		return renderFreeGamesCSV(out, games)
	default:
		return fmt.Errorf("unsupported output format %q", cfg.outputFormat)
	}
}

// renderFreeGamesTable writes a human-readable summary of the giveaways. With
// details enabled it adds worth, users, and a truncated description.
func renderFreeGamesTable(out io.Writer, games []freeGame, details bool) error {
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)

	header := "TITLE\tPLATFORMS\tSTART\tEND\tURL\n"
	if details {
		header = "TITLE\tTYPE\tPLATFORMS\tWORTH\tUSERS\tSTART\tEND\tDESCRIPTION\tURL\n"
	}

	if _, err := fmt.Fprint(w, header); err != nil {
		return fmt.Errorf("print table header: %w", err)
	}

	for _, game := range games {
		var err error

		if details {
			_, err = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\t%s\t%s\t%s\n",
				game.title, game.gameType, game.platforms, game.worth, game.users,
				game.start, game.end, truncateFreeGameDescription(game.description), game.url,
			)
		} else {
			_, err = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				game.title, game.platforms, game.start, game.end, game.url,
			)
		}

		if err != nil {
			return fmt.Errorf("print table row: %w", err)
		}
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush table: %w", err)
	}

	return nil
}

// truncateFreeGameDescription collapses the description to a single line and
// caps it at freeGameDescriptionWidth runes so the table stays readable.
func truncateFreeGameDescription(description string) string {
	flat := strings.Join(strings.Fields(description), " ")

	runes := []rune(flat)
	if len(runes) <= freeGameDescriptionWidth {
		return flat
	}

	return string(runes[:freeGameDescriptionWidth]) + "..."
}

// freeGamesCSVHeader is the fixed CSV schema of the free-games export.
var freeGamesCSVHeader = []string{"Title", "Platforms", "Type", "Worth", "Users", "URL", "Start", "End"}

// renderFreeGamesCSV writes the flat CSV export (RFC 4180 via encoding/csv).
func renderFreeGamesCSV(out io.Writer, games []freeGame) error {
	w := csv.NewWriter(out)

	if err := w.Write(freeGamesCSVHeader); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}

	for _, game := range games {
		row := []string{
			game.title,
			game.platforms,
			game.gameType,
			game.worth,
			strconv.Itoa(game.users),
			game.url,
			game.start,
			game.end,
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
