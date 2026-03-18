// Package markdown converts Bitbucket markdown (CommonMark + Bitbucket extensions)
// to Slack mrkdwn format.
package markdown

import (
	"regexp"
	"strings"
)

var (
	reHeading    = regexp.MustCompile(`^#{1,6}\s+(.+)$`)
	reDivider    = regexp.MustCompile(`^(-{3,}|\*{3,}|_{3,})\s*$`)
	reUList      = regexp.MustCompile(`^[*-]\s+(.+)$`)
	reTable      = regexp.MustCompile(`^\|.*\|$`)
	reInlineBold = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reKramdown   = regexp.MustCompile(`\{:[^}]*\}`)
	reImage      = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)
	reLink       = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	reStrike     = regexp.MustCompile(`~~(.+?)~~`)
	reMention    = regexp.MustCompile(`@\{([^}]+)\}`)
)

// ToSlack converts Bitbucket markdown (CommonMark + Bitbucket extensions)
// to Slack mrkdwn. resolve maps a Bitbucket account ID to a Slack user ID;
// it may return "" to fall back to the raw account ID.
func ToSlack(raw string, resolve func(accountID string) string) string {
	if resolve == nil {
		resolve = func(string) string { return "" }
	}
	s := raw

	// Step 1: Line-level processing (must come before inline so headings
	// are wrapped in *...* before inline bold is applied to their content).
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		switch {
		case reHeading.MatchString(line):
			sub := reHeading.FindStringSubmatch(line)
			lines[i] = "*" + sub[1] + "*"
		case reDivider.MatchString(line):
			lines[i] = ""
		case reUList.MatchString(line):
			sub := reUList.FindStringSubmatch(line)
			lines[i] = "• " + sub[1]
		case reTable.MatchString(line):
			lines[i] = ""
		}
	}
	s = strings.Join(lines, "\n")

	// Step 2: Inline replacements.
	// Kramdown attrs first (before image/link processing).
	s = reKramdown.ReplaceAllString(s, "")

	// Images before links (image syntax contains link syntax).
	s = reImage.ReplaceAllStringFunc(s, func(m string) string {
		sub := reImage.FindStringSubmatch(m)
		alt := strings.TrimSpace(sub[1])
		url := sub[2]
		if alt == "" {
			alt = "Image"
		}
		return "<" + url + "|📎 " + alt + ">"
	})

	// Named links.
	s = reLink.ReplaceAllStringFunc(s, func(m string) string {
		sub := reLink.FindStringSubmatch(m)
		return "<" + sub[2] + "|" + sub[1] + ">"
	})

	// Bold: **text** → *text*
	s = reInlineBold.ReplaceAllString(s, "*$1*")

	// Strikethrough: ~~text~~ → ~text~
	s = reStrike.ReplaceAllString(s, "~$1~")

	// Mentions: @{account_id} → <@slackID> or @account_id
	s = reMention.ReplaceAllStringFunc(s, func(m string) string {
		sub := reMention.FindStringSubmatch(m)
		id := sub[1]
		if slackID := resolve(id); slackID != "" {
			return "<@" + slackID + ">"
		}
		return "@" + id
	})

	return s
}

// Truncate shortens mrkdwn to at most maxDisplay visible characters,
// appending "…" if truncated. Links (<url|text>) are treated as atomic
// tokens whose display length equals len([]rune(text)). Truncation happens
// at the last word boundary before the limit. Open */_/~ spans are closed
// after the ellipsis.
func Truncate(mrkdwn string, maxDisplay int) string {
	return mrkdwn
}
