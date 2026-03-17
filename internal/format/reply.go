package format

import (
	"fmt"

	"github.com/hasanMshawrab/bitslack/internal/event"
)

// Reply produces a plain-text reply string for the given event.
// Note: KeyPRCreated and KeyPRUpdated are intentionally absent --
// created is handled by the opening message, updated by chat.update.
func Reply(ev *event.Event, resolve UserResolver) (string, error) {
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
		return formatCommentCreated(ev.PullRequest, resolve), nil
	case event.KeyCommitStatusCreated, event.KeyCommitStatusUpdated:
		return formatCommitStatus(ev.CommitStatus), nil
	case event.KeyPipelineSpanCreated:
		return formatPipelineRun(ev.Pipeline), nil
	default:
		return "", fmt.Errorf("format: unknown event key %q", ev.Key)
	}
}

func formatApproved(ev *event.PullRequestEvent, resolve UserResolver) string {
	return fmt.Sprintf("%s approved this pull request", mention(ev.Actor.AccountID, ev.Actor.Nickname, resolve))
}

func formatUnapproved(ev *event.PullRequestEvent, resolve UserResolver) string {
	return fmt.Sprintf("%s removed their approval", mention(ev.Actor.AccountID, ev.Actor.Nickname, resolve))
}

func formatFulfilled(ev *event.PullRequestEvent, resolve UserResolver) string {
	return fmt.Sprintf("%s merged this pull request", mention(ev.Actor.AccountID, ev.Actor.Nickname, resolve))
}

func formatRejected(ev *event.PullRequestEvent, resolve UserResolver) string {
	msg := fmt.Sprintf("%s declined this pull request", mention(ev.Actor.AccountID, ev.Actor.Nickname, resolve))
	if ev.PullRequest.Reason != "" {
		msg += fmt.Sprintf("\n> %s", ev.PullRequest.Reason)
	}
	return msg
}

func formatCommentCreated(ev *event.PullRequestEvent, resolve UserResolver) string {
	actor := mention(ev.Actor.AccountID, ev.Actor.Nickname, resolve)
	comment := ev.Comment
	if comment == nil {
		return fmt.Sprintf("%s commented", actor)
	}

	var msg string
	if comment.Inline != nil {
		msg = fmt.Sprintf("%s commented on `%s:%d`\n> %s\n%s",
			actor, comment.Inline.Path, comment.Inline.To, comment.Content.Raw, comment.HTMLURL)
	} else {
		msg = fmt.Sprintf("%s commented\n> %s\n%s",
			actor, comment.Content.Raw, comment.HTMLURL)
	}
	return msg
}

func formatCommitStatus(ev *event.CommitStatusEvent) string {
	cs := ev.CommitStatus
	stateText := stateToText(cs.State)
	return fmt.Sprintf("%s %s (%s)\n%s", cs.Name, stateText, cs.Key, cs.URL)
}

func formatPipelineRun(ev *event.PipelineRunEvent) string {
	run := ev.PipelineRun
	emoji := pipelineResultEmoji(run.Result)
	return fmt.Sprintf("%s Pipeline <%s|#%d> • %s • %s",
		emoji, run.URL, run.RunNumber, run.Trigger, run.RefName)
}

func pipelineResultEmoji(result string) string {
	switch result {
	case "SUCCESSFUL":
		return "✅"
	case "FAILED":
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
	case "SUCCESSFUL":
		return "passed"
	case "FAILED":
		return "failed"
	default:
		return state
	}
}
