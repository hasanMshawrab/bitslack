package slack

import "fmt"

// Block is a Slack Block Kit block.
type Block struct {
	Type string      `json:"type"`
	Text *TextObject `json:"text,omitempty"`
}

// TextObject is a Slack text element.
type TextObject struct {
	Type string `json:"type"` // "mrkdwn" or "plain_text"
	Text string `json:"text"`
}

// Error is returned when the Slack API responds with ok=false.
type Error struct {
	Code string
}

func (e *Error) Error() string {
	return fmt.Sprintf("slack: %s", e.Code)
}

// Request/response wire types (unexported).
type postMessageRequest struct {
	Channel  string  `json:"channel"`
	Text     string  `json:"text"`
	ThreadTS string  `json:"thread_ts,omitempty"`
	Blocks   []Block `json:"blocks,omitempty"`
}

type updateMessageRequest struct {
	Channel string  `json:"channel"`
	TS      string  `json:"ts"`
	Text    string  `json:"text"`
	Blocks  []Block `json:"blocks,omitempty"`
}

type slackResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
	TS    string `json:"ts"`
}
