// Package format produces Slack message text and Block Kit blocks from
// parsed Bitbucket webhook events.
package format

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hasanMshawrab/bitslack/internal/event"
	"github.com/hasanMshawrab/bitslack/internal/slack"
)

// UserResolver maps a Bitbucket account ID to a Slack user ID.
// Returns empty string if no mapping exists.
type UserResolver func(accountID string) string

var clickUpURLRe = regexp.MustCompile(`https://app\.clickup\.com/t/[^\s)>\]]+`)

// extractClickUpURL returns the first ClickUp task URL found in s, or "".
func extractClickUpURL(s string) string {
	return clickUpURLRe.FindString(s)
}

// approvedReviewerIDs returns the set of account IDs that appear as
// REVIEWER participants with approved=true.
func approvedReviewerIDs(participants []event.Participant) map[string]bool {
	approved := make(map[string]bool)
	for _, p := range participants {
		if p.Role == "REVIEWER" && p.Approved {
			approved[p.AccountID] = true
		}
	}
	return approved
}

// alsoApprovedParticipants returns participants with role="PARTICIPANT" who have approved.
func alsoApprovedParticipants(participants []event.Participant) []event.Participant {
	var out []event.Participant
	for _, p := range participants {
		if p.Role == "PARTICIPANT" && p.Approved {
			out = append(out, p)
		}
	}
	return out
}

// OpeningMessage produces Block Kit blocks for the PR opening message.
// Returns a plain-text fallback string and structured blocks.
func OpeningMessage(pr *event.PullRequest, resolve UserResolver) (string, []slack.Block) {
	repoName := pr.Source.Repository.Name
	repoURL := pr.Source.Repository.HTMLURL
	if repoName == "" {
		repoName = pr.Destination.Repository.Name
		repoURL = pr.Destination.Repository.HTMLURL
	}

	// Link the repo name when a URL is available.
	var repoLabel string
	if repoURL != "" {
		repoLabel = fmt.Sprintf("<%s|%s>", repoURL, repoName)
	} else {
		repoLabel = repoName
	}

	// Header: 🔀 *[{repo}] Pull Request <{url}|#{id}>* • {source} → {dest}
	prLink := fmt.Sprintf("<%s|#%d>", pr.HTMLURL, pr.ID)
	header := fmt.Sprintf("🔀 *[%s] Pull Request %s* • %s → %s",
		repoLabel, prLink,
		pr.Source.Branch.Name, pr.Destination.Branch.Name)

	headerBlock := slack.Block{
		Type: "section",
		Text: &slack.TextObject{
			Type: "mrkdwn",
			Text: header,
		},
	}

	var fields []string
	fields = append(fields, fmt.Sprintf("*Title:* %s", pr.Title))
	fields = append(fields, fmt.Sprintf("*Status:* %s", prStateLabel(pr.State)))
	fields = append(
		fields,
		fmt.Sprintf("*Author:* %s", mention(pr.Author.AccountID, displayNameOf(pr.Author), resolve)),
	)

	// Reviewers with approval checkmarks
	if len(pr.Reviewers) > 0 {
		approved := approvedReviewerIDs(pr.Participants)
		reviewerParts := make([]string, len(pr.Reviewers))
		for i, r := range pr.Reviewers {
			m := mention(r.AccountID, displayNameOf(r), resolve)
			if approved[r.AccountID] {
				reviewerParts[i] = "✅ " + m
			} else {
				reviewerParts[i] = m
			}
		}
		fields = append(fields, fmt.Sprintf("*Reviewers:* %s", strings.Join(reviewerParts, " • ")))
	}

	// Also approved: non-reviewer participants who approved
	alsoApproved := alsoApprovedParticipants(pr.Participants)
	if len(alsoApproved) > 0 {
		parts := make([]string, len(alsoApproved))
		for i, p := range alsoApproved {
			parts[i] = mention(p.AccountID, p.DisplayName, resolve)
		}
		fields = append(fields, fmt.Sprintf("*Also approved:* %s", strings.Join(parts, " • ")))
	}

	// ClickUp ticket link
	if ticketURL := extractClickUpURL(pr.Description); ticketURL != "" {
		fields = append(fields, fmt.Sprintf("*Ticket:* <%s|View Ticket>", ticketURL))
	}

	fieldsBlock := slack.Block{
		Type: "section",
		Text: &slack.TextObject{
			Type: "mrkdwn",
			Text: strings.Join(fields, "\n"),
		},
	}

	blocks := []slack.Block{headerBlock, fieldsBlock}

	// Plain-text fallback
	fallback := fmt.Sprintf("%s | %s", pr.Title, repoName)

	return fallback, blocks
}

// prStateLabel maps Bitbucket PR states to display strings.
func prStateLabel(state string) string {
	switch state {
	case "OPEN":
		return "Open"
	case "MERGED":
		return "Merged"
	case "DECLINED":
		return "Closed"
	default:
		return state
	}
}

// mention returns "<@slackID>" if mapped, or a Bitbucket profile link as fallback.
// When accountID is empty the displayName is returned as plain text.
func mention(accountID, displayName string, resolve UserResolver) string {
	if id := resolve(accountID); id != "" {
		return fmt.Sprintf("<@%s>", id)
	}
	if accountID != "" {
		label := displayName
		if label == "" {
			label = accountID
		}
		return fmt.Sprintf("<https://bitbucket.org/%s|%s>", accountID, label)
	}
	return displayName
}

// displayNameOf returns the best available display name for a user.
// Prefers DisplayName; falls back to Nickname.
func displayNameOf(u event.User) string {
	if u.DisplayName != "" {
		return u.DisplayName
	}
	return u.Nickname
}
