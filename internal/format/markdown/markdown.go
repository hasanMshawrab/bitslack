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
	reLinkToken  = regexp.MustCompile(`<[^|>]+\|([^>]*)>`)
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

type segment struct {
	raw        string
	displayLen int
	isLink     bool
}

func tokenize(mrkdwn string) []segment {
	var segs []segment
	matches := reLinkToken.FindAllStringIndex(mrkdwn, -1)
	pos := 0
	for _, loc := range matches {
		if loc[0] > pos {
			text := mrkdwn[pos:loc[0]]
			segs = append(segs, segment{raw: text, displayLen: len([]rune(text))})
		}
		raw := mrkdwn[loc[0]:loc[1]]
		sub := reLinkToken.FindStringSubmatch(raw)
		segs = append(segs, segment{
			raw:        raw,
			displayLen: len([]rune(sub[1])),
			isLink:     true,
		})
		pos = loc[1]
	}
	if pos < len(mrkdwn) {
		text := mrkdwn[pos:]
		segs = append(segs, segment{raw: text, displayLen: len([]rune(text))})
	}
	return segs
}

// processSeg applies one segment to result, returns updated remaining and whether to stop.
func processSeg(seg segment, result *strings.Builder, remaining int) (int, bool) {
	if seg.isLink {
		if seg.displayLen <= remaining {
			result.WriteString(seg.raw)
			return remaining - seg.displayLen, false
		}
		return 0, true
	}
	if seg.displayLen <= remaining {
		result.WriteString(seg.raw)
		return remaining - seg.displayLen, false
	}
	// Truncate at last word boundary.
	runes := []rune(seg.raw)
	cut := min(remaining, len(runes))
	for cut > 0 && runes[cut-1] != ' ' {
		cut--
	}
	for cut > 0 && runes[cut-1] == ' ' {
		cut--
	}
	result.WriteString(string(runes[:cut]))
	return 0, true
}

// Truncate shortens mrkdwn to at most maxDisplay visible characters,
// appending "…" if truncated. Links (<url|text>) are treated as atomic
// tokens whose display length equals len([]rune(text)). Truncation happens
// at the last word boundary before the limit. Open */_/~ spans are closed
// after the ellipsis.
func Truncate(mrkdwn string, maxDisplay int) string {
	if maxDisplay <= 0 {
		return mrkdwn
	}
	segs := tokenize(mrkdwn)

	total := 0
	for _, s := range segs {
		total += s.displayLen
	}
	if total <= maxDisplay {
		return mrkdwn
	}

	var result strings.Builder
	remaining := maxDisplay

	for _, seg := range segs {
		if remaining <= 0 {
			break
		}
		var stop bool
		remaining, stop = processSeg(seg, &result, remaining)
		if stop {
			break
		}
	}

	truncated := strings.TrimRight(result.String(), " ")
	return truncated + "…" + closedSpans(truncated)
}

func closedSpans(s string) string {
	stripped := reLinkToken.ReplaceAllString(s, "")
	var b strings.Builder
	for _, marker := range []string{"~", "_", "*"} {
		if strings.Count(stripped, marker)%2 != 0 {
			b.WriteString(marker)
		}
	}
	return b.String()
}
