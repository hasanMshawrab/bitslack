Review recent changes and update all affected documentation.

## When to update CLAUDE.md
- A new adapter interface is added or an existing one changes signature
- A new webhook event type is supported
- The core flow changes (e.g. new steps in the thread-lookup or backfill logic)
- The Slack API usage changes (e.g. new API methods used)
- A new top-level directory or architectural concept is introduced

## When to update README.md
- A new public API field, type, or method is added to `Config`, `Client`, or any adapter interface
- Any code example in README (Quick Start, Configuration, adapter interfaces) no longer compiles or misrepresents the actual API
- A new event family or feature is added that consumers need to know about
- The `ConfigStore`, `ThreadStore`, or `Logger` interface signatures or parameter names change

## When to update SETUP.md
- The required Bitbucket or Slack credentials change (new fields, renamed fields)
- A new OAuth scope or API permission is required

## When to update testdata/webhooks/FIXTURES.md
- A new fixture file is added to `testdata/webhooks/`
- An existing fixture is modified in a way that affects what it tests
- A new intentional design decision is embedded in the fixtures (shared IDs, edge-case fields, etc.)

## Steps

1. Read `CLAUDE.md`, `README.md`, `SETUP.md`, and `testdata/webhooks/FIXTURES.md`.
2. For each file, check whether recent changes affect any section — pay special attention to code examples in README.md, which must match the actual Go API exactly (field names, parameter names, required vs optional).
3. Edit only the sections that are out of date — do not rewrite unaffected sections.
4. Keep descriptions factual and concise. Do not add implementation advice that belongs in code comments.
