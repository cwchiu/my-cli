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
    "worth": "$4.99",
    "thumbnail": "https://www.gamerpower.com/offers/1b_alonewithyou.jpg",
    "image": "https://www.gamerpower.com/offers/1_alonewithyou.jpg",
    "description": "Alone With You is a sci-fi romance adventure.",
    "instructions": "1. Click the button to claim.\r\n2. Redeem on Google Play.",
    "type": "Game",
    "platforms": "PC, Android, iOS",
    "open_giveaway_url": "https://www.gamerpower.com/open/alone-with-you",
    "gamerpower_url": "https://www.gamerpower.com/alone-with-you-mobile-giveaway",
    "published_date": "2099-09-10 19:02:45",
    "end_date": "2099-09-17 19:02:45",
    "users": 4200,
    "status": "Active"
  },
  {
    "id": 1551,
    "title": "Astral Ascent Giveaway",
    "worth": "$24.99",
    "thumbnail": "https://www.gamerpower.com/offers/1b_astralascent.jpg",
    "image": "https://www.gamerpower.com/offers/1_astralascent.jpg",
    "description": "Astral Ascent is a 2D platformer roguelite.",
    "instructions": "1. Click the button to claim.\r\n2. Redeem on Epic.",
    "type": "Game",
    "platforms": "PC, Epic Games Store",
    "open_giveaway_url": "https://www.gamerpower.com/open/astral-ascent",
    "gamerpower_url": "https://www.gamerpower.com/astral-ascent-giveaway",
    "published_date": "2099-09-10 17:02:45",
    "end_date": "2099-09-17 17:02:45",
    "users": 690710,
    "status": "Active"
  },
  {
    "id": 1550,
    "title": "SWAPMEAT Steam Key Giveaway",
    "worth": "$9.99",
    "thumbnail": "https://www.gamerpower.com/offers/1b_swapmeat.jpg",
    "image": "https://www.gamerpower.com/offers/1_swapmeat.jpg",
    "description": "SWAPMEAT is a body-swapping shooter.",
    "instructions": "1. Click the button to claim.",
    "type": "Game",
    "platforms": "PC, Steam",
    "open_giveaway_url": "https://www.gamerpower.com/open/swapmeat",
    "gamerpower_url": "https://www.gamerpower.com/swapmeat-steam-key-giveaway",
    "published_date": "2001-09-04 19:02:45",
    "end_date": "2001-09-11 19:02:45",
    "users": 1550,
    "status": "Active"
  },
  {
    "id": 1549,
    "title": "Dwarven Realms Giveaway",
    "worth": "N/A",
    "thumbnail": "https://www.gamerpower.com/offers/1b_dwarvenrealms.jpg",
    "image": "https://www.gamerpower.com/offers/1_dwarvenrealms.jpg",
    "description": "Dwarven Realms is an action RPG.",
    "instructions": "1. Click the button to claim.",
    "type": "Game",
    "platforms": "PC, Steam",
    "open_giveaway_url": "https://www.gamerpower.com/open/dwarven-realms",
    "gamerpower_url": "https://www.gamerpower.com/dwarven-realms-giveaway",
    "published_date": "2099-09-03 19:02:45",
    "end_date": "N/A",
    "users": 900,
    "status": "Active"
  },
  {
    "id": 1548,
    "title": "Some DLC Pack Giveaway",
    "worth": "$2.99",
    "thumbnail": "https://www.gamerpower.com/offers/1b_somedlc.jpg",
    "image": "https://www.gamerpower.com/offers/1_somedlc.jpg",
    "description": "A cosmetic DLC pack.",
    "instructions": "1. Click the button to claim.",
    "type": "DLC",
    "platforms": "PC, Steam",
    "open_giveaway_url": "https://www.gamerpower.com/open/some-dlc",
    "gamerpower_url": "https://www.gamerpower.com/some-dlc-pack-giveaway",
    "published_date": "2099-09-02 19:02:45",
    "end_date": "2099-09-30 19:02:45",
    "users": 300,
    "status": "Active"
  },
  {
    "id": 1547,
    "title": "GamerPower Mobile App Giveaway",
    "worth": "N/A",
    "thumbnail": "https://www.gamerpower.com/offers/1b_mobileapp.jpg",
    "image": "https://www.gamerpower.com/offers/1_mobileapp.jpg",
    "description": "The GamerPower companion app.",
    "instructions": "1. Click the button to claim.",
    "type": "Game",
    "platforms": "Android, iOS",
    "open_giveaway_url": "https://www.gamerpower.com/open/mobile-app",
    "gamerpower_url": "https://www.gamerpower.com/gamerpower-mobile-app-giveaway",
    "published_date": "2099-09-01 19:02:45",
    "end_date": "2099-09-20 19:02:45",
    "users": 12000,
    "status": "Active"
  },
  {
    "id": 1546,
    "title": "A New Beginning: Final Cut (GOG) Giveaway",
    "worth": "$9.99",
    "thumbnail": "https://www.gamerpower.com/offers/1b_anewbeginning.jpg",
    "image": "https://www.gamerpower.com/offers/1_anewbeginning.jpg",
    "description": "A New Beginning: Final Cut is a point-and-click adventure.",
    "instructions": "1. Click the button to claim.\r\n2. Redeem on GOG.",
    "type": "Game",
    "platforms": "PC, GOG, DRM-Free",
    "open_giveaway_url": "https://www.gamerpower.com/open/a-new-beginning-final-cut",
    "gamerpower_url": "https://www.gamerpower.com/a-new-beginning-final-cut-gog-giveaway",
    "published_date": "2099-09-05 19:02:45",
    "end_date": "2099-09-25 19:02:45",
    "users": 3100,
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

// argFlagType is the --type flag used across free-games tests.
const argFlagType = "--type"

// argExtra is a positional argument used by tests across the package to
// exercise "unknown command" rejection; goconst flags the repeated literal
// otherwise.
const argExtra = "extra"

// priceyGameTitle is the expensive fixture entry used by the min-worth and
// sort-worth cases; goconst flags the repeated literal otherwise.
const priceyGameTitle = "Pricey Game"

// freeGamesCase is one table-driven case for the free-games command tests.
type freeGamesCase struct {
	name     string
	args     []string
	checkOut func(t *testing.T, out string)
}

// runFreeGamesCases executes table-driven free-games cases against a fresh
// command tree per case.
func runFreeGamesCases(t *testing.T, tests []freeGamesCase) {
	t.Helper()

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

func TestFreeGamesCommand(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := startIPInfoTestServer(t, http.StatusOK, freeGamesTestPayload)

	tests := []freeGamesCase{
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
	}

	runFreeGamesCases(t, tests)
}

func TestFreeGamesCommandOutput(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := startIPInfoTestServer(t, http.StatusOK, freeGamesTestPayload)

	tests := []freeGamesCase{
		{
			name: "csv output has fixed header and rows",
			args: []string{argFreeGames, argFlagBaseURL, baseURL, argFlagOutput, outputFormatCSV},
			checkOut: func(t *testing.T, out string) {
				t.Helper()

				is := assert.New(t)
				is.Contains(out, "Title,Platforms,Type,Worth,Users,URL,Start,End")
				// encoding/csv quotes only the platforms field because it
				// contains a comma (RFC 4180); the title has no comma.
				is.Contains(out, `Astral Ascent Giveaway,"PC, Epic Games Store",Game,$24.99,690710,https://www.gamerpower.com/open/astral-ascent,2099-09-10 17:02:45,2099-09-17 17:02:45`)
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
				is.Len(decoded, 5)
				is.Equal("Astral Ascent Giveaway", decoded[0]["title"])
				is.InEpsilon(float64(1551), decoded[0]["id"], 0)
				is.Equal("Active", decoded[0]["status"])
				// Fields the table does not show survive the JSON path.
				is.Equal("$24.99", decoded[0]["worth"])
				is.Equal("https://www.gamerpower.com/offers/1_astralascent.jpg", decoded[0]["image"])
				is.Contains(decoded[0]["instructions"], "Redeem on Epic")
			},
		},
		{
			name: "gog platform selector matches the GOG token",
			args: []string{argFreeGames, argFlagBaseURL, baseURL, argFlagPlatform, platformGOG},
			checkOut: func(t *testing.T, out string) {
				t.Helper()

				is := assert.New(t)
				is.Contains(out, "A New Beginning: Final Cut")
				is.NotContains(out, "Astral Ascent")
				is.NotContains(out, "Dwarven Realms")
			},
		},
		{
			name: "drm-free platform selector matches IndieGala-style entries",
			args: []string{argFreeGames, argFlagBaseURL, baseURL, argFlagPlatform, platformDRMFree},
			checkOut: func(t *testing.T, out string) {
				t.Helper()

				is := assert.New(t)
				is.Contains(out, "A New Beginning: Final Cut")
				is.NotContains(out, "Astral Ascent")
			},
		},
	}

	runFreeGamesCases(t, tests)
}

func TestFreeGamesCommandFilters(t *testing.T) {
	t.Parallel()

	baseURL, _, _ := startIPInfoTestServer(t, http.StatusOK, freeGamesTestPayload)

	tests := []freeGamesCase{
		{
			name: "type all includes DLC",
			args: []string{argFreeGames, argFlagBaseURL, baseURL, argFlagType, freeGameTypeAll},
			checkOut: func(t *testing.T, out string) {
				t.Helper()

				assert.Contains(t, out, "Some DLC Pack")
			},
		},
		{
			name: "type dlc keeps only DLC",
			args: []string{argFreeGames, argFlagBaseURL, baseURL, argFlagType, freeGameTypeDLC},
			checkOut: func(t *testing.T, out string) {
				t.Helper()

				is := assert.New(t)
				is.Contains(out, "Some DLC Pack")
				is.NotContains(out, "Astral Ascent")
			},
		},
		{
			name: "min-worth drops cheaper and unpriced giveaways",
			args: []string{argFreeGames, argFlagBaseURL, baseURL, "--min-worth", "10"},
			checkOut: func(t *testing.T, out string) {
				t.Helper()

				is := assert.New(t)
				is.Contains(out, "Astral Ascent")
				is.NotContains(out, "Alone With You")
				is.NotContains(out, "Dwarven Realms")
			},
		},
		{
			name: "sort worth orders by descending price",
			args: []string{argFreeGames, argFlagBaseURL, baseURL, "--sort", freeGameSortWorth},
			checkOut: func(t *testing.T, out string) {
				t.Helper()

				is := assert.New(t)
				iAstral := strings.Index(out, "Astral Ascent")
				iAlone := strings.Index(out, "Alone With You")
				iDwarven := strings.Index(out, "Dwarven Realms")

				is.Greater(iAstral, -1)
				is.Less(iAstral, iAlone, "$24.99 sorts before $4.99")
				is.Less(iAlone, iDwarven, "priced entries sort before N/A")
			},
		},
		{
			name: "sort title orders alphabetically",
			args: []string{argFreeGames, argFlagBaseURL, baseURL, "--sort", freeGameSortTitle},
			checkOut: func(t *testing.T, out string) {
				t.Helper()

				is := assert.New(t)
				is.Less(
					strings.Index(out, "A New Beginning"),
					strings.Index(out, "Alone With You"),
				)
			},
		},
		{
			name: "details adds worth users and description columns",
			args: []string{argFreeGames, argFlagBaseURL, baseURL, "--details"},
			checkOut: func(t *testing.T, out string) {
				t.Helper()

				is := assert.New(t)
				for _, header := range []string{"TYPE", "WORTH", "USERS", "DESCRIPTION"} {
					is.Contains(out, header)
				}

				is.Contains(out, "$24.99")
				is.Contains(out, "690710")
				is.Contains(out, "Astral Ascent is a 2D platformer roguelite.")
			},
		},
	}

	runFreeGamesCases(t, tests)
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
			name:    "unknown type is a usage error",
			args:    []string{argFreeGames, argFlagBaseURL, okURL, argFlagType, "loot"},
			wantErr: `unsupported type "loot"`,
			isUsage: true,
		},
		{
			name:    "unknown sort is a usage error",
			args:    []string{argFreeGames, argFlagBaseURL, okURL, "--sort", "price"},
			wantErr: `unsupported sort "price"`,
			isUsage: true,
		},
		{
			name:    "negative min-worth is a usage error",
			args:    []string{argFreeGames, argFlagBaseURL, okURL, "--min-worth", "-1"},
			wantErr: "min-worth must not be negative",
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
			wantErr: errUnknownCommand,
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
			want: []string{
				platformSteam, platformEpic, platformGOG, platformItch, platformDRMFree, platformAndroid,
			},
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
			name:      "gog itch and drm-free are accepted",
			selectors: []string{"GOG", "Itch", "DRM-Free"},
			want:      []string{platformGOG, platformItch, platformDRMFree},
		},
		{
			name:      "unknown selector is rejected",
			selectors: []string{"battlenet"},
			wantErr:   `unsupported platform "battlenet"`,
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
		json.RawMessage(`{"title":"Live Game","type":"Game","platforms":"PC, Steam","worth":"$9.99","open_giveaway_url":"u1","published_date":"2026-09-10 10:00:00","end_date":"2026-09-20 10:00:00"}`),
		json.RawMessage(`{"title":"No End Game","type":"Game","platforms":"Steam","worth":"N/A","open_giveaway_url":"u2","published_date":"2026-09-10 10:00:00","end_date":"N/A"}`),
		json.RawMessage(`{"title":"Expired Game","type":"Game","platforms":"Steam","worth":"$1.99","open_giveaway_url":"u3","published_date":"2026-09-01 10:00:00","end_date":"2026-09-11 10:00:00"}`),
		json.RawMessage(`{"title":"Wrong Type","type":"DLC","platforms":"Steam","worth":"$2.99","open_giveaway_url":"u4","published_date":"2026-09-10 10:00:00","end_date":"2026-09-20 10:00:00"}`),
		json.RawMessage(`{"title":"Wrong Platform","type":"Game","platforms":"Battle.net","worth":"$4.99","open_giveaway_url":"u5","published_date":"2026-09-10 10:00:00","end_date":"2026-09-20 10:00:00"}`),
		json.RawMessage(`{not json}`),
	}

	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	cfg := freeGamesConfig{
		platforms: freeGamePlatformOrder,
		types:     []string{freeGameTypeGame},
		sortKey:   freeGameSortEnd,
	}

	games := collectFreeGames(entries, cfg, now)

	is := assert.New(t)
	is.Len(games, 2)
	is.Equal("Live Game", games[0].title, "dated entry sorts before N/A")
	is.Equal("No End Game", games[1].title)
	is.True(games[0].hasEnd)
	is.False(games[1].hasEnd)
	is.InEpsilon(9.99, games[0].worthValue, 0.001)
	is.True(games[0].hasWorth)
	is.False(games[1].hasWorth, "N/A carries no price")
}

func TestCollectFreeGamesFilters(t *testing.T) {
	t.Parallel()

	entries := []json.RawMessage{
		json.RawMessage(`{"title":"Cheap Game","type":"Game","platforms":"PC, Steam","worth":"$1.99","end_date":"2099-09-20 10:00:00"}`),
		json.RawMessage(`{"title":"` + priceyGameTitle + `","type":"Game","platforms":"PC, Steam","worth":"$29.99","end_date":"2099-09-20 10:00:00"}`),
		json.RawMessage(`{"title":"Unpriced Game","type":"Game","platforms":"PC, Steam","worth":"N/A","end_date":"2099-09-20 10:00:00"}`),
		json.RawMessage(`{"title":"DLC Pack","type":"DLC","platforms":"PC, Steam","worth":"$29.99","end_date":"2099-09-20 10:00:00"}`),
	}

	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		cfg  freeGamesConfig
		want []string
	}{
		{
			name: "min-worth keeps only entries at or above the threshold",
			cfg: freeGamesConfig{
				platforms: freeGamePlatformOrder,
				types:     []string{freeGameTypeGame},
				minWorth:  10,
				sortKey:   freeGameSortEnd,
			},
			want: []string{priceyGameTitle},
		},
		{
			name: "type all keeps every type",
			cfg: freeGamesConfig{
				platforms: freeGamePlatformOrder,
				types:     []string{freeGameTypeAll},
				sortKey:   freeGameSortTitle,
			},
			want: []string{"Cheap Game", "DLC Pack", priceyGameTitle, "Unpriced Game"},
		},
		{
			name: "sort worth orders descending with N/A last",
			cfg: freeGamesConfig{
				platforms: freeGamePlatformOrder,
				types:     []string{freeGameTypeGame},
				sortKey:   freeGameSortWorth,
			},
			want: []string{priceyGameTitle, "Cheap Game", "Unpriced Game"},
		},
		{
			name: "sort published orders newest first",
			cfg: freeGamesConfig{
				platforms: freeGamePlatformOrder,
				types:     []string{freeGameTypeGame},
				sortKey:   freeGameSortPublished,
			},
			want: []string{"Cheap Game", priceyGameTitle, "Unpriced Game"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			games := collectFreeGames(entries, tt.cfg, now)

			titles := make([]string, 0, len(games))
			for _, game := range games {
				titles = append(titles, game.title)
			}

			assert.Equal(t, tt.want, titles)
		})
	}
}

func TestParseFreeGameWorth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		worth     string
		wantValue float64
		wantOK    bool
	}{
		{name: "dollar amount", worth: "$6.99", wantValue: 6.99, wantOK: true},
		{name: "large amount", worth: "$384.00", wantValue: 384, wantOK: true},
		{name: "no cents", worth: "$10.00", wantValue: 10, wantOK: true},
		{name: "surrounding spaces", worth: " $2.00 ", wantValue: 2, wantOK: true},
		{name: "N/A carries no price", worth: "N/A", wantValue: 0, wantOK: false},
		{name: "empty carries no price", worth: "", wantValue: 0, wantOK: false},
		{name: "garbage carries no price", worth: "free", wantValue: 0, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			value, ok := parseFreeGameWorth(tt.worth)

			is := assert.New(t)
			is.InDelta(tt.wantValue, value, 0.001)
			is.Equal(tt.wantOK, ok)
		})
	}
}

func TestNormalizeFreeGameTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		selectors []string
		want      []string
		wantErr   string
	}{
		{
			name:      "empty selection means full games only",
			selectors: nil,
			want:      []string{freeGameTypeGame},
		},
		{
			name:      "case is normalized",
			selectors: []string{"DLC", "Early-Access"},
			want:      []string{freeGameTypeDLC, freeGameTypeEarlyAccess},
		},
		{
			name:      "duplicates are dropped",
			selectors: []string{freeGameTypeDLC, freeGameTypeDLC},
			want:      []string{freeGameTypeDLC},
		},
		{
			name:      "all short-circuits the selection",
			selectors: []string{freeGameTypeDLC, freeGameTypeAll},
			want:      []string{freeGameTypeAll},
		},
		{
			name:      "unknown selector is rejected",
			selectors: []string{"loot"},
			wantErr:   `unsupported type "loot"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := normalizeFreeGameTypes(tt.selectors)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTruncateFreeGameDescription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		description string
		want        string
	}{
		{name: "short text is unchanged", description: "Short blurb.", want: "Short blurb."},
		{name: "newlines collapse to spaces", description: "line one\r\nline two", want: "line one line two"},
		{
			name:        "long text is capped",
			description: strings.Repeat("a", freeGameDescriptionWidth+10),
			want:        strings.Repeat("a", freeGameDescriptionWidth) + "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, truncateFreeGameDescription(tt.description))
		})
	}
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
