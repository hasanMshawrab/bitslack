package bitbucket

import (
	"context"
	"fmt"
	"net/url"

	"github.com/hasanMshawrab/bitslack/internal/event"
)

// Wire types for Bitbucket API JSON responses.

type userRef struct {
	Nickname    string `json:"nickname"`
	DisplayName string `json:"display_name"`
	UUID        string `json:"uuid"`
	AccountID   string `json:"account_id"`
}

type commitRef struct {
	Hash string `json:"hash"`
}

type branchRef struct {
	Name string `json:"name"`
}

type repoRef struct {
	FullName string       `json:"full_name"`
	Name     string       `json:"name"`
	Links    repoLinksRef `json:"links"`
}

type repoLinksRef struct {
	HTML hrefRef `json:"html"`
}

type endpointRef struct {
	Branch     branchRef `json:"branch"`
	Commit     commitRef `json:"commit"`
	Repository repoRef   `json:"repository"`
}

type hrefRef struct {
	Href string `json:"href"`
}

type prLinksRef struct {
	HTML hrefRef `json:"html"`
}

type participantRef struct {
	User     userRef `json:"user"`
	Role     string  `json:"role"`
	Approved bool    `json:"approved"`
}

type prResponse struct {
	ID                int              `json:"id"`
	Title             string           `json:"title"`
	Description       string           `json:"description"`
	State             string           `json:"state"`
	Author            userRef          `json:"author"`
	Source            endpointRef      `json:"source"`
	Destination       endpointRef      `json:"destination"`
	Reviewers         []userRef        `json:"reviewers"`
	Participants      []participantRef `json:"participants"`
	Reason            string           `json:"reason"`
	MergeCommit       *commitRef       `json:"merge_commit"`
	ClosedBy          *userRef         `json:"closed_by"`
	Links             prLinksRef       `json:"links"`
	CreatedOn         string           `json:"created_on"`
	UpdatedOn         string           `json:"updated_on"`
	CloseSourceBranch bool             `json:"close_source_branch"`
}

type prListResponse struct {
	Values []prResponse `json:"values"`
}

// GetPullRequest fetches a single PR by ID.
func (c *Client) GetPullRequest(ctx context.Context, workspace, repo string, prID int) (*event.PullRequest, error) {
	path := fmt.Sprintf("/repositories/%s/%s/pullrequests/%d", workspace, repo, prID)
	var raw prResponse
	if err := c.get(ctx, path, &raw); err != nil {
		return nil, err
	}
	return toPullRequest(raw), nil
}

// GetPullRequestsForCommit returns all PRs associated with a commit hash.
func (c *Client) GetPullRequestsForCommit(
	ctx context.Context,
	workspace, repo, hash string,
) ([]*event.PullRequest, error) {
	path := fmt.Sprintf("/repositories/%s/%s/commit/%s/pullrequests", workspace, repo, hash)
	var raw prListResponse
	if err := c.get(ctx, path, &raw); err != nil {
		return nil, err
	}
	prs := make([]*event.PullRequest, len(raw.Values))
	for i, r := range raw.Values {
		prs[i] = toPullRequest(r)
	}
	return prs, nil
}

// GetOpenPRForBranch returns the first open PR whose source branch matches the given name,
// or nil if none exists.
func (c *Client) GetOpenPRForBranch(ctx context.Context, workspace, repo, branch string) (*event.PullRequest, error) {
	params := url.Values{}
	params.Set("q", fmt.Sprintf(`source.branch.name="%s"`, branch))
	params.Set("state", "OPEN")
	path := fmt.Sprintf("/repositories/%s/%s/pullrequests?%s", workspace, repo, params.Encode())

	var raw prListResponse
	if err := c.get(ctx, path, &raw); err != nil {
		return nil, err
	}
	if len(raw.Values) == 0 {
		return nil, nil //nolint:nilnil // nil PR signals "not found" without an error; caller checks for nil
	}
	return toPullRequest(raw.Values[0]), nil
}

// toPullRequest maps a Bitbucket API response to the canonical event.PullRequest type.
func toPullRequest(r prResponse) *event.PullRequest {
	pr := &event.PullRequest{
		ID:          r.ID,
		Title:       r.Title,
		Description: r.Description,
		State:       r.State,
		Author: event.User{
			Nickname:    r.Author.Nickname,
			DisplayName: r.Author.DisplayName,
			UUID:        r.Author.UUID,
			AccountID:   r.Author.AccountID,
		},
		Source: event.Endpoint{
			Branch:     event.Branch{Name: r.Source.Branch.Name},
			Commit:     event.Commit{Hash: r.Source.Commit.Hash},
			Repository: toRepository(r.Source.Repository),
		},
		Destination: event.Endpoint{
			Branch:     event.Branch{Name: r.Destination.Branch.Name},
			Commit:     event.Commit{Hash: r.Destination.Commit.Hash},
			Repository: toRepository(r.Destination.Repository),
		},
		Reason:            r.Reason,
		CloseSourceBranch: r.CloseSourceBranch,
		CreatedOn:         r.CreatedOn,
		UpdatedOn:         r.UpdatedOn,
		HTMLURL:           r.Links.HTML.Href,
	}

	pr.Reviewers = make([]event.User, len(r.Reviewers))
	for i, rev := range r.Reviewers {
		pr.Reviewers[i] = event.User{
			Nickname:    rev.Nickname,
			DisplayName: rev.DisplayName,
			UUID:        rev.UUID,
			AccountID:   rev.AccountID,
		}
	}

	pr.Participants = make([]event.Participant, len(r.Participants))
	for i, p := range r.Participants {
		pr.Participants[i] = event.Participant{
			AccountID:   p.User.AccountID,
			Nickname:    p.User.Nickname,
			DisplayName: p.User.DisplayName,
			Role:        p.Role,
			Approved:    p.Approved,
		}
	}

	if r.MergeCommit != nil {
		pr.MergeCommit = &event.Commit{Hash: r.MergeCommit.Hash}
	}
	if r.ClosedBy != nil {
		pr.ClosedBy = &event.User{
			Nickname:    r.ClosedBy.Nickname,
			DisplayName: r.ClosedBy.DisplayName,
			UUID:        r.ClosedBy.UUID,
			AccountID:   r.ClosedBy.AccountID,
		}
	}

	return pr
}

func toRepository(r repoRef) event.Repository {
	return event.Repository{
		FullName: r.FullName,
		Name:     r.Name,
		HTMLURL:  r.Links.HTML.Href,
	}
}

// repoFullResponse is the wire type for a standalone repository API response.
type repoFullResponse struct {
	FullName  string `json:"full_name"`
	Name      string `json:"name"`
	Workspace struct {
		Slug string `json:"slug"`
	} `json:"workspace"`
	Links repoLinksRef `json:"links"`
}

// GetRepository fetches a repository by workspace and repo identifiers.
// workspace and repo may be slugs or UUIDs (with curly braces, e.g. "{uuid}").
func (c *Client) GetRepository(ctx context.Context, workspace, repo string) (*event.Repository, error) {
	path := fmt.Sprintf("/repositories/%s/%s", workspace, repo)
	var raw repoFullResponse
	if err := c.get(ctx, path, &raw); err != nil {
		return nil, err
	}
	return &event.Repository{
		FullName:  raw.FullName,
		Name:      raw.Name,
		Workspace: event.Workspace{Slug: raw.Workspace.Slug},
		HTMLURL:   raw.Links.HTML.Href,
	}, nil
}
