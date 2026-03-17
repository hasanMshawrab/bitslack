// Package bitslack routes Bitbucket webhook events to Slack as threaded messages.
// All events for a given pull request are posted as replies under a single
// opening message, keeping the Slack channel organised.
//
// Consumers embed the library by constructing a [Client] and calling
// [Client.Handler] from their HTTP server:
//
//	client, err := bitslack.New(bitslack.Config{
//	    SlackToken:        "xoxb-...",
//	    BitbucketUsername: "user@example.com",
//	    BitbucketToken:    "atlassian-api-token",
//	    ThreadStore:       myThreadStore,
//	    ConfigStore:       myConfigStore,
//	})
//	// in your HTTP handler:
//	client.Handler(r.Context(), r.Header.Get("X-Event-Key"), body)
//
// The library is backend-agnostic. Callers supply concrete implementations of
// [ThreadStore], [ConfigStore], and optionally [Logger].
package bitslack

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/hasanMshawrab/bitslack/internal/bitbucket"
	"github.com/hasanMshawrab/bitslack/internal/slack"
)

const defaultHTTPTimeout = 10 * time.Second

// EventFamily identifies a group of related Bitbucket webhook event keys.
type EventFamily string

const (
	// EventFamilyPullRequest enables pullrequest:* events.
	EventFamilyPullRequest EventFamily = "pullrequest"
	// EventFamilyCommitStatus enables repo:commit_status_* events.
	EventFamilyCommitStatus EventFamily = "commit_status"
	// EventFamilyPipeline enables pipeline:* events (reserved for future use).
	EventFamilyPipeline EventFamily = "pipeline"
)

// Config holds all dependencies needed to construct a Client.
type Config struct {
	// SlackToken is the Slack bot token (xoxb-...). Required.
	SlackToken string

	// BitbucketUsername is the Atlassian account email for API auth. Required.
	BitbucketUsername string

	// BitbucketToken is the Bitbucket API token. Required.
	// Used with BitbucketUsername for HTTP Basic auth.
	BitbucketToken string

	// BitbucketBaseURL overrides the Bitbucket API base URL.
	// Defaults to "https://api.bitbucket.org/2.0".
	// Set to an httptest server URL in tests.
	BitbucketBaseURL string

	// SlackBaseURL overrides the Slack API base URL.
	// Defaults to "https://slack.com/api".
	// Set to an httptest server URL in tests.
	SlackBaseURL string

	// ThreadStore is the adapter for PR-to-thread mapping. Required.
	ThreadStore ThreadStore

	// ConfigStore is the adapter for repo/user lookups. Required.
	ConfigStore ConfigStore

	// Logger for library messages. Defaults to no-op if nil.
	Logger Logger

	// HTTPClient for outbound API calls. Defaults to 10s timeout if nil.
	HTTPClient *http.Client

	// EnabledEvents declares which event families the client will process.
	// Defaults to [EventFamilyPullRequest] if nil or empty.
	// Consumers using Bitbucket Pipelines should set this to
	// [EventFamilyPullRequest, EventFamilyPipeline] and omit EventFamilyCommitStatus
	// to avoid duplicate notifications (Bitbucket Pipelines fires both).
	EnabledEvents []EventFamily
}

// Client is the bitslack engine. Safe for concurrent use.
type Client struct {
	threadStore     ThreadStore
	configStore     ConfigStore
	logger          Logger
	bbClient        *bitbucket.Client
	slackClient     *slack.Client
	enabledFamilies map[EventFamily]struct{}
}

// New validates the config and constructs a Client.
func New(cfg Config) (*Client, error) {
	if cfg.SlackToken == "" {
		return nil, errors.New("bitslack: SlackToken is required")
	}
	if cfg.BitbucketUsername == "" {
		return nil, errors.New("bitslack: BitbucketUsername is required")
	}
	if cfg.BitbucketToken == "" {
		return nil, errors.New("bitslack: BitbucketToken is required")
	}
	if cfg.ThreadStore == nil {
		return nil, errors.New("bitslack: ThreadStore is required")
	}
	if cfg.ConfigStore == nil {
		return nil, errors.New("bitslack: ConfigStore is required")
	}

	if cfg.BitbucketBaseURL == "" {
		cfg.BitbucketBaseURL = "https://api.bitbucket.org/2.0"
	}
	if cfg.SlackBaseURL == "" {
		cfg.SlackBaseURL = "https://slack.com/api"
	}
	cfg.BitbucketBaseURL = strings.TrimRight(cfg.BitbucketBaseURL, "/")
	cfg.SlackBaseURL = strings.TrimRight(cfg.SlackBaseURL, "/")

	if cfg.Logger == nil {
		cfg.Logger = noopLogger{}
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: defaultHTTPTimeout}
	}

	if len(cfg.EnabledEvents) == 0 {
		cfg.EnabledEvents = []EventFamily{EventFamilyPullRequest}
	}
	enabledFamilies := make(map[EventFamily]struct{}, len(cfg.EnabledEvents))
	for _, f := range cfg.EnabledEvents {
		enabledFamilies[f] = struct{}{}
	}

	return &Client{
		threadStore:     cfg.ThreadStore,
		configStore:     cfg.ConfigStore,
		logger:          cfg.Logger,
		enabledFamilies: enabledFamilies,
		bbClient: bitbucket.NewClient(
			cfg.BitbucketBaseURL,
			cfg.BitbucketUsername,
			cfg.BitbucketToken,
			bitbucket.WithHTTPClient(cfg.HTTPClient),
		),
		slackClient: slack.NewClient(
			cfg.SlackToken,
			slack.WithHTTPClient(cfg.HTTPClient),
			slack.WithBaseURL(cfg.SlackBaseURL),
		),
	}, nil
}
