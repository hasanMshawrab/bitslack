package bitbucket_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hasanMshawrab/bitslack/internal/bitbucket"
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

	client := bitbucket.NewClient(srv.URL, "test-token", bitbucket.WithHTTPClient(srv.Client()))
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

	client := bitbucket.NewClient(srv.URL, "test-token", bitbucket.WithHTTPClient(srv.Client()))
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

	client := bitbucket.NewClient(srv.URL, "test-token", bitbucket.WithHTTPClient(srv.Client()))
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

	client := bitbucket.NewClient(srv.URL, "test-token", bitbucket.WithHTTPClient(srv.Client()))
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

	client := bitbucket.NewClient(srv.URL, "test-token", bitbucket.WithHTTPClient(srv.Client()))
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

	client := bitbucket.NewClient(srv.URL, "test-token", bitbucket.WithHTTPClient(srv.Client()))
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

	client := bitbucket.NewClient(srv.URL, "test-token", bitbucket.WithHTTPClient(srv.Client()))
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

	client := bitbucket.NewClient(srv.URL, "test-token", bitbucket.WithHTTPClient(srv.Client()))
	_, _ = client.GetPullRequest(context.Background(), "myworkspace", "my-repo", 42)

	want := "Bearer test-token"
	if gotAuth != want {
		t.Errorf("Authorization header = %q, want %q", gotAuth, want)
	}
}
