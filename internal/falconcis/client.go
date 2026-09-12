package falconcis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// maxErrorBodyLen caps how much of an error response body is echoed back,
// so a misbehaving server cannot flood the terminal.
const maxErrorBodyLen = 512

// maxRequestRetries caps the retry attempts for transient request failures
// (HTTP 429 and 5xx).
const maxRequestRetries = 3

// Client is a minimal CrowdStrike Falcon container-compliance API client.
// It is safe for concurrent use.
type Client struct {
	baseURL      string
	clientID     string
	clientSecret string
	httpClient   *http.Client
	tokens       tokenCache
}

// NewClient returns a client for the Falcon cloud at baseURL, authenticating
// with the given API client credentials. The secret is never included in
// returned errors.
func NewClient(baseURL, clientID, clientSecret string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &Client{
		baseURL:      strings.TrimRight(baseURL, "/"),
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   httpClient,
	}
}

// accessToken returns a valid cached token, fetching a fresh one when the
// cache is empty or about to expire.
func (c *Client) accessToken(ctx context.Context) (string, error) {
	if c.tokens.valid(time.Now()) {
		token, _ := c.tokens.get()
		return token, nil
	}

	token, lifetime, err := c.fetchToken(ctx)
	if err != nil {
		return "", err
	}

	c.tokens.set(token, lifetime, time.Now())

	return token, nil
}

// getJSON performs an authenticated GET against apiPath with the given query
// parameters and decodes the JSON body into out. Transient failures are
// retried; a 401 triggers one token refresh and retry.
func (c *Client) getJSON(ctx context.Context, apiPath string, query url.Values, out any) error {
	endpoint := c.baseURL + apiPath
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	var lastErr error

	for attempt := range maxRequestRetries {
		token, err := c.accessToken(ctx)
		if err != nil {
			return err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return fmt.Errorf("build request %s: %w", apiPath, err)
		}

		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("send request %s: %w", apiPath, err)
		}

		body, readErr := io.ReadAll(resp.Body)
		if cerr := resp.Body.Close(); cerr != nil {
			_ = cerr // body already read; nothing useful to do
		}

		if readErr != nil {
			return fmt.Errorf("read response %s: %w", apiPath, readErr)
		}

		handlerErr := c.handleJSONResponse(ctx, apiPath, resp.StatusCode, body, attempt)
		switch {
		case errors.Is(handlerErr, errRetryable):
			lastErr = fmt.Errorf("request %s failed: status %d: %s",
				apiPath, resp.StatusCode, truncateBody(body))

			// Retry the request after refreshing the token or backing off.
			continue
		case handlerErr != nil:
			return handlerErr
		}

		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("parse response %s: %w", apiPath, err)
		}

		return nil
	}

	return fmt.Errorf("request %s failed after %d attempts: %w", apiPath, maxRequestRetries, lastErr)
}

var errRetryable = errors.New("retryable")

func (c *Client) handleJSONResponse(ctx context.Context, apiPath string, status int, body []byte, attempt int) error {
	switch {
	case status == http.StatusUnauthorized:
		c.tokens.invalidate()
		return errRetryable
	case status == http.StatusTooManyRequests || status >= http.StatusInternalServerError:
		if !sleepBackoff(ctx, attempt, "") {
			return ctx.Err()
		}

		return errRetryable
	case status < http.StatusOK || status >= http.StatusMultipleChoices:
		return fmt.Errorf("request %s failed: status %d: %s",
			apiPath, status, truncateBody(body))
	default:
		return nil
	}
}

// truncateBody shortens an error body for safe display.
func truncateBody(body []byte) string {
	detail := strings.TrimSpace(string(body))
	if len(detail) > maxErrorBodyLen {
		return detail[:maxErrorBodyLen] + "..."
	}

	return detail
}

// sleepBackoff waits before the next retry attempt: the server-provided
// Retry-After seconds when present, otherwise exponential backoff. It
// returns false when ctx is cancelled while waiting.
func sleepBackoff(ctx context.Context, attempt int, retryAfter string) bool {
	delay := time.Duration(1<<uint(attempt)) * time.Second

	if retryAfter != "" {
		if seconds, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && seconds > 0 {
			delay = time.Duration(seconds) * time.Second
		}
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
