package bitbucket

import (
	"context"
	"fmt"
	"net/url"

	"github.com/hasanMshawrab/bbthread/internal/event"
)

// pipelineListItemWire is the wire type for a single entry in the pipelines list API.
// Note: the real Bitbucket API does not include an "html" key in the "links" object
// for pipeline list items — only "self" and "steps" are present. The run URL is
// constructed from workspace, repo, and build_number instead.
type pipelineListItemWire struct {
	BuildNumber int `json:"build_number"`
	State       struct {
		Name   string `json:"name"`
		Result *struct {
			Name string `json:"name"`
		} `json:"result"`
	} `json:"state"`
}

// pipelineListWire is the wire type for the Bitbucket pipelines list API response.
type pipelineListWire struct {
	Values []pipelineListItemWire `json:"values"`
}

// GetLatestPipelineForBranch fetches the most recent pipeline run for the given branch.
// Returns nil, nil if no runs exist. Returns an error on API failure (including 403).
func (c *Client) GetLatestPipelineForBranch(
	ctx context.Context,
	workspace, repo, branch string,
) (*event.LatestPipelineRun, error) {
	params := url.Values{}
	params.Set("sort", "-created_on")
	params.Set("pagelen", "1")
	params.Set("target.branch", branch)
	path := fmt.Sprintf("/repositories/%s/%s/pipelines/?%s", workspace, repo, params.Encode())

	var raw pipelineListWire
	if err := c.get(ctx, path, &raw); err != nil {
		return nil, err
	}
	if len(raw.Values) == 0 {
		return nil, nil //nolint:nilnil // nil signals "no runs found"; caller handles gracefully
	}

	item := raw.Values[0]
	result := item.State.Name
	if item.State.Result != nil && item.State.Result.Name != "" {
		result = item.State.Result.Name
	}

	runURL := fmt.Sprintf("https://bitbucket.org/%s/%s/pipelines/results/%d", workspace, repo, item.BuildNumber)

	return &event.LatestPipelineRun{
		RunNumber: item.BuildNumber,
		Result:    result,
		URL:       runURL,
	}, nil
}

// pipelineWire is the wire type for a Bitbucket pipeline API response (partial).
type pipelineWire struct {
	Creator struct {
		AccountID   string `json:"account_id"`
		DisplayName string `json:"display_name"`
		UUID        string `json:"uuid"`
		Nickname    string `json:"nickname"`
	} `json:"creator"`
}

// GetPipelineCreator fetches the creator of a pipeline run.
// pipelineUUID is pipeline.uuid from the OTel span (the build UUID, not the run UUID).
func (c *Client) GetPipelineCreator(ctx context.Context, workspace, repo, pipelineUUID string) (*event.User, error) {
	path := fmt.Sprintf("/repositories/%s/%s/pipelines/%s",
		workspace,
		repo,
		url.PathEscape(pipelineUUID),
	)
	var raw pipelineWire
	if err := c.get(ctx, path, &raw); err != nil {
		return nil, err
	}
	return &event.User{
		AccountID:   raw.Creator.AccountID,
		DisplayName: raw.Creator.DisplayName,
		UUID:        raw.Creator.UUID,
		Nickname:    raw.Creator.Nickname,
	}, nil
}

// stepResultWire is the wire type for a Bitbucket pipeline step state result.
type stepResultWire struct {
	Name string `json:"name"`
}

// stepStateWire is the wire type for a Bitbucket pipeline step state.
type stepStateWire struct {
	Name   string          `json:"name"`
	Result *stepResultWire `json:"result"`
}

// pipelineStepWire is the wire type for a single step in the Bitbucket API response.
type pipelineStepWire struct {
	UUID              string        `json:"uuid"`
	Name              string        `json:"name"`
	State             stepStateWire `json:"state"`
	DurationInSeconds int           `json:"duration_in_seconds"`
}

// pipelineStepsWire is the wire type for the Bitbucket pipeline steps list API.
type pipelineStepsWire struct {
	Values []pipelineStepWire `json:"values"`
}

// GetPipelineSteps fetches the steps for a given pipeline run.
// pipelineUUID is the pipeline_run.uuid from the OTel span.
func (c *Client) GetPipelineSteps(
	ctx context.Context, workspace, repo, pipelineUUID string,
) ([]event.PipelineStep, error) {
	path := fmt.Sprintf("/repositories/%s/%s/pipelines/%s/steps/",
		workspace,
		repo,
		url.PathEscape(pipelineUUID),
	)
	var raw pipelineStepsWire
	if err := c.get(ctx, path, &raw); err != nil {
		return nil, err
	}
	steps := make([]event.PipelineStep, len(raw.Values))
	for i, s := range raw.Values {
		result := "NOT_RUN"
		if s.State.Result != nil && s.State.Result.Name != "" {
			result = s.State.Result.Name
		} else if s.State.Name == "STOPPED" {
			result = "STOPPED"
		}
		steps[i] = event.PipelineStep{
			UUID:         s.UUID,
			Name:         s.Name,
			Result:       result,
			DurationSecs: s.DurationInSeconds,
		}
	}
	return steps, nil
}
