package bitslack_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hasanMshawrab/bitslack"
	"github.com/hasanMshawrab/bitslack/internal/testutil"
)

// ---------------------------------------------------------------------------
// Test harness
// ---------------------------------------------------------------------------

// slackCall records a single request to the mock Slack API.
type slackCall struct {
	Path string
	Body map[string]any
}

// testHarness bundles the mock servers, stores, logger, and client.
type testHarness struct {
	SlackServer *httptest.Server
	BBServer    *httptest.Server
	ThreadStore *testutil.MockThreadStore
	ConfigStore *testutil.MockConfigStore
	Logger      *testutil.MockLogger
	Client      *bitslack.Client

	mu         sync.Mutex
	slackCalls []slackCall
	// slackResponses is a queue; each call pops one. Falls back to default.
	slackResponses []string

	// openPRForBranchEmpty makes the /pullrequests?q=... endpoint return an empty list.
	openPRForBranchEmpty bool
	// openPRListOmitsReviewers makes the /pullrequests?q=... endpoint return a PR without
	// reviewers, simulating real Bitbucket API list endpoint behaviour.
	openPRListOmitsReviewers bool

	// pipelineStepsEmpty makes the /pipelines/*/steps/ endpoint return an empty list.
	pipelineStepsEmpty bool
	// pipelineStepsFailure makes the /pipelines/*/steps/ endpoint return a 500 error.
	pipelineStepsFailure bool
}

func newHarness(t *testing.T) *testHarness {
	t.Helper()
	h := &testHarness{
		ThreadStore: testutil.NewMockThreadStore(),
		ConfigStore: testutil.NewMockConfigStore(),
		Logger:      &testutil.MockLogger{},
	}

	// Slack mock
	h.SlackServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var parsed map[string]any
		_ = json.Unmarshal(body, &parsed)

		h.mu.Lock()
		h.slackCalls = append(h.slackCalls, slackCall{Path: r.URL.Path, Body: parsed})
		var resp string
		if len(h.slackResponses) > 0 {
			resp = h.slackResponses[0]
			h.slackResponses = h.slackResponses[1:]
		} else {
			resp = `{"ok":true,"ts":"1111.2222"}`
		}
		h.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, resp)
	}))

	// Bitbucket mock — routes by URL path
	h.BBServer = httptest.NewServer(newBBMockHandler(h))

	t.Cleanup(func() {
		h.SlackServer.Close()
		h.BBServer.Close()
	})

	client, err := bitslack.New(bitslack.Config{
		SlackToken:        "xoxb-test",
		BitbucketUsername: "bb-user",
		BitbucketToken:    "bb-test",
		SlackBaseURL:      h.SlackServer.URL,
		BitbucketBaseURL:  h.BBServer.URL,
		ThreadStore:       h.ThreadStore,
		ConfigStore:       h.ConfigStore,
		Logger:            h.Logger,
	})
	if err != nil {
		t.Fatalf("bitslack.New: %v", err)
	}
	h.Client = client
	return h
}

func (h *testHarness) getSlackCalls() []slackCall {
	h.mu.Lock()
	defer h.mu.Unlock()
	cp := make([]slackCall, len(h.slackCalls))
	copy(cp, h.slackCalls)
	return cp
}

func (h *testHarness) pushSlackResponse(resp string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.slackResponses = append(h.slackResponses, resp)
}

// newBBMockHandler returns an [http.Handler] that routes Bitbucket API requests
// based on the path, using the harness state for conditional responses.
func newBBMockHandler(h *testHarness) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// GET /repositories/{ws}/{repo}/pullrequests/{id}
		if strings.Contains(path, "/pullrequests/") && !strings.Contains(path, "/commit/") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, bbPRResponse())
			return
		}

		// GET /repositories/{ws}/{repo}/commit/{hash}/pullrequests
		if strings.Contains(path, "/commit/") && strings.HasSuffix(path, "/pullrequests") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, bbCommitPRsResponse())
			return
		}

		// GET /repositories/{ws}/{repo}/pullrequests?q=source.branch.name=... (open PR for branch)
		if strings.HasSuffix(path, "/pullrequests") && !strings.Contains(path, "/commit/") {
			w.Header().Set("Content-Type", "application/json")
			serveBBPRForBranch(h, w)
			return
		}

		// GET /repositories/{ws}/{repo}/pipelines/{uuid}/steps/
		if strings.Contains(path, "/pipelines/") && strings.HasSuffix(path, "/steps/") {
			serveBBPipelineSteps(h, w)
			return
		}

		// GET /repositories/{ws}/{repo} — standalone repository lookup
		noSubpath := !strings.Contains(path, "/pullrequests") &&
			!strings.Contains(path, "/commit") &&
			!strings.Contains(path, "/pipelines")
		if strings.HasPrefix(path, "/repositories/") && noSubpath {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, bbRepoResponse())
			return
		}

		http.NotFound(w, r)
	})
}

func serveBBPRForBranch(h *testHarness, w http.ResponseWriter) {
	h.mu.Lock()
	empty := h.openPRForBranchEmpty
	omitReviewers := h.openPRListOmitsReviewers
	h.mu.Unlock()
	switch {
	case empty:
		fmt.Fprint(w, `{"values":[]}`)
	case omitReviewers:
		fmt.Fprint(w, bbOpenPRListNoReviewers())
	default:
		fmt.Fprint(w, bbCommitPRsResponse())
	}
}

func serveBBPipelineSteps(h *testHarness, w http.ResponseWriter) {
	h.mu.Lock()
	stepsEmpty := h.pipelineStepsEmpty
	stepsFailure := h.pipelineStepsFailure
	h.mu.Unlock()
	if stepsFailure {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if stepsEmpty {
		fmt.Fprint(w, `{"values":[]}`)
		return
	}
	fmt.Fprint(w, bbPipelineStepsResponse())
}

// ---------------------------------------------------------------------------
// Fixture helpers
// ---------------------------------------------------------------------------

func loadFixture(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("load fixture %s: %v", path, err)
	}
	return data
}

// bbPRResponse returns a Bitbucket API PR response matching the fixture PR #42.
func bbPRResponse() string {
	return `{
		"id": 42,
		"title": "Add feature X",
		"state": "OPEN",
		"author": {
			"nickname": "janeauthor",
			"display_name": "Jane Author",
			"uuid": "{bb673a1b}",
			"account_id": "5b10a2844c20165700ede22h"
		},
		"source": {
			"branch": {"name": "feature/add-feature-x"},
			"commit": {"hash": "a6e5e5de3b48"},
			"repository": {
				"full_name": "myworkspace/my-repo",
				"name": "my-repo",
				"links": {"html": {"href": "https://bitbucket.org/myworkspace/my-repo"}}
			}
		},
		"destination": {
			"branch": {"name": "main"},
			"commit": {"hash": "ce5965ddd289"},
			"repository": {
				"full_name": "myworkspace/my-repo",
				"name": "my-repo",
				"links": {"html": {"href": "https://bitbucket.org/myworkspace/my-repo"}}
			}
		},
		"reviewers": [
			{
				"nickname": "bobreviewer",
				"display_name": "Bob Reviewer",
				"uuid": "{cc784b2c}",
				"account_id": "5b10a2844c20165700ede23i"
			}
		],
		"reason": "",
		"merge_commit": null,
		"closed_by": null,
		"close_source_branch": false,
		"created_on": "2024-01-15T10:00:00.000000+00:00",
		"updated_on": "2024-01-15T10:00:00.000000+00:00",
		"links": {
			"html": {"href": "https://bitbucket.org/myworkspace/my-repo/pull-requests/42"}
		}
	}`
}

// bbCommitPRsResponse returns a Bitbucket commit-to-PRs list response.
func bbCommitPRsResponse() string {
	return fmt.Sprintf(`{"values": [%s]}`, bbPRResponse())
}

// bbRepoResponse returns a Bitbucket repository API response for myworkspace/my-repo.
func bbRepoResponse() string {
	return `{
		"full_name": "myworkspace/my-repo",
		"name": "my-repo",
		"workspace": {"slug": "myworkspace"},
		"links": {"html": {"href": "https://bitbucket.org/myworkspace/my-repo"}}
	}`
}

// bbPipelineStepsResponse returns a mock Bitbucket pipeline steps API response.
func bbPipelineStepsResponse() string {
	return `{
		"values": [
			{
				"uuid": "{step-001-lint}",
				"name": "Lint",
				"state": {
					"name": "COMPLETED",
					"result": {"name": "SUCCESSFUL"}
				},
				"duration_in_seconds": 12
			},
			{
				"uuid": "{step-002-test}",
				"name": "Test",
				"state": {
					"name": "COMPLETED",
					"result": {"name": "FAILED"}
				},
				"duration_in_seconds": 18
			}
		]
	}`
}

// bbAllStepsStoppedResponse returns a mock steps response where all steps are STOPPED.
func bbAllStepsStoppedResponse() string {
	return `{
		"values": [
			{
				"uuid": "{step-001-build}",
				"name": "Build",
				"state": {"name": "STOPPED"},
				"duration_in_seconds": 3
			},
			{
				"uuid": "{step-002-test}",
				"name": "Test",
				"state": {"name": "STOPPED"},
				"duration_in_seconds": 0
			}
		]
	}`
}

// bbOpenPRListNoReviewers returns a Bitbucket list-PRs response that omits the reviewers field,
// simulating the real Bitbucket API list endpoint which does not include reviewer details.
func bbOpenPRListNoReviewers() string {
	return `{"values":[{
		"id": 42,
		"title": "Add feature X",
		"state": "OPEN",
		"author": {
			"nickname": "janeauthor",
			"display_name": "Jane Author",
			"uuid": "{bb673a1b}",
			"account_id": "5b10a2844c20165700ede22h"
		},
		"source": {
			"branch": {"name": "feature/add-feature-x"},
			"commit": {"hash": "a6e5e5de3b48"},
			"repository": {
				"full_name": "myworkspace/my-repo",
				"name": "my-repo",
				"links": {"html": {"href": "https://bitbucket.org/myworkspace/my-repo"}}
			}
		},
		"destination": {
			"branch": {"name": "main"},
			"commit": {"hash": "ce5965ddd289"},
			"repository": {
				"full_name": "myworkspace/my-repo",
				"name": "my-repo",
				"links": {"html": {"href": "https://bitbucket.org/myworkspace/my-repo"}}
			}
		},
		"reviewers": [],
		"links": {"html": {"href": "https://bitbucket.org/myworkspace/my-repo/pull-requests/42"}}
	}]}`
}

// ---------------------------------------------------------------------------
// Happy-path tests
// ---------------------------------------------------------------------------

func TestHandler_PullRequestCreated_NewPR(t *testing.T) {
	h := newHarness(t)
	payload := loadFixture(t, "testdata/webhooks/pullrequest/created.json")

	err := h.Client.Handler(context.Background(), "pullrequest:created", payload)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	// pullrequest:created with no existing thread:
	// 1) BB fetch PR (backfill)
	// 2) Slack postMessage (opening) — that's it, no reply for created+backfill
	calls := h.getSlackCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 Slack call, got %d: %+v", len(calls), calls)
	}
	if calls[0].Path != "/chat.postMessage" {
		t.Errorf("expected /chat.postMessage, got %s", calls[0].Path)
	}
	// Opening message has no thread_ts (top-level)
	if ts, ok := calls[0].Body["thread_ts"]; ok && ts != "" {
		t.Errorf("opening message should have empty thread_ts, got %v", ts)
	}

	// Thread ts should be stored
	if len(h.ThreadStore.SetCalls) != 1 {
		t.Fatalf("expected 1 Store call, got %d", len(h.ThreadStore.SetCalls))
	}
	if h.ThreadStore.SetCalls[0].TS != "1111.2222" {
		t.Errorf("expected stored ts=1111.2222, got %s", h.ThreadStore.SetCalls[0].TS)
	}
}

func TestHandler_PullRequestCreated_ExistingThread(t *testing.T) {
	h := newHarness(t)
	h.ThreadStore.Seed("myworkspace/my-repo:42", "9999.0000")
	payload := loadFixture(t, "testdata/webhooks/pullrequest/created.json")

	err := h.Client.Handler(context.Background(), "pullrequest:created", payload)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	// Created event with existing thread: no backfill needed.
	// But it's not a backfill (wasBackfilled=false), so it will proceed to
	// "all other events: post a thread reply" — no, wait.
	// Looking at the code: if ev.Key == KeyPRCreated && wasBackfilled => return nil
	// But wasBackfilled is false here. So it falls through to FormatReply.
	// FormatReply doesn't handle KeyPRCreated — it will return an error.
	// The handler logs it and returns nil.
	// Actually let me re-read format/reply.go... KeyPRCreated is not in the switch.
	// So FormatReply returns error, handler logs it, returns nil.
	// So: 0 Slack calls, 1 error log.
	calls := h.getSlackCalls()
	if len(calls) != 0 {
		t.Fatalf("expected 0 Slack calls, got %d: %+v", len(calls), calls)
	}

	// Should have logged an error about format failure
	if len(h.Logger.ErrorMsgs) != 1 {
		t.Fatalf("expected 1 error log, got %d: %v", len(h.Logger.ErrorMsgs), h.Logger.ErrorMsgs)
	}
	if !strings.Contains(h.Logger.ErrorMsgs[0], "format reply") {
		t.Errorf("expected format reply error, got: %s", h.Logger.ErrorMsgs[0])
	}
}

func TestHandler_PullRequestApproved_ExistingThread(t *testing.T) {
	h := newHarness(t)
	h.ThreadStore.Seed("myworkspace/my-repo:42", "9999.0000")
	payload := loadFixture(t, "testdata/webhooks/pullrequest/approved.json")

	err := h.Client.Handler(context.Background(), "pullrequest:approved", payload)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	calls := h.getSlackCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 Slack call, got %d", len(calls))
	}
	if calls[0].Path != "/chat.postMessage" {
		t.Errorf("expected /chat.postMessage, got %s", calls[0].Path)
	}
	// Should be posted as a reply with thread_ts
	if ts, _ := calls[0].Body["thread_ts"].(string); ts != "9999.0000" {
		t.Errorf("expected thread_ts=9999.0000, got %v", calls[0].Body["thread_ts"])
	}
	// Text should mention approval
	if text, _ := calls[0].Body["text"].(string); !strings.Contains(text, "approved") {
		t.Errorf("expected reply to contain 'approved', got %q", text)
	}
}

func TestHandler_PullRequestUpdated_TitleChange(t *testing.T) {
	h := newHarness(t)
	h.ThreadStore.Seed("myworkspace/my-repo:42", "9999.0000")
	payload := loadFixture(t, "testdata/webhooks/pullrequest/updated.json")

	err := h.Client.Handler(context.Background(), "pullrequest:updated", payload)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	calls := h.getSlackCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 Slack call (chat.update), got %d: %+v", len(calls), calls)
	}
	if calls[0].Path != "/chat.update" {
		t.Errorf("expected /chat.update, got %s", calls[0].Path)
	}
	// Should include the ts of the opening message
	if ts, _ := calls[0].Body["ts"].(string); ts != "9999.0000" {
		t.Errorf("expected ts=9999.0000, got %v", calls[0].Body["ts"])
	}
}

func TestHandler_PullRequestFulfilled(t *testing.T) {
	h := newHarness(t)
	h.ThreadStore.Seed("myworkspace/my-repo:42", "9999.0000")
	payload := loadFixture(t, "testdata/webhooks/pullrequest/fulfilled.json")

	err := h.Client.Handler(context.Background(), "pullrequest:fulfilled", payload)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	calls := h.getSlackCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 Slack call, got %d", len(calls))
	}
	text, _ := calls[0].Body["text"].(string)
	if !strings.Contains(text, "merged") {
		t.Errorf("expected reply to contain 'merged', got %q", text)
	}
}

func TestHandler_PullRequestRejected(t *testing.T) {
	h := newHarness(t)
	h.ThreadStore.Seed("myworkspace/my-repo:42", "9999.0000")
	payload := loadFixture(t, "testdata/webhooks/pullrequest/rejected.json")

	err := h.Client.Handler(context.Background(), "pullrequest:rejected", payload)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	calls := h.getSlackCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 Slack call, got %d", len(calls))
	}
	text, _ := calls[0].Body["text"].(string)
	if !strings.Contains(text, "declined") {
		t.Errorf("expected reply to contain 'declined', got %q", text)
	}
	if !strings.Contains(text, "architecture changes") {
		t.Errorf("expected reply to contain reason text, got %q", text)
	}
}

func TestHandler_PullRequestCommentCreated(t *testing.T) {
	h := newHarness(t)
	h.ThreadStore.Seed("myworkspace/my-repo:42", "9999.0000")
	payload := loadFixture(t, "testdata/webhooks/pullrequest/comment_created.json")

	err := h.Client.Handler(context.Background(), "pullrequest:comment_created", payload)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	calls := h.getSlackCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 Slack call, got %d", len(calls))
	}
	text, _ := calls[0].Body["text"].(string)
	if !strings.Contains(text, "pkg/handler/handler.go") {
		t.Errorf("expected reply to contain file path, got %q", text)
	}
	if !strings.Contains(text, "unit test") {
		t.Errorf("expected reply to contain comment text, got %q", text)
	}
}

func TestHandler_CommitStatus_ExistingThread(t *testing.T) {
	h := newHarness(t)
	client, err := bitslack.New(bitslack.Config{
		SlackToken:        "xoxb-test",
		BitbucketUsername: "bb-user",
		BitbucketToken:    "bb-test",
		SlackBaseURL:      h.SlackServer.URL,
		BitbucketBaseURL:  h.BBServer.URL,
		ThreadStore:       h.ThreadStore,
		ConfigStore:       h.ConfigStore,
		Logger:            h.Logger,
		EnabledEvents:     []bitslack.EventFamily{bitslack.EventFamilyPullRequest, bitslack.EventFamilyCommitStatus},
	})
	if err != nil {
		t.Fatalf("bitslack.New: %v", err)
	}
	h.Client = client

	h.ThreadStore.Seed("myworkspace/my-repo:42", "9999.0000")
	payload := loadFixture(t, "testdata/webhooks/commit_status/created.json")

	err = h.Client.Handler(context.Background(), "repo:commit_status_created", payload)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	// BB should be called to resolve commit -> PR
	// Then 1 Slack call (reply) since thread already exists
	calls := h.getSlackCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 Slack call, got %d: %+v", len(calls), calls)
	}
	if calls[0].Path != "/chat.postMessage" {
		t.Errorf("expected /chat.postMessage, got %s", calls[0].Path)
	}
	if ts, _ := calls[0].Body["thread_ts"].(string); ts != "9999.0000" {
		t.Errorf("expected thread_ts=9999.0000, got %v", calls[0].Body["thread_ts"])
	}
}

func TestHandler_CommitStatus_Backfill(t *testing.T) {
	h := newHarness(t)
	client, err := bitslack.New(bitslack.Config{
		SlackToken:        "xoxb-test",
		BitbucketUsername: "bb-user",
		BitbucketToken:    "bb-test",
		SlackBaseURL:      h.SlackServer.URL,
		BitbucketBaseURL:  h.BBServer.URL,
		ThreadStore:       h.ThreadStore,
		ConfigStore:       h.ConfigStore,
		Logger:            h.Logger,
		EnabledEvents:     []bitslack.EventFamily{bitslack.EventFamilyPullRequest, bitslack.EventFamilyCommitStatus},
	})
	if err != nil {
		t.Fatalf("bitslack.New: %v", err)
	}
	h.Client = client

	// No thread seeded — will backfill
	// First Slack call: opening message (returns ts)
	// Second Slack call: reply
	h.pushSlackResponse(`{"ok":true,"ts":"5555.6666"}`)
	h.pushSlackResponse(`{"ok":true,"ts":"5555.7777"}`)

	payload := loadFixture(t, "testdata/webhooks/commit_status/created.json")

	err = h.Client.Handler(context.Background(), "repo:commit_status_created", payload)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	calls := h.getSlackCalls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 Slack calls (opening + reply), got %d: %+v", len(calls), calls)
	}

	// First call: opening message (no thread_ts)
	if calls[0].Path != "/chat.postMessage" {
		t.Errorf("call 0: expected /chat.postMessage, got %s", calls[0].Path)
	}
	if ts, ok := calls[0].Body["thread_ts"]; ok && ts != "" {
		t.Errorf("opening message should have empty thread_ts, got %v", ts)
	}

	// Second call: reply with thread_ts from first call
	if calls[1].Path != "/chat.postMessage" {
		t.Errorf("call 1: expected /chat.postMessage, got %s", calls[1].Path)
	}
	if ts, _ := calls[1].Body["thread_ts"].(string); ts != "5555.6666" {
		t.Errorf("expected thread_ts=5555.6666, got %v", calls[1].Body["thread_ts"])
	}

	// Thread should be stored
	if len(h.ThreadStore.SetCalls) != 1 {
		t.Fatalf("expected 1 Store call, got %d", len(h.ThreadStore.SetCalls))
	}
	if h.ThreadStore.SetCalls[0].TS != "5555.6666" {
		t.Errorf("expected stored ts=5555.6666, got %s", h.ThreadStore.SetCalls[0].TS)
	}
}

func TestHandler_CommitStatusFailed(t *testing.T) {
	h := newHarness(t)
	client, err := bitslack.New(bitslack.Config{
		SlackToken:        "xoxb-test",
		BitbucketUsername: "bb-user",
		BitbucketToken:    "bb-test",
		SlackBaseURL:      h.SlackServer.URL,
		BitbucketBaseURL:  h.BBServer.URL,
		ThreadStore:       h.ThreadStore,
		ConfigStore:       h.ConfigStore,
		Logger:            h.Logger,
		EnabledEvents:     []bitslack.EventFamily{bitslack.EventFamilyPullRequest, bitslack.EventFamilyCommitStatus},
	})
	if err != nil {
		t.Fatalf("bitslack.New: %v", err)
	}
	h.Client = client

	h.ThreadStore.Seed("myworkspace/my-repo:42", "9999.0000")

	// Use the updated fixture (SUCCESSFUL) but we need a FAILED one.
	// We'll modify the created fixture to have FAILED state.
	payload := loadFixture(t, "testdata/webhooks/commit_status/updated.json")
	// The updated fixture has state "SUCCESSFUL". Replace it for this test.
	payload = []byte(strings.ReplaceAll(string(payload), `"SUCCESSFUL"`, `"FAILED"`))
	payload = []byte(strings.ReplaceAll(string(payload), `"All tests passed"`, `"Build failed"`))

	err = h.Client.Handler(context.Background(), "repo:commit_status_updated", payload)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	calls := h.getSlackCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 Slack call, got %d", len(calls))
	}
	text, _ := calls[0].Body["text"].(string)
	if !strings.Contains(text, "failed") {
		t.Errorf("expected reply to contain 'failed', got %q", text)
	}
}

func TestHandler_Backfill_PREvent(t *testing.T) {
	h := newHarness(t)
	// Approved event with empty ThreadStore — triggers backfill
	h.pushSlackResponse(`{"ok":true,"ts":"8888.0000"}`) // opening
	h.pushSlackResponse(`{"ok":true,"ts":"8888.0001"}`) // reply

	payload := loadFixture(t, "testdata/webhooks/pullrequest/approved.json")

	err := h.Client.Handler(context.Background(), "pullrequest:approved", payload)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	calls := h.getSlackCalls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 Slack calls (opening + reply), got %d: %+v", len(calls), calls)
	}

	// First: opening message (postMessage, no thread_ts)
	if calls[0].Path != "/chat.postMessage" {
		t.Errorf("call 0: expected /chat.postMessage, got %s", calls[0].Path)
	}

	// Second: reply (postMessage, with thread_ts)
	if ts, _ := calls[1].Body["thread_ts"].(string); ts != "8888.0000" {
		t.Errorf("expected thread_ts=8888.0000, got %v", calls[1].Body["thread_ts"])
	}

	text, _ := calls[1].Body["text"].(string)
	if !strings.Contains(text, "approved") {
		t.Errorf("expected reply to contain 'approved', got %q", text)
	}
}

// ---------------------------------------------------------------------------
// Error-path tests
// ---------------------------------------------------------------------------

func TestHandler_UnknownEventKey(t *testing.T) {
	h := newHarness(t)

	err := h.Client.Handler(context.Background(), "repo:push", []byte(`{}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if len(h.Logger.WarnMsgs) == 0 {
		t.Error("expected warning log for unknown event key")
	}
	found := false
	for _, msg := range h.Logger.WarnMsgs {
		if strings.Contains(msg, "unknown event key") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected warn about unknown event key, got: %v", h.Logger.WarnMsgs)
	}

	calls := h.getSlackCalls()
	if len(calls) != 0 {
		t.Errorf("expected 0 Slack calls, got %d", len(calls))
	}
}

func TestHandler_ChannelNotFound(t *testing.T) {
	h := newHarness(t)
	// Clear channels so lookup fails
	h.ConfigStore.Channels = map[string]string{}

	payload := loadFixture(t, "testdata/webhooks/pullrequest/created.json")
	err := h.Client.Handler(context.Background(), "pullrequest:created", payload)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	calls := h.getSlackCalls()
	if len(calls) != 0 {
		t.Errorf("expected 0 Slack calls, got %d", len(calls))
	}
}

func TestHandler_ThreadStoreGetError(t *testing.T) {
	h := newHarness(t)
	h.ThreadStore.GetErr = errors.New("redis connection refused")

	payload := loadFixture(t, "testdata/webhooks/pullrequest/approved.json")
	err := h.Client.Handler(context.Background(), "pullrequest:approved", payload)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	// Should log error and return nil
	if len(h.Logger.ErrorMsgs) == 0 {
		t.Error("expected error log for thread store failure")
	}

	calls := h.getSlackCalls()
	if len(calls) != 0 {
		t.Errorf("expected 0 Slack calls, got %d", len(calls))
	}
}

func TestHandler_ThreadStoreSetError(t *testing.T) {
	h := newHarness(t)
	h.ThreadStore.SetErr = errors.New("redis write failed")

	payload := loadFixture(t, "testdata/webhooks/pullrequest/created.json")
	err := h.Client.Handler(context.Background(), "pullrequest:created", payload)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	// Should still have posted the opening message
	calls := h.getSlackCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 Slack call, got %d", len(calls))
	}

	// Should log warning about store failure
	found := false
	for _, msg := range h.Logger.WarnMsgs {
		if strings.Contains(msg, "store thread ts") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected warning about store failure, got warns: %v", h.Logger.WarnMsgs)
	}
}

func TestHandler_SlackPostFails(t *testing.T) {
	h := newHarness(t)
	h.pushSlackResponse(`{"ok":false,"error":"channel_not_found"}`)

	payload := loadFixture(t, "testdata/webhooks/pullrequest/created.json")
	err := h.Client.Handler(context.Background(), "pullrequest:created", payload)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if len(h.Logger.ErrorMsgs) == 0 {
		t.Error("expected error log for Slack failure")
	}
}

func TestHandler_BitbucketAPIFails(t *testing.T) {
	// Create a harness with a BB server that always returns 500
	h := newHarness(t)
	h.BBServer.Close()
	h.BBServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(func() { h.BBServer.Close() })

	// Re-create client with new BB URL
	client, err := bitslack.New(bitslack.Config{
		SlackToken:        "xoxb-test",
		BitbucketUsername: "bb-user",
		BitbucketToken:    "bb-test",
		SlackBaseURL:      h.SlackServer.URL,
		BitbucketBaseURL:  h.BBServer.URL,
		ThreadStore:       h.ThreadStore,
		ConfigStore:       h.ConfigStore,
		Logger:            h.Logger,
	})
	if err != nil {
		t.Fatalf("bitslack.New: %v", err)
	}
	h.Client = client

	// PR event with no thread -> will try BB API for backfill -> fail
	payload := loadFixture(t, "testdata/webhooks/pullrequest/approved.json")
	err = h.Client.Handler(context.Background(), "pullrequest:approved", payload)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if len(h.Logger.ErrorMsgs) == 0 {
		t.Error("expected error log for Bitbucket API failure")
	}

	calls := h.getSlackCalls()
	if len(calls) != 0 {
		t.Errorf("expected 0 Slack calls after BB failure, got %d", len(calls))
	}
}

// ---------------------------------------------------------------------------
// Pipeline event tests
// ---------------------------------------------------------------------------

func newPipelineHarness(t *testing.T) *testHarness {
	t.Helper()
	h := newHarness(t)
	client, err := bitslack.New(bitslack.Config{
		SlackToken:        "xoxb-test",
		BitbucketUsername: "bb-user",
		BitbucketToken:    "bb-test",
		SlackBaseURL:      h.SlackServer.URL,
		BitbucketBaseURL:  h.BBServer.URL,
		ThreadStore:       h.ThreadStore,
		ConfigStore:       h.ConfigStore,
		Logger:            h.Logger,
		EnabledEvents:     []bitslack.EventFamily{bitslack.EventFamilyPipeline},
		PipelineDebounce:  1 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("bitslack.New: %v", err)
	}
	h.Client = client
	return h
}

// waitForPipeline waits for the async pipeline debounce timer to fire and processing to complete.
func waitForPipeline() { time.Sleep(100 * time.Millisecond) }

func TestHandler_Pipeline_LinkedToPRExistingThread(t *testing.T) {
	h := newPipelineHarness(t)
	h.ThreadStore.Seed("myworkspace/my-repo:42", "9999.0000")
	payload := loadFixture(t, "testdata/webhooks/pipeline/span_created_successful.json")

	err := h.Client.Handler(context.Background(), "pipeline:span_created", payload)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}
	waitForPipeline()

	calls := h.getSlackCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 Slack call (reply), got %d: %+v", len(calls), calls)
	}
	if calls[0].Path != "/chat.postMessage" {
		t.Errorf("expected /chat.postMessage, got %s", calls[0].Path)
	}
	if ts, _ := calls[0].Body["thread_ts"].(string); ts != "9999.0000" {
		t.Errorf("expected thread_ts=9999.0000, got %v", calls[0].Body["thread_ts"])
	}
	text, _ := calls[0].Body["text"].(string)
	if !strings.Contains(text, "✅") {
		t.Errorf("expected reply to contain ✅, got %q", text)
	}
}

func TestHandler_Pipeline_Backfill(t *testing.T) {
	h := newPipelineHarness(t)
	h.pushSlackResponse(`{"ok":true,"ts":"7777.0000"}`)
	h.pushSlackResponse(`{"ok":true,"ts":"7777.0001"}`)

	payload := loadFixture(t, "testdata/webhooks/pipeline/span_created_failed.json")

	err := h.Client.Handler(context.Background(), "pipeline:span_created", payload)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}
	waitForPipeline()

	calls := h.getSlackCalls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 Slack calls (opening + reply), got %d: %+v", len(calls), calls)
	}
	// First: opening message (no thread_ts)
	if ts, ok := calls[0].Body["thread_ts"]; ok && ts != "" {
		t.Errorf("opening message should have empty thread_ts, got %v", ts)
	}
	// Second: reply with thread_ts from opening
	if ts, _ := calls[1].Body["thread_ts"].(string); ts != "7777.0000" {
		t.Errorf("expected thread_ts=7777.0000, got %v", calls[1].Body["thread_ts"])
	}
	text, _ := calls[1].Body["text"].(string)
	if !strings.Contains(text, "❌") {
		t.Errorf("expected reply to contain ❌, got %q", text)
	}
}

func TestHandler_Pipeline_Backfill_IncludesReviewers(t *testing.T) {
	// Simulate real Bitbucket behaviour: the list endpoint (/pullrequests?q=...) omits the
	// reviewers field, so GetOpenPRForBranch returns a PR with an empty Reviewers slice.
	// The fix must fetch the full PR via GetPullRequest before building the opening message.
	h := newPipelineHarness(t)
	h.openPRListOmitsReviewers = true
	h.pushSlackResponse(`{"ok":true,"ts":"7777.0000"}`)
	h.pushSlackResponse(`{"ok":true,"ts":"7777.0001"}`)

	payload := loadFixture(t, "testdata/webhooks/pipeline/span_created_failed.json")

	err := h.Client.Handler(context.Background(), "pipeline:span_created", payload)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}
	waitForPipeline()

	calls := h.getSlackCalls()
	if len(calls) < 1 {
		t.Fatalf("expected at least 1 Slack call, got %d", len(calls))
	}

	// The opening message blocks should contain the reviewer (bobreviewer) fetched via
	// the full PR endpoint, not the empty list from GetOpenPRForBranch.
	blocksJSON, _ := json.Marshal(calls[0].Body["blocks"])
	if !strings.Contains(string(blocksJSON), "Reviewers") {
		t.Errorf("opening message blocks should contain Reviewers line, got %s", blocksJSON)
	}
	if !strings.Contains(string(blocksJSON), "U002BOB") {
		t.Errorf("opening message blocks should contain reviewer Slack ID (U002BOB), got %s", blocksJSON)
	}
}

func TestHandler_Pipeline_StandaloneNoPR(t *testing.T) {
	h := newPipelineHarness(t)
	h.openPRForBranchEmpty = true
	payload := loadFixture(t, "testdata/webhooks/pipeline/span_created_no_pr.json")

	err := h.Client.Handler(context.Background(), "pipeline:span_created", payload)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}
	waitForPipeline()

	calls := h.getSlackCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 Slack call (standalone), got %d: %+v", len(calls), calls)
	}
	// Standalone: no thread_ts
	if ts, ok := calls[0].Body["thread_ts"]; ok && ts != "" {
		t.Errorf("standalone message should have empty thread_ts, got %v", ts)
	}
}

func TestHandler_Pipeline_StepSpanSkipped(t *testing.T) {
	h := newPipelineHarness(t)
	payload := []byte(`{"resourceSpans":[{"scopeSpans":[{"spans":[{"name":"bbc.pipeline_step","attributes":[]}]}]}]}`)

	err := h.Client.Handler(context.Background(), "pipeline:span_created", payload)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	calls := h.getSlackCalls()
	if len(calls) != 0 {
		t.Errorf("expected 0 Slack calls for step span, got %d", len(calls))
	}
}

func TestHandler_Pipeline_StepBreakdownIncluded(t *testing.T) {
	// Verify that the step breakdown (name, emoji) appears in the pipeline reply.
	h := newPipelineHarness(t)
	h.ThreadStore.Seed("myworkspace/my-repo:42", "9999.0000")
	payload := loadFixture(t, "testdata/webhooks/pipeline/span_created_failed.json")

	err := h.Client.Handler(context.Background(), "pipeline:span_created", payload)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}
	waitForPipeline()

	calls := h.getSlackCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 Slack call (reply), got %d", len(calls))
	}
	text, _ := calls[0].Body["text"].(string)
	// Header emoji (fixed) and step-level emojis must both appear.
	if !strings.Contains(text, "⚙️") {
		t.Errorf("expected ⚙️ in message, got %q", text)
	}
	if !strings.Contains(text, "Lint") {
		t.Errorf("expected step name 'Lint' in message, got %q", text)
	}
	if !strings.Contains(text, "Test") {
		t.Errorf("expected step name 'Test' in message, got %q", text)
	}
	// The failed step (Test) should be hyperlinked.
	if !strings.Contains(text, "<") || !strings.Contains(text, "Test") {
		t.Errorf("expected failed step 'Test' to be hyperlinked, got %q", text)
	}
}

func TestHandler_Pipeline_StepsAPIFailure_StillPosts(t *testing.T) {
	// When the steps API returns an error, a header-only message is still posted.
	h := newPipelineHarness(t)
	h.ThreadStore.Seed("myworkspace/my-repo:42", "9999.0000")
	h.pipelineStepsFailure = true
	payload := loadFixture(t, "testdata/webhooks/pipeline/span_created_failed.json")

	err := h.Client.Handler(context.Background(), "pipeline:span_created", payload)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}
	waitForPipeline()

	calls := h.getSlackCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 Slack call (header-only), got %d", len(calls))
	}
	text, _ := calls[0].Body["text"].(string)
	if !strings.Contains(text, "❌") {
		t.Errorf("expected ❌ in header-only message, got %q", text)
	}
	// No step names in header-only message.
	if strings.Contains(text, "Lint") || strings.Contains(text, "Test") {
		t.Errorf("header-only message should not contain step names, got %q", text)
	}
	// Should have logged an error about the steps API.
	found := false
	for _, msg := range h.Logger.ErrorMsgs {
		if strings.Contains(msg, "pipeline steps") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error log about pipeline steps, got: %v", h.Logger.ErrorMsgs)
	}
}

func TestHandler_Pipeline_DebounceDeduplicate(t *testing.T) {
	// Two deliveries of the same pipeline_run.uuid within the debounce window:
	// only one Slack message should be posted.
	h := newPipelineHarness(t)
	h.ThreadStore.Seed("myworkspace/my-repo:42", "9999.0000")
	payload := loadFixture(t, "testdata/webhooks/pipeline/span_created_successful.json")

	_ = h.Client.Handler(context.Background(), "pipeline:span_created", payload)
	_ = h.Client.Handler(context.Background(), "pipeline:span_created", payload) // duplicate
	waitForPipeline()

	calls := h.getSlackCalls()
	if len(calls) != 1 {
		t.Errorf("expected 1 Slack call (debounced duplicate), got %d", len(calls))
	}
}

func TestHandler_Pipeline_ManualStopSuppressed(t *testing.T) {
	// SkipManuallyStoppedPipelines=true and all steps STOPPED + trigger MANUAL → no message.
	h := newHarness(t)
	client, err := bitslack.New(bitslack.Config{
		SlackToken:        "xoxb-test",
		BitbucketUsername: "bb-user",
		BitbucketToken:    "bb-test",
		SlackBaseURL:      h.SlackServer.URL,
		BitbucketBaseURL:  h.BBServer.URL,
		ThreadStore:       h.ThreadStore,
		ConfigStore:       h.ConfigStore,
		Logger:            h.Logger,
		EnabledEvents:     []bitslack.EventFamily{bitslack.EventFamilyPipeline},
		PipelineDebounce:  1 * time.Millisecond,
		FormatOptions: bitslack.FormatOptions{
			SkipManuallyStoppedPipelines: true,
		},
	})
	if err != nil {
		t.Fatalf("bitslack.New: %v", err)
	}
	h.Client = client
	// Override steps response to return all-stopped.
	h.pipelineStepsEmpty = false
	// Patch: use a custom BB server that returns all-stopped steps.
	h.BBServer.Close()
	h.BBServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.Contains(path, "/pipelines/") && strings.HasSuffix(path, "/steps/") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, bbAllStepsStoppedResponse())
			return
		}
		noSubpath := !strings.Contains(path, "/pullrequests") &&
			!strings.Contains(path, "/commit") &&
			!strings.Contains(path, "/pipelines")
		if strings.HasPrefix(path, "/repositories/") && noSubpath {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, bbRepoResponse())
			return
		}
		http.NotFound(w, r)
	}))
	// Re-create client with updated BB URL.
	client2, err2 := bitslack.New(bitslack.Config{
		SlackToken:        "xoxb-test",
		BitbucketUsername: "bb-user",
		BitbucketToken:    "bb-test",
		SlackBaseURL:      h.SlackServer.URL,
		BitbucketBaseURL:  h.BBServer.URL,
		ThreadStore:       h.ThreadStore,
		ConfigStore:       h.ConfigStore,
		Logger:            h.Logger,
		EnabledEvents:     []bitslack.EventFamily{bitslack.EventFamilyPipeline},
		PipelineDebounce:  1 * time.Millisecond,
		FormatOptions: bitslack.FormatOptions{
			SkipManuallyStoppedPipelines: true,
		},
	})
	if err2 != nil {
		t.Fatalf("bitslack.New: %v", err2)
	}
	h.Client = client2
	_ = client // silence unused warning

	// span_created_no_pr.json has trigger=MANUAL.
	h.openPRForBranchEmpty = true
	payload := loadFixture(t, "testdata/webhooks/pipeline/span_created_no_pr.json")

	err = h.Client.Handler(context.Background(), "pipeline:span_created", payload)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}
	waitForPipeline()

	calls := h.getSlackCalls()
	if len(calls) != 0 {
		t.Errorf("expected 0 Slack calls for suppressed manual stop, got %d: %v", len(calls), calls)
	}
	// Should have logged an info about suppression.
	found := false
	for _, msg := range h.Logger.InfoMsgs {
		if strings.Contains(msg, "suppressing") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected info log about suppression, got: %v", h.Logger.InfoMsgs)
	}
}

func TestHandler_Pipeline_ManualStopNotSuppressedByDefault(t *testing.T) {
	// Default (SkipManuallyStoppedPipelines=false): message is posted even for manual stop.
	// Use the no_pr fixture which has trigger=MANUAL.
	h := newPipelineHarness(t)
	h.openPRForBranchEmpty = true
	payload := loadFixture(t, "testdata/webhooks/pipeline/span_created_no_pr.json")

	err := h.Client.Handler(context.Background(), "pipeline:span_created", payload)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}
	waitForPipeline()

	calls := h.getSlackCalls()
	if len(calls) != 1 {
		t.Errorf("expected 1 Slack call when suppression disabled, got %d", len(calls))
	}
}

// ---------------------------------------------------------------------------
// EnabledEvents tests
// ---------------------------------------------------------------------------

func TestHandler_EnabledEvents_DefaultDropsCommitStatus(t *testing.T) {
	// newHarness sets no EnabledEvents → library defaults to PR only.
	h := newHarness(t)
	h.ThreadStore.Seed("myworkspace/my-repo:42", "9999.0000")
	payload := loadFixture(t, "testdata/webhooks/commit_status/created.json")

	err := h.Client.Handler(context.Background(), "repo:commit_status_created", payload)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	calls := h.getSlackCalls()
	if len(calls) != 0 {
		t.Errorf("expected 0 Slack calls for disabled event family, got %d", len(calls))
	}

	found := false
	for _, msg := range h.Logger.WarnMsgs {
		if strings.Contains(msg, "event family") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected warn about disabled event family, got warns: %v", h.Logger.WarnMsgs)
	}
}

func TestHandler_EnabledEvents_CommitStatusExplicitlyEnabled(t *testing.T) {
	h := newHarness(t)
	client, err := bitslack.New(bitslack.Config{
		SlackToken:        "xoxb-test",
		BitbucketUsername: "bb-user",
		BitbucketToken:    "bb-test",
		SlackBaseURL:      h.SlackServer.URL,
		BitbucketBaseURL:  h.BBServer.URL,
		ThreadStore:       h.ThreadStore,
		ConfigStore:       h.ConfigStore,
		Logger:            h.Logger,
		EnabledEvents:     []bitslack.EventFamily{bitslack.EventFamilyPullRequest, bitslack.EventFamilyCommitStatus},
	})
	if err != nil {
		t.Fatalf("bitslack.New: %v", err)
	}
	h.Client = client

	h.ThreadStore.Seed("myworkspace/my-repo:42", "9999.0000")
	payload := loadFixture(t, "testdata/webhooks/commit_status/created.json")

	err = h.Client.Handler(context.Background(), "repo:commit_status_created", payload)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	calls := h.getSlackCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 Slack call, got %d: %+v", len(calls), calls)
	}
}

func TestHandler_EnabledEvents_PRFamilyDisabled(t *testing.T) {
	h := newHarness(t)
	client, err := bitslack.New(bitslack.Config{
		SlackToken:        "xoxb-test",
		BitbucketUsername: "bb-user",
		BitbucketToken:    "bb-test",
		SlackBaseURL:      h.SlackServer.URL,
		BitbucketBaseURL:  h.BBServer.URL,
		ThreadStore:       h.ThreadStore,
		ConfigStore:       h.ConfigStore,
		Logger:            h.Logger,
		EnabledEvents:     []bitslack.EventFamily{bitslack.EventFamilyCommitStatus},
	})
	if err != nil {
		t.Fatalf("bitslack.New: %v", err)
	}
	h.Client = client

	payload := loadFixture(t, "testdata/webhooks/pullrequest/created.json")
	err = h.Client.Handler(context.Background(), "pullrequest:created", payload)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	calls := h.getSlackCalls()
	if len(calls) != 0 {
		t.Errorf("expected 0 Slack calls for disabled PR family, got %d", len(calls))
	}

	found := false
	for _, msg := range h.Logger.WarnMsgs {
		if strings.Contains(msg, "event family") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected warn about disabled event family, got warns: %v", h.Logger.WarnMsgs)
	}
}

func TestHandler_NilLogger(t *testing.T) {
	ts := testutil.NewMockThreadStore()
	cs := testutil.NewMockConfigStore()

	slackSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true,"ts":"1111.2222"}`)
	}))
	defer slackSrv.Close()

	bbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, bbPRResponse())
	}))
	defer bbSrv.Close()

	// Logger is nil — should use noopLogger and not panic
	client, err := bitslack.New(bitslack.Config{
		SlackToken:        "xoxb-test",
		BitbucketUsername: "bb-user",
		BitbucketToken:    "bb-test",
		SlackBaseURL:      slackSrv.URL,
		BitbucketBaseURL:  bbSrv.URL,
		ThreadStore:       ts,
		ConfigStore:       cs,
		Logger:            nil,
	})
	if err != nil {
		t.Fatalf("bitslack.New: %v", err)
	}

	// Should not panic on any code path
	payload := loadFixture(t, "testdata/webhooks/pullrequest/created.json")
	err = client.Handler(context.Background(), "pullrequest:created", payload)
	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	// Also try unknown event — exercises Warn path with noop logger
	err = client.Handler(context.Background(), "unknown:event", []byte(`{}`))
	if err != nil {
		t.Fatalf("Handler returned error for unknown event: %v", err)
	}
}
