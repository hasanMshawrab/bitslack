package bitbucket_test

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hasanMshawrab/bbthread/internal/bitbucket"
)

const cannedPRJSON = `{
  "id": 42,
  "title": "Add feature X",
  "state": "OPEN",
  "author": {
    "nickname": "janeauthor",
    "display_name": "Jane Author",
    "uuid": "{user-uuid-jane}",
    "account_id": "abc123"
  },
  "source": {
    "branch": { "name": "feature/x" },
    "commit": { "hash": "abc123def456" },
    "repository": {
      "full_name": "myworkspace/my-repo",
      "name": "my-repo",
      "links": { "html": { "href": "https://bitbucket.org/myworkspace/my-repo" } }
    }
  },
  "destination": {
    "branch": { "name": "main" },
    "commit": { "hash": "000111222333" },
    "repository": {
      "full_name": "myworkspace/my-repo",
      "name": "my-repo",
      "links": { "html": { "href": "https://bitbucket.org/myworkspace/my-repo" } }
    }
  },
  "reviewers": [
    {
      "nickname": "bobreviewer",
      "display_name": "Bob Reviewer",
      "uuid": "{user-uuid-bob}",
      "account_id": "def456"
    }
  ],
  "reason": "",
  "merge_commit": null,
  "closed_by": null,
  "close_source_branch": true,
  "created_on": "2025-01-15T10:00:00.000000+00:00",
  "updated_on": "2025-01-15T12:00:00.000000+00:00",
  "links": {
    "html": { "href": "https://bitbucket.org/myworkspace/my-repo/pull-requests/42" }
  }
}`

const cannedPRListJSON = `{
  "values": [` + cannedPRJSON + `]
}`

const cannedEmptyListJSON = `{
  "values": []
}`

func TestGetPullRequest_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repositories/myworkspace/my-repo/pullrequests/42" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(cannedPRJSON))
	}))
	defer srv.Close()

	client := bitbucket.NewClient(srv.URL, "test-user", "test-token", bitbucket.WithHTTPClient(srv.Client()))
	pr, err := client.GetPullRequest(context.Background(), "myworkspace", "my-repo", 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pr.ID != 42 {
		t.Errorf("ID = %d, want 42", pr.ID)
	}
	if pr.Title != "Add feature X" {
		t.Errorf("Title = %q, want %q", pr.Title, "Add feature X")
	}
	if pr.State != "OPEN" {
		t.Errorf("State = %q, want %q", pr.State, "OPEN")
	}
	if pr.Author.Nickname != "janeauthor" {
		t.Errorf("Author.Nickname = %q, want %q", pr.Author.Nickname, "janeauthor")
	}
	if len(pr.Reviewers) != 1 {
		t.Fatalf("len(Reviewers) = %d, want 1", len(pr.Reviewers))
	}
	if pr.Reviewers[0].Nickname != "bobreviewer" {
		t.Errorf("Reviewers[0].Nickname = %q, want %q", pr.Reviewers[0].Nickname, "bobreviewer")
	}
	if pr.Source.Branch.Name != "feature/x" {
		t.Errorf("Source.Branch.Name = %q, want %q", pr.Source.Branch.Name, "feature/x")
	}
	if pr.Destination.Branch.Name != "main" {
		t.Errorf("Destination.Branch.Name = %q, want %q", pr.Destination.Branch.Name, "main")
	}
	if pr.HTMLURL != "https://bitbucket.org/myworkspace/my-repo/pull-requests/42" {
		t.Errorf("HTMLURL = %q, want PR link", pr.HTMLURL)
	}
	if !pr.CloseSourceBranch {
		t.Error("CloseSourceBranch = false, want true")
	}
}

func TestGetPullRequest_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	client := bitbucket.NewClient(srv.URL, "test-user", "test-token", bitbucket.WithHTTPClient(srv.Client()))
	_, err := client.GetPullRequest(context.Background(), "myworkspace", "my-repo", 999)
	if !errors.Is(err, bitbucket.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestGetPullRequest_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := bitbucket.NewClient(srv.URL, "test-user", "test-token", bitbucket.WithHTTPClient(srv.Client()))
	_, err := client.GetPullRequest(context.Background(), "myworkspace", "my-repo", 42)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestGetPullRequest_MalformedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{not valid json`))
	}))
	defer srv.Close()

	client := bitbucket.NewClient(srv.URL, "test-user", "test-token", bitbucket.WithHTTPClient(srv.Client()))
	_, err := client.GetPullRequest(context.Background(), "myworkspace", "my-repo", 42)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestGetPullRequestsForCommit_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repositories/myworkspace/my-repo/commit/b7f6f6ef4c59/pullrequests" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(cannedPRListJSON))
	}))
	defer srv.Close()

	client := bitbucket.NewClient(srv.URL, "test-user", "test-token", bitbucket.WithHTTPClient(srv.Client()))
	prs, err := client.GetPullRequestsForCommit(context.Background(), "myworkspace", "my-repo", "b7f6f6ef4c59")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("len(prs) = %d, want 1", len(prs))
	}
	if prs[0].ID != 42 {
		t.Errorf("prs[0].ID = %d, want 42", prs[0].ID)
	}
}

func TestGetPullRequestsForCommit_NoPRs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(cannedEmptyListJSON))
	}))
	defer srv.Close()

	client := bitbucket.NewClient(srv.URL, "test-user", "test-token", bitbucket.WithHTTPClient(srv.Client()))
	prs, err := client.GetPullRequestsForCommit(context.Background(), "myworkspace", "my-repo", "deadbeef")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prs) != 0 {
		t.Errorf("len(prs) = %d, want 0", len(prs))
	}
}

func TestGetPullRequestsForCommit_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := bitbucket.NewClient(srv.URL, "test-user", "test-token", bitbucket.WithHTTPClient(srv.Client()))
	_, err := client.GetPullRequestsForCommit(context.Background(), "myworkspace", "my-repo", "abc123")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestClient_AuthorizationHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(cannedPRJSON))
	}))
	defer srv.Close()

	client := bitbucket.NewClient(srv.URL, "test-user", "test-token", bitbucket.WithHTTPClient(srv.Client()))
	_, _ = client.GetPullRequest(context.Background(), "myworkspace", "my-repo", 42)

	want := "Basic " + base64Encode("test-user:test-token")
	if gotAuth != want {
		t.Errorf("Authorization header = %q, want %q", gotAuth, want)
	}
}

func base64Encode(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

func TestGetOpenPRForBranch_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repositories/myworkspace/my-repo/pullrequests" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(cannedPRListJSON))
	}))
	defer srv.Close()

	client := bitbucket.NewClient(srv.URL, "test-user", "test-token", bitbucket.WithHTTPClient(srv.Client()))
	pr, err := client.GetOpenPRForBranch(context.Background(), "myworkspace", "my-repo", "feature/x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr == nil {
		t.Fatal("expected non-nil PR")
	}
	if pr.ID != 42 {
		t.Errorf("ID = %d, want 42", pr.ID)
	}
}

func TestGetOpenPRForBranch_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(cannedEmptyListJSON))
	}))
	defer srv.Close()

	client := bitbucket.NewClient(srv.URL, "test-user", "test-token", bitbucket.WithHTTPClient(srv.Client()))
	pr, err := client.GetOpenPRForBranch(context.Background(), "myworkspace", "my-repo", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr != nil {
		t.Errorf("expected nil PR for empty list, got %+v", pr)
	}
}

func TestGetOpenPRForBranch_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := bitbucket.NewClient(srv.URL, "test-user", "test-token", bitbucket.WithHTTPClient(srv.Client()))
	_, err := client.GetOpenPRForBranch(context.Background(), "myworkspace", "my-repo", "feature/x")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

const cannedPipelineStepsJSON = `{
  "values": [
    {
      "uuid": "{step-001}",
      "name": "Build",
      "state": {
        "name": "COMPLETED",
        "result": {"name": "SUCCESSFUL"}
      },
      "duration_in_seconds": 12
    },
    {
      "uuid": "{step-002}",
      "name": "Test",
      "state": {
        "name": "COMPLETED",
        "result": {"name": "FAILED"}
      },
      "duration_in_seconds": 18
    },
    {
      "uuid": "{step-003}",
      "name": "Deploy",
      "state": {
        "name": "NOT_RUN"
      },
      "duration_in_seconds": 0
    }
  ]
}`

func TestGetPipelineSteps_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// r.URL.Path is the URL-decoded path; {} are not path-reserved chars in RFC 3986 path segments.
		const wantPath = "/repositories/myworkspace/my-repo/pipelines/{pipeline-uuid}/steps/"
		if r.URL.Path != wantPath {
			t.Errorf("unexpected path: %s, want %s", r.URL.Path, wantPath)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(cannedPipelineStepsJSON))
	}))
	defer srv.Close()

	client := bitbucket.NewClient(srv.URL, "test-user", "test-token", bitbucket.WithHTTPClient(srv.Client()))
	steps, err := client.GetPipelineSteps(context.Background(), "myworkspace", "my-repo", "{pipeline-uuid}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 3 {
		t.Fatalf("len(steps) = %d, want 3", len(steps))
	}

	if steps[0].Name != "Build" {
		t.Errorf("steps[0].Name = %q, want %q", steps[0].Name, "Build")
	}
	if steps[0].Result != "SUCCESSFUL" {
		t.Errorf("steps[0].Result = %q, want SUCCESSFUL", steps[0].Result)
	}
	if steps[0].DurationSecs != 12 {
		t.Errorf("steps[0].DurationSecs = %d, want 12", steps[0].DurationSecs)
	}

	if steps[1].Name != "Test" {
		t.Errorf("steps[1].Name = %q, want %q", steps[1].Name, "Test")
	}
	if steps[1].Result != "FAILED" {
		t.Errorf("steps[1].Result = %q, want FAILED", steps[1].Result)
	}

	// NOT_RUN step: no result in state, falls back to "NOT_RUN".
	if steps[2].Result != "NOT_RUN" {
		t.Errorf("steps[2].Result = %q, want NOT_RUN", steps[2].Result)
	}
}

func TestGetPipelineSteps_Stopped(t *testing.T) {
	const stoppedJSON = `{
		"values": [
			{
				"uuid": "{step-001}",
				"name": "Build",
				"state": {"name": "STOPPED"},
				"duration_in_seconds": 5
			}
		]
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(stoppedJSON))
	}))
	defer srv.Close()

	client := bitbucket.NewClient(srv.URL, "test-user", "test-token", bitbucket.WithHTTPClient(srv.Client()))
	steps, err := client.GetPipelineSteps(context.Background(), "myworkspace", "my-repo", "{uuid}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("len(steps) = %d, want 1", len(steps))
	}
	if steps[0].Result != "STOPPED" {
		t.Errorf("steps[0].Result = %q, want STOPPED", steps[0].Result)
	}
}

func TestGetPipelineSteps_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := bitbucket.NewClient(srv.URL, "test-user", "test-token", bitbucket.WithHTTPClient(srv.Client()))
	_, err := client.GetPipelineSteps(context.Background(), "myworkspace", "my-repo", "{uuid}")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

const cannedPipelineListSuccessfulJSON = `{
  "values": [
    {
      "build_number": 42,
      "state": {
        "name": "COMPLETED",
        "result": {"name": "SUCCESSFUL"}
      },
      "links": {
        "html": {"href": "https://bitbucket.org/myworkspace/my-repo/pipelines/results/42"}
      }
    }
  ]
}`

const cannedPipelineListInProgressJSON = `{
  "values": [
    {
      "build_number": 7,
      "state": {
        "name": "IN_PROGRESS"
      },
      "links": {
        "html": {"href": "https://bitbucket.org/myworkspace/my-repo/pipelines/results/7"}
      }
    }
  ]
}`

func TestGetLatestPipelineForBranch_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repositories/myworkspace/my-repo/pipelines/" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("sort") != "-created_on" {
			t.Errorf("expected sort=-created_on, got %q", r.URL.Query().Get("sort"))
		}
		if r.URL.Query().Get("pagelen") != "1" {
			t.Errorf("expected pagelen=1, got %q", r.URL.Query().Get("pagelen"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(cannedPipelineListSuccessfulJSON))
	}))
	defer srv.Close()

	client := bitbucket.NewClient(srv.URL, "test-user", "test-token", bitbucket.WithHTTPClient(srv.Client()))
	run, err := client.GetLatestPipelineForBranch(context.Background(), "myworkspace", "my-repo", "feature/x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if run == nil {
		t.Fatal("expected non-nil run")
	}
	if run.RunNumber != 42 {
		t.Errorf("RunNumber = %d, want 42", run.RunNumber)
	}
	if run.Result != "SUCCESSFUL" {
		t.Errorf("Result = %q, want SUCCESSFUL", run.Result)
	}
	if run.URL != "https://bitbucket.org/myworkspace/my-repo/pipelines/results/42" {
		t.Errorf("URL = %q, want pipeline URL", run.URL)
	}
}

func TestGetLatestPipelineForBranch_InProgress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(cannedPipelineListInProgressJSON))
	}))
	defer srv.Close()

	client := bitbucket.NewClient(srv.URL, "test-user", "test-token", bitbucket.WithHTTPClient(srv.Client()))
	run, err := client.GetLatestPipelineForBranch(context.Background(), "myworkspace", "my-repo", "feature/x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if run == nil {
		t.Fatal("expected non-nil run")
	}
	if run.Result != "IN_PROGRESS" {
		t.Errorf("Result = %q, want IN_PROGRESS", run.Result)
	}
}

func TestGetLatestPipelineForBranch_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"values":[]}`))
	}))
	defer srv.Close()

	client := bitbucket.NewClient(srv.URL, "test-user", "test-token", bitbucket.WithHTTPClient(srv.Client()))
	run, err := client.GetLatestPipelineForBranch(context.Background(), "myworkspace", "my-repo", "feature/x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if run != nil {
		t.Errorf("expected nil run for empty list, got %+v", run)
	}
}

func TestGetLatestPipelineForBranch_Forbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	client := bitbucket.NewClient(srv.URL, "test-user", "test-token", bitbucket.WithHTTPClient(srv.Client()))
	run, err := client.GetLatestPipelineForBranch(context.Background(), "myworkspace", "my-repo", "feature/x")
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
	if run != nil {
		t.Errorf("expected nil run on error, got %+v", run)
	}
}
