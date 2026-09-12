// Package nexus provides a minimal read-only client for the Sonatype
// Nexus Repository 3 REST API, focused on exporting repository settings.
//
// The client authenticates with HTTP Basic credentials (or anonymously when
// both are empty) and never includes the password in returned errors.
package nexus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// maxErrorBodyLen caps how much of an error response body is echoed back,
// so a misbehaving server cannot flood the terminal.
const maxErrorBodyLen = 512

// pathRepositories is the NXRM3 REST v1 endpoint listing all repositories.
// It is an endpoint path, not a credential.
const pathRepositories = "/service/rest/v1/repositories"

// Client is a minimal Nexus Repository 3 API client. It is safe for
// concurrent use.
type Client struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
}

// NewClient returns a client for the Nexus server at baseURL. When username
// is empty the request is anonymous; otherwise HTTP Basic authentication is
// used with password. The password is never included in returned errors.
func NewClient(baseURL, username, password string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		username:   username,
		password:   password,
		httpClient: httpClient,
	}
}

// get performs a GET against apiPath and returns the raw response body.
// Non-2xx responses become errors whose message includes a truncated body;
// the Basic password is never echoed.
func (c *Client) get(ctx context.Context, apiPath string) ([]byte, error) {
	endpoint := c.baseURL + apiPath

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build request %s: %w", apiPath, err)
	}

	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request %s: %w", apiPath, err)
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response %s: %w", apiPath, err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("request %s failed: status 401: invalid credentials", apiPath)
		}

		return nil, fmt.Errorf("request %s failed: status %d: %s",
			apiPath, resp.StatusCode, truncateBody(body))
	}

	return body, nil
}

// ListRepositoriesRaw fetches all repositories and returns the untouched
// JSON response body. Callers that need to preserve every field (including
// ones unknown to this package) should use this instead of ListRepositories,
// because decoding into a struct would silently drop unknown fields.
func (c *Client) ListRepositoriesRaw(ctx context.Context) (json.RawMessage, error) {
	body, err := c.get(ctx, pathRepositories)
	if err != nil {
		return nil, fmt.Errorf("list repositories: %w", err)
	}

	return json.RawMessage(body), nil
}

// truncateBody shortens an error body for safe display.
func truncateBody(body []byte) string {
	detail := strings.TrimSpace(string(body))
	if len(detail) > maxErrorBodyLen {
		return detail[:maxErrorBodyLen] + "..."
	}

	return detail
}
