// Package bitbucket provides a minimal Bitbucket REST API client used internally
// by bitslack to resolve commit hashes and branch names to pull requests.
package bitbucket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// ErrNotFound is returned when the Bitbucket API returns 404.
var ErrNotFound = errors.New("bitbucket: not found")

// Client is an authenticated Bitbucket REST API v2 client.
type Client struct {
	baseURL    string
	username   string
	token      string
	httpClient *http.Client
}

// Option configures the Client.
type Option func(*Client)

// WithHTTPClient overrides the default [http.Client].
func WithHTTPClient(c *http.Client) Option {
	return func(cl *Client) { cl.httpClient = c }
}

// NewClient constructs a Bitbucket API client.
func NewClient(baseURL, username, token string, opts ...Option) *Client {
	c := &Client{
		baseURL:    baseURL,
		username:   username,
		token:      token,
		httpClient: http.DefaultClient,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// get performs an authenticated GET, decodes JSON into dst.
func (c *Client) get(ctx context.Context, path string, dst any) error {
	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("bitbucket: create request: %w", err)
	}
	req.SetBasicAuth(c.username, c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("bitbucket: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("bitbucket: unexpected status %d", resp.StatusCode)
	}

	if decErr := json.NewDecoder(resp.Body).Decode(dst); decErr != nil {
		return fmt.Errorf("bitbucket: decode response: %w", decErr)
	}
	return nil
}
