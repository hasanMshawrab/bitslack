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
	stateError      = "ERROR"
	stateStopped    = "STOPPED"

	defaultCommentSummaryLength = 200
	secondsPerMinute            = 60

	emojiCheck = "✅"
	emojiCross = "❌"
	emojiRed   = "🔴"
	emojiStop  = "⏹"
	emojiSpin  = "🔄"
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

	// PipelineLinkedToPR indicates the pipeline message is posted as a thread reply
	// under a PR. When true, the repo name and branch are omitted from the header
	// because they are already visible in the opening message above.
	PipelineLinkedToPR bool
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
		return formatPipelineRun(ev.Pipeline, resolve, opts.PipelineLinkedToPR), nil
	default:
		return "", fmt.Errorf("format: unknown event key %q", ev.Key)
	}
}

func formatApproved(ev *event.PullRequestEvent, resolve UserResolver) string {
	return fmt.Sprintf("✅ %s approved this pull request", mention(ev.Actor.AccountID, displayNameOf(ev.Actor), resolve))
}

func formatUnapproved(ev *event.PullRequestEvent, resolve UserResolver) string {
	return fmt.Sprintf("↩️ %s removed their approval", mention(ev.Actor.AccountID, displayNameOf(ev.Actor), resolve))
}

func formatFulfilled(ev *event.PullRequestEvent, resolve UserResolver) string {
	return fmt.Sprintf("🎉 %s merged this pull request", mention(ev.Actor.AccountID, displayNameOf(ev.Actor), resolve))
}

func formatRejected(ev *event.PullRequestEvent, resolve UserResolver) string {
	msg := fmt.Sprintf("🚫 %s declined this pull request", mention(ev.Actor.AccountID, displayNameOf(ev.Actor), resolve))
	if ev.PullRequest.Reason != "" {
		msg += fmt.Sprintf("\n> %s", ev.PullRequest.Reason)
	}
	return msg
}

func formatCommentCreated(ev *event.PullRequestEvent, resolve UserResolver, opts Options) string {
	actor := mention(ev.Actor.AccountID, displayNameOf(ev.Actor), resolve)
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

func formatPipelineRun(ev *event.PipelineRunEvent, resolve UserResolver, linked bool) string {
	run := ev.PipelineRun

	overallEmoji := pipelineResultEmoji(run.Result)
	overallText := pipelineResultText(run.Result)
	resultPart := overallEmoji
	if overallText != "" {
		resultPart = overallEmoji + " " + overallText
	}

	var header string
	if linked {
		// Thread reply under a PR: repo and branch are visible in the opening message above.
		header = fmt.Sprintf("⚙️ *Pipeline <%s|#%d>* • %s — %s",
			run.URL, run.RunNumber, pipelineTriggerLabel(run.Trigger), resultPart)
	} else {
		// Standalone message: include repo name (linked if URL available) and branch.
		repoLabel := run.Repository.Name
		if run.Repository.HTMLURL != "" {
			repoLabel = fmt.Sprintf("<%s|%s>", run.Repository.HTMLURL, run.Repository.Name)
		}
		header = fmt.Sprintf("⚙️ *[%s] Pipeline <%s|#%d>* • %s • %s — %s",
			repoLabel, run.URL, run.RunNumber, run.RefName,
			pipelineTriggerLabel(run.Trigger), resultPart)
	}
	if d := formatDuration(run.DurationSecs); d != "" {
		header += " • " + d
	}

	var sb strings.Builder
	sb.WriteString(header)
	for _, step := range ev.Steps {
		emoji := stepResultEmoji(step.Result)
		stepDuration := ""
		if d := formatDuration(step.DurationSecs); d != "" {
			stepDuration = " • " + d
		}
		if stepNeedsLink(step.Result) && step.URL != "" {
			sb.WriteString(fmt.Sprintf("\n    %s <%s|%s>%s", emoji, step.URL, step.Name, stepDuration))
		} else {
			sb.WriteString(fmt.Sprintf("\n    %s %s%s", emoji, step.Name, stepDuration))
		}
	}

	// Triggered by: append creator attribution if available.
	if ev.Creator != nil {
		sb.WriteString("\nTriggered by " + mention(ev.Creator.AccountID, displayNameOf(*ev.Creator), resolve))
	}

	return sb.String()
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
		return emojiCheck
	case stateFailed:
		return emojiCross
	case stateError:
		return emojiRed
	case stateStopped:
		return emojiStop
	default:
		return emojiSpin
	}
}

func pipelineResultText(result string) string {
	switch result {
	case "COMPLETE", stateSuccessful:
		return "Passed"
	case stateFailed:
		return "Failed"
	case stateError:
		return "Error"
	case stateStopped:
		return "Stopped"
	default:
		return ""
	}
}

func stepResultEmoji(result string) string {
	switch result {
	case stateSuccessful:
		return "✓"
	case stateFailed, stateError:
		return "✗"
	case stateStopped, "NOT_RUN":
		return "–"
	default:
		return "–"
	}
}

// stepNeedsLink returns true for results that warrant linking to the step log.
func stepNeedsLink(result string) bool {
	return result == stateFailed || result == stateError
}

func formatDuration(secs int) string {
	if secs <= 0 {
		return ""
	}
	if secs < secondsPerMinute {
		return fmt.Sprintf("%ds", secs)
	}
	return fmt.Sprintf("%dm %ds", secs/secondsPerMinute, secs%secondsPerMinute)
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
