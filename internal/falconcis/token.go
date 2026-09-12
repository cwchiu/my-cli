// Package falconcis provides a minimal client for the CrowdStrike Falcon
// Kubernetes Container Compliance API: OAuth2 token management, filtered
// GET requests with pagination, and the endpoints needed to export CIS
// benchmark violations.
package falconcis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// pathOAuth2 is the OAuth2 token endpoint path on the Falcon cloud.
//
// #nosec G101 -- this is an OAuth2 endpoint path, not a secret.
const pathOAuth2 = "/oauth2/token"

// tokenExpiryMargin is how long before the reported expiry a cached token
// is considered stale and refreshed, to avoid using a token that expires
// mid-request.
const tokenExpiryMargin = 30 * time.Second

// maxTokenRetries caps the retry attempts for transient token failures
// (HTTP 429 and 5xx).
const maxTokenRetries = 3

// tokenResponse is the success payload of POST /oauth2/token.
type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	ExpiresInSeconds int    `json:"expires_in"`
}

// tokenCache holds a cached OAuth2 access token and its expiry, guarded by
// a mutex so a single client is safe for concurrent use.
type tokenCache struct {
	mu     sync.Mutex
	token  string
	expiry time.Time
}

// valid reports whether the cached token exists and is not about to expire.
func (c *tokenCache) valid(now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.token != "" && now.Before(c.expiry)
}

// get returns the cached token and its expiry.
func (c *tokenCache) get() (string, time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.token, c.expiry
}

// set stores a token with the given remaining lifetime.
func (c *tokenCache) set(value string, lifetime time.Duration, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.token = value
	c.expiry = now.Add(lifetime - tokenExpiryMargin)
}

// invalidate clears the cached token, forcing the next call to re-authenticate.
func (c *tokenCache) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.token = ""
	c.expiry = time.Time{}
}

// fetchToken requests a fresh OAuth2 token using the client-credentials
// grant. Transient failures (429, 5xx) are retried with exponential backoff.
func (c *Client) fetchToken(ctx context.Context) (string, time.Duration, error) {
	form := strings.NewReader(fmt.Sprintf(
		"client_id=%s&client_secret=%s&grant_type=client_credentials",
		c.clientID, c.clientSecret,
	))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+pathOAuth2, form)
	if err != nil {
		return "", 0, fmt.Errorf("build token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var lastErr error

	for attempt := range maxTokenRetries {
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return "", 0, fmt.Errorf("send token request: %w", err)
		}

		body, readErr := io.ReadAll(resp.Body)
		if cerr := resp.Body.Close(); cerr != nil {
			_ = cerr // body already read; nothing useful to do
		}

		if readErr != nil {
			return "", 0, fmt.Errorf("read token response: %w", readErr)
		}

		shouldRetry, tokenErr := handleTokenStatus(ctx, resp.StatusCode, body, attempt, resp.Header.Get("Retry-After"))
		if shouldRetry {
			lastErr = tokenErr
			continue
		}

		if tokenErr != nil {
			return "", 0, tokenErr
		}

		parsed, err := parseTokenResponse(body)
		if err != nil {
			return "", 0, err
		}

		lifetime := time.Duration(parsed.ExpiresInSeconds) * time.Second
		if lifetime <= 0 {
			lifetime = defaultTokenLifetime
		}

		return parsed.AccessToken, lifetime, nil
	}

	return "", 0, fmt.Errorf("token request failed after %d attempts: %w", maxTokenRetries, lastErr)
}

func handleTokenStatus(ctx context.Context, status int, body []byte, attempt int, retryAfter string) (bool, error) {
	switch {
	case status == http.StatusTooManyRequests || status >= http.StatusInternalServerError:
		if !sleepBackoff(ctx, attempt, retryAfter) {
			return false, ctx.Err()
		}

		return true, tokenHTTPError(status, body)
	case status < http.StatusOK || status >= http.StatusMultipleChoices:
		return false, tokenHTTPError(status, body)
	default:
		return false, nil
	}
}

func parseTokenResponse(body []byte) (tokenResponse, error) {
	var parsed tokenResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return tokenResponse{}, fmt.Errorf("parse token response: %w", err)
	}

	if parsed.AccessToken == "" {
		return tokenResponse{}, errors.New("token response contains no access_token")
	}

	return parsed, nil
}

// defaultTokenLifetime is assumed when the API does not report expires_in.
const defaultTokenLifetime = 15 * time.Minute

// tokenHTTPError builds the error for a non-2xx token response. The client
// secret is scrubbed so it can never leak into output.
func tokenHTTPError(status int, body []byte) error {
	detail := strings.TrimSpace(string(body))
	if len(detail) > maxErrorBodyLen {
		detail = detail[:maxErrorBodyLen] + "..."
	}

	return fmt.Errorf("token request failed: status %d: %s", status, detail)
}
