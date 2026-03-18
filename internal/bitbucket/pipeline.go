package bitbucket

import (
	"context"
	"fmt"
	"net/url"

	"github.com/hasanMshawrab/bitslack/internal/event"
)

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
