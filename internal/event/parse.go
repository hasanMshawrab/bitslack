package event

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const nanosPerSecond = 1_000_000_000

// CommitHashFromHref extracts the commit hash from a Bitbucket API href URL.
// Expected format: "https://api.bitbucket.org/2.0/repositories/{workspace}/{repo}/commit/{hash}"
func CommitHashFromHref(href string) (string, error) {
	const marker = "/commit/"
	idx := strings.LastIndex(href, marker)
	if idx == -1 {
		return "", fmt.Errorf("no /commit/ segment found in href: %s", href)
	}
	rest := href[idx+len(marker):]
	// Strip anything after the hash (e.g. "/statuses/build/...")
	if slashIdx := strings.Index(rest, "/"); slashIdx != -1 {
		rest = rest[:slashIdx]
	}
	if rest == "" {
		return "", fmt.Errorf("empty commit hash in href: %s", href)
	}
	return rest, nil
}

// Parse unmarshals a Bitbucket webhook payload into an Event.
// eventKey is the X-Event-Key header value (e.g. "pullrequest:created").
// Returns an error for unknown event keys, malformed JSON, or empty payload.
func Parse(eventKey string, payload []byte) (*Event, error) {
	if len(payload) == 0 {
		return nil, errors.New("empty payload")
	}

	switch {
	case strings.HasPrefix(eventKey, "pullrequest:"):
		return parsePullRequestEvent(eventKey, payload)
	case strings.HasPrefix(eventKey, "repo:commit_status_"):
		return parseCommitStatusEvent(eventKey, payload)
	case strings.HasPrefix(eventKey, "pipeline:"):
		return parsePipelineSpanEvent(eventKey, payload)
	default:
		return nil, fmt.Errorf("unknown event key: %s", eventKey)
	}
}

// --- Wire types for JSON unmarshaling ---

type wireUser struct {
	Nickname    string `json:"nickname"`
	DisplayName string `json:"display_name"`
	UUID        string `json:"uuid"`
	AccountID   string `json:"account_id"`
}

type wireBranch struct {
	Name string `json:"name"`
}

type wireCommit struct {
	Hash string `json:"hash"`
}

type wireRepository struct {
	FullName  string        `json:"full_name"`
	Name      string        `json:"name"`
	Workspace wireWorkspace `json:"workspace"`
	Links     wireRepoLinks `json:"links"`
}

type wireWorkspace struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type wireRepoLinks struct {
	HTML wireHref `json:"html"`
}

type wireHref struct {
	Href string `json:"href"`
}

type wireEndpoint struct {
	Branch     wireBranch     `json:"branch"`
	Commit     wireCommit     `json:"commit"`
	Repository wireRepository `json:"repository"`
}

type wireApproval struct {
	Date string   `json:"date"`
	User wireUser `json:"user"`
}

type wireCommentContent struct {
	Raw string `json:"raw"`
}

type wireInline struct {
	Path string `json:"path"`
	To   int    `json:"to"`
}

type wireCommentParent struct {
	ID int `json:"id"`
}

type wireComment struct {
	ID      int                `json:"id"`
	Content wireCommentContent `json:"content"`
	Inline  *wireInline        `json:"inline"`
	Parent  *wireCommentParent `json:"parent"`
	Links   wireCommentLinks   `json:"links"`
}

type wireCommentLinks struct {
	HTML wireHref `json:"html"`
}

type wirePR struct {
	ID                int          `json:"id"`
	Title             string       `json:"title"`
	State             string       `json:"state"`
	Reason            string       `json:"reason"`
	Author            wireUser     `json:"author"`
	Source            wireEndpoint `json:"source"`
	Destination       wireEndpoint `json:"destination"`
	Reviewers         []wireUser   `json:"reviewers"`
	MergeCommit       *wireCommit  `json:"merge_commit"`
	ClosedBy          *wireUser    `json:"closed_by"`
	CloseSourceBranch bool         `json:"close_source_branch"`
	CreatedOn         string       `json:"created_on"`
	UpdatedOn         string       `json:"updated_on"`
	Links             wirePRLinks  `json:"links"`
}

type wirePRLinks struct {
	HTML wireHref `json:"html"`
}

type wirePRPayload struct {
	Actor       wireUser       `json:"actor"`
	PullRequest wirePR         `json:"pullrequest"`
	Repository  wireRepository `json:"repository"`
	Approval    *wireApproval  `json:"approval"`
	Comment     *wireComment   `json:"comment"`
}

type wireCommitStatusLinks struct {
	Commit wireHref `json:"commit"`
}

type wireCommitStatus struct {
	Name        string                `json:"name"`
	Description string                `json:"description"`
	State       string                `json:"state"`
	Key         string                `json:"key"`
	URL         string                `json:"url"`
	CreatedOn   string                `json:"created_on"`
	UpdatedOn   string                `json:"updated_on"`
	Links       wireCommitStatusLinks `json:"links"`
}

type wireCommitStatusPayload struct {
	Actor        wireUser         `json:"actor"`
	CommitStatus wireCommitStatus `json:"commit_status"`
	Repository   wireRepository   `json:"repository"`
}

// --- Mapping functions ---

func mapUser(w wireUser) User {
	return User(w)
}

func mapRepository(w wireRepository) Repository {
	return Repository{
		FullName:  w.FullName,
		Name:      w.Name,
		Workspace: Workspace{Slug: w.Workspace.Slug, Name: w.Workspace.Name},
		HTMLURL:   w.Links.HTML.Href,
	}
}

func mapEndpoint(w wireEndpoint) Endpoint {
	return Endpoint{
		Branch:     Branch{Name: w.Branch.Name},
		Commit:     Commit{Hash: w.Commit.Hash},
		Repository: mapRepository(w.Repository),
	}
}

func mapUsers(ws []wireUser) []User {
	users := make([]User, len(ws))
	for i, w := range ws {
		users[i] = mapUser(w)
	}
	return users
}

func parsePullRequestEvent(eventKey string, payload []byte) (*Event, error) {
	var w wirePRPayload
	if err := json.Unmarshal(payload, &w); err != nil {
		return nil, fmt.Errorf("parsing pull request event: %w", err)
	}

	pr := PullRequest{
		ID:                w.PullRequest.ID,
		Title:             w.PullRequest.Title,
		State:             w.PullRequest.State,
		Author:            mapUser(w.PullRequest.Author),
		Source:            mapEndpoint(w.PullRequest.Source),
		Destination:       mapEndpoint(w.PullRequest.Destination),
		Reviewers:         mapUsers(w.PullRequest.Reviewers),
		Reason:            w.PullRequest.Reason,
		CloseSourceBranch: w.PullRequest.CloseSourceBranch,
		CreatedOn:         w.PullRequest.CreatedOn,
		UpdatedOn:         w.PullRequest.UpdatedOn,
		HTMLURL:           w.PullRequest.Links.HTML.Href,
	}

	if w.PullRequest.MergeCommit != nil {
		pr.MergeCommit = &Commit{Hash: w.PullRequest.MergeCommit.Hash}
	}
	if w.PullRequest.ClosedBy != nil {
		u := mapUser(*w.PullRequest.ClosedBy)
		pr.ClosedBy = &u
	}

	evt := &PullRequestEvent{
		Actor:       mapUser(w.Actor),
		PullRequest: pr,
		Repository:  mapRepository(w.Repository),
	}

	if w.Approval != nil {
		evt.Approval = &Approval{
			Date: w.Approval.Date,
			User: mapUser(w.Approval.User),
		}
	}

	if w.Comment != nil {
		c := &Comment{
			ID:      w.Comment.ID,
			Content: CommentContent{Raw: w.Comment.Content.Raw},
			HTMLURL: w.Comment.Links.HTML.Href,
		}
		if w.Comment.Inline != nil {
			c.Inline = &InlineLocation{
				Path: w.Comment.Inline.Path,
				To:   w.Comment.Inline.To,
			}
		}
		if w.Comment.Parent != nil {
			c.ParentID = w.Comment.Parent.ID
		}
		evt.Comment = c
	}

	return &Event{
		Key:         eventKey,
		PullRequest: evt,
	}, nil
}

// --- OTel wire types for pipeline:span_created ---

type wireOTelValue struct {
	StringValue string `json:"stringValue"`
	IntValue    string `json:"intValue"` // OTel int64 serialised as string in protobuf JSON
}

type wireOTelAttribute struct {
	Key   string        `json:"key"`
	Value wireOTelValue `json:"value"`
}

type wireOTelSpan struct {
	Name              string              `json:"name"`
	StartTimeUnixNano string              `json:"startTimeUnixNano"`
	EndTimeUnixNano   string              `json:"endTimeUnixNano"`
	Attributes        []wireOTelAttribute `json:"attributes"`
}

type wireOTelScopeSpan struct {
	Spans []wireOTelSpan `json:"spans"`
}

type wireOTelResourceSpan struct {
	ScopeSpans []wireOTelScopeSpan `json:"scopeSpans"`
}

type wirePipelineSpanPayload struct {
	ResourceSpans []wireOTelResourceSpan `json:"resourceSpans"`
}

func parsePipelineSpanEvent(eventKey string, payload []byte) (*Event, error) {
	var w wirePipelineSpanPayload
	if err := json.Unmarshal(payload, &w); err != nil {
		return nil, fmt.Errorf("parsing pipeline span event: %w", err)
	}

	// Find the first bbc.pipeline_run span — ignore step/command/container spans.
	for _, rs := range w.ResourceSpans {
		for _, ss := range rs.ScopeSpans {
			for _, span := range ss.Spans {
				if span.Name != "bbc.pipeline_run" {
					continue
				}
				return buildPipelineRunEvent(eventKey, span), nil
			}
		}
	}

	// No bbc.pipeline_run span found (e.g. step/command/container span) — no action needed.
	return &Event{Key: eventKey}, nil
}

// buildPipelineRunEvent constructs the Event for a bbc.pipeline_run OTel span.
func buildPipelineRunEvent(eventKey string, span wireOTelSpan) *Event {
	// Build an attribute map for O(1) lookup.
	attrs := make(map[string]wireOTelValue, len(span.Attributes))
	for _, a := range span.Attributes {
		attrs[a.Key] = a.Value
	}

	fullName := attrs["pipeline.repository.full_name"].StringValue
	var repo Repository
	repo.FullName = fullName
	if workspace, repoSlug, ok := strings.Cut(fullName, "/"); ok {
		repo.Workspace = Workspace{Slug: workspace}
		repo.Name = repoSlug
	}

	runNumber, _ := strconv.Atoi(attrs["pipeline_run.run_number"].StringValue)
	uuid := attrs["pipeline_run.uuid"].StringValue
	url := attrs["pipeline_run.url"].StringValue

	var durationSecs int
	startNano, startErr := strconv.ParseInt(span.StartTimeUnixNano, 10, 64)
	endNano, endErr := strconv.ParseInt(span.EndTimeUnixNano, 10, 64)
	if startErr == nil && endErr == nil && endNano > startNano {
		durationSecs = int((endNano - startNano) / nanosPerSecond)
	}

	return &Event{
		Key: eventKey,
		Pipeline: &PipelineRunEvent{
			PipelineRun: PipelineRun{
				UUID:         uuid,
				RunNumber:    runNumber,
				Result:       attrs["pipeline.state.result.name"].StringValue,
				Trigger:      attrs["pipeline.trigger.name"].StringValue,
				RefName:      attrs["pipeline.target.ref_name"].StringValue,
				RefType:      attrs["pipeline.target.ref_type"].StringValue,
				Repository:   repo,
				RepoUUID:     attrs["pipeline.repository.uuid"].StringValue,
				AccountUUID:  attrs["pipeline.account.uuid"].StringValue,
				URL:          url,
				DurationSecs: durationSecs,
			},
		},
	}
}

func parseCommitStatusEvent(eventKey string, payload []byte) (*Event, error) {
	var w wireCommitStatusPayload
	if err := json.Unmarshal(payload, &w); err != nil {
		return nil, fmt.Errorf("parsing commit status event: %w", err)
	}

	commitHash, err := CommitHashFromHref(w.CommitStatus.Links.Commit.Href)
	if err != nil {
		return nil, fmt.Errorf("extracting commit hash: %w", err)
	}

	cs := CommitStatus{
		Name:        w.CommitStatus.Name,
		Description: w.CommitStatus.Description,
		State:       w.CommitStatus.State,
		Key:         w.CommitStatus.Key,
		URL:         w.CommitStatus.URL,
		CreatedOn:   w.CommitStatus.CreatedOn,
		UpdatedOn:   w.CommitStatus.UpdatedOn,
		CommitHash:  commitHash,
	}

	return &Event{
		Key: eventKey,
		CommitStatus: &CommitStatusEvent{
			Actor:        mapUser(w.Actor),
			CommitStatus: cs,
			Repository:   mapRepository(w.Repository),
		},
	}, nil
}
