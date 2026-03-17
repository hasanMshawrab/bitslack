package bitslack

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hasanMshawrab/bitslack/internal/event"
	"github.com/hasanMshawrab/bitslack/internal/format"
)

// Handler processes a Bitbucket webhook event.
// eventKey is the X-Event-Key header value.
// payload is the raw JSON body.
func (c *Client) Handler(ctx context.Context, eventKey string, payload []byte) error {
	ev, err := event.Parse(eventKey, payload)
	if err != nil {
		if strings.HasPrefix(eventKey, "pullrequest:") || strings.HasPrefix(eventKey, "repo:commit_status_") {
			return fmt.Errorf("bitslack: parse %s: %w", eventKey, err)
		}
		// Unknown event key — log and drop
		c.logger.Warn(fmt.Sprintf("bitslack: unknown event key %q", eventKey))
		return nil
	}

	if ev.PullRequest != nil {
		return c.handlePullRequestEvent(ctx, ev)
	}
	if ev.CommitStatus != nil {
		return c.handleCommitStatusEvent(ctx, ev)
	}
	return nil
}

// handlePullRequestEvent processes any pullrequest:* webhook event.
func (c *Client) handlePullRequestEvent(ctx context.Context, ev *event.Event) error {
	var err error

	pre := ev.PullRequest
	repoFullName := pre.Repository.FullName
	workspace := pre.Repository.Workspace.Slug
	repoSlug := pre.Repository.Name
	prID := pre.PullRequest.ID

	// Look up the Slack channel for this repository.
	channel, ok := c.configStore.GetChannel(repoFullName)
	if !ok {
		c.logger.Warn(fmt.Sprintf("bitslack: no channel mapping for repo %q", repoFullName))
		return nil
	}

	prKey := buildPRKey(repoFullName, prID)

	// Look up existing thread.
	ts, found, err := c.threadStore.Get(ctx, prKey)
	if err != nil {
		c.logger.Error(fmt.Sprintf("bitslack: thread store get %q: %v", prKey, err))
		return nil
	}

	resolve := userResolver(c.configStore)
	wasBackfilled := false

	// Backfill: no existing thread — fetch PR details, post opening message.
	if !found {
		var pr *event.PullRequest
		pr, err = c.bbClient.GetPullRequest(ctx, workspace, repoSlug, prID)
		if err != nil {
			c.logger.Error(fmt.Sprintf("bitslack: fetch PR %s#%d: %v", repoFullName, prID, err))
			return nil
		}

		text, blocks := format.OpeningMessage(pr, resolve)
		ts, err = c.slackClient.PostMessage(ctx, channel, "", text, blocks)
		if err != nil {
			c.logger.Error(fmt.Sprintf("bitslack: post opening message for %s: %v", prKey, err))
			return nil
		}

		if storeErr := c.threadStore.Store(ctx, prKey, ts); storeErr != nil {
			c.logger.Warn(fmt.Sprintf("bitslack: store thread ts for %s: %v", prKey, storeErr))
		}
		wasBackfilled = true
	}

	// Updated events: edit the opening message in place (no reply).
	if ev.Key == event.KeyPRUpdated {
		if !wasBackfilled {
			text, blocks := format.OpeningMessage(&pre.PullRequest, resolve)
			err = c.slackClient.UpdateMessage(ctx, channel, ts, text, blocks)
			if err != nil {
				c.logger.Error(fmt.Sprintf("bitslack: update opening message for %s: %v", prKey, err))
			}
		}
		return nil
	}

	// Created event with backfill: the opening message IS the notification.
	if ev.Key == event.KeyPRCreated && wasBackfilled {
		return nil
	}

	// All other events: post a thread reply.
	replyText, err := format.Reply(ev, resolve)
	if err != nil {
		c.logger.Error(fmt.Sprintf("bitslack: format reply for %s: %v", prKey, err))
		return nil
	}

	_, err = c.slackClient.PostMessage(ctx, channel, ts, replyText, nil)
	if err != nil {
		c.logger.Error(fmt.Sprintf("bitslack: post reply for %s: %v", prKey, err))
		return nil
	}

	return nil
}

// handleCommitStatusEvent processes repo:commit_status_* webhook events.
func (c *Client) handleCommitStatusEvent(ctx context.Context, ev *event.Event) error {
	var err error

	cse := ev.CommitStatus
	commitHash := cse.CommitStatus.CommitHash
	workspace := cse.Repository.Workspace.Slug
	repoSlug := cse.Repository.Name
	repoFullName := cse.Repository.FullName

	// Resolve commit hash to PRs.
	prs, err := c.bbClient.GetPullRequestsForCommit(ctx, workspace, repoSlug, commitHash)
	if err != nil {
		c.logger.Error(fmt.Sprintf("bitslack: resolve commit %s to PRs: %v", commitHash, err))
		return nil
	}
	if len(prs) == 0 {
		c.logger.Warn(fmt.Sprintf("bitslack: no PRs found for commit %s", commitHash))
		return nil
	}

	pr := prs[0]
	prKey := buildPRKey(repoFullName, pr.ID)

	channel, ok := c.configStore.GetChannel(repoFullName)
	if !ok {
		c.logger.Warn(fmt.Sprintf("bitslack: no channel mapping for repo %q", repoFullName))
		return nil
	}

	// Look up existing thread.
	ts, found, err := c.threadStore.Get(ctx, prKey)
	if err != nil {
		c.logger.Error(fmt.Sprintf("bitslack: thread store get %q: %v", prKey, err))
		return nil
	}

	resolve := userResolver(c.configStore)

	// Backfill if no thread exists.
	if !found {
		text, blocks := format.OpeningMessage(pr, resolve)
		ts, err = c.slackClient.PostMessage(ctx, channel, "", text, blocks)
		if err != nil {
			c.logger.Error(fmt.Sprintf("bitslack: post opening message for %s: %v", prKey, err))
			return nil
		}

		if storeErr := c.threadStore.Store(ctx, prKey, ts); storeErr != nil {
			c.logger.Warn(fmt.Sprintf("bitslack: store thread ts for %s: %v", prKey, storeErr))
		}
	}

	// Post the build status as a thread reply.
	replyText, err := format.Reply(ev, resolve)
	if err != nil {
		c.logger.Error(fmt.Sprintf("bitslack: format reply for %s: %v", prKey, err))
		return nil
	}

	_, err = c.slackClient.PostMessage(ctx, channel, ts, replyText, nil)
	if err != nil {
		c.logger.Error(fmt.Sprintf("bitslack: post reply for %s: %v", prKey, err))
		return nil
	}

	return nil
}

// buildPRKey constructs the thread store key for a PR.
func buildPRKey(repoFullName string, prID int) string {
	return repoFullName + ":" + strconv.Itoa(prID)
}

// userResolver returns a format.UserResolver that looks up Slack user IDs
// via the ConfigStore.
func userResolver(cs ConfigStore) format.UserResolver {
	return func(accountID string) string {
		id, _ := cs.GetSlackUserID(accountID)
		return id
	}
}
