package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// PostMessage posts a message to a Slack channel. If threadTS is non-empty,
// the message is posted as a reply in that thread. Returns the message
// timestamp on success.
func (c *Client) PostMessage(ctx context.Context, channel, threadTS, text string, blocks []Block) (string, error) {
	req := postMessageRequest{
		Channel:  channel,
		Text:     text,
		ThreadTS: threadTS,
		Blocks:   blocks,
	}
	var resp slackResponse
	if err := c.post(ctx, "/chat.postMessage", req, &resp); err != nil {
		return "", err
	}
	if !resp.OK {
		return "", &Error{Code: resp.Error}
	}
	return resp.TS, nil
}

// UpdateMessage updates an existing Slack message identified by channel and ts.
func (c *Client) UpdateMessage(ctx context.Context, channel, ts, text string, blocks []Block) error {
	req := updateMessageRequest{
		Channel: channel,
		TS:      ts,
		Text:    text,
		Blocks:  blocks,
	}
	var resp slackResponse
	if err := c.post(ctx, "/chat.update", req, &resp); err != nil {
		return err
	}
	if !resp.OK {
		return &Error{Code: resp.Error}
	}
	return nil
}

// post sends a JSON POST request to the given Slack API path and decodes the
// response into dst.
func (c *Client) post(ctx context.Context, path string, body any, dst any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("slack: marshal request: %w", err)
	}

	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("slack: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("slack: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("slack: unexpected status %d", resp.StatusCode)
	}

	decErr := json.NewDecoder(resp.Body).Decode(dst)
	if decErr != nil {
		return fmt.Errorf("slack: decode response: %w", decErr)
	}
	return nil
}
