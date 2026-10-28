package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const youtubeTestLanguageHant = "zh-Hant"

func TestParseYouTubeVideoID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		expected    string
		wantErrPart string
	}{
		{
			name:     "watch URL",
			input:    "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
			expected: "dQw4w9WgXcQ",
		},
		{
			name:     "short URL",
			input:    "https://youtu.be/dQw4w9WgXcQ?t=10",
			expected: "dQw4w9WgXcQ",
		},
		{
			name:     "shorts URL",
			input:    "https://youtube.com/shorts/dQw4w9WgXcQ",
			expected: "dQw4w9WgXcQ",
		},
		{
			name:        "untrusted host",
			input:       "https://youtube.com.evil.example/watch?v=dQw4w9WgXcQ",
			wantErrPart: "invalid youtube URL",
		},
		{
			name:        "invalid video ID",
			input:       "https://youtube.com/watch?v=short",
			wantErrPart: "invalid youtube URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseYouTubeVideoID(tt.input)
			if tt.wantErrPart != "" {
				requireErrorContains(t, err, tt.wantErrPart)

				return
			}

			if err != nil {
				t.Fatalf("parseYouTubeVideoID() error = %v", err)
			}

			if got != tt.expected {
				t.Errorf("parseYouTubeVideoID() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestSelectYouTubeLanguage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		requested string
		available []youtubeCaptionTrack
		expected  string
	}{
		{
			name:      "requested language",
			requested: youtubeTestLanguageHant,
			available: []youtubeCaptionTrack{{LanguageCode: "en"}, {LanguageCode: youtubeTestLanguageHant}},
			expected:  youtubeTestLanguageHant,
		},
		{
			name:      "fallback to english variant",
			requested: "fr",
			available: []youtubeCaptionTrack{{LanguageCode: youtubeTestLanguageHant}, {LanguageCode: "en-US"}},
			expected:  "en-US",
		},
		{
			name:      "fallback to first available",
			requested: "fr",
			available: []youtubeCaptionTrack{{LanguageCode: "ja"}, {LanguageCode: youtubeTestLanguageHant}},
			expected:  "ja",
		},
		{
			name:      "no caption metadata",
			requested: "fr",
			expected:  "fr",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := selectYouTubeLanguage(tt.requested, tt.available); got != tt.expected {
				t.Errorf("selectYouTubeLanguage() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestYouTubeTranscriptCmd(t *testing.T) {
	t.Parallel()

	const videoID = "dQw4w9WgXcQ"

	const captionResponse = `{
  "events": [
    {"tStartMs": 1200, "dDurationMs": 1200, "segs": [{"utf8": "Hello"}, {"utf8": " world"}]},
    {"tStartMs": 2400, "dDurationMs": 1200, "segs": [{"utf8": "Next line"}]},
    {"tStartMs": 3600, "dDurationMs": 1200, "segs": [{"utf8": "\n"}]}
  ]
}`

	tests := []struct {
		name       string
		args       []string
		wantOutput string
		wantErr    string
	}{
		{
			name:       "text output",
			args:       []string{"https://youtu.be/" + videoID, "--lang", "fr"},
			wantOutput: "[00:00:01.200] Hello world\n[00:00:02.400] Next line\n",
		},
		{
			name:       "json output",
			args:       []string{"https://youtube.com/watch?v=" + videoID, argFlagOutput, youtubeTranscriptOutputJSON},
			wantOutput: `"actualLang": "en"`,
		},
		{
			name:    "invalid output",
			args:    []string{"https://youtu.be/" + videoID, argFlagOutput, argOutputXML},
			wantErr: outputFormatErrorPrefix,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := newYouTubeTranscriptTestServer(t, videoID, captionResponse)

			command := newYouTubeTranscriptCmdWithEndpoints(youtubeEndpoints{
				pageURL:   server.URL + "/watch",
				playerURL: server.URL + "/player",
			})

			output := new(strings.Builder)
			command.SetOut(output)
			command.SetErr(new(strings.Builder))
			command.SetArgs(tt.args)
			command.SetContext(t.Context())

			err := command.Execute()
			if tt.wantErr != "" {
				requireErrorContains(t, err, tt.wantErr)

				return
			}

			if err != nil {
				t.Fatalf("execute youtube-transcript: %v", err)
			}

			assertContains(t, output.String(), tt.wantOutput)
		})
	}
}

func newYouTubeTranscriptTestServer(
	t *testing.T,
	videoID string,
	captionResponse string,
) *httptest.Server {
	t.Helper()

	var server *httptest.Server

	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/watch":
			pageResponse := map[string]any{
				"captionTracks": testYouTubeCaptionTracks(server.URL),
			}
			writeYouTubeTestJSON(t, writer, pageResponse)
		case "/player":
			assertYouTubePlayerRequest(t, request, videoID)

			var response youtubePlayerResponse

			captionTracks := testYouTubeCaptionTracks(server.URL)
			response.Captions.PlayerCaptionsTracklistRenderer.CaptionTracks = captionTracks
			writeYouTubeTestJSON(t, writer, response)
		case "/captions":
			assertYouTubeCaptionRequest(t, request)

			if _, err := writer.Write([]byte(captionResponse)); err != nil {
				t.Errorf("write caption response: %v", err)
			}
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	return server
}

func testYouTubeCaptionTracks(origin string) []youtubeRawCaptionTrack {
	return []youtubeRawCaptionTrack{
		{BaseURL: origin + "/captions?lang=en", LanguageCode: "en", Kind: "asr"},
		{BaseURL: origin + "/captions?lang=zh-Hant", LanguageCode: "zh-Hant"},
	}
}

func writeYouTubeTestJSON(t *testing.T, writer http.ResponseWriter, response any) {
	t.Helper()

	if err := json.NewEncoder(writer).Encode(response); err != nil {
		t.Errorf("write JSON test response: %v", err)
	}
}

func assertYouTubePlayerRequest(t *testing.T, request *http.Request, videoID string) {
	t.Helper()

	if request.Method != http.MethodPost {
		t.Errorf("player request method = %q, want POST", request.Method)
	}

	if got := request.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("content type = %q, want application/json", got)
	}

	var payload youtubePlayerRequest
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		t.Errorf("decode player request: %v", err)

		return
	}

	if payload.Context.Client.ClientName != "ANDROID" {
		t.Errorf("client name = %q, want ANDROID", payload.Context.Client.ClientName)
	}

	if payload.Context.Client.ClientVersion != youtubeAndroidClientVersion {
		t.Errorf("client version = %q, want %q", payload.Context.Client.ClientVersion, youtubeAndroidClientVersion)
	}

	if payload.VideoID != videoID {
		t.Errorf("player request video ID = %q, want %q", payload.VideoID, videoID)
	}
}

func assertYouTubeCaptionRequest(t *testing.T, request *http.Request) {
	t.Helper()

	if request.Method != http.MethodGet {
		t.Errorf("caption request method = %q, want GET", request.Method)
	}

	if got := request.URL.Query().Get("fmt"); got != "json3" {
		t.Errorf("caption format = %q, want json3", got)
	}
}

func TestDoYouTubeRequestRejectsHTTPError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		body        string
		wantErrPart string
	}{
		{
			name:        "includes bounded API error detail",
			body:        `{"error":{"message":"invalid transcript request"}}`,
			wantErrPart: "invalid transcript request",
		},
		{
			name:        "truncates oversized error detail",
			body:        strings.Repeat("x", maxYouTubeErrorBodyPreview+20),
			wantErrPart: "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(http.StatusBadRequest)

				if _, err := writer.Write([]byte(tt.body)); err != nil {
					t.Errorf("write error response: %v", err)
				}
			}))
			t.Cleanup(server.Close)

			request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
			if err != nil {
				t.Fatal(err)
			}

			_, err = doYouTubeRequest(server.Client(), request)
			requireErrorContains(t, err, "request returned status 400")
			requireErrorContains(t, err, tt.wantErrPart)

			if tt.name == "truncates oversized error detail" &&
				len(err.Error()) > len("request returned status 400: \"\"...")+maxYouTubeErrorBodyPreview {
				t.Errorf("error message was not bounded: got %d bytes", len(err.Error()))
			}
		})
	}
}

func TestFetchYouTubeCaptionTracksFallsBackToIOS(t *testing.T) {
	t.Parallel()

	var clients []string

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var payload youtubePlayerRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode player request: %v", err)
		}

		clients = append(clients, payload.Context.Client.ClientName)

		if payload.Context.Client.ClientName == "ANDROID" {
			writer.WriteHeader(http.StatusBadRequest)

			return
		}

		var response youtubePlayerResponse

		response.Captions.PlayerCaptionsTracklistRenderer.CaptionTracks = []youtubeRawCaptionTrack{
			{BaseURL: "http://" + request.Host + "/captions", LanguageCode: "en"},
		}
		writeYouTubeTestJSON(t, writer, response)
	}))
	t.Cleanup(server.Close)

	tracks, err := fetchYouTubeCaptionTracks(
		t.Context(),
		server.Client(),
		"dQw4w9WgXcQ",
		youtubeEndpoints{playerURL: server.URL + "/player", pageURL: server.URL + "/watch"},
	)
	if err != nil {
		t.Fatalf("fetch caption tracks: %v", err)
	}

	if len(tracks) != 1 || tracks[0].LanguageCode != "en" {
		t.Fatalf("caption tracks = %#v, want one English track", tracks)
	}

	if strings.Join(clients, ",") != "ANDROID,IOS" {
		t.Errorf("player clients = %v, want Android then iOS", clients)
	}
}

func TestFetchYouTubeCaptionTracksFallsBackToWatchPage(t *testing.T) {
	t.Parallel()

	var server *httptest.Server

	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/player":
			writer.WriteHeader(http.StatusBadRequest)
		case "/watch":
			pageResponse := map[string]any{
				"captionTracks": testYouTubeCaptionTracks(server.URL),
			}
			writeYouTubeTestJSON(t, writer, pageResponse)
		}
	}))
	t.Cleanup(server.Close)

	tracks, err := fetchYouTubeCaptionTracks(
		t.Context(),
		server.Client(),
		"dQw4w9WgXcQ",
		youtubeEndpoints{playerURL: server.URL + "/player", pageURL: server.URL + "/watch"},
	)
	if err != nil {
		t.Fatalf("fetch caption tracks: %v", err)
	}

	if len(tracks) != 2 || tracks[0].LanguageCode != "en" {
		t.Fatalf("caption tracks = %#v, want English and zh-Hant tracks", tracks)
	}
}

func TestFetchYouTubeCaptionLinesRejectsForeignOrigin(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)

	_, err := fetchYouTubeCaptionLines(
		t.Context(),
		server.Client(),
		youtubeRawCaptionTrack{BaseURL: "https://example.com/captions"},
		server.URL+"/player",
	)
	requireErrorContains(t, err, "does not match the player origin")
}

func TestFetchYouTubeCaptionLinesRejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if _, err := writer.Write([]byte("{")); err != nil {
			t.Errorf("write invalid caption response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	_, err := fetchYouTubeCaptionLines(
		t.Context(),
		server.Client(),
		youtubeRawCaptionTrack{BaseURL: server.URL + "/captions"},
		server.URL+"/player",
	)
	requireErrorContains(t, err, "decode youtube captions")
}

func TestResolveYouTubeTranscriptConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  youtubeTranscriptConfig
		wantErr string
	}{
		{
			name: "invalid language",
			config: youtubeTranscriptConfig{
				language:     "en/../../x",
				timeout:      time.Second,
				outputFormat: youtubeTranscriptOutputText,
			},
			wantErr: "invalid language code",
		},
		{
			name: "non-positive timeout",
			config: youtubeTranscriptConfig{
				language:     "en",
				outputFormat: youtubeTranscriptOutputText,
			},
			wantErr: "positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := resolveYouTubeTranscriptConfig(tt.config)
			requireErrorContains(t, err, tt.wantErr)
		})
	}
}

func assertContains(t *testing.T, actual, expected string) {
	t.Helper()

	if !strings.Contains(actual, expected) {
		t.Errorf("output %q does not contain %q", actual, expected)
	}
}

func requireErrorContains(t *testing.T, err error, expected string) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected error containing %q, got nil", expected)
	}

	if !strings.Contains(err.Error(), expected) {
		t.Fatalf("error %q does not contain %q", err, expected)
	}
}
