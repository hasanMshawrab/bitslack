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
	h.BBServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

		http.NotFound(w, r)
	}))

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
	h.ThreadStore.Seed("myworkspace/my-repo:42", "9999.0000")
	payload := loadFixture(t, "testdata/webhooks/commit_status/created.json")

	err := h.Client.Handler(context.Background(), "repo:commit_status_created", payload)
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
	// No thread seeded — will backfill
	// First Slack call: opening message (returns ts)
	// Second Slack call: reply
	h.pushSlackResponse(`{"ok":true,"ts":"5555.6666"}`)
	h.pushSlackResponse(`{"ok":true,"ts":"5555.7777"}`)

	payload := loadFixture(t, "testdata/webhooks/commit_status/created.json")

	err := h.Client.Handler(context.Background(), "repo:commit_status_created", payload)
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
	h.ThreadStore.Seed("myworkspace/my-repo:42", "9999.0000")

	// Use the updated fixture (SUCCESSFUL) but we need a FAILED one.
	// We'll modify the created fixture to have FAILED state.
	payload := loadFixture(t, "testdata/webhooks/commit_status/updated.json")
	// The updated fixture has state "SUCCESSFUL". Replace it for this test.
	payload = []byte(strings.ReplaceAll(string(payload), `"SUCCESSFUL"`, `"FAILED"`))
	payload = []byte(strings.ReplaceAll(string(payload), `"All tests passed"`, `"Build failed"`))

	err := h.Client.Handler(context.Background(), "repo:commit_status_updated", payload)
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
