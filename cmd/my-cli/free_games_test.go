package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// freeGamesTestPayload mirrors the real GamerPower /api/giveaways response
// (Source of Truth, see spec): an array of giveaway entries with type,
// comma-separated platforms, and end_date of either a timestamp or "N/A".
//
// Dates are clock-independent: live entries end in 2099 (never expires),
// the expired entry ended in 2001 (always expired).
const freeGamesTestPayload = `[
  {
    "id": 1552,
    "title": "Alone With You (Mobile) Giveaway",
    "type": "Game",
    "platforms": "PC, Android, iOS",
    "open_giveaway_url": "https://www.gamerpower.com/open/alone-with-you",
    "published_date": "2099-09-10 19:02:45",
    "end_date": "2099-09-17 19:02:45",
    "status": "Active"
  },
  {
    "id": 1551,
    "title": "Astral Ascent Giveaway",
    "type": "Game",
    "platforms": "PC, Epic Games Store",
    "open_giveaway_url": "https://www.gamerpower.com/open/astral-ascent",
    "published_date": "2099-09-10 17:02:45",
    "end_date": "2099-09-17 17:02:45",
    "status": "Active"
  },
  {
    "id": 1550,
    "title": "SWAPMEAT Steam Key Giveaway",
    "type": "Game",
    "platforms": "PC, Steam",
    "open_giveaway_url": "https://www.gamerpower.com/open/swapmeat",
    "published_date": "2001-09-04 19:02:45",
    "end_date": "2001-09-11 19:02:45",
    "status": "Active"
  },
  {
    "id": 1549,
    "title": "Dwarven Realms Giveaway",
    "type": "Game",
    "platforms": "PC, Steam",
    "open_giveaway_url": "https://www.gamerpower.com/open/dwarven-realms",
    "published_date": "2099-09-03 19:02:45",
    "end_date": "N/A",
    "status": "Active"
  },
  {
    "id": 1548,
    "title": "Some DLC Pack Giveaway",
    "type": "DLC",
    "platforms": "PC, Steam",
    "open_giveaway_url": "https://www.gamerpower.com/open/some-dlc",
    "published_date": "2099-09-02 19:02:45",
    "end_date": "2099-09-30 19:02:45",
    "status": "Active"
  },
  {
    "id": 1547,
    "title": "GamerPower Mobile App Giveaway",
    "type": "Game",
    "platforms": "Android, iOS",
    "open_giveaway_url": "https://www.gamerpower.com/open/mobile-app",
    "published_date": "2099-09-01 19:02:45",
    "end_date": "2099-09-20 19:02:45",
    "status": "Active"
  }
]`

// freeGamesErrorPayload is the 404 envelope the API returns for a broken
// platform parameter (Source of Truth, see spec).
const freeGamesErrorPayload = `{"status":0,"status_message":"No category found, please check the correct parameters."}`

// argFreeGames is the subcommand name used across free-games tests; goconst
// flags the repeated literal otherwise.
const argFreeGames = "free-games"

// argFlagPlatform is the --platform flag used across free-games tests.
const argFlagPlatform = "--platform"

// argExtra is a positional argument used by tests across the package to
// exercise "unknown command" rejection; goconst flags the repeated literal
// otherwise.
const argExtra = "extra"

func TestFreeGamesCommand(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := startIPInfoTestServer(t, http.StatusOK, freeGamesTestPayload)

	tests := []struct {
		name     string
		args     []string
		checkOut func(t *testing.T, out string)
	}{
		{
			name: "table output lists live games sorted by end time",
			args: []string{argFreeGames, argFlagBaseURL, baseURL},
			checkOut: func(t *testing.T, out string) {
				t.Helper()

				is := assert.New(t)
				for _, header := range []string{"TITLE", "PLATFORMS", "START", "END", "URL"} {
					is.Contains(out, header)
				}

				// Sorted by end time ascending: Astral Ascent (17:02)
				// before Alone With You (19:02), dated entries before N/A.
				iAstral := strings.Index(out, "Astral Ascent")
				iAlone := strings.Index(out, "Alone With You")
				iDwarven := strings.Index(out, "Dwarven Realms")

				is.Greater(iAstral, -1)
				is.Greater(iAlone, -1)
				is.Greater(iDwarven, -1)
				is.Less(iAstral, iAlone, "17:02 sorts before 19:02")
				is.Less(iAlone, iDwarven, "dated entries sort before N/A")

				// Expired SWAPMEAT (ended 2001) is dropped.
				is.NotContains(out, "SWAPMEAT")
			},
		},
		{
			name: "platform filter keeps only matching platforms",
			args: []string{argFreeGames, argFlagBaseURL, baseURL, argFlagPlatform, platformSteam},
			checkOut: func(t *testing.T, out string) {
				t.Helper()

				is := assert.New(t)
				is.Contains(out, "Dwarven Realms")
				is.NotContains(out, "Alone With You")
				is.NotContains(out, "Astral Ascent")
			},
		},
		{
			name: "comma-separated platforms are accepted",
			args: []string{argFreeGames, argFlagBaseURL, baseURL, argFlagPlatform, "epic,android"},
			checkOut: func(t *testing.T, out string) {
				t.Helper()

				is := assert.New(t)
				is.Contains(out, "Astral Ascent")
				is.Contains(out, "Alone With You")
				is.NotContains(out, "Dwarven Realms")
			},
		},
		{
			name: "repeated platform flags are accepted",
			args: []string{
				argFreeGames, argFlagBaseURL, baseURL,
				argFlagPlatform, platformEpic, argFlagPlatform, platformAndroid,
			},
			checkOut: func(t *testing.T, out string) {
				t.Helper()

				is := assert.New(t)
				is.Contains(out, "Astral Ascent")
				is.Contains(out, "Alone With You")
				is.NotContains(out, "Dwarven Realms")
			},
		},
		{
			name: "DLC type is excluded",
			args: []string{argFreeGames, argFlagBaseURL, baseURL},
			checkOut: func(t *testing.T, out string) {
				t.Helper()

				assert.NotContains(t, out, "Some DLC Pack")
			},
		},
		{
			name: "csv output has fixed header and rows",
			args: []string{argFreeGames, argFlagBaseURL, baseURL, argFlagOutput, outputFormatCSV},
			checkOut: func(t *testing.T, out string) {
				t.Helper()

				is := assert.New(t)
				is.Contains(out, "Title,Platforms,URL,Start,End")
				// encoding/csv quotes only the platforms field because it
				// contains a comma (RFC 4180); the title has no comma.
				is.Contains(out, `Astral Ascent Giveaway,"PC, Epic Games Store",https://www.gamerpower.com/open/astral-ascent,2099-09-10 17:02:45,2099-09-17 17:02:45`)
				is.Contains(out, "Dwarven Realms")
				is.NotContains(out, "SWAPMEAT")
			},
		},
		{
			name: "json output is lossless per entry",
			args: []string{argFreeGames, argFlagBaseURL, baseURL, argFlagOutput, outputFormatJSON},
			checkOut: func(t *testing.T, out string) {
				t.Helper()

				var decoded []map[string]any
				require.NoError(t, json.Unmarshal([]byte(out), &decoded))
				is := assert.New(t)
				is.Len(decoded, 4)
				is.Equal("Astral Ascent Giveaway", decoded[0]["title"])
				is.InEpsilon(float64(1551), decoded[0]["id"], 0)
				is.Equal("Active", decoded[0]["status"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// The fixture dates are clock-independent (2099 live / 2001
			// expired), so the command's time.Now() needs no injection.
			out, err := executeCommand(t, tt.args...)
			require.NoError(t, err)
			tt.checkOut(t, out)
		})
	}
}

func TestFreeGamesSingleRequest(t *testing.T) {
	t.Parallel()

	assert := assert.New(t)

	baseURL, wait, paths := startIPInfoTestServer(t, http.StatusOK, freeGamesTestPayload)

	_, err := executeCommand(t, argFreeGames, argFlagBaseURL, baseURL)
	require.NoError(t, err)
	wait()

	// One request covers every platform (the API's comma syntax is broken;
	// filtering is client-side).
	assert.Equal([]string{"/api/giveaways"}, *paths)

	_, err = executeCommand(t, argFreeGames, argFlagBaseURL, baseURL, argFlagPlatform, platformSteam)
	require.NoError(t, err)
	wait()
	assert.Equal([]string{"/api/giveaways", "/api/giveaways"}, *paths)
}

func TestFreeGamesErrors(t *testing.T) {
	t.Parallel()

	okURL, _, _ := startIPInfoTestServer(t, http.StatusOK, freeGamesTestPayload)
	notFoundURL, _, _ := startIPInfoTestServer(t, http.StatusNotFound, freeGamesErrorPayload)
	plainURL, _, _ := startIPInfoTestServer(t, http.StatusInternalServerError, "boom")
	badJSONURL, _, _ := startIPInfoTestServer(t, http.StatusOK, "{not json")
	notArrayURL, _, _ := startIPInfoTestServer(t, http.StatusOK, `{"status":0}`)

	tests := []struct {
		name    string
		args    []string
		wantErr string
		isUsage bool
	}{
		{
			name:    "unknown platform is a usage error",
			args:    []string{argFreeGames, argFlagBaseURL, okURL, argFlagPlatform, "ios"},
			wantErr: `unsupported platform "ios"`,
			isUsage: true,
		},
		{
			name:    "invalid output format is a usage error",
			args:    []string{argFreeGames, argFlagBaseURL, okURL, argFlagOutput, argOutputXML},
			wantErr: wantBadFormat,
			isUsage: true,
		},
		{
			name:    "non-positive timeout is a usage error",
			args:    []string{argFreeGames, argFlagBaseURL, okURL, "--timeout", "0s"},
			wantErr: "timeout must be positive",
			isUsage: true,
		},
		{
			name:    "positional args are rejected",
			args:    []string{argFreeGames, argFlagBaseURL, okURL, argExtra},
			wantErr: "unknown command",
			isUsage: false,
		},
		{
			name:    "404 with envelope surfaces the status message",
			args:    []string{argFreeGames, argFlagBaseURL, notFoundURL},
			wantErr: "No category found",
		},
		{
			name:    "500 with plain body is truncated",
			args:    []string{argFreeGames, argFlagBaseURL, plainURL},
			wantErr: "status 500: boom",
		},
		{
			name:    "invalid json from server is an error",
			args:    []string{argFreeGames, argFlagBaseURL, badJSONURL},
			wantErr: "parse giveaways response",
		},
		{
			name:    "non-array json from server is an error",
			args:    []string{argFreeGames, argFlagBaseURL, notArrayURL},
			wantErr: "parse giveaways response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := executeCommand(t, tt.args...)
			require.ErrorContains(t, err, tt.wantErr)

			if tt.isUsage {
				assert.ErrorIs(t, err, errUsage)
			}
		})
	}
}

func TestNormalizeFreeGamePlatforms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		selectors []string
		want      []string
		wantErr   string
	}{
		{
			name:      "empty selection means all platforms",
			selectors: nil,
			want:      []string{platformSteam, platformEpic, platformAndroid},
		},
		{
			name:      "case is normalized",
			selectors: []string{"Steam", "EPIC"},
			want:      []string{platformSteam, platformEpic},
		},
		{
			name:      "duplicates are dropped",
			selectors: []string{platformSteam, platformSteam, platformAndroid},
			want:      []string{platformSteam, platformAndroid},
		},
		{
			name:      "unknown selector is rejected",
			selectors: []string{"gog"},
			wantErr:   `unsupported platform "gog"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := normalizeFreeGamePlatforms(tt.selectors)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCollectFreeGames(t *testing.T) {
	t.Parallel()

	entries := []json.RawMessage{
		json.RawMessage(`{"title":"Live Game","type":"Game","platforms":"PC, Steam","open_giveaway_url":"u1","published_date":"2026-09-10 10:00:00","end_date":"2026-09-20 10:00:00"}`),
		json.RawMessage(`{"title":"No End Game","type":"Game","platforms":"Steam","open_giveaway_url":"u2","published_date":"2026-09-10 10:00:00","end_date":"N/A"}`),
		json.RawMessage(`{"title":"Expired Game","type":"Game","platforms":"Steam","open_giveaway_url":"u3","published_date":"2026-09-01 10:00:00","end_date":"2026-09-11 10:00:00"}`),
		json.RawMessage(`{"title":"Wrong Type","type":"DLC","platforms":"Steam","open_giveaway_url":"u4","published_date":"2026-09-10 10:00:00","end_date":"2026-09-20 10:00:00"}`),
		json.RawMessage(`{"title":"Wrong Platform","type":"Game","platforms":"Battle.net","open_giveaway_url":"u5","published_date":"2026-09-10 10:00:00","end_date":"2026-09-20 10:00:00"}`),
		json.RawMessage(`{not json}`),
	}

	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)

	games := collectFreeGames(entries, freeGamePlatformOrder, now)

	is := assert.New(t)
	is.Len(games, 2)
	is.Equal("Live Game", games[0].title, "dated entry sorts before N/A")
	is.Equal("No End Game", games[1].title)
	is.True(games[0].hasEnd)
	is.False(games[1].hasEnd)
}

func TestCompareFreeGames(t *testing.T) {
	t.Parallel()

	dated := freeGame{title: "b", endTime: time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC), hasEnd: true}
	otherDated := freeGame{title: "a", endTime: time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC), hasEnd: true}
	noEnd := freeGame{title: "a"}

	tests := []struct {
		name string
		a    freeGame
		b    freeGame
		want int
	}{
		{name: "dated before no-end", a: dated, b: noEnd, want: -1},
		{name: "no-end after dated", a: noEnd, b: dated, want: 1},
		{name: "same time breaks by title", a: dated, b: otherDated, want: 1},
		{name: "earlier end first", a: freeGame{endTime: dated.endTime.Add(-time.Hour), hasEnd: true}, b: dated, want: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, compareFreeGames(tt.a, tt.b))
		})
	}
}
