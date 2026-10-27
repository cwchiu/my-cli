package main

import (
	"encoding/base64"
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

func TestBuildYouTubeTranscriptParams(t *testing.T) {
	t.Parallel()

	params := buildYouTubeTranscriptParams("dQw4w9WgXcQ", youtubeTestLanguageHant)

	decoded, err := base64.StdEncoding.DecodeString(params)
	if err != nil {
		t.Fatalf("decode transcript params: %v", err)
	}

	assertContains(t, string(decoded), "dQw4w9WgXcQ")
	assertContains(t, string(decoded), "engagement-panel-searchable-transcript-search-panel")
}

func TestYouTubeTranscriptCmd(t *testing.T) {
	t.Parallel()

	const videoID = "dQw4w9WgXcQ"

	const transcriptResponse = `{
  "actions": [{
    "updateEngagementPanelAction": {
      "content": {
        "transcriptRenderer": {
          "content": {
            "transcriptSearchPanelRenderer": {
              "body": {
                "transcriptSegmentListRenderer": {
                  "initialSegments": [
                    {
                      "transcriptSegmentRenderer": {
                        "startMs": "1200",
                        "endMs": "2400",
                        "snippet": {"runs": [{"text": "Hello world"}]}
                      }
                    },
                    {
                      "transcriptSegmentRenderer": {
                        "startMs": 2400,
                        "endMs": 3600,
                        "snippet": {"elementsAttributedString": {"content": "Next line"}}
                      }
                    }
                  ]
                }
              }
            }
          }
        }
      }
    }
  }]
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

			server := newYouTubeTranscriptTestServer(t, videoID, transcriptResponse)

			command := newYouTubeTranscriptCmdWithEndpoints(youtubeEndpoints{
				pageURL:       server.URL + "/watch",
				transcriptURL: server.URL + "/transcript",
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
	transcriptResponse string,
) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/watch":
			if request.Method != http.MethodGet {
				t.Errorf("watch request method = %q, want GET", request.Method)
			}

			if got := request.URL.Query().Get("v"); got != videoID {
				t.Errorf("video ID query = %q, want %q", got, videoID)
			}

			pageResponse := `{
  "visitorData":"visitor",
  "captionTracks": [
    {"languageCode": "en", "name": {"simpleText": "English"}},
    {"languageCode": "zh-Hant", "name": {"simpleText": "繁體中文"}}
  ]
}`
			if _, err := writer.Write([]byte(pageResponse)); err != nil {
				t.Errorf("write video page: %v", err)
			}
		case "/transcript":
			if request.Method != http.MethodPost {
				t.Errorf("transcript request method = %q, want POST", request.Method)
			}

			if got := request.Header.Get("Content-Type"); got != "application/json" {
				t.Errorf("content type = %q, want application/json", got)
			}

			var payload struct {
				Context struct {
					Client struct {
						ClientName string `json:"clientName"`
					} `json:"client"`
				} `json:"context"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Errorf("decode transcript request: %v", err)
			} else if payload.Context.Client.ClientName != "ANDROID" {
				t.Errorf("client name = %q, want ANDROID", payload.Context.Client.ClientName)
			}

			if _, err := writer.Write([]byte(transcriptResponse)); err != nil {
				t.Errorf("write transcript response: %v", err)
			}
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	return server
}

func TestDoYouTubeRequestRejectsHTTPError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = doYouTubeRequest(server.Client(), request)
	requireErrorContains(t, err, "request returned status 403")
}

func TestParseYouTubeTranscriptResponseErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		body        string
		wantErrPart string
	}{
		{
			name:        "invalid JSON",
			body:        `{`,
			wantErrPart: "decode youtube transcript response",
		},
		{
			name:        "API error",
			body:        `{"error":{"code":400,"message":"Bad request"}}`,
			wantErrPart: "youtube transcript api error",
		},
		{
			name:        "no transcript",
			body:        `{"actions":[]}`,
			wantErrPart: "no transcript available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := parseYouTubeTranscriptResponse([]byte(tt.body), "en", "en", []youtubeCaptionTrack{})
			requireErrorContains(t, err, tt.wantErrPart)
		})
	}
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
