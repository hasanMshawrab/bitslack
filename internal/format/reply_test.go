package format_test

import (
	"testing"

	"github.com/hasanMshawrab/bitslack/internal/event"
	"github.com/hasanMshawrab/bitslack/internal/format"
)

func defaultResolver() format.UserResolver {
	return mapResolver(map[string]string{
		"acct-jane": "U001JANE",
		"acct-bob":  "U002BOB",
	})
}

func TestReply_Approved(t *testing.T) {
	ev := &event.Event{
		Key: event.KeyPRApproved,
		PullRequest: &event.PullRequestEvent{
			Actor: event.User{Nickname: "bobreviewer", AccountID: "acct-bob"},
		},
	}
	text, err := format.Reply(ev, defaultResolver())
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "approved")
	assertContains(t, text, "<@U002BOB>")
}

func TestReply_Unapproved(t *testing.T) {
	ev := &event.Event{
		Key: event.KeyPRUnapproved,
		PullRequest: &event.PullRequestEvent{
			Actor: event.User{Nickname: "bobreviewer", AccountID: "acct-bob"},
		},
	}
	text, err := format.Reply(ev, defaultResolver())
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "removed")
}

func TestReply_Fulfilled(t *testing.T) {
	ev := &event.Event{
		Key: event.KeyPRFulfilled,
		PullRequest: &event.PullRequestEvent{
			Actor: event.User{Nickname: "janeauthor", AccountID: "acct-jane"},
		},
	}
	text, err := format.Reply(ev, defaultResolver())
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "merged")
}

func TestReply_Rejected(t *testing.T) {
	ev := &event.Event{
		Key: event.KeyPRRejected,
		PullRequest: &event.PullRequestEvent{
			Actor:       event.User{Nickname: "janeauthor", AccountID: "acct-jane"},
			PullRequest: event.PullRequest{Reason: "needs more work"},
		},
	}
	text, err := format.Reply(ev, defaultResolver())
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "declined")
	assertContains(t, text, "needs more work")
}

func TestReply_CommentCreated_Inline(t *testing.T) {
	ev := &event.Event{
		Key: event.KeyPRCommentCreated,
		PullRequest: &event.PullRequestEvent{
			Actor: event.User{Nickname: "bobreviewer", AccountID: "acct-bob"},
			Comment: &event.Comment{
				Content: event.CommentContent{Raw: "fix this"},
				Inline:  &event.InlineLocation{Path: "main.go", To: 42},
				HTMLURL: "https://bitbucket.org/comment/1",
			},
		},
	}
	text, err := format.Reply(ev, defaultResolver())
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "main.go")
	assertContains(t, text, "42")
	assertContains(t, text, "fix this")
}

func TestReply_CommentCreated_TopLevel(t *testing.T) {
	ev := &event.Event{
		Key: event.KeyPRCommentCreated,
		PullRequest: &event.PullRequestEvent{
			Actor: event.User{Nickname: "bobreviewer", AccountID: "acct-bob"},
			Comment: &event.Comment{
				Content: event.CommentContent{Raw: "looks good"},
				Inline:  nil,
				HTMLURL: "https://bitbucket.org/comment/2",
			},
		},
	}
	text, err := format.Reply(ev, defaultResolver())
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "looks good")
	assertNotContains(t, text, "main.go")
}

func TestReply_CommitStatusInProgress(t *testing.T) {
	ev := &event.Event{
		Key: event.KeyCommitStatusCreated,
		CommitStatus: &event.CommitStatusEvent{
			CommitStatus: event.CommitStatus{
				Name:  "CI Pipeline",
				State: "INPROGRESS",
				Key:   "ci/build",
				URL:   "https://ci.example.com/1",
			},
		},
	}
	text, err := format.Reply(ev, defaultResolver())
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "CI Pipeline")
	assertContains(t, text, "is running")
}

func TestReply_CommitStatusSuccessful(t *testing.T) {
	ev := &event.Event{
		Key: event.KeyCommitStatusUpdated,
		CommitStatus: &event.CommitStatusEvent{
			CommitStatus: event.CommitStatus{
				Name:  "CI Pipeline",
				State: "SUCCESSFUL",
				Key:   "ci/build",
				URL:   "https://ci.example.com/1",
			},
		},
	}
	text, err := format.Reply(ev, defaultResolver())
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "passed")
}

func TestReply_CommitStatusFailed(t *testing.T) {
	ev := &event.Event{
		Key: event.KeyCommitStatusUpdated,
		CommitStatus: &event.CommitStatusEvent{
			CommitStatus: event.CommitStatus{
				Name:  "CI Pipeline",
				State: "FAILED",
				Key:   "ci/build",
				URL:   "https://ci.example.com/1",
			},
		},
	}
	text, err := format.Reply(ev, defaultResolver())
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "failed")
}

func TestReply_UnknownKey(t *testing.T) {
	// Completely unknown key
	ev := &event.Event{Key: "repo:push"}
	_, err := format.Reply(ev, defaultResolver())
	if err == nil {
		t.Fatal("expected error for unknown key")
	}

	// KeyPRCreated should also return error (handled by opening message, not reply)
	ev2 := &event.Event{
		Key:         event.KeyPRCreated,
		PullRequest: &event.PullRequestEvent{},
	}
	_, err2 := format.Reply(ev2, defaultResolver())
	if err2 == nil {
		t.Fatal("expected error for KeyPRCreated")
	}

	// KeyPRUpdated should also return error (handled by chat.update, not reply)
	ev3 := &event.Event{
		Key:         event.KeyPRUpdated,
		PullRequest: &event.PullRequestEvent{},
	}
	_, err3 := format.Reply(ev3, defaultResolver())
	if err3 == nil {
		t.Fatal("expected error for KeyPRUpdated")
	}
}

func TestReply_UnmappedActor(t *testing.T) {
	ev := &event.Event{
		Key: event.KeyPRApproved,
		PullRequest: &event.PullRequestEvent{
			Actor: event.User{Nickname: "unknownuser", AccountID: "acct-unknown"},
		},
	}
	resolve := mapResolver(map[string]string{}) // empty
	text, err := format.Reply(ev, resolve)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "@unknownuser")
	assertNotContains(t, text, "<@")
}
