package slack

import (
	"net/http"
)

// Client is a Slack Web API client.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// Option configures the Client.
type Option func(*Client)

// WithHTTPClient overrides the default [http.Client].
func WithHTTPClient(c *http.Client) Option {
	return func(cl *Client) { cl.httpClient = c }
}

// WithBaseURL overrides the default Slack API base URL.
func WithBaseURL(url string) Option {
	return func(cl *Client) { cl.baseURL = url }
}

// NewClient constructs a Slack API client.
func NewClient(token string, opts ...Option) *Client {
	c := &Client{
		baseURL:    "https://slack.com/api",
		token:      token,
		httpClient: http.DefaultClient,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}
