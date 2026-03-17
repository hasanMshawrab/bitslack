package format

import (
	"fmt"
	"strings"

	"github.com/hasanMshawrab/bitslack/internal/event"
	"github.com/hasanMshawrab/bitslack/internal/slack"
)

// UserResolver maps a Bitbucket account ID to a Slack user ID.
// Returns empty string if no mapping exists.
type UserResolver func(accountID string) string

// OpeningMessage produces Block Kit blocks for the PR opening message.
// Returns a plain-text fallback string and structured blocks.
func OpeningMessage(pr *event.PullRequest, resolve UserResolver) (string, []slack.Block) {
	// Title block: *{title}*  |  {repo.Name}
	titleText := fmt.Sprintf("*%s*  |  %s", pr.Title, pr.Destination.Repository.Name)
	titleBlock := slack.Block{
		Type: "section",
		Text: &slack.TextObject{
			Type: "mrkdwn",
			Text: titleText,
		},
	}

	// People block: Author: {mention}  |  Reviewers: {mention1}, {mention2}
	peopleText := fmt.Sprintf("Author: %s", mention(pr.Author.AccountID, pr.Author.Nickname, resolve))
	if len(pr.Reviewers) > 0 {
		reviewerMentions := make([]string, len(pr.Reviewers))
		for i, r := range pr.Reviewers {
			reviewerMentions[i] = mention(r.AccountID, r.Nickname, resolve)
		}
		peopleText += fmt.Sprintf("  |  Reviewers: %s", strings.Join(reviewerMentions, ", "))
	}
	peopleBlock := slack.Block{
		Type: "section",
		Text: &slack.TextObject{
			Type: "mrkdwn",
			Text: peopleText,
		},
	}

	// Link block: PR URL
	linkBlock := slack.Block{
		Type: "section",
		Text: &slack.TextObject{
			Type: "mrkdwn",
			Text: pr.HTMLURL,
		},
	}

	blocks := []slack.Block{titleBlock, peopleBlock, linkBlock}

	// Plain-text fallback
	fallback := fmt.Sprintf("%s | %s", pr.Title, pr.Destination.Repository.Name)

	return fallback, blocks
}

// mention returns "<@slackID>" if mapped, or "@nickname" as fallback.
func mention(accountID, nickname string, resolve UserResolver) string {
	if id := resolve(accountID); id != "" {
		return fmt.Sprintf("<@%s>", id)
	}
	return "@" + nickname
}
