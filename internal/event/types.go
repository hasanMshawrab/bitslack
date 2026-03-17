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
	ID      int
	Content CommentContent
	Inline  *InlineLocation // nil for top-level comments
	HTMLURL string
}

// Approval wraps the approval data on approved/unapproved events.
type Approval struct {
	Date string
	User User
}

// PullRequest is the canonical PR type used across the library.
// Also returned by the Bitbucket API client.
type PullRequest struct {
	ID                int
	Title             string
	State             string // "OPEN", "MERGED", "DECLINED"
	Author            User
	Source            Endpoint
	Destination       Endpoint
	Reviewers         []User
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
	UUID       string
	RunNumber  int
	Result     string // "SUCCESSFUL", "FAILED", "ERROR", "STOPPED"
	Trigger    string // "PUSH", "MANUAL", "SCHEDULED"
	RefName    string // branch or tag name
	RefType    string // "BRANCH" or "TAG"
	Repository Repository
	URL        string // link to the pipeline run in Bitbucket UI
}

// PipelineRunEvent is the parsed form of a pipeline:span_created bbc.pipeline_run span.
type PipelineRunEvent struct {
	PipelineRun PipelineRun
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
