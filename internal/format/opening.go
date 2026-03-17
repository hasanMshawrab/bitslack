// Package format produces Slack message text and Block Kit blocks from
// parsed Bitbucket webhook events.
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
	// Metadata block: Repository, PR Title, PR No (as clickable link)
	prLink := fmt.Sprintf("<%s|#%d>", pr.HTMLURL, pr.ID)
	metaText := fmt.Sprintf("*Repository:* %s\n*PR Title:* %s\n*PR No:* %s",
		pr.Destination.Repository.Name, pr.Title, prLink)
	metaBlock := slack.Block{
		Type: "section",
		Text: &slack.TextObject{
			Type: "mrkdwn",
			Text: metaText,
		},
	}

	// People block: Author and optional Reviewers, each on its own line
	peopleText := fmt.Sprintf("*Author:* %s", mention(pr.Author.AccountID, pr.Author.Nickname, resolve))
	if len(pr.Reviewers) > 0 {
		reviewerMentions := make([]string, len(pr.Reviewers))
		for i, r := range pr.Reviewers {
			reviewerMentions[i] = mention(r.AccountID, r.Nickname, resolve)
		}
		peopleText += fmt.Sprintf("\n*Reviewers:* %s", strings.Join(reviewerMentions, ", "))
	}
	peopleBlock := slack.Block{
		Type: "section",
		Text: &slack.TextObject{
			Type: "mrkdwn",
			Text: peopleText,
		},
	}

	blocks := []slack.Block{metaBlock, peopleBlock}

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
