# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Go module: `github.com/hasanMshawrab/bitslack`
Go version: 1.24.2

A Go **library** that receives Bitbucket webhook events and forwards them to Slack as **threaded messages** — all events for a given PR appear as replies under the original message rather than as new top-level messages.

Consumers embed this library into their own server and wire it up by providing adapter implementations. The library defines interfaces; callers supply the concrete backends.

## Common Commands

```bash
make check                            # Full check: build + vet + lint + arch-lint + test
make build                            # Build all packages
make test                             # Run all tests (excluding e2e)
make test-unit                        # Run unit tests only (internal/)
make test-integration                 # Run integration tests (handler)
make lint                             # Run golangci-lint
make lint-fix                         # Run golangci-lint with auto-fix
make arch-lint                        # Run architecture dependency linter
make fmt                              # Format code (goimports + golines)
make tools                            # Install pinned dev tools
go test ./internal/... -run TestName  # Run a single test by name
```

## File System Structure

```
bitslack/
├── bitslack.go          # Public API: Config struct, New() constructor, Client
├── adapter.go           # Interface definitions: ThreadStore, ConfigStore, Logger
├── handler.go           # Client.Handler(ctx, eventKey, payload) — core flow orchestration
├── handler_test.go      # Integration tests: full flow with mocks + httptest stubs
│
├── internal/
│   ├── bitbucket/       # Bitbucket REST API client (PR resolution from commit hash)
│   ├── slack/           # Slack API client (chat.postMessage, chat.update)
│   ├── event/           # Webhook event types, JSON parsing, routing by event key
│   ├── format/          # Slack message formatting (opening message, reply text)
│   └── testutil/        # Mock adapters: MockThreadStore, MockConfigStore, MockLogger
│
├── examples/
│   └── server/
│       ├── main.go          # Reference server wired with in-memory adapters
│       ├── docker-compose.yml  # Placeholder for future E2E testing
│       └── e2e_test.go      # E2E test scaffold (//go:build e2e)
│
├── testdata/
│   └── webhooks/
│       ├── FIXTURES.md          # Explains fixture design decisions
│       ├── pullrequest/         # One JSON file per pullrequest:* event
│       └── commit_status/       # One JSON file per repo:commit_status_* event
│
├── .claude/
│   ├── commands/        # Custom slash commands: /plan, /create-issue, /open-pr, /update-docs
│   └── skills/          # Superpowers skills: committing
│
├── .github/
│   ├── workflows/
│   │   ├── ci.yml           # CI: test (Go 1.24 + stable, ubuntu + macos), lint, arch-lint, govulncheck
│   │   └── release.yml      # Auto-create GitHub Release on version tags
│   ├── ISSUE_TEMPLATE/  # bug_report.md, feature_request.md
│   └── pull_request_template.md
│
├── .golangci.yml        # golangci-lint v2.8.0 config (75+ linters)
├── .go-arch-lint.yml    # Architecture dependency rules
├── Makefile             # Build, test, lint, arch-lint, fmt, tools targets
├── README.md            # Usage guide and API documentation
├── SETUP.md             # Step-by-step credential and ID setup guide for consumers
├── LICENSE              # MIT
├── .plan/               # Local planning scratch space — gitignored
├── .gitignore
├── go.mod
└── CLAUDE.md
```

### Key boundaries

- **Public surface** (`bitslack.go`, `adapter.go`, `handler.go`) — everything a consumer needs to import and wire up. Keep this minimal and stable.
- **`internal/`** — all implementation details. Nothing in `internal/` is importable by consumers. Each sub-package has a single clear responsibility.
- **`examples/`** — the only place that may use concrete third-party adapter implementations. The core library never depends on them.
- **`testdata/`** — Go test convention; files here are accessible via `os.Open("testdata/...")` in tests without any path manipulation.

## Architecture

### Adapter / Plugin Model

The library is backend-agnostic. Consumers construct the core engine by injecting adapters that satisfy these interfaces:

- **ConfigStore** — provides repo→channel mapping and Bitbucket account ID→Slack user ID lookup. The library only calls lookup methods (`GetChannel`, `GetSlackUserID`); it never loads or caches data itself. The consumer controls their own data lifecycle — preloading, caching, or fetching on demand is entirely up to them. `GetSlackUserID` accepts a Bitbucket `account_id` (not `nickname`) because `account_id` is stable across webhook payloads and REST API responses, whereas `nickname` is user-editable and inconsistent between the two sources.
- **ThreadStore** — stores and retrieves the PR→Slack thread `ts` mapping. Needs TTL support (30-day expiry per PR). Could be backed by Redis, Memcached, an in-process map, etc.
- **Logger** — structured logging with three methods: `Info(message string)`, `Warn(message string)`, `Error(message string)`. If none is provided, the library defaults to a no-op logger.

The library ships no concrete adapter implementations — those live in the caller's codebase or in separate companion packages.

### Opening Message Format

The first message posted for a PR (either on `pullrequest:created` or backfilled) must display:
- **Repository** name
- **PR title**
- **PR number** — rendered as a clickable Slack link (`<URL|#id>`)
- **Author** — Slack @mention if an account ID mapping exists, otherwise plain Bitbucket nickname
- **Reviewers** — each as a Slack @mention if mapped, otherwise plain nickname

Each field appears on its own line with a bold label (e.g. `*PR Title:* …`). The metadata fields (repository, title, PR number) are grouped in one Block Kit section; the people fields (author, reviewers) in a second section.

### Opening Message Updates

The opening message is a live document — it is edited (via `chat.update`) to stay in sync with PR state changes:

- `pullrequest:updated` — if the title or reviewer list changed, update the opening message in place
- **Adding a reviewer** — edit the message to add their @mention; Slack will automatically notify them (no separate notification needed)
- **Removing a reviewer** — edit the message to remove their @mention; Slack will not notify them of the removal. If they have not yet engaged with the thread (no reply, no click-through), they will stop receiving future thread notifications. If they have already engaged, Slack marks them as a thread follower and they will continue to receive updates regardless — this is a known Slack limitation.

### Core Flow

1. Caller's HTTP server receives a Bitbucket webhook and calls `client.Handler(ctx, eventKey, payload)`.
2. The library parses the event and identifies the PR (see "Build Status Events" below).
3. Look up the Slack channel via `ConfigStore.GetChannel(repo)`.
4. Look up the thread `ts` for that PR via `ThreadStore`.
5. If no `ts` exists (new PR **or** an existing PR that predates the integration):
   - Call the Bitbucket API to fetch full PR details (`GET /repositories/{workspace}/{repo}/pullrequests/{id}`)
   - Post a synthetic opening message to Slack → store the returned `ts` via `ThreadStore`
   - If either step fails, log the error and drop the event gracefully (no panic, no partial state)
6. Event-specific behavior:
   - `pullrequest:created` — the opening message IS the notification; no separate reply is posted
   - `pullrequest:updated` — edit the opening message via `chat.update`; no reply posted
   - All other events — post as a threaded reply using `thread_ts`

### Error Handling

- **Hard errors** (malformed JSON for recognized event keys) — returned to the caller.
- **Soft errors** (API failures, store failures, missing channel) — logged and swallowed, returning nil. This ensures the consumer can always respond 200 to Bitbucket, preventing retry storms.

### Build Status Events

Bitbucket `repo:commit_status_created` / `repo:commit_status_updated` events do **not** include a PR ID — only a commit hash. To resolve the PR, call the Bitbucket API:

```
GET /repositories/{workspace}/{repo}/commit/{hash}/pullrequests
```

### Slack Integration

Uses `chat.postMessage` with the `thread_ts` field to post replies into an existing thread.

The caller provides tokens at construction time — the library has no opinion on how they are stored or retrieved:

```go
client := bitslack.New(bitslack.Config{
    SlackToken:       "xoxb-...",
    BitbucketUsername: "user@example.com",
    BitbucketToken:   "atlassian-api-token",
    // adapters...
})
```

- **Slack**: Bot token (`xoxb-...`). Required OAuth scopes: `chat:write`. Add `chat:write.public` if the bot needs to post to channels it hasn't been explicitly invited to.
- **Bitbucket**: Atlassian API token with `read:repository:bitbucket` and `read:pullrequest:bitbucket` scopes. Uses HTTP Basic auth (username + token).

### Testing Strategy

All tests run offline with zero external dependencies:

- **Unit** — test event parsing (`internal/event/`), message formatting (`internal/format/`), and API client behavior (`internal/slack/`, `internal/bitbucket/`) using `httptest` stubs.
- **Integration** — `handler_test.go` tests the full public API flow using real fixture JSON files, mock adapters (`internal/testutil/`), and `httptest` servers for Slack and Bitbucket APIs. This is the most important test layer.
- **E2E** — scaffolded in `examples/server/e2e_test.go` behind `//go:build e2e` tag for future use with the docker-compose stack.

### Supported Webhook Events

**Pull Request**
- `pullrequest:created`
- `pullrequest:updated`
- `pullrequest:approved`
- `pullrequest:unapproved`
- `pullrequest:fulfilled` (merged)
- `pullrequest:rejected` (declined)
- `pullrequest:comment_created`

**Build Status**
- `repo:commit_status_created`
- `repo:commit_status_updated`
