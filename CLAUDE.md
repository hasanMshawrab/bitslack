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
├── bitslack.go          # Public API: Config struct, FormatOptions, CommentDisplay, New() constructor, Client
├── adapter.go           # Interface definitions: ThreadStore, ConfigStore, Logger
├── handler.go           # Client.Handler(ctx, eventKey, payload) — core flow orchestration
├── handler_test.go      # Integration tests: full flow with mocks + httptest stubs
│
├── internal/
│   ├── bitbucket/       # Bitbucket REST API client (PR/commit/branch lookups)
│   ├── slack/           # Slack API client (chat.postMessage, chat.update)
│   ├── event/           # Webhook event types, JSON parsing, routing by event key
│   ├── format/          # Slack message formatting (opening message, reply text)
│   │   └── markdown/    # Bitbucket markdown → Slack mrkdwn converter + smart truncation
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
│       ├── commit_status/       # One JSON file per repo:commit_status_* event
│       └── pipeline/            # OTel span fixtures for pipeline:span_created
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

The first message posted for a PR (either on `pullrequest:created` or backfilled) renders in two Block Kit sections:

**Header section** (one line):
```
🔀 *[{repo}] Pull Request <{url}|#{id}>* • {source branch} → {destination branch}
```

**Fields section** (indented 4 spaces):
```
    *Title:* {pr title}
    *Status:* Open | Merged | Closed
    *Author:* {mention}
    *Reviewers:* ✅ {approved mention} • {pending mention}   ← ✅ only for approved reviewers
    *Also approved:* {mention}                               ← only when non-reviewer participants approved
    *Ticket:* <url|View Ticket>                             ← only when a ClickUp URL is in description
```

- `*Repository:*` labeled field is omitted — the repo name is embedded in the header.
- Each reviewer shows `✅` if they appear in `participants` with `role="REVIEWER"` and `approved=true`; plain name otherwise.
- `*Also approved:*` lists participants with `role="PARTICIPANT"` and `approved=true`. Omitted when no such participants exist.
- `*Ticket:*` is shown only when the PR description contains a `https://app.clickup.com/t/…` URL. The first match is used.
- `PullRequest.Participants` (parsed from the `participants` array in both webhook payloads and Bitbucket API responses) drives the approval markers.

### Opening Message Updates

The opening message is a live document — it is edited (via `chat.update`) to stay in sync with PR state changes:

- `pullrequest:updated` — if the title or reviewer list changed, update the opening message in place
- `pullrequest:approved` / `pullrequest:unapproved` — after posting the thread reply, fetch the full PR from the Bitbucket API (`GET /repositories/{workspace}/{repo}/pullrequests/{id}`) and re-render the opening message to reflect current approval state. Fetching from the API is required because the webhook payload only contains the single approval actor, not the full participant list.
- `pullrequest:fulfilled` / `pullrequest:rejected` — after posting the thread reply, fetch the full PR and call `chat.update` to flip `*Status:*` to `Merged` or `Closed`.
- **Adding a reviewer** — edit the message to add their @mention; Slack will automatically notify them (no separate notification needed)
- **Removing a reviewer** — edit the message to remove their @mention; Slack will not notify them of the removal. If they have not yet engaged with the thread (no reply, no click-through), they will stop receiving future thread notifications. If they have already engaged, Slack marks them as a thread follower and they will continue to receive updates regardless — this is a known Slack limitation.

### Comment Formatting

Comment reply formatting is controlled by `Config.FormatOptions` (`FormatOptions` struct):

- **`DistinguishCommentReplies bool`** — when `true`, a comment that has a `parent.id` (i.e. a reply to another comment) is labelled `"replied to a comment"` instead of `"commented"`. Default `false` — both show `"commented"`.
- **`CommentContent CommentDisplay`** — controls how much of the comment body is shown:
  - `CommentDisplayFull` (default, 0) — full body shown inline, converted from Bitbucket markdown to Slack mrkdwn
  - `CommentDisplaySummary` — converted body truncated to `CommentSummaryLength` display characters with `…` appended
  - `CommentDisplayNone` — body omitted entirely
- **`CommentSummaryLength int`** — max display characters for summary mode. Slack mrkdwn link tokens (`<url|text>`) count by the display text length, not the raw token length. Zero uses the default (200).
- **`ShowCommentLink bool`** — when `true`, appends `<url|View comment>` (Slack mrkdwn). Default `false` — no link.

`Comment.ParentID` (parsed from `comment.parent.id` in the webhook payload) is `0` for top-level comments and non-zero for replies.

**Markdown conversion** (`internal/format/markdown`): comment bodies are converted from Bitbucket CommonMark/extensions to Slack mrkdwn before display. Conversion rules:
- `**bold**` → `*bold*`
- `_**bold italic**_` / `**_bold italic_**` → `*_bold italic_*` (combined patterns handled before individual bold to prevent marker bleed)
- `~~strike~~` → `~strike~`
- `[text](url)` → `<url|text>`
- `![alt](url)` → `<url|📎 alt>`
- `@{account_id}` → `<@slackID>` (resolved) or `@account_id` (unresolved)
- Headings → `*text*`
- Unordered list items → `• item`
- Dividers (`---`) → stripped
- Tables → reformatted as an aligned plain-text table wrapped in a ` ``` ` code block; bold markers stripped from header cells; separator row uses dashes matching column width
- Ordered lists and inline/fenced code pass through unchanged

### Core Flow

1. Caller's HTTP server receives a Bitbucket webhook and calls `client.Handler(ctx, eventKey, payload)`.
2. The library checks the event's family against `Config.EnabledEvents`. If the family is not enabled, log a `Warn` and return nil (soft-drop).
3. The library parses the event and identifies the PR (see "Build Status Events" below).
4. Look up the Slack channel via `ConfigStore.GetChannel(repo)`.
5. Look up the thread `ts` for that PR via `ThreadStore`.
6. If no `ts` exists (new PR **or** an existing PR that predates the integration):
   - Call the Bitbucket API to fetch full PR details (`GET /repositories/{workspace}/{repo}/pullrequests/{id}`)
   - Post a synthetic opening message to Slack → store the returned `ts` via `ThreadStore`
   - If either step fails, log the error and drop the event gracefully (no panic, no partial state)
7. Event-specific behavior:
   - `pullrequest:created` — the opening message IS the notification; no separate reply is posted
   - `pullrequest:updated` — edit the opening message via `chat.update`; no reply posted
   - `pullrequest:approved` / `pullrequest:unapproved` — post a threaded reply, then fetch the full PR from Bitbucket and call `chat.update` to refresh the opening message with current approval state
   - `pullrequest:fulfilled` / `pullrequest:rejected` — post a threaded reply, then call `chat.update` to update `*Status:*` to `Merged` or `Closed`
   - All other PR and commit_status events — post as a threaded reply using `thread_ts`
   - Pipeline events — see "Pipeline Events" below

### Error Handling

- **Hard errors** (malformed JSON for recognized event keys) — returned to the caller.
- **Soft errors** (API failures, store failures, missing channel) — logged and swallowed, returning nil. This ensures the consumer can always respond 200 to Bitbucket, preventing retry storms.

### Build Status Events

Bitbucket `repo:commit_status_created` / `repo:commit_status_updated` events do **not** include a PR ID — only a commit hash. To resolve the PR, call the Bitbucket API:

```
GET /repositories/{workspace}/{repo}/commit/{hash}/pullrequests
```

### Pipeline Events

`pipeline:span_created` delivers an OpenTelemetry trace. Only `bbc.pipeline_run` spans are processed; `bbc.pipeline_step`, `bbc.command`, and `bbc.container` spans are silently skipped.

**Repository resolution:** Real Bitbucket OTel payloads omit `pipeline.repository.full_name` and only include `pipeline.repository.uuid` and `pipeline.account.uuid`. When `full_name` is absent, the handler resolves the repository via the Bitbucket API:
```
GET /repositories/{accountUUID}/{repoUUID}
```
The `account.uuid` is used as the workspace identifier (Bitbucket accepts UUIDs in place of slugs). The resolved `full_name` is then used for all subsequent channel and thread lookups.

**Result values:** OTel `pipeline.state.result.name` uses different values than the REST API. The mapping is: `COMPLETE` → ✅, `FAILED` → ❌, `ERROR` → 🔴, `STOPPED` → ⏹. These differ from `repo:commit_status_*` which uses `SUCCESSFUL`/`FAILED`/`INPROGRESS`.

**Run number:** The `pipeline_run.run_number` OTel attribute is delivered as a `stringValue` (not `intValue`) and is parsed via `strconv.Atoi`.

**Run URL:** The `pipeline_run.url` span attribute is used directly as the link in the Slack message.

**Duration:** Computed from OTel `startTimeUnixNano` and `endTimeUnixNano` string fields on the span. Formatted as `Xs` for runs under a minute, `Xm Ys` for longer runs.

**Message format:** Pipeline results are formatted as a header line followed by indented per-step lines:
```
⚙️ *[{repo}] Pipeline <{url}|#{run}>* • {branch} • {trigger label} — {emoji} {result text} • {duration}
    {step emoji} {step name or <url|step name>} • {step duration}
    ...
```
Trigger labels: `PUSH` → `automatic trigger`, `MANUAL` → `manual trigger`, `SCHEDULE` → `scheduled trigger`, anything else → lowercased + ` trigger`.

Result text: `COMPLETE` → `Passed`, `FAILED` → `Failed`, `ERROR` → `Error`, `STOPPED` → `Stopped`.

**Step breakdown:** After the debounce delay, the handler calls `GET /repositories/{workspace}/{repo}/pipelines/{uuid}/steps/` to fetch step details. Each step is rendered on its own line below the header. Step result emojis: `✅` SUCCESSFUL, `❌` FAILED, `🔴` ERROR, `🛑` STOPPED, `⏭` NOT_RUN. Failed and errored steps are hyperlinked to the Bitbucket UI; other steps show a plain name.

**Debounce:** `Handler` returns nil immediately. The first delivery of a `pipeline_run.uuid` schedules `processPipelineRun` via `time.AfterFunc` after `Config.PipelineDebounce` (default 3 s). A `sync.Mutex`-protected `map[string]struct{}` tracks in-flight UUIDs — subsequent deliveries of the same UUID are silently dropped until the goroutine cleans up. The goroutine uses `context.Background()` because the HTTP request context has expired by the time the timer fires.

**Manual-stop suppression:** If `FormatOptions.SkipManuallyStoppedPipelines` is `true` and all steps are `STOPPED` and the trigger is `MANUAL`, no Slack message is posted. Default `false` — all pipeline results are posted.

PR linkage for pipeline events:
- If `pipeline.target.ref_type = BRANCH`: call the Bitbucket API to find the open PR for that branch:
  ```
  GET /repositories/{workspace}/{repo}/pullrequests?q=source.branch.name="{branch}"&state=OPEN
  ```
  This avoids the shared-commit-hash ambiguity of commit_status events, where a hash present on multiple branches could match the wrong PR thread.
- If a PR is found: post the pipeline result as a threaded reply (backfilling opening message if needed). The backfill path calls `GetPullRequest` (single-resource endpoint) after `GetOpenPRForBranch`, because Bitbucket's list endpoint omits the `reviewers` field — the full PR details are required to build a complete opening message.
- If no PR is found, or `ref_type = TAG`: post a standalone top-level message to the repo channel.

Consumers using Bitbucket Pipelines should enable `EventFamilyPipeline` and omit `EventFamilyCommitStatus` — Bitbucket Pipelines fires commit statuses too, and enabling both produces duplicate Slack messages for the same pipeline run.

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

### Event Families and Opt-In

Consumers declare which event families to handle via `Config.EnabledEvents`. Defaults to `[EventFamilyPullRequest]` if unset. Events from disabled families are soft-dropped (Warn log, nil return).

```go
client := bitslack.New(bitslack.Config{
    // ...
    EnabledEvents: []bitslack.EventFamily{
        bitslack.EventFamilyPullRequest,
        bitslack.EventFamilyCommitStatus,
    },
})
```

Consumers using Bitbucket Pipelines should enable `EventFamilyPipeline` and omit `EventFamilyCommitStatus` — Bitbucket Pipelines fires both, so enabling both produces duplicate notifications.

### Supported Webhook Events

**Pull Request** (`EventFamilyPullRequest` — default)
- `pullrequest:created`
- `pullrequest:updated`
- `pullrequest:approved`
- `pullrequest:unapproved`
- `pullrequest:fulfilled` (merged)
- `pullrequest:rejected` (declined)
- `pullrequest:comment_created`

**Build Status** (`EventFamilyCommitStatus` — opt-in)
- `repo:commit_status_created`
- `repo:commit_status_updated`

**Pipeline** (`EventFamilyPipeline` — opt-in)
- `pipeline:span_created` (only `bbc.pipeline_run` spans; step/command/container spans are skipped)
