package bitslack

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/hasanMshawrab/bitslack/internal/bitbucket"
	"github.com/hasanMshawrab/bitslack/internal/event"
	"github.com/hasanMshawrab/bitslack/internal/format"
)

// eventFamily returns the EventFamily for a given event key, or "" if unknown.
func eventFamily(eventKey string) EventFamily {
	switch {
	case strings.HasPrefix(eventKey, "pullrequest:"):
		return EventFamilyPullRequest
	case strings.HasPrefix(eventKey, "repo:commit_status_"):
		return EventFamilyCommitStatus
	case strings.HasPrefix(eventKey, "pipeline:"):
		return EventFamilyPipeline
	default:
		return ""
	}
}

// Handler processes a Bitbucket webhook event.
// eventKey is the X-Event-Key header value.
// payload is the raw JSON body.
func (c *Client) Handler(ctx context.Context, eventKey string, payload []byte) error {
	if family := eventFamily(eventKey); family != "" {
		if _, ok := c.enabledFamilies[family]; !ok {
			c.logger.Warn(fmt.Sprintf("bitslack: event family %q is not enabled, dropping %q", family, eventKey))
			return nil
		}
	}

	ev, err := event.Parse(eventKey, payload)
	if err != nil {
		if strings.HasPrefix(eventKey, "pullrequest:") ||
			strings.HasPrefix(eventKey, "repo:commit_status_") ||
			strings.HasPrefix(eventKey, "pipeline:") {
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
	if ev.Pipeline != nil {
		return c.handlePipelineEvent(ctx, ev)
	}
	return nil
}

// handlePullRequestEvent processes any pullrequest:* webhook event.
func (c *Client) handlePullRequestEvent(ctx context.Context, ev *event.Event) error {
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
	replyText, err := format.Reply(ev, resolve, c.formatOpts)
	if err != nil {
		c.logger.Error(fmt.Sprintf("bitslack: format reply for %s: %v", prKey, err))
		return nil
	}

	_, err = c.slackClient.PostMessage(ctx, channel, ts, replyText, nil)
	if err != nil {
		c.logger.Error(fmt.Sprintf("bitslack: post reply for %s: %v", prKey, err))
		return nil
	}

	// Approved/unapproved: also update the opening message to reflect live approval status.
	if isApprovalEvent(ev.Key) {
		c.refreshOpeningMessage(ctx, workspace, repoSlug, repoFullName, prID, prKey, channel, ts, resolve)
	}

	return nil
}

// refreshOpeningMessage fetches the latest PR state from Bitbucket and updates the opening message
// in Slack to reflect current approval status.
func (c *Client) refreshOpeningMessage(
	ctx context.Context,
	workspace, repoSlug, repoFullName string,
	prID int,
	prKey, channel, ts string,
	resolve format.UserResolver,
) {
	fullPR, fetchErr := c.bbClient.GetPullRequest(ctx, workspace, repoSlug, prID)
	if fetchErr != nil {
		c.logger.Error(
			fmt.Sprintf("bitslack: fetch PR for approval update %s#%d: %v", repoFullName, prID, fetchErr),
		)
		return
	}
	text, blocks := format.OpeningMessage(fullPR, resolve)
	if updateErr := c.slackClient.UpdateMessage(ctx, channel, ts, text, blocks); updateErr != nil {
		c.logger.Error(fmt.Sprintf("bitslack: update opening message on approval for %s: %v", prKey, updateErr))
	}
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
	replyText, err := format.Reply(ev, resolve, c.formatOpts)
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

// handlePipelineEvent processes pipeline:span_created webhook events.
// Only bbc.pipeline_run spans are handled; other span types produce a nil Pipeline field and are no-ops.
// Returns immediately after scheduling the actual work behind a debounce timer to de-duplicate
// retried webhook deliveries for the same pipeline run.
func (c *Client) handlePipelineEvent(ctx context.Context, ev *event.Event) error {
	run := ev.Pipeline.PipelineRun

	// Real Bitbucket OTel payloads omit pipeline.repository.full_name.
	// Resolve the repo via the API using the UUID attributes when full_name is absent.
	if run.Repository.FullName == "" && run.RepoUUID != "" {
		repo, err := c.bbClient.GetRepository(ctx, run.AccountUUID, run.RepoUUID)
		if err != nil {
			c.logger.Error(fmt.Sprintf("bitslack: resolve repo UUID %q: %v", run.RepoUUID, err))
			return nil
		}
		ev.Pipeline.PipelineRun.Repository = *repo
	}

	uuid := ev.Pipeline.PipelineRun.UUID

	// Debounce: first delivery for this UUID wins; duplicates within the window are dropped.
	c.pipelineDebounceMu.Lock()
	if _, inFlight := c.pipelineDebounce[uuid]; inFlight {
		c.pipelineDebounceMu.Unlock()
		return nil
	}
	c.pipelineDebounce[uuid] = struct{}{}
	c.pipelineDebounceMu.Unlock()

	time.AfterFunc(c.pipelineDebounceDelay, func() { c.processPipelineRun(ev) })
	return nil
}

// processPipelineRun is called by the debounce timer. It fetches step details,
// applies suppression logic, formats the message, and posts to Slack.
func (c *Client) processPipelineRun(ev *event.Event) {
	run := ev.Pipeline.PipelineRun

	// Remove debounce entry when done.
	defer func() {
		c.pipelineDebounceMu.Lock()
		delete(c.pipelineDebounce, run.UUID)
		c.pipelineDebounceMu.Unlock()
	}()

	ctx := context.Background()
	workspace := run.Repository.Workspace.Slug
	repoSlug := run.Repository.Name
	repoFullName := run.Repository.FullName

	// Fetch per-step details from the Bitbucket API.
	steps, err := c.bbClient.GetPipelineSteps(ctx, workspace, repoSlug, run.UUID)
	if err != nil {
		c.logger.Error(fmt.Sprintf("bitslack: get pipeline steps for %s pipeline %s: %v",
			repoFullName, run.UUID, err))
		// Continue with no step data — header-only message will be posted.
	} else {
		for i := range steps {
			steps[i].URL = buildStepURL(run.AccountUUID, run.RepoUUID, run.UUID, steps[i].UUID)
		}
		ev.Pipeline.Steps = steps
	}

	// Apply manual-stop suppression before posting.
	if c.skipManuallyStoppedPipelines && run.Trigger == "MANUAL" && allStepsStopped(steps) {
		c.logger.Info(fmt.Sprintf("bitslack: suppressing manually stopped pipeline %s", run.UUID))
		return
	}

	channel, ok := c.configStore.GetChannel(repoFullName)
	if !ok {
		c.logger.Warn(fmt.Sprintf("bitslack: no channel mapping for repo %q", repoFullName))
		return
	}

	replyText, fmtErr := format.Reply(ev, userResolver(c.configStore), c.formatOpts)
	if fmtErr != nil {
		c.logger.Error(fmt.Sprintf("bitslack: format pipeline reply for %s: %v", repoFullName, fmtErr))
		return
	}

	if run.RefType == "BRANCH" && c.postPipelineToLinkedPR(ctx, run, channel, replyText) {
		return
	}

	// No linked PR (no open PR for branch, or TAG target): post standalone top-level message.
	if _, postErr := c.slackClient.PostMessage(ctx, channel, "", replyText, nil); postErr != nil {
		c.logger.Error(fmt.Sprintf("bitslack: post standalone pipeline message for %s: %v", repoFullName, postErr))
	}
}

// allStepsStopped returns true if steps is non-empty and every step has Result == "STOPPED".
func allStepsStopped(steps []event.PipelineStep) bool {
	if len(steps) == 0 {
		return false
	}
	for _, s := range steps {
		if s.Result != "STOPPED" {
			return false
		}
	}
	return true
}

// buildStepURL constructs the Bitbucket UI URL for a specific pipeline step log.
func buildStepURL(accountUUID, repoUUID, pipelineRunUUID, stepUUID string) string {
	return fmt.Sprintf("https://bitbucket.org/%s/%s/pipelines/results/%s/runs/%s/steps/%s",
		url.PathEscape(accountUUID),
		url.PathEscape(repoUUID),
		url.PathEscape(pipelineRunUUID),
		url.PathEscape(pipelineRunUUID),
		url.PathEscape(stepUUID),
	)
}

// postPipelineToLinkedPR finds an open PR for the pipeline's branch and posts the reply to its thread.
// Returns true if the message was posted (PR found), false otherwise. Errors are logged internally.
func (c *Client) postPipelineToLinkedPR(
	ctx context.Context,
	run event.PipelineRun,
	channel, replyText string,
) bool {
	repoFullName := run.Repository.FullName
	workspace := run.Repository.Workspace.Slug
	repoSlug := run.Repository.Name

	pr, err := c.bbClient.GetOpenPRForBranch(ctx, workspace, repoSlug, run.RefName)
	if err != nil && !errors.Is(err, bitbucket.ErrNotFound) {
		c.logger.Error(fmt.Sprintf("bitslack: find PR for branch %q: %v", run.RefName, err))
		return false
	}
	if pr == nil {
		return false
	}

	prKey := buildPRKey(repoFullName, pr.ID)
	ts, found, storeErr := c.threadStore.Get(ctx, prKey)
	if storeErr != nil {
		c.logger.Error(fmt.Sprintf("bitslack: thread store get %q: %v", prKey, storeErr))
		return false
	}

	if !found {
		// GetOpenPRForBranch uses the list endpoint which omits the reviewers field.
		// Fetch the full PR details so the opening message includes reviewers.
		fullPR, fetchErr := c.bbClient.GetPullRequest(ctx, workspace, repoSlug, pr.ID)
		if fetchErr != nil {
			c.logger.Error(fmt.Sprintf("bitslack: fetch PR %s#%d: %v", repoFullName, pr.ID, fetchErr))
			return false
		}

		text, blocks := format.OpeningMessage(fullPR, userResolver(c.configStore))
		ts, err = c.slackClient.PostMessage(ctx, channel, "", text, blocks)
		if err != nil {
			c.logger.Error(fmt.Sprintf("bitslack: post opening message for %s: %v", prKey, err))
			return false
		}
		if saveErr := c.threadStore.Store(ctx, prKey, ts); saveErr != nil {
			c.logger.Warn(fmt.Sprintf("bitslack: store thread ts for %s: %v", prKey, saveErr))
		}
	}

	if _, err = c.slackClient.PostMessage(ctx, channel, ts, replyText, nil); err != nil {
		c.logger.Error(fmt.Sprintf("bitslack: post pipeline reply for %s: %v", prKey, err))
	}
	return true
}

// isApprovalEvent returns true for events that require refreshing the opening message.
func isApprovalEvent(key string) bool {
	return key == event.KeyPRApproved || key == event.KeyPRUnapproved ||
		key == event.KeyPRFulfilled || key == event.KeyPRRejected
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
