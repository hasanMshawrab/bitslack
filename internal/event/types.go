// Package event defines the canonical types for Bitbucket webhook payloads
// and provides the Parse function for routing raw JSON to those types.
package event

// Event key constants — single source of truth for routing.
const (
	KeyPRCreated           = "pullrequest:created"
	KeyPRUpdated           = "pullrequest:updated"
	KeyPRApproved          = "pullrequest:approved"
	KeyPRUnapproved        = "pullrequest:unapproved"
	KeyPRFulfilled         = "pullrequest:fulfilled"
	KeyPRRejected          = "pullrequest:rejected"
	KeyPRCommentCreated    = "pullrequest:comment_created"
	KeyCommitStatusCreated = "repo:commit_status_created"
	KeyCommitStatusUpdated = "repo:commit_status_updated"
	KeyPipelineSpanCreated = "pipeline:span_created"
)

// User represents any Bitbucket user reference.
type User struct {
	Nickname    string
	DisplayName string
	UUID        string
	AccountID   string
}

// Commit holds a commit hash reference.
type Commit struct {
	Hash string
}

// Branch holds a branch name.
type Branch struct {
	Name string
}

// Endpoint is one side of a PR (source or destination).
type Endpoint struct {
	Branch     Branch
	Commit     Commit
	Repository Repository
}

// Repository is the repo context from webhooks.
type Repository struct {
	FullName  string // "myworkspace/my-repo"
	Name      string // "my-repo"
	Workspace Workspace
	HTMLURL   string
}

// Workspace provides the slug for API URL construction.
type Workspace struct {
	Slug string
	Name string
}

// CommentContent holds the raw text of a comment.
type CommentContent struct {
	Raw string
}

// InlineLocation describes where an inline comment is anchored.
type InlineLocation struct {
	Path string
	To   int
}

// Comment represents a PR comment.
type Comment struct {
	ID       int
	Content  CommentContent
	Inline   *InlineLocation // nil for top-level comments
	ParentID int             // non-zero when this comment is a reply to another comment
	HTMLURL  string
}

// Approval wraps the approval data on approved/unapproved events.
type Approval struct {
	Date string
	User User
}

// Participant represents a PR participant (reviewer, author, or observer).
type Participant struct {
	AccountID   string
	Nickname    string
	DisplayName string
	Role        string // "REVIEWER", "AUTHOR", "PARTICIPANT"
	Approved    bool
}

// PullRequest is the canonical PR type used across the library.
// Also returned by the Bitbucket API client.
type PullRequest struct {
	ID                int
	Title             string
	Description       string
	State             string // "OPEN", "MERGED", "DECLINED"
	Author            User
	Source            Endpoint
	Destination       Endpoint
	Reviewers         []User
	Participants      []Participant
	Reason            string  // non-empty only on rejected
	MergeCommit       *Commit // non-nil only on fulfilled
	ClosedBy          *User   // non-nil on fulfilled and rejected
	CloseSourceBranch bool
	CreatedOn         string
	UpdatedOn         string
	HTMLURL           string
}

// PullRequestEvent is the parsed form of any pullrequest:* webhook.
type PullRequestEvent struct {
	Actor       User
	PullRequest PullRequest
	Repository  Repository
	Approval    *Approval // only on approved/unapproved
	Comment     *Comment  // only on comment_created
}

// CommitStatus is the build status data.
type CommitStatus struct {
	Name        string
	Description string
	State       string // "INPROGRESS", "SUCCESSFUL", "FAILED"
	Key         string
	URL         string
	CreatedOn   string
	UpdatedOn   string
	CommitHash  string // extracted from links.commit.href during parsing
}

// CommitStatusEvent is the parsed form of repo:commit_status_* webhooks.
type CommitStatusEvent struct {
	Actor        User
	CommitStatus CommitStatus
	Repository   Repository
}

// PipelineRun holds data for a single Bitbucket Pipelines run,
// extracted from a bbc.pipeline_run OTel span.
type PipelineRun struct {
	UUID         string // pipeline_run.uuid — used for debounce deduplication
	PipelineUUID string // pipeline.uuid — used for the Bitbucket steps API
	RunNumber    int
	Result       string // OTel values: "COMPLETE", "FAILED", "ERROR", "STOPPED"
	Trigger      string // "PUSH", "MANUAL", "SCHEDULED"
	RefName      string // branch or tag name
	RefType      string // "BRANCH" or "TAG"
	Repository   Repository
	RepoUUID     string // pipeline.repository.uuid — used to resolve repo when full_name is absent
	AccountUUID  string // pipeline.account.uuid — used to resolve repo when full_name is absent
	URL          string // link to the pipeline run in Bitbucket UI
	DurationSecs int    // total run duration computed from OTel span timestamps
}

// PipelineStep holds data for a single step within a pipeline run,
// fetched from the Bitbucket Pipelines REST API after the span is received.
type PipelineStep struct {
	UUID         string
	Name         string
	Result       string // REST API values: "SUCCESSFUL", "FAILED", "ERROR", "STOPPED", "NOT_RUN"
	DurationSecs int
	URL          string // link to the step log in Bitbucket UI
}

// PipelineRunEvent is the parsed form of a pipeline:span_created bbc.pipeline_run span.
// Steps is populated by the handler after fetching from the Bitbucket API.
// Creator is populated by the handler after fetching pipeline details from the Bitbucket API.
type PipelineRunEvent struct {
	PipelineRun PipelineRun
	Steps       []PipelineStep
	Creator     *User // nil if unavailable
}

// LatestPipelineRun holds the minimum fields needed to render the *Builds:* field
// in the PR opening message. Populated by the Bitbucket pipelines list API.
type LatestPipelineRun struct {
	RunNumber int
	Result    string // REST API values: SUCCESSFUL, FAILED, ERROR, STOPPED, IN_PROGRESS, PENDING
	URL       string
}

// Event is a discriminated union of all event families.
// Exactly one of PullRequest, CommitStatus, or Pipeline is non-nil after Parse
// (Pipeline may be nil for non-pipeline_run span types).
type Event struct {
	Key          string
	PullRequest  *PullRequestEvent
	CommitStatus *CommitStatusEvent
	Pipeline     *PipelineRunEvent
}
