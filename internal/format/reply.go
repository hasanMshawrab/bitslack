package format

import (
	"fmt"
	"strings"

	"github.com/hasanMshawrab/bitslack/internal/event"
	"github.com/hasanMshawrab/bitslack/internal/format/markdown"
)

const (
	stateSuccessful = "SUCCESSFUL"
	stateFailed     = "FAILED"

	defaultCommentSummaryLength = 200
)

// CommentDisplay controls how much of a comment's body is shown in Slack.
type CommentDisplay int

const (
	// CommentDisplayFull shows the entire comment content (default).
	CommentDisplayFull CommentDisplay = iota
	// CommentDisplaySummary truncates to Options.CommentSummaryLength display characters
	// and appends "…".
	CommentDisplaySummary
	// CommentDisplayNone omits the comment body entirely.
	CommentDisplayNone
)

// Options controls how Slack reply messages are rendered.
// All fields are optional — zero values apply the defaults.
type Options struct {
	// DistinguishCommentReplies controls whether a reply to a comment is labelled
	// differently from a top-level comment.
	// false (default) → both show "💬 @user commented"
	// true → top-level: "💬 @user commented", reply: "💬 @user replied to a comment"
	DistinguishCommentReplies bool

	// CommentContent controls how much of the comment body is included.
	// Default: CommentDisplayFull
	CommentContent CommentDisplay

	// CommentSummaryLength is the maximum number of display characters shown
	// when CommentContent == CommentDisplaySummary. Default: 200.
	CommentSummaryLength int

	// ShowCommentLink controls whether a "View comment" link is appended.
	// false (default) → link omitted
	// true → appended as "<url|View comment>"
	ShowCommentLink bool
}

// Reply produces a plain-text reply string for the given event.
// Note: KeyPRCreated and KeyPRUpdated are intentionally absent --
// created is handled by the opening message, updated by chat.update.
func Reply(ev *event.Event, resolve UserResolver, opts Options) (string, error) {
	switch ev.Key {
	case event.KeyPRApproved:
		return formatApproved(ev.PullRequest, resolve), nil
	case event.KeyPRUnapproved:
		return formatUnapproved(ev.PullRequest, resolve), nil
	case event.KeyPRFulfilled:
		return formatFulfilled(ev.PullRequest, resolve), nil
	case event.KeyPRRejected:
		return formatRejected(ev.PullRequest, resolve), nil
	case event.KeyPRCommentCreated:
		return formatCommentCreated(ev.PullRequest, resolve, opts), nil
	case event.KeyCommitStatusCreated, event.KeyCommitStatusUpdated:
		return formatCommitStatus(ev.CommitStatus), nil
	case event.KeyPipelineSpanCreated:
		return formatPipelineRun(ev.Pipeline), nil
	default:
		return "", fmt.Errorf("format: unknown event key %q", ev.Key)
	}
}

func formatApproved(ev *event.PullRequestEvent, resolve UserResolver) string {
	return fmt.Sprintf("✅ %s approved this pull request", mention(ev.Actor.AccountID, ev.Actor.Nickname, resolve))
}

func formatUnapproved(ev *event.PullRequestEvent, resolve UserResolver) string {
	return fmt.Sprintf("↩️ %s removed their approval", mention(ev.Actor.AccountID, ev.Actor.Nickname, resolve))
}

func formatFulfilled(ev *event.PullRequestEvent, resolve UserResolver) string {
	return fmt.Sprintf("🎉 %s merged this pull request", mention(ev.Actor.AccountID, ev.Actor.Nickname, resolve))
}

func formatRejected(ev *event.PullRequestEvent, resolve UserResolver) string {
	msg := fmt.Sprintf("🚫 %s declined this pull request", mention(ev.Actor.AccountID, ev.Actor.Nickname, resolve))
	if ev.PullRequest.Reason != "" {
		msg += fmt.Sprintf("\n> %s", ev.PullRequest.Reason)
	}
	return msg
}

func formatCommentCreated(ev *event.PullRequestEvent, resolve UserResolver, opts Options) string {
	actor := mention(ev.Actor.AccountID, ev.Actor.Nickname, resolve)
	comment := ev.Comment
	if comment == nil {
		return fmt.Sprintf("💬 %s commented", actor)
	}

	// Determine action verb.
	action := "commented"
	if opts.DistinguishCommentReplies && comment.ParentID != 0 {
		action = "replied to a comment"
	}

	// Build inline location suffix.
	var location string
	if comment.Inline != nil {
		location = fmt.Sprintf(" on `%s:%d`", comment.Inline.Path, comment.Inline.To)
	}

	header := fmt.Sprintf("💬 %s %s%s", actor, action, location)

	// Convert comment body from Bitbucket markdown to Slack mrkdwn.
	converted := markdown.ToSlack(comment.Content.Raw, resolve)

	// Build content block.
	var content string
	switch opts.CommentContent {
	case CommentDisplayFull:
		if converted != "" {
			content = "\n" + converted
		}
	case CommentDisplaySummary:
		summaryLen := opts.CommentSummaryLength
		if summaryLen <= 0 {
			summaryLen = defaultCommentSummaryLength
		}
		truncated := markdown.Truncate(converted, summaryLen)
		if truncated != "" {
			content = "\n" + truncated
		}
	case CommentDisplayNone:
		// omit body
	}

	// Build link.
	var link string
	if opts.ShowCommentLink && comment.HTMLURL != "" {
		link = fmt.Sprintf("\n<%s|View comment>", comment.HTMLURL)
	}

	return header + content + link
}

func formatCommitStatus(ev *event.CommitStatusEvent) string {
	cs := ev.CommitStatus
	emoji := commitStatusEmoji(cs.State)
	stateText := stateToText(cs.State)
	return fmt.Sprintf("%s %s %s (%s)\n%s", emoji, cs.Name, stateText, cs.Key, cs.URL)
}

func commitStatusEmoji(state string) string {
	switch state {
	case "INPROGRESS":
		return "🔄"
	case stateSuccessful:
		return "✅"
	case stateFailed:
		return "❌"
	default:
		return "❓"
	}
}

func formatPipelineRun(ev *event.PipelineRunEvent) string {
	run := ev.PipelineRun
	emoji := pipelineResultEmoji(run.Result)
	return fmt.Sprintf("%s *[%s] Pipeline <%s|#%d>* • %s • %s",
		emoji, run.Repository.Name, run.URL, run.RunNumber, run.RefName, pipelineTriggerLabel(run.Trigger))
}

func pipelineTriggerLabel(trigger string) string {
	switch trigger {
	case "PUSH":
		return "automatic trigger"
	case "MANUAL":
		return "manual trigger"
	case "SCHEDULE":
		return "scheduled trigger"
	default:
		return strings.ToLower(trigger) + " trigger"
	}
}

func pipelineResultEmoji(result string) string {
	switch result {
	case "COMPLETE", stateSuccessful: // OTel uses COMPLETE; REST API uses SUCCESSFUL
		return "✅"
	case stateFailed:
		return "❌"
	case "ERROR":
		return "🔴"
	case "STOPPED":
		return "⏹"
	default:
		return "🔄"
	}
}

func stateToText(state string) string {
	switch state {
	case "INPROGRESS":
		return "is running"
	case stateSuccessful:
		return "passed"
	case stateFailed:
		return "failed"
	default:
		return state
	}
}
