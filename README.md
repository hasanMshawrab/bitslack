# bitslack

A Go library that receives Bitbucket webhook events and forwards them to Slack as **threaded messages** — all events for a given pull request appear as replies under a single opening message.

## Why

Bitbucket webhooks fire individual events with no threading. If your team uses Slack for PR notifications, you end up with a wall of disconnected messages. bitslack groups everything per PR into a single Slack thread: the opening message shows PR details, and every subsequent event (approval, comment, build status, merge) appears as a reply.

## Features

- Threads all PR events under one Slack message per pull request
- Backfills opening messages for PRs that predate the integration
- Updates the opening message when PR title or reviewers change
- Resolves build status events (commit hash → PR) via the Bitbucket API
- Maps Bitbucket usernames to Slack @mentions
- Zero third-party dependencies — standard library only

### Supported Events

**Pull Request** (default): `created`, `updated`, `approved`, `unapproved`, `fulfilled` (merged), `rejected` (declined), `comment_created`

**Build Status** (opt-in): `repo:commit_status_created`, `repo:commit_status_updated`

**Pipeline** (opt-in): `pipeline:span_created` — only `bbc.pipeline_run` spans; step/command/container spans are skipped silently

## Installation

```bash
go get github.com/hasanMshawrab/bitslack
```

Requires Go 1.24.2 or later.

## Quick Start

```go
package main

import (
    "context"
    "io"
    "log"
    "net/http"

    "github.com/hasanMshawrab/bitslack"
)

func main() {
    client, err := bitslack.New(bitslack.Config{
        SlackToken:        "xoxb-your-slack-bot-token",
        BitbucketUsername: "user@example.com",          // Atlassian account email
        BitbucketToken:    "your-bitbucket-token",
        ThreadStore:       myThreadStore,               // your implementation
        ConfigStore:       myConfigStore,               // your implementation
    })
    if err != nil {
        log.Fatal(err)
    }

    http.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
        eventKey := r.Header.Get("X-Event-Key")
        body, _ := io.ReadAll(r.Body)

        if err := client.Handler(r.Context(), eventKey, body); err != nil {
            log.Printf("handler error: %v", err)
        }

        w.WriteHeader(http.StatusOK) // always 200 to prevent Bitbucket retries
    })

    log.Fatal(http.ListenAndServe(":8080", nil))
}
```

## Adapters

The library is backend-agnostic. You provide implementations of three interfaces:

### ThreadStore

Stores the PR → Slack thread timestamp mapping. Needs TTL support (30-day expiry recommended).

```go
type ThreadStore interface {
    Get(ctx context.Context, prKey string) (ts string, ok bool, err error)
    Store(ctx context.Context, prKey string, ts string) error
}
```

Could be backed by Redis, Memcached, a database, or an in-memory map.

### ConfigStore

Provides two lookups: which Slack channel a repo posts to, and Bitbucket account ID → Slack user ID mapping for @mentions.

```go
type ConfigStore interface {
    GetChannel(repo string) (channelID string, ok bool)
    GetSlackUserID(accountID string) (slackID string, ok bool)
}
```

Could be backed by a config file, environment variables, or a database.

### Logger

Optional structured logging. Defaults to no-op if nil.

```go
type Logger interface {
    Info(msg string)
    Warn(msg string)
    Error(msg string)
}
```

## Configuration

```go
bitslack.Config{
    SlackToken:        "xoxb-...",                           // required
    BitbucketUsername: "user@example.com",                   // required (Atlassian account email)
    BitbucketToken:    "...",                                // required
    ThreadStore:       myThreadStore,                        // required
    ConfigStore:       myConfigStore,                        // required
    Logger:            myLogger,                             // optional (defaults to no-op)
    HTTPClient:        &http.Client{Timeout: 15*time.Second}, // optional (defaults to 10s)
    BitbucketBaseURL:  "https://api.bitbucket.org/2.0",     // optional (for testing)
    SlackBaseURL:      "https://slack.com/api",              // optional (for testing)

    // Which webhook event families to handle. Defaults to [EventFamilyPullRequest].
    EnabledEvents: []bitslack.EventFamily{
        bitslack.EventFamilyPullRequest,
        bitslack.EventFamilyPipeline, // opt-in; omit EventFamilyCommitStatus to avoid duplicates
    },

    // Controls how PR comment bodies are rendered in Slack.
    FormatOptions: bitslack.FormatOptions{
        CommentContent:            bitslack.CommentDisplaySummary, // Full (default), Summary, or None
        CommentSummaryLength:      200,                            // max display chars in summary mode
        ShowCommentLink:           true,                           // append "<url|View comment>"
        DistinguishCommentReplies: true,                           // label replies differently from top-level comments
    },
}
```

### Setup

See [SETUP.md](SETUP.md) for step-by-step instructions on:
- Creating a Slack app and getting the bot token
- Finding Slack user IDs and channel IDs
- Generating a Bitbucket API token
- Finding Bitbucket account IDs for user mapping
- Adding the Bitbucket webhook to your repository

## How It Works

1. Your server receives a Bitbucket webhook and passes the event key + payload to `client.Handler()`
2. The library parses the event and identifies the PR
3. It looks up the Slack thread timestamp for that PR via `ThreadStore`
4. If no thread exists, it creates an opening message with PR title, author, repo, and reviewers
5. It posts the event as a threaded reply

For `pullrequest:updated`, the opening message is edited in place (title/reviewer changes). For build status events, the library resolves the commit hash to a PR via the Bitbucket API.

## Development

```bash
make tools     # install dev tools (golangci-lint, go-arch-lint, goimports, golines)
make check     # full check: build + vet + lint + arch-lint + test
make test      # run all tests
make lint      # run golangci-lint
make arch-lint # run architecture dependency linter
make help      # show all available commands
```

See `examples/server/` for a complete reference implementation.

## License

[MIT](LICENSE)
