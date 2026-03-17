package bitslack

import "context"

// ThreadStore persists the mapping from PR key to Slack thread timestamp.
// Implementations must handle TTL (30-day expiry) themselves.
type ThreadStore interface {
	// Get retrieves the Slack thread ts for a PR.
	// Returns ok=false with nil error when the key is not found.
	// Returns non-nil error only for infrastructure failures.
	Get(ctx context.Context, prKey string) (ts string, ok bool, err error)

	// Store persists the Slack thread ts for a PR.
	Store(ctx context.Context, prKey string, ts string) error
}

// ConfigStore provides repository-to-channel and username-to-Slack-ID lookups.
// The library only calls lookup methods — it never loads or caches data.
// Consumer controls their own data lifecycle.
type ConfigStore interface {
	// GetChannel maps a repository full name (e.g. "myworkspace/my-repo")
	// to a Slack channel ID. Returns ok=false when no mapping exists.
	GetChannel(repo string) (channelID string, ok bool)

	// GetSlackUserID maps a Bitbucket account ID to a Slack user ID (U...).
	// Returns ok=false when no mapping exists.
	GetSlackUserID(accountID string) (slackID string, ok bool)
}

// Logger provides structured logging. If nil is passed to New(),
// the library uses a no-op logger.
type Logger interface {
	Info(msg string)
	Warn(msg string)
	Error(msg string)
}

type noopLogger struct{}

func (noopLogger) Info(string)  {}
func (noopLogger) Warn(string)  {}
func (noopLogger) Error(string) {}
