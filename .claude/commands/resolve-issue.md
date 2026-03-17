Resolve a GitHub issue end-to-end: read the issue, implement the fix, update docs, and open a PR.

The argument is an issue number (e.g. `9`) or a full GitHub issue URL.

## Steps

### 1. Sync main

```bash
git checkout main && git pull origin main
```

### 2. Read the issue

```bash
gh issue view <number> --repo hasanMshawrab/bitslack
```

Extract:
- The problem description
- Expected vs actual behaviour
- Any reproduction steps or relevant file references

### 3. Create a branch

Name the branch after the issue type and a short slug derived from the issue title:
- Bug → `fix/<slug>`
- Feature → `feat/<slug>`
- Refactor → `refactor/<slug>`

```bash
git checkout -b <branch-name>
```

### 4. Implement the fix

- Read all relevant source files before editing any of them.
- Write or update tests first where the issue description makes the expected behaviour clear.
- Follow the existing patterns in `handler_test.go` for integration tests and `internal/*/` for unit tests.
- Run `make check` when done. All checks must pass before proceeding.

```bash
make check
```

### 5. Commit

Write one or more commits with conventional-commit prefixes (`fix:`, `feat:`, etc.).
Include `Closes #<number>` in the commit message body of the primary commit.

```bash
git commit -m "$(cat <<'EOF'
<prefix>: <short description>

<why this fixes the issue>

Closes #<number>

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

### 6. Update documentation

Run the `/update-docs` command to review and update `CLAUDE.md` and `testdata/webhooks/FIXTURES.md` for any affected sections.

### 7. Push

```bash
git push -u origin <branch-name>
```

### 8. Open a PR

Run the `/open-pr` command to create the pull request using the standard project template, referencing the issue number.

## Notes
- Do not skip `make check` — linting and arch-lint failures block CI.
- Do not open a PR to `main` for breaking adapter interface changes without prior discussion in the issue.
- Do not create a GitHub Release; releases are handled separately via version tags.
