package event_test

import (
	"os"
	"strings"
	"testing"

	"github.com/hasanMshawrab/bitslack/internal/event"
)

const fixtureDir = "../../testdata/webhooks/"

func loadFixture(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(fixtureDir + path)
	if err != nil {
		t.Fatalf("failed to load fixture %s: %v", path, err)
	}
	return data
}

func TestParse_PullRequestCreated(t *testing.T) {
	payload := loadFixture(t, "pullrequest/created.json")
	evt, err := event.Parse(event.KeyPRCreated, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.PullRequest == nil {
		t.Fatal("expected PullRequest to be non-nil")
	}
	if evt.CommitStatus != nil {
		t.Fatal("expected CommitStatus to be nil")
	}
	if evt.Key != event.KeyPRCreated {
		t.Errorf("Key = %q, want %q", evt.Key, event.KeyPRCreated)
	}

	pr := evt.PullRequest.PullRequest
	if pr.ID != 42 {
		t.Errorf("PR.ID = %d, want 42", pr.ID)
	}
	if pr.Title != "Add feature X" {
		t.Errorf("PR.Title = %q, want %q", pr.Title, "Add feature X")
	}
	if pr.Author.Nickname != "janeauthor" {
		t.Errorf("PR.Author.Nickname = %q, want %q", pr.Author.Nickname, "janeauthor")
	}
	if len(pr.Reviewers) != 1 {
		t.Fatalf("len(Reviewers) = %d, want 1", len(pr.Reviewers))
	}
	if pr.State != "OPEN" {
		t.Errorf("PR.State = %q, want %q", pr.State, "OPEN")
	}
	if pr.Source.Branch.Name != "feature/add-feature-x" {
		t.Errorf("Source.Branch = %q, want %q", pr.Source.Branch.Name, "feature/add-feature-x")
	}
	if pr.Destination.Branch.Name != "main" {
		t.Errorf("Destination.Branch = %q, want %q", pr.Destination.Branch.Name, "main")
	}

	repo := evt.PullRequest.Repository
	if repo.FullName != "myworkspace/my-repo" {
		t.Errorf("Repository.FullName = %q, want %q", repo.FullName, "myworkspace/my-repo")
	}
	if repo.Workspace.Slug != "myworkspace" {
		t.Errorf("Workspace.Slug = %q, want %q", repo.Workspace.Slug, "myworkspace")
	}
}

func TestParse_PullRequestUpdated(t *testing.T) {
	payload := loadFixture(t, "pullrequest/updated.json")
	evt, err := event.Parse(event.KeyPRUpdated, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pr := evt.PullRequest.PullRequest
	if pr.Title != "Add feature X (updated)" {
		t.Errorf("PR.Title = %q, want %q", pr.Title, "Add feature X (updated)")
	}
	if len(pr.Reviewers) != 2 {
		t.Fatalf("len(Reviewers) = %d, want 2", len(pr.Reviewers))
	}
	if pr.Reviewers[1].Nickname != "alicereviewer" {
		t.Errorf("Reviewers[1].Nickname = %q, want %q", pr.Reviewers[1].Nickname, "alicereviewer")
	}
}

func TestParse_PullRequestApproved(t *testing.T) {
	payload := loadFixture(t, "pullrequest/approved.json")
	evt, err := event.Parse(event.KeyPRApproved, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.PullRequest.Approval == nil {
		t.Fatal("expected Approval to be non-nil")
	}
	if evt.PullRequest.Approval.User.Nickname != "bobreviewer" {
		t.Errorf("Approval.User.Nickname = %q, want %q", evt.PullRequest.Approval.User.Nickname, "bobreviewer")
	}
}

func TestParse_PullRequestUnapproved(t *testing.T) {
	payload := loadFixture(t, "pullrequest/unapproved.json")
	evt, err := event.Parse(event.KeyPRUnapproved, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.PullRequest.Approval == nil {
		t.Fatal("expected Approval to be non-nil")
	}
	if evt.PullRequest.Approval.User.Nickname != "bobreviewer" {
		t.Errorf("Approval.User.Nickname = %q, want %q", evt.PullRequest.Approval.User.Nickname, "bobreviewer")
	}
}

func TestParse_PullRequestFulfilled(t *testing.T) {
	payload := loadFixture(t, "pullrequest/fulfilled.json")
	evt, err := event.Parse(event.KeyPRFulfilled, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pr := evt.PullRequest.PullRequest
	if pr.State != "MERGED" {
		t.Errorf("PR.State = %q, want %q", pr.State, "MERGED")
	}
	if pr.MergeCommit == nil {
		t.Fatal("expected MergeCommit to be non-nil")
	}
	if pr.MergeCommit.Hash != "764413d85e29" {
		t.Errorf("MergeCommit.Hash = %q, want %q", pr.MergeCommit.Hash, "764413d85e29")
	}
	if pr.ClosedBy == nil {
		t.Fatal("expected ClosedBy to be non-nil")
	}
	if pr.ClosedBy.Nickname != "janeauthor" {
		t.Errorf("ClosedBy.Nickname = %q, want %q", pr.ClosedBy.Nickname, "janeauthor")
	}
	if !pr.CloseSourceBranch {
		t.Error("expected CloseSourceBranch to be true")
	}
}

func TestParse_PullRequestRejected(t *testing.T) {
	payload := loadFixture(t, "pullrequest/rejected.json")
	evt, err := event.Parse(event.KeyPRRejected, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pr := evt.PullRequest.PullRequest
	if pr.State != "DECLINED" {
		t.Errorf("PR.State = %q, want %q", pr.State, "DECLINED")
	}
	if !strings.Contains(pr.Reason, "conflicts") {
		t.Errorf("PR.Reason = %q, expected to contain %q", pr.Reason, "conflicts")
	}
	if pr.ClosedBy == nil {
		t.Fatal("expected ClosedBy to be non-nil")
	}
	if pr.ClosedBy.Nickname != "bobreviewer" {
		t.Errorf("ClosedBy.Nickname = %q, want %q", pr.ClosedBy.Nickname, "bobreviewer")
	}
}

func TestParse_PullRequestCommentCreated(t *testing.T) {
	payload := loadFixture(t, "pullrequest/comment_created.json")
	evt, err := event.Parse(event.KeyPRCommentCreated, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.PullRequest.Comment == nil {
		t.Fatal("expected Comment to be non-nil")
	}

	c := evt.PullRequest.Comment
	if c.ID != 17 {
		t.Errorf("Comment.ID = %d, want 17", c.ID)
	}
	if !strings.Contains(c.Content.Raw, "unit test") {
		t.Errorf("Comment.Content.Raw = %q, expected to contain %q", c.Content.Raw, "unit test")
	}
	if c.Inline == nil {
		t.Fatal("expected Inline to be non-nil")
	}
	if c.Inline.Path != "pkg/handler/handler.go" {
		t.Errorf("Inline.Path = %q, want %q", c.Inline.Path, "pkg/handler/handler.go")
	}
	if c.Inline.To != 42 {
		t.Errorf("Inline.To = %d, want 42", c.Inline.To)
	}
}

func TestParse_PullRequestCommentCreated_ParentID(t *testing.T) {
	payload := []byte(`{
		"actor": {"nickname": "bob", "account_id": "acct-bob"},
		"pullrequest": {"id": 7, "title": "PR", "state": "OPEN",
			"author": {"nickname": "bob", "account_id": "acct-bob"},
			"source": {"branch": {"name": "feat"}, "commit": {"hash": "abc"}, "repository": {}},
			"destination": {"branch": {"name": "main"}, "commit": {"hash": "def"}, "repository": {}},
			"links": {"html": {"href": "https://bitbucket.org/pr/7"}}},
		"repository": {"full_name": "ws/repo", "name": "repo", "workspace": {"slug": "ws"}},
		"comment": {
			"id": 770172245,
			"content": {"raw": "reply to message"},
			"parent": {"id": 770164514},
			"links": {"html": {"href": "https://bitbucket.org/comment/770172245"}}
		}
	}`)
	evt, err := event.Parse(event.KeyPRCommentCreated, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	c := evt.PullRequest.Comment
	if c == nil {
		t.Fatal("expected Comment to be non-nil")
	}
	if c.ParentID != 770164514 {
		t.Errorf("Comment.ParentID = %d, want 770164514", c.ParentID)
	}
}

func TestParse_PullRequestCommentCreated_TopLevelParentID(t *testing.T) {
	// Existing comment_created fixture has no parent — ParentID should be 0.
	payload := loadFixture(t, "pullrequest/comment_created.json")
	evt, err := event.Parse(event.KeyPRCommentCreated, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.PullRequest.Comment.ParentID != 0 {
		t.Errorf("Comment.ParentID = %d, want 0 for top-level comment", evt.PullRequest.Comment.ParentID)
	}
}

func TestParse_CommitStatusCreated(t *testing.T) {
	payload := loadFixture(t, "commit_status/created.json")
	evt, err := event.Parse(event.KeyCommitStatusCreated, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.CommitStatus == nil {
		t.Fatal("expected CommitStatus to be non-nil")
	}
	if evt.PullRequest != nil {
		t.Fatal("expected PullRequest to be nil")
	}

	cs := evt.CommitStatus.CommitStatus
	if cs.State != "INPROGRESS" {
		t.Errorf("State = %q, want %q", cs.State, "INPROGRESS")
	}
	if cs.Key != "my-ci-tool" {
		t.Errorf("Key = %q, want %q", cs.Key, "my-ci-tool")
	}
	if cs.CommitHash != "b7f6f6ef4c59" {
		t.Errorf("CommitHash = %q, want %q", cs.CommitHash, "b7f6f6ef4c59")
	}
	if cs.Name != "Unit Tests" {
		t.Errorf("Name = %q, want %q", cs.Name, "Unit Tests")
	}

	repo := evt.CommitStatus.Repository
	if repo.FullName != "myworkspace/my-repo" {
		t.Errorf("Repository.FullName = %q, want %q", repo.FullName, "myworkspace/my-repo")
	}
}

func TestParse_CommitStatusUpdated(t *testing.T) {
	payload := loadFixture(t, "commit_status/updated.json")
	evt, err := event.Parse(event.KeyCommitStatusUpdated, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cs := evt.CommitStatus.CommitStatus
	if cs.State != "SUCCESSFUL" {
		t.Errorf("State = %q, want %q", cs.State, "SUCCESSFUL")
	}
}

func TestParse_UnknownEventKey(t *testing.T) {
	_, err := event.Parse("repo:push", []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for unknown event key")
	}
	if !strings.Contains(err.Error(), "unknown event key") {
		t.Errorf("error = %q, expected to contain %q", err.Error(), "unknown event key")
	}
}

func TestParse_MalformedJSON(t *testing.T) {
	_, err := event.Parse(event.KeyPRCreated, []byte(`{not json`))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestParse_EmptyPayload(t *testing.T) {
	_, err := event.Parse(event.KeyPRCreated, []byte{})
	if err == nil {
		t.Fatal("expected error for empty payload")
	}
	if !strings.Contains(err.Error(), "empty payload") {
		t.Errorf("error = %q, expected to contain %q", err.Error(), "empty payload")
	}
}

func TestParse_PipelineSpanCreated_PipelineRun(t *testing.T) {
	payload := loadFixture(t, "pipeline/span_created_successful.json")
	evt, err := event.Parse(event.KeyPipelineSpanCreated, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.Pipeline == nil {
		t.Fatal("expected Pipeline to be non-nil")
	}
	if evt.PullRequest != nil || evt.CommitStatus != nil {
		t.Fatal("expected PullRequest and CommitStatus to be nil")
	}

	run := evt.Pipeline.PipelineRun
	if run.RefName != "feature/add-feature-x" {
		t.Errorf("RefName = %q, want %q", run.RefName, "feature/add-feature-x")
	}
	if run.RefType != "BRANCH" {
		t.Errorf("RefType = %q, want %q", run.RefType, "BRANCH")
	}
	if run.UUID != "{aa111111-1111-1111-1111-111111111111}" {
		t.Errorf("UUID = %q, want %q", run.UUID, "{aa111111-1111-1111-1111-111111111111}")
	}
	if run.RunNumber != 5 {
		t.Errorf("RunNumber = %d, want 5", run.RunNumber)
	}
	// Real Bitbucket OTel payloads use COMPLETE (not SUCCESSFUL) for successful runs.
	if run.Result != "COMPLETE" {
		t.Errorf("Result = %q, want %q", run.Result, "COMPLETE")
	}
	if run.Trigger != "PUSH" {
		t.Errorf("Trigger = %q, want %q", run.Trigger, "PUSH")
	}
	// Real payloads omit pipeline.repository.full_name; use UUIDs instead.
	if run.RepoUUID != "{aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee}" {
		t.Errorf("RepoUUID = %q, want %q", run.RepoUUID, "{aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee}")
	}
	if run.AccountUUID != "{ffffffff-0000-1111-2222-333333333333}" {
		t.Errorf("AccountUUID = %q, want %q", run.AccountUUID, "{ffffffff-0000-1111-2222-333333333333}")
	}
	// Repository fields are populated by the handler after API resolution, not by the parser.
	if run.Repository.FullName != "" {
		t.Errorf("Repository.FullName = %q, want empty (resolved by handler)", run.Repository.FullName)
	}
	// URL comes directly from the pipeline_run.url span attribute.
	wantURL := "https://bitbucket.org/%7Bffffffff%7D/%7Baaaaaaaa%7D/pipelines/results/5"
	if run.URL != wantURL {
		t.Errorf("URL = %q, want %q", run.URL, wantURL)
	}
}

func TestParse_PipelineSpanCreated_NonPipelineRunSpan(t *testing.T) {
	// A pipeline:span_created payload containing only a step span — not a pipeline_run.
	// Should parse without error and return an Event with nil Pipeline.
	payload := []byte(`{
		"resourceSpans": [{
			"scopeSpans": [{
				"spans": [{
					"name": "bbc.pipeline_step",
					"attributes": []
				}]
			}]
		}]
	}`)

	evt, err := event.Parse(event.KeyPipelineSpanCreated, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.Pipeline != nil {
		t.Error("expected Pipeline to be nil for non-pipeline_run span")
	}
}

func TestCommitHashFromHref(t *testing.T) {
	href := "https://api.bitbucket.org/2.0/repositories/myworkspace/my-repo/commit/b7f6f6ef4c59"
	hash, err := event.CommitHashFromHref(href)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash != "b7f6f6ef4c59" {
		t.Errorf("hash = %q, want %q", hash, "b7f6f6ef4c59")
	}
}

func TestCommitHashFromHref_NoCommitSegment(t *testing.T) {
	_, err := event.CommitHashFromHref("https://example.com/repos/foo/bar")
	if err == nil {
		t.Fatal("expected error for href without /commit/ segment")
	}
}
