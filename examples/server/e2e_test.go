//go:build e2e

package main_test

// Future E2E tests — requires docker-compose stack running.
// Run with: go test -tags e2e ./examples/server/ -v
//
// TestE2E_PullRequestCreated:
//   POST /webhook with X-Event-Key: pullrequest:created
//   Assert Slack stub received opening message
//
// TestE2E_CommitStatusBackfill:
//   POST /webhook with X-Event-Key: repo:commit_status_created
//   Assert Bitbucket stub called for hash resolution
//   Assert Slack stub received opening message + reply
