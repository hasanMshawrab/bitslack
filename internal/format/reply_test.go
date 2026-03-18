package format_test

import (
	"strings"
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
	text, err := format.Reply(ev, defaultResolver(), format.Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "✅")
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
	text, err := format.Reply(ev, defaultResolver(), format.Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "↩️")
	assertContains(t, text, "removed")
}

func TestReply_Fulfilled(t *testing.T) {
	ev := &event.Event{
		Key: event.KeyPRFulfilled,
		PullRequest: &event.PullRequestEvent{
			Actor: event.User{Nickname: "janeauthor", AccountID: "acct-jane"},
		},
	}
	text, err := format.Reply(ev, defaultResolver(), format.Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "🎉")
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
	text, err := format.Reply(ev, defaultResolver(), format.Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "🚫")
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
	text, err := format.Reply(ev, defaultResolver(), format.Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "💬")
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
	text, err := format.Reply(ev, defaultResolver(), format.Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "💬")
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
	text, err := format.Reply(ev, defaultResolver(), format.Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "🔄")
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
	text, err := format.Reply(ev, defaultResolver(), format.Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "✅")
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
	text, err := format.Reply(ev, defaultResolver(), format.Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "❌")
	assertContains(t, text, "failed")
}

func TestReply_UnknownKey(t *testing.T) {
	// Completely unknown key
	ev := &event.Event{Key: "repo:push"}
	_, err := format.Reply(ev, defaultResolver(), format.Options{})
	if err == nil {
		t.Fatal("expected error for unknown key")
	}

	// KeyPRCreated should also return error (handled by opening message, not reply)
	ev2 := &event.Event{
		Key:         event.KeyPRCreated,
		PullRequest: &event.PullRequestEvent{},
	}
	_, err2 := format.Reply(ev2, defaultResolver(), format.Options{})
	if err2 == nil {
		t.Fatal("expected error for KeyPRCreated")
	}

	// KeyPRUpdated should also return error (handled by chat.update, not reply)
	ev3 := &event.Event{
		Key:         event.KeyPRUpdated,
		PullRequest: &event.PullRequestEvent{},
	}
	_, err3 := format.Reply(ev3, defaultResolver(), format.Options{})
	if err3 == nil {
		t.Fatal("expected error for KeyPRUpdated")
	}
}

func TestReply_PipelineSuccessful(t *testing.T) {
	ev := &event.Event{
		Key: event.KeyPipelineSpanCreated,
		Pipeline: &event.PipelineRunEvent{
			PipelineRun: event.PipelineRun{
				RunNumber:  5,
				Result:     "SUCCESSFUL",
				Trigger:    "PUSH",
				RefName:    "feature/add-feature-x",
				URL:        "https://bitbucket.org/myworkspace/my-repo/pipelines/results/{aa111111}",
				Repository: event.Repository{Name: "my-repo"},
			},
		},
	}
	text, err := format.Reply(ev, defaultResolver(), format.Options{})
	if err != nil {
		t.Fatal(err)
	}
	// New format: ⚙️ fixed leading emoji; result shown after "—"
	assertContains(t, text, "⚙️")
	assertContains(t, text, "✅")
	assertContains(t, text, "Passed")
	const pipelineURL = "https://bitbucket.org/myworkspace/my-repo/pipelines/results/{aa111111}"
	assertContains(t, text, "*[my-repo] Pipeline <"+pipelineURL+"|#5>*")
	assertContains(t, text, "feature/add-feature-x")
	assertContains(t, text, "automatic trigger")
}

func TestReply_PipelineFailed(t *testing.T) {
	ev := &event.Event{
		Key: event.KeyPipelineSpanCreated,
		Pipeline: &event.PipelineRunEvent{
			PipelineRun: event.PipelineRun{
				RunNumber: 6,
				Result:    "FAILED",
				Trigger:   "PUSH",
				RefName:   "feature/add-feature-x",
				URL:       "https://bitbucket.org/myworkspace/my-repo/pipelines/results/{bb222222}",
			},
		},
	}
	text, err := format.Reply(ev, defaultResolver(), format.Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "❌")
	assertContains(t, text, "Failed")
	assertContains(t, text, "#6")
}

func TestReply_PipelineError(t *testing.T) {
	ev := &event.Event{
		Key: event.KeyPipelineSpanCreated,
		Pipeline: &event.PipelineRunEvent{
			PipelineRun: event.PipelineRun{Result: "ERROR", RefName: "main", URL: "https://example.com"},
		},
	}
	text, err := format.Reply(ev, defaultResolver(), format.Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "🔴")
	assertContains(t, text, "Error")
}

func TestReply_PipelineStopped(t *testing.T) {
	ev := &event.Event{
		Key: event.KeyPipelineSpanCreated,
		Pipeline: &event.PipelineRunEvent{
			PipelineRun: event.PipelineRun{Result: "STOPPED", RefName: "main", URL: "https://example.com"},
		},
	}
	text, err := format.Reply(ev, defaultResolver(), format.Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "⏹")
	assertContains(t, text, "Stopped")
}

func TestReply_PipelineComplete(t *testing.T) {
	// OTel result "COMPLETE" maps to ✅ Passed.
	ev := &event.Event{
		Key: event.KeyPipelineSpanCreated,
		Pipeline: &event.PipelineRunEvent{
			PipelineRun: event.PipelineRun{
				Result:  "COMPLETE",
				RefName: "main",
				URL:     "https://example.com",
			},
		},
	}
	text, err := format.Reply(ev, defaultResolver(), format.Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "✅")
	assertContains(t, text, "Passed")
}

func TestReply_PipelineLinkedToPR_OmitsRepoAndBranch(t *testing.T) {
	ev := &event.Event{
		Key: event.KeyPipelineSpanCreated,
		Pipeline: &event.PipelineRunEvent{
			PipelineRun: event.PipelineRun{
				Result:  "COMPLETE",
				RefName: "feature/cool-thing",
				URL:     "https://example.com/pipelines/1",
				RunNumber: 7,
				Repository: event.Repository{Name: "my-repo"},
			},
		},
	}
	text, err := format.Reply(ev, defaultResolver(), format.Options{PipelineLinkedToPR: true})
	if err != nil {
		t.Fatal(err)
	}
	assertNotContains(t, text, "my-repo")
	assertNotContains(t, text, "feature/cool-thing")
	assertContains(t, text, "#7")
	assertContains(t, text, "✅")
	assertContains(t, text, "Passed")
}

func TestReply_PipelineWithSteps_FailedStepLinked(t *testing.T) {
	// Failed and error steps should be hyperlinked; successful and not-run steps should not.
	ev := &event.Event{
		Key: event.KeyPipelineSpanCreated,
		Pipeline: &event.PipelineRunEvent{
			PipelineRun: event.PipelineRun{
				Result:  "FAILED",
				RefName: "main",
				URL:     "https://example.com/pipeline",
			},
			Steps: []event.PipelineStep{
				{Name: "Lint", Result: "SUCCESSFUL", DurationSecs: 12},
				{Name: "Test", Result: "FAILED", DurationSecs: 18, URL: "https://example.com/step/2"},
				{Name: "Deploy", Result: "NOT_RUN"},
			},
		},
	}
	text, err := format.Reply(ev, defaultResolver(), format.Options{})
	if err != nil {
		t.Fatal(err)
	}
	// ⚙️ header with overall result
	assertContains(t, text, "⚙️")
	assertContains(t, text, "❌")
	assertContains(t, text, "Failed")
	// Step breakdown
	assertContains(t, text, "✅ Lint")
	assertContains(t, text, "• 12s")
	// Failed step is linked.
	assertContains(t, text, "<https://example.com/step/2|Test>")
	assertContains(t, text, "• 18s")
	// NOT_RUN step shown with ⏭ and no link.
	assertContains(t, text, "⏭ Deploy")
}

func TestReply_PipelineWithSteps_ErrorStepLinked(t *testing.T) {
	ev := &event.Event{
		Key: event.KeyPipelineSpanCreated,
		Pipeline: &event.PipelineRunEvent{
			PipelineRun: event.PipelineRun{
				Result:  "ERROR",
				RefName: "main",
				URL:     "https://example.com/pipeline",
			},
			Steps: []event.PipelineStep{
				{Name: "Build", Result: "ERROR", URL: "https://example.com/step/1"},
			},
		},
	}
	text, err := format.Reply(ev, defaultResolver(), format.Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "<https://example.com/step/1|Build>")
}

func TestReply_PipelineWithSteps_StoppedStepNotLinked(t *testing.T) {
	ev := &event.Event{
		Key: event.KeyPipelineSpanCreated,
		Pipeline: &event.PipelineRunEvent{
			PipelineRun: event.PipelineRun{
				Result:  "STOPPED",
				RefName: "main",
				URL:     "https://example.com/pipeline",
			},
			Steps: []event.PipelineStep{
				{Name: "Build", Result: "STOPPED", URL: "https://example.com/step/1"},
			},
		},
	}
	text, err := format.Reply(ev, defaultResolver(), format.Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "🛑 Build")
	// Stopped step is NOT linked.
	assertNotContains(t, text, "<https://example.com/step/1|Build>")
}

func TestReply_PipelineDuration(t *testing.T) {
	ev := &event.Event{
		Key: event.KeyPipelineSpanCreated,
		Pipeline: &event.PipelineRunEvent{
			PipelineRun: event.PipelineRun{
				Result:       "COMPLETE",
				RefName:      "main",
				URL:          "https://example.com",
				DurationSecs: 300, // 5 minutes
			},
		},
	}
	text, err := format.Reply(ev, defaultResolver(), format.Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "5m 0s")
}

func TestReply_PipelineNoDuration(t *testing.T) {
	ev := &event.Event{
		Key: event.KeyPipelineSpanCreated,
		Pipeline: &event.PipelineRunEvent{
			PipelineRun: event.PipelineRun{
				Result:       "COMPLETE",
				RefName:      "main",
				URL:          "https://example.com",
				DurationSecs: 0, // no duration
			},
		},
	}
	text, err := format.Reply(ev, defaultResolver(), format.Options{})
	if err != nil {
		t.Fatal(err)
	}
	// No duration bullet in the output.
	if strings.Count(text, "•") != 2 { // only "• branch • trigger"
		t.Errorf("expected exactly 2 bullets (branch + trigger) with no duration, got: %q", text)
	}
}

func TestReply_CommentCreated_ShowLink(t *testing.T) {
	ev := &event.Event{
		Key: event.KeyPRCommentCreated,
		PullRequest: &event.PullRequestEvent{
			Actor: event.User{Nickname: "bobreviewer", AccountID: "acct-bob"},
			Comment: &event.Comment{
				Content: event.CommentContent{Raw: "looks good"},
				HTMLURL: "https://bitbucket.org/comment/99",
			},
		},
	}
	text, err := format.Reply(ev, defaultResolver(), format.Options{ShowCommentLink: true})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "<https://bitbucket.org/comment/99|View comment>")
	assertNotContains(t, text, "https://bitbucket.org/comment/99\n")
}

func TestReply_CommentCreated_HideLink(t *testing.T) {
	ev := &event.Event{
		Key: event.KeyPRCommentCreated,
		PullRequest: &event.PullRequestEvent{
			Actor: event.User{Nickname: "bobreviewer", AccountID: "acct-bob"},
			Comment: &event.Comment{
				Content: event.CommentContent{Raw: "looks good"},
				HTMLURL: "https://bitbucket.org/comment/99",
			},
		},
	}
	text, err := format.Reply(ev, defaultResolver(), format.Options{ShowCommentLink: false})
	if err != nil {
		t.Fatal(err)
	}
	assertNotContains(t, text, "bitbucket.org")
}

func TestReply_CommentCreated_ReplyDistinguished(t *testing.T) {
	ev := &event.Event{
		Key: event.KeyPRCommentCreated,
		PullRequest: &event.PullRequestEvent{
			Actor: event.User{Nickname: "bobreviewer", AccountID: "acct-bob"},
			Comment: &event.Comment{
				Content:  event.CommentContent{Raw: "thanks"},
				ParentID: 770164514,
			},
		},
	}
	text, err := format.Reply(ev, defaultResolver(), format.Options{DistinguishCommentReplies: true})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "replied to a comment")
	assertNotContains(t, text, "commented\n")
}

func TestReply_CommentCreated_TopLevelNotDistinguished(t *testing.T) {
	ev := &event.Event{
		Key: event.KeyPRCommentCreated,
		PullRequest: &event.PullRequestEvent{
			Actor: event.User{Nickname: "bobreviewer", AccountID: "acct-bob"},
			Comment: &event.Comment{
				Content:  event.CommentContent{Raw: "lgtm"},
				ParentID: 0, // top-level
			},
		},
	}
	text, err := format.Reply(ev, defaultResolver(), format.Options{DistinguishCommentReplies: true})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "commented")
	assertNotContains(t, text, "replied")
}

func TestReply_CommentCreated_Summary(t *testing.T) {
	longText := "This is a very long comment that should be truncated to the summary length"
	ev := &event.Event{
		Key: event.KeyPRCommentCreated,
		PullRequest: &event.PullRequestEvent{
			Actor: event.User{Nickname: "bobreviewer", AccountID: "acct-bob"},
			Comment: &event.Comment{
				Content: event.CommentContent{Raw: longText},
			},
		},
	}
	text, err := format.Reply(ev, defaultResolver(), format.Options{
		CommentContent:       format.CommentDisplaySummary,
		CommentSummaryLength: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "…")
	assertNotContains(t, text, longText)
}

func TestReply_CommentCreated_SummaryWithinLength(t *testing.T) {
	shortText := "short"
	ev := &event.Event{
		Key: event.KeyPRCommentCreated,
		PullRequest: &event.PullRequestEvent{
			Actor: event.User{Nickname: "bobreviewer", AccountID: "acct-bob"},
			Comment: &event.Comment{
				Content: event.CommentContent{Raw: shortText},
			},
		},
	}
	text, err := format.Reply(ev, defaultResolver(), format.Options{
		CommentContent:       format.CommentDisplaySummary,
		CommentSummaryLength: 200,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, shortText)
	assertNotContains(t, text, "…")
}

func TestReply_CommentCreated_NoContent(t *testing.T) {
	ev := &event.Event{
		Key: event.KeyPRCommentCreated,
		PullRequest: &event.PullRequestEvent{
			Actor: event.User{Nickname: "bobreviewer", AccountID: "acct-bob"},
			Comment: &event.Comment{
				Content: event.CommentContent{Raw: "this should not appear"},
			},
		},
	}
	text, err := format.Reply(ev, defaultResolver(), format.Options{CommentContent: format.CommentDisplayNone})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "💬")
	assertNotContains(t, text, "this should not appear")
}

func TestReply_CommentCreated_MarkdownBoldConverted(t *testing.T) {
	ev := &event.Event{
		Key: event.KeyPRCommentCreated,
		PullRequest: &event.PullRequestEvent{
			Actor: event.User{Nickname: "bobreviewer", AccountID: "acct-bob"},
			Comment: &event.Comment{
				Content: event.CommentContent{Raw: "**bold** and ~~strike~~"},
			},
		},
	}
	text, err := format.Reply(ev, defaultResolver(), format.Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "*bold*")
	assertContains(t, text, "~strike~")
	assertNotContains(t, text, "**bold**")
	assertNotContains(t, text, "~~strike~~")
}

func TestReply_CommentCreated_NoBlockquoteWrapper(t *testing.T) {
	ev := &event.Event{
		Key: event.KeyPRCommentCreated,
		PullRequest: &event.PullRequestEvent{
			Actor: event.User{Nickname: "bobreviewer", AccountID: "acct-bob"},
			Comment: &event.Comment{
				Content: event.CommentContent{Raw: "a comment"},
			},
		},
	}
	text, err := format.Reply(ev, defaultResolver(), format.Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertNotContains(t, text, "\n> ")
}

func TestReply_CommentCreated_EmptyBodyOmitted(t *testing.T) {
	ev := &event.Event{
		Key: event.KeyPRCommentCreated,
		PullRequest: &event.PullRequestEvent{
			Actor: event.User{Nickname: "bobreviewer", AccountID: "acct-bob"},
			Comment: &event.Comment{
				Content: event.CommentContent{Raw: ""},
			},
		},
	}
	text, err := format.Reply(ev, defaultResolver(), format.Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "💬")
	assertNotContains(t, text, "\n\n")
}

func TestReply_CommentCreated_SummaryUsesDisplayChars(t *testing.T) {
	// "[click here](https://example.com)" converts to "<https://example.com|click here>"
	// display len 10; limit 10 → no truncation
	ev := &event.Event{
		Key: event.KeyPRCommentCreated,
		PullRequest: &event.PullRequestEvent{
			Actor: event.User{Nickname: "bobreviewer", AccountID: "acct-bob"},
			Comment: &event.Comment{
				Content: event.CommentContent{Raw: "[click here](https://example.com)"},
			},
		},
	}
	text, err := format.Reply(ev, defaultResolver(), format.Options{
		CommentContent:       format.CommentDisplaySummary,
		CommentSummaryLength: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "<https://example.com|click here>")
	assertNotContains(t, text, "…")
}

func TestReply_UnmappedActor(t *testing.T) {
	ev := &event.Event{
		Key: event.KeyPRApproved,
		PullRequest: &event.PullRequestEvent{
			Actor: event.User{Nickname: "unknownuser", AccountID: "acct-unknown"},
		},
	}
	resolve := mapResolver(map[string]string{}) // empty
	text, err := format.Reply(ev, resolve, format.Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, text, "@unknownuser")
	assertNotContains(t, text, "<@")
}
