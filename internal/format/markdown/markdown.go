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
	reTableSep   = regexp.MustCompile(`^\|[\s\-:|]+\|$`)
	reItalicBold = regexp.MustCompile(`_\*\*(.+?)\*\*_`)
	reBoldItalic = regexp.MustCompile(`\*\*_(.+?)_\*\*`)
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
	// Tables are collected as consecutive blocks and formatted together.
	lines := strings.Split(s, "\n")
	result := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		line := lines[i]
		switch {
		case reHeading.MatchString(line):
			sub := reHeading.FindStringSubmatch(line)
			result = append(result, "*"+sub[1]+"*")
			i++
		case reDivider.MatchString(line):
			result = append(result, "")
			i++
		case reUList.MatchString(line):
			sub := reUList.FindStringSubmatch(line)
			result = append(result, "• "+sub[1])
			i++
		case reTable.MatchString(line):
			// Collect all consecutive table lines, then format as one block.
			j := i
			for j < len(lines) && reTable.MatchString(lines[j]) {
				j++
			}
			result = append(result, formatTable(lines[i:j]))
			i = j
		default:
			result = append(result, line)
			i++
		}
	}
	s = strings.Join(result, "\n")

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

	// Bold+italic combinations must be handled before individual bold,
	// otherwise **...** is consumed first and the outer _ becomes orphaned.
	// Both forms → *_text_* (bold outer, italic inner — Slack renders both).
	s = reItalicBold.ReplaceAllString(s, "*_${1}_*")
	s = reBoldItalic.ReplaceAllString(s, "*_${1}_*")

	// Bold: **text** → *text*
	s = reInlineBold.ReplaceAllString(s, "*$1*")

	// Strikethrough: ~~text~~ → ~text~
	s = reStrike.ReplaceAllString(s, "~$1~")

	// Mentions: @{account_id} → <@slackID> or Bitbucket profile link
	s = reMention.ReplaceAllStringFunc(s, func(m string) string {
		sub := reMention.FindStringSubmatch(m)
		id := sub[1]
		if slackID := resolve(id); slackID != "" {
			return "<@" + slackID + ">"
		}
		return "<https://bitbucket.org/" + id + "|@" + id + ">"
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

// formatTable converts consecutive Markdown table lines into an aligned
// plain-text table wrapped in a Slack code block.
func formatTable(tableLines []string) string {
	var rows [][]string
	for _, line := range tableLines {
		if reTableSep.MatchString(line) {
			continue // skip separator rows (| --- | --- |)
		}
		rows = append(rows, parseTableRow(line))
	}
	if len(rows) == 0 {
		return ""
	}

	// Calculate max column widths across all rows.
	numCols := 0
	for _, row := range rows {
		if len(row) > numCols {
			numCols = len(row)
		}
	}
	widths := make([]int, numCols)
	for _, row := range rows {
		for j, cell := range row {
			if w := len([]rune(cell)); w > widths[j] {
				widths[j] = w
			}
		}
	}

	var sb strings.Builder
	sb.WriteString("```\n")
	for idx, row := range rows {
		writeTableRow(&sb, row, widths, numCols)
		if idx == 0 {
			// Separator line after the header row.
			sb.WriteString("| ")
			for j := range numCols {
				sb.WriteString(strings.Repeat("-", widths[j]))
				sb.WriteString(" | ")
			}
			sb.WriteString("\n")
		}
	}
	sb.WriteString("```")
	return sb.String()
}

// parseTableRow splits a Markdown table row on "|", trims whitespace,
// strips bold markers from each cell, and drops the leading/trailing empty
// tokens produced by the surrounding pipes.
func parseTableRow(line string) []string {
	parts := strings.Split(line, "|")
	cells := make([]string, 0, len(parts))
	for _, p := range parts {
		cell := strings.TrimSpace(p)
		cell = reInlineBold.ReplaceAllString(cell, "$1")
		cells = append(cells, cell)
	}
	// "| a | b |".split("|") → ["", " a ", " b ", ""] — drop surrounding empties.
	for len(cells) > 0 && cells[0] == "" {
		cells = cells[1:]
	}
	for len(cells) > 0 && cells[len(cells)-1] == "" {
		cells = cells[:len(cells)-1]
	}
	return cells
}

// writeTableRow writes one padded row to sb.
func writeTableRow(sb *strings.Builder, row []string, widths []int, numCols int) {
	sb.WriteString("| ")
	for j := range numCols {
		var cell string
		if j < len(row) {
			cell = row[j]
		}
		runes := []rune(cell)
		sb.WriteString(string(runes))
		sb.WriteString(strings.Repeat(" ", widths[j]-len(runes)))
		sb.WriteString(" | ")
	}
	sb.WriteString("\n")
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
