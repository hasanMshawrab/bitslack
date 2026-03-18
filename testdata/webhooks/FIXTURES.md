# Test Fixture Design

This document explains the intentional design decisions in the webhook payload fixtures, so tests can be written with full awareness of what each file represents and why.

## Shared Identifiers

All fixtures use consistent fake data to allow testing cross-event scenarios:

| Entity | Field | Value |
|--------|-------|-------|
| PR | `id` | `42` |
| PR source commit | `hash` | `b7f6f6ef4c59` |
| PR destination branch | `name` | `main` |
| Repository | `full_name` | `myworkspace/my-repo` |
| Author | `nickname` | `janeauthor` |
| Author | `account_id` | `5b10a2844c20165700ede22h` |
| Reviewer 1 | `nickname` | `bobreviewer` |
| Reviewer 1 | `account_id` | `5b10a2844c20165700ede23i` |
| Reviewer 2 | `nickname` | `alicereviewer` |
| Reviewer 2 | `account_id` | `5b10a2844c20165700ede24j` |

`account_id` values are the stable Bitbucket identifiers used as keys in `ConfigStore.GetSlackUserID`. They are consistent across webhook payloads and REST API responses, unlike `nickname` which is user-editable. `MockConfigStore` in `internal/testutil` is keyed by these account IDs.

The commit status payloads reference the same commit hash (`b7f6f6ef4c59`) as the PR's source commit. This makes them suitable for testing the PR-resolution flow: given a commit hash from a build status event, call the Bitbucket API and resolve it back to PR `42`.

The pipeline payloads for `span_created_successful.json` and `span_created_failed.json` use `feature/add-feature-x` as the target branch — the same source branch as PR `42` in the pull request fixtures. This allows testing the branch→PR linkage path without a separate mock PR. `span_created_no_pr.json` uses `main` as the target branch, which has no open PR in the test harness, exercising the standalone message path.

All three pipeline fixtures omit `pipeline.repository.full_name` (absent from real Bitbucket OTel payloads) and instead include `pipeline.repository.uuid` (`{aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee}`) and `pipeline.account.uuid` (`{ffffffff-0000-1111-2222-333333333333}`). The test harness mock handles `GET /repositories/{accountUUID}/{repoUUID}` and returns `myworkspace/my-repo`, matching the channel and thread store seeds. Each fixture also includes a `pipeline_run.url` attribute used directly as the Slack message link.

## Per-File Notes

### `pullrequest/created.json`
Baseline payload. One reviewer (Bob). Use this to test:
- First-ever event for a PR → opening message is posted and `ts` is stored
- Opening message content: title, author, repo, reviewers

### `pullrequest/updated.json`
Title changed to `"Add feature X (updated)"` and a second reviewer (Alice) added. Use this to test:
- Opening message is edited via `chat.update` to reflect the new title
- Alice's `@mention` is added to the opening message (Slack will notify her automatically)
- Bob remains in the reviewers list (no spurious re-notification)

### `pullrequest/approved.json`
Bob's participant entry has `"approved": true` and `"state": "approved"`. Use this to test:
- Approval event is posted as a thread reply, not a new top-level message
- The actor and the approving user are the same person (Bob)
- After the reply, the handler fetches the full PR and calls `chat.update` to refresh the opening message with current approval state (Bob's reviewer entry should show ✅)

### `pullrequest/unapproved.json`
Bob's participant entry reverts to `"approved": false` and `"state": null`. Use this to test:
- Unapproval is posted as a thread reply
- After the reply, the handler fetches the full PR and calls `chat.update` to remove Bob's ✅ from the opening message
- Structurally identical to `approved.json` except for the participant state — both share the same `approval` wrapper field

### `pullrequest/fulfilled.json`
PR state is `"MERGED"`. Includes `merge_commit` (`764413d85e29`) and `closed_by` (Jane). Use this to test:
- Merge event is posted as a thread reply
- After the reply, the handler fetches the full PR and calls `chat.update` to set `*Status:*` to `Merged`
- `close_source_branch: true` is set — relevant if the message surface area includes branch cleanup info

### `pullrequest/rejected.json`
PR state is `"DECLINED"`. Includes a `reason` field and `closed_by` (Bob). Use this to test:
- Decline event is posted as a thread reply
- After the reply, the handler fetches the full PR and calls `chat.update` to set `*Status:*` to `Closed`
- `reason` is present — it should be surfaced in the Slack message if non-empty
- The actor is a reviewer, not the author (Bob declined Jane's PR)

### `pullrequest/comment_created.json`
Includes an `inline` comment on `pkg/handler/handler.go` line 42. Bob's participant role has shifted to `"PARTICIPANT"` (not `"REVIEWER"`). The `comment` object has no `parent` field, so `Comment.ParentID` will be `0` (top-level comment). Use this to test:
- Comment event is posted as a thread reply
- `comment.content.raw` is the plain text to surface; `html` can be ignored
- Inline comments include a file path and line number — useful if the message links to the diff
- `ParentID == 0` exercises the top-level comment path in `DistinguishCommentReplies` logic

### `commit_status/created.json`
Build state is `"INPROGRESS"`. No PR ID in the payload — only commit hash `b7f6f6ef4c59`. Use this to test:
- The PR-resolution path: hash → Bitbucket API → PR `42`
- If the thread `ts` already exists (from a prior PR event), the build notification is posted as a reply
- If no `ts` exists yet (build triggered before any PR event), the backfill path is exercised: fetch PR details, post opening message, then post build status as reply

### `commit_status/updated.json`
Same build (`my-ci-tool`, commit `b7f6f6ef4c59`), state now `"SUCCESSFUL"`. Use this to test:
- A follow-up build event on the same commit is correctly threaded under the same PR message
- `updated_on` differs from `created_on` — both timestamps are present for either display or filtering logic

### `pipeline/span_created_successful.json`
`bbc.pipeline_run` span. Target branch `feature/add-feature-x` (same as PR `42`'s source branch), result `COMPLETE` (OTel value for a successful run), trigger `PUSH`, run number `5` (delivered as `stringValue`, matching real Bitbucket OTel payloads). Uses UUID attributes instead of `full_name`. Use this to test:
- Repository UUID resolution: `GetRepository` is called with the account and repo UUIDs, returning `myworkspace/my-repo`
- Branch→PR lookup: Bitbucket API is called with the branch name, returns PR `42`
- Pipeline result `COMPLETE` maps to `✅` emoji
- Pipeline result is posted as a threaded reply under PR `42`'s thread
- Backfill path: if no thread exists for PR `42`, opening message is posted first

### `pipeline/span_created_failed.json`
Same branch (`feature/add-feature-x`), result `FAILED`, run number `6`. Uses UUID attributes. Use this to test:
- Failed pipeline result produces a `❌` reply in the PR thread
- Backfill path with a failed run (opening message still posted even when the build failed)

### `pipeline/span_created_no_pr.json`
Target branch `main`, result `COMPLETE`, trigger `MANUAL`. Uses UUID attributes. No open PR exists for `main` in the test harness. Use this to test:
- Repository UUID resolution still runs before the branch→PR lookup
- When no open PR is found for the target branch, a standalone top-level message is posted
- The standalone message has no `thread_ts`
