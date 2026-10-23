// Package mcpserver exposes boAIrd services through authenticated MCP tools.
package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"boAIrd/internal/agent"
	"boAIrd/internal/brain"
	"boAIrd/internal/capability"
	"boAIrd/internal/conversation"
	"boAIrd/internal/heartbeat"
	"boAIrd/internal/mcptoken"
	"boAIrd/internal/project"
	"boAIrd/internal/run"
	"boAIrd/internal/task"
	governedtool "boAIrd/internal/tool"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const maxToolPageSize = 100

type Services struct {
	Agents         AgentService
	Projects       ProjectService
	Tasks          TaskService
	Denials        AuthorizationDenialService
	Conversations  ConversationService
	Brains         BrainService
	Runs           RunService
	Heartbeats     HeartbeatService
	Tools          ToolExecutor
	GitTools       ToolExecutor
	GitHubTools    ToolExecutor
	ExtensionTools ToolExecutor
}

type ToolExecutor interface {
	Execute(context.Context, governedtool.Request) (json.RawMessage, error)
}

type TokenVerifier interface {
	Verify(context.Context, string) (mcptoken.Identity, error)
}

// HeartbeatService records authenticated activity and derives Agent status.
type HeartbeatService interface {
	Heartbeat(context.Context, heartbeat.Identity) (agent.Observed, error)
	Statuses(context.Context, []string) (map[string]agent.Observed, error)
}
type AuthorizationDenialService interface {
	RecordAuthorizationDenial(context.Context, string, string, string, string, string) error
}

type AgentService interface {
	Get(context.Context, string) (agent.Agent, error)
}
type ProjectService interface {
	Get(context.Context, string) (project.Project, error)
	List(context.Context, int, int) ([]project.Project, error)
}
type TaskService interface {
	Get(context.Context, string) (task.Task, error)
	List(context.Context, string, task.Status, string, int, int) ([]task.Task, error)
	AssignAtVersion(context.Context, string, string, int64, string, task.Actor) (task.Task, error)
	Claim(context.Context, string, string, int64, string, task.Actor) (task.Task, error)
	Transition(context.Context, string, task.Status, string, task.Actor) (task.Task, error)
}
type ConversationService interface {
	GetPageForAgent(context.Context, string, string, string, int) (conversation.Conversation, []conversation.Message, string, error)
	GetPageForBrain(context.Context, string, string, string, int) (conversation.Conversation, []conversation.Message, string, error)
	Append(context.Context, string, conversation.SenderType, string, conversation.MessageInput) (conversation.Message, error)
}
type BrainService interface {
	Get(context.Context, string) (brain.Brain, error)
	Execute(context.Context, string, string, int) (brain.Execution, error)
}
type RunService interface {
	Get(context.Context, string) (run.Run, error)
}

type Server struct {
	services Services
	tokens   TokenVerifier
}

func New(services Services, tokens TokenVerifier) (*Server, error) {
	if services.Agents == nil || services.Projects == nil || services.Tasks == nil || services.Conversations == nil || services.Brains == nil || services.Runs == nil {
		return nil, errors.New("MCP services are required")
	}
	if tokens == nil {
		return nil, errors.New("MCP token verifier is required")
	}
	return &Server{services: services, tokens: tokens}, nil
}

func (s *Server) Handler() http.Handler {
	server := mcp.NewServer(&mcp.Implementation{Name: "boAIrd", Version: "0.1.0"}, &mcp.ServerOptions{SupportedProtocolVersions: []string{"2025-11-25"}})
	s.registerTools(server)
	transport := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{SessionTimeout: 30 * time.Minute, MaxRequestBodyBytes: 1 << 20})
	return auth.RequireBearerToken(s.verifyToken, &auth.RequireBearerTokenOptions{AllowMissingExpiration: true})(transport)
}

func (s *Server) verifyToken(ctx context.Context, raw string, _ *http.Request) (*auth.TokenInfo, error) {
	identity, err := s.tokens.Verify(ctx, raw)
	if err != nil || strings.TrimSpace(identity.AgentID) == "" {
		return nil, auth.ErrInvalidToken
	}
	if _, err := s.services.Agents.Get(ctx, identity.AgentID); err != nil {
		return nil, auth.ErrInvalidToken
	}
	if s.services.Heartbeats != nil {
		// Liveness is observability, not authorization: a failed activity write
		// must never reject an otherwise valid credential.
		if _, err := s.services.Heartbeats.Heartbeat(ctx, heartbeat.Identity{AgentID: identity.AgentID, TokenID: identity.TokenID}); err != nil {
			slog.Error("record MCP agent activity", "error", err, "agent_id", identity.AgentID)
		}
	}
	return &auth.TokenInfo{UserID: identity.AgentID, Extra: map[string]any{"token_id": identity.TokenID}}, nil
}

func (s *Server) registerTools(server *mcp.Server) {
	add := func(name, description string) *mcp.Tool { return &mcp.Tool{Name: name, Description: description} }
	mcp.AddTool[projectInput, projectOutput](server, add("boaird.project.get", "Get a project visible to the authenticated Agent."), s.projectGet)
	mcp.AddTool[emptyInput, []agentOutput](server, add("boaird.agent.list", "List only the authenticated Agent."), s.agentList)
	mcp.AddTool[agentInput, agentOutput](server, add("boaird.agent.get", "Get the authenticated Agent; other Agent identities are not exposed."), s.agentGet)
	mcp.AddTool[agentInput, agentOutput](server, add("boaird.agent.status", "Get the server-derived status of the authenticated Agent."), s.agentGet)
	mcp.AddTool[heartbeatInput, agentOutput](server, add("boaird.agent.heartbeat", "Report authenticated activity for the calling Agent and return its server-derived status."), s.agentHeartbeat)
	mcp.AddTool[taskListInput, []taskOutput](server, add("boaird.task.list", "List tasks in a project, or only tasks assigned to the authenticated Agent across projects."), s.taskList)
	mcp.AddTool[taskInput, taskOutput](server, add("boaird.task.get", "Get a task assigned to this Agent or in its explicit project scope."), s.taskGet)
	mcp.AddTool[taskAssignInput, taskOutput](server, add("boaird.task.assign", "Assign a project task to an eligible Agent as the active Brain or an authorized project Agent; include the observed task version."), s.taskAssign)
	mcp.AddTool[taskActionInput, taskOutput](server, add("boaird.task.claim", "Claim a READY task as its assigned Worker, or self-assign an unassigned task when task.assign is explicitly granted."), s.taskClaim)
	mcp.AddTool[taskUpdateInput, taskOutput](server, add("boaird.task.update", "Move an assigned task as its Worker, or a project task as the active Brain under Brain policy."), s.taskUpdate)
	mcp.AddTool[taskActionInput, taskOutput](server, add("boaird.task.complete", "Complete an assigned task or a project task as the active Brain when capability, autonomy, workflow, and Human Review policy allow it."), s.taskComplete)
	mcp.AddTool[taskActionInput, taskOutput](server, add("boaird.task.fail", "Mark an assigned task failed as its Worker, or a project task as the active Brain under Brain policy."), s.taskFail)
	mcp.AddTool[conversationReadInput, conversationOutput](server, add("boaird.conversation.read", "Read a bounded page of messages for an assigned task or a task in the authenticated active Brain's project."), s.conversationRead)
	mcp.AddTool[conversationSendInput, messageOutput](server, add("boaird.conversation.send", "Send a message as the authenticated Agent on an assigned task, or as the active Brain within its project."), s.conversationSend)
	mcp.AddTool[taskInput, contextOutput](server, add("boaird.context.get", "Get task context with a bounded recent conversation page when assigned to this Agent or scoped to the authenticated active Brain's project."), s.contextGet)
	mcp.AddTool[brainRequestInput, brainOutput](server, add("boaird.brain.request", "Submit a planning request to an API-backed active Brain. An MCP Brain performs work through its authenticated tools."), s.brainRequest)
	mcp.AddTool[runInput, runOutput](server, add("boaird.run.get", "Get a run in a project explicitly assigned to this Agent."), s.runGet)
	if s.services.Tools != nil {
		mcp.AddTool[shellExecuteInput, governedtool.ShellOutput](server, add("boaird.shell.execute", "Run one administrator-allowlisted executable with structured arguments inside an authorized project workspace."), s.shellExecute)
	}
	if s.services.GitTools != nil {
		mcp.AddTool[gitReadMCPInput, governedtool.GitReadOutput](server, add("boaird.git.read", "Inspect repository status, log, a ref, or tracked files inside an authorized project workspace."), s.gitRead)
		mcp.AddTool[gitDiffMCPInput, governedtool.GitDiffOutput](server, add("boaird.git.diff", "Show bounded working-tree, staged, or reference diffs inside an authorized project workspace."), s.gitDiff)
		mcp.AddTool[gitBranchMCPInput, governedtool.GitBranchOutput](server, add("boaird.git.branch", "List repository branches and the current branch inside an authorized project workspace."), s.gitBranch)
		mcp.AddTool[gitCommitMCPInput, governedtool.GitCommitOutput](server, add("boaird.git.commit", "Commit staged changes with a message inside an authorized project workspace."), s.gitCommit)
		mcp.AddTool[gitPushMCPInput, governedtool.GitPushOutput](server, add("boaird.git.push", "Push a branch to a named remote inside an authorized project workspace; denied by default and subject to Human Review."), s.gitPush)
		mcp.AddTool[gitMergeMCPInput, governedtool.GitMergeOutput](server, add("boaird.git.merge", "Merge a reference into the current branch inside an authorized project workspace; denied by default and subject to Human Review."), s.gitMerge)
	}
	if s.services.GitHubTools != nil {
		mcp.AddTool[githubIssueReadMCPInput, governedtool.GitHubIssueReadOutput](server, add("boaird.github.issue.read", "Read an issue or list issues in the project's approved GitHub repository."), s.githubIssueRead)
		mcp.AddTool[githubIssueCreateMCPInput, governedtool.GitHubIssueCreateOutput](server, add("boaird.github.issue.create", "Create an issue in the project's approved GitHub repository."), s.githubIssueCreate)
		mcp.AddTool[githubIssueUpdateMCPInput, governedtool.GitHubIssueUpdateOutput](server, add("boaird.github.issue.update", "Update an issue in the project's approved GitHub repository."), s.githubIssueUpdate)
		mcp.AddTool[githubPRReadMCPInput, governedtool.GitHubPRReadOutput](server, add("boaird.github.pr.read", "Read a pull request or list pull requests in the project's approved GitHub repository."), s.githubPRRead)
		mcp.AddTool[githubPRCreateMCPInput, governedtool.GitHubPRCreateOutput](server, add("boaird.github.pr.create", "Create a pull request in the project's approved GitHub repository."), s.githubPRCreate)
		mcp.AddTool[githubPRReviewMCPInput, governedtool.GitHubPRReviewOutput](server, add("boaird.github.pr.review", "Submit a review on a pull request in the project's approved GitHub repository."), s.githubPRReview)
		mcp.AddTool[githubPRMergeMCPInput, governedtool.GitHubPRMergeOutput](server, add("boaird.github.pr.merge", "Merge a pull request in the project's approved GitHub repository; denied by default and subject to Human Review."), s.githubPRMerge)
	}
	if s.services.ExtensionTools != nil {
		mcp.AddTool[extensionExecuteMCPInput, map[string]any](server, add("boaird.extension.execute", "Execute one activated project extension tool through capability, project-scope, schema, and audit enforcement."), s.extensionExecute)
	}
}

type emptyInput struct{}
type projectInput struct {
	ProjectID string `json:"project_id"`
}
type projectOutput struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
}
type agentInput struct {
	AgentID string `json:"agent_id,omitempty"`
}
type agentOutput struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Role         string     `json:"role"`
	Status       string     `json:"status"`
	Runtime      string     `json:"runtime"`
	Capabilities []string   `json:"capabilities"`
	LastSeenAt   *time.Time `json:"last_seen_at,omitempty"`
	SessionOpen  bool       `json:"session_open"`
}

// heartbeatInput carries no status: identity comes from the verified credential.
// agent_id is accepted only so a mismatched identity is rejected explicitly.
type heartbeatInput struct {
	AgentID string `json:"agent_id,omitempty"`
}
type taskListInput struct {
	ProjectID string `json:"project_id,omitempty"`
	Status    string `json:"status,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
}
type taskInput struct {
	TaskID string `json:"task_id"`
}
type taskActionInput struct {
	TaskID string `json:"task_id"`
	Reason string `json:"reason"`
}
type taskAssignInput struct {
	TaskID          string `json:"task_id"`
	AgentID         string `json:"agent_id"`
	ExpectedVersion int64  `json:"expected_version"`
	Reason          string `json:"reason"`
}
type taskUpdateInput struct {
	TaskID string `json:"task_id"`
	Status string `json:"status"`
	Reason string `json:"reason"`
}
type taskOutput struct {
	ID          string `json:"id"`
	ProjectID   string `json:"project_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Priority    string `json:"priority"`
	AssignedTo  string `json:"assigned_to,omitempty"`
	HumanReview bool   `json:"human_review"`
	Version     int64  `json:"version"`
}
type conversationReadInput struct {
	TaskID string `json:"task_id"`
	Before string `json:"before,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}
type conversationOutput struct {
	ConversationID string          `json:"conversation_id"`
	TaskID         string          `json:"task_id"`
	Messages       []messageOutput `json:"messages"`
	NextBefore     string          `json:"next_before,omitempty"`
}
type conversationSendInput struct {
	TaskID      string `json:"task_id"`
	Content     string `json:"content"`
	ContentType string `json:"content_type,omitempty"`
}
type messageOutput struct {
	ID          string    `json:"id"`
	SenderType  string    `json:"sender_type"`
	SenderID    string    `json:"sender_id"`
	Content     string    `json:"content"`
	ContentType string    `json:"content_type"`
	CreatedAt   time.Time `json:"created_at"`
}
type contextOutput struct {
	Project      *projectOutput     `json:"project,omitempty"`
	Task         taskOutput         `json:"task"`
	Conversation conversationOutput `json:"conversation"`
}
type brainRequestInput struct {
	ProjectID string `json:"project_id"`
	Objective string `json:"objective"`
	MaxTokens int    `json:"max_tokens,omitempty"`
}
type brainOutput struct {
	RunID   string `json:"run_id"`
	Content string `json:"content"`
}
type runInput struct {
	RunID string `json:"run_id"`
}
type shellExecuteInput struct {
	ProjectID        string   `json:"project_id"`
	Executable       string   `json:"executable"`
	Arguments        []string `json:"arguments,omitempty"`
	WorkingDirectory string   `json:"working_directory,omitempty"`
}

type gitReadMCPInput struct {
	ProjectID        string `json:"project_id"`
	TaskID           string `json:"task_id,omitempty"`
	RunID            string `json:"run_id,omitempty"`
	Query            string `json:"query"`
	Reference        string `json:"reference,omitempty"`
	WorkingDirectory string `json:"working_directory,omitempty"`
}

type gitDiffMCPInput struct {
	ProjectID        string `json:"project_id"`
	TaskID           string `json:"task_id,omitempty"`
	RunID            string `json:"run_id,omitempty"`
	Reference        string `json:"reference,omitempty"`
	Staged           bool   `json:"staged,omitempty"`
	WorkingDirectory string `json:"working_directory,omitempty"`
}

type gitBranchMCPInput struct {
	ProjectID        string `json:"project_id"`
	TaskID           string `json:"task_id,omitempty"`
	RunID            string `json:"run_id,omitempty"`
	WorkingDirectory string `json:"working_directory,omitempty"`
}

type gitCommitMCPInput struct {
	ProjectID        string `json:"project_id"`
	TaskID           string `json:"task_id,omitempty"`
	RunID            string `json:"run_id,omitempty"`
	Message          string `json:"message"`
	WorkingDirectory string `json:"working_directory,omitempty"`
}

type gitPushMCPInput struct {
	ProjectID        string `json:"project_id"`
	TaskID           string `json:"task_id,omitempty"`
	RunID            string `json:"run_id,omitempty"`
	Remote           string `json:"remote,omitempty"`
	Branch           string `json:"branch,omitempty"`
	WorkingDirectory string `json:"working_directory,omitempty"`
}

type gitMergeMCPInput struct {
	ProjectID        string `json:"project_id"`
	TaskID           string `json:"task_id,omitempty"`
	RunID            string `json:"run_id,omitempty"`
	Reference        string `json:"reference"`
	WorkingDirectory string `json:"working_directory,omitempty"`
}

type githubIssueReadMCPInput struct {
	ProjectID string `json:"project_id"`
	TaskID    string `json:"task_id,omitempty"`
	RunID     string `json:"run_id,omitempty"`
	Action    string `json:"action"`
	Number    int    `json:"number,omitempty"`
	State     string `json:"state,omitempty"`
}

type githubIssueCreateMCPInput struct {
	ProjectID string `json:"project_id"`
	TaskID    string `json:"task_id,omitempty"`
	RunID     string `json:"run_id,omitempty"`
	Title     string `json:"title"`
	Body      string `json:"body,omitempty"`
}

type githubIssueUpdateMCPInput struct {
	ProjectID string `json:"project_id"`
	TaskID    string `json:"task_id,omitempty"`
	RunID     string `json:"run_id,omitempty"`
	Number    int    `json:"number"`
	Body      string `json:"body,omitempty"`
	State     string `json:"state,omitempty"`
}

type githubPRReadMCPInput struct {
	ProjectID string `json:"project_id"`
	TaskID    string `json:"task_id,omitempty"`
	RunID     string `json:"run_id,omitempty"`
	Action    string `json:"action"`
	Number    int    `json:"number,omitempty"`
	State     string `json:"state,omitempty"`
}

type githubPRCreateMCPInput struct {
	ProjectID string `json:"project_id"`
	TaskID    string `json:"task_id,omitempty"`
	RunID     string `json:"run_id,omitempty"`
	Title     string `json:"title"`
	Head      string `json:"head"`
	Base      string `json:"base"`
	Body      string `json:"body,omitempty"`
}

type githubPRReviewMCPInput struct {
	ProjectID string `json:"project_id"`
	TaskID    string `json:"task_id,omitempty"`
	RunID     string `json:"run_id,omitempty"`
	Number    int    `json:"number"`
	Event     string `json:"event"`
	Body      string `json:"body,omitempty"`
}

type githubPRMergeMCPInput struct {
	ProjectID       string `json:"project_id"`
	TaskID          string `json:"task_id,omitempty"`
	RunID           string `json:"run_id,omitempty"`
	Number          int    `json:"number"`
	ExpectedHeadSHA string `json:"expected_head_sha"`
	MergeMethod     string `json:"merge_method,omitempty"`
}

type extensionExecuteMCPInput struct {
	ProjectID string         `json:"project_id"`
	ToolName  string         `json:"tool_name"`
	TaskID    string         `json:"task_id,omitempty"`
	RunID     string         `json:"run_id,omitempty"`
	Input     map[string]any `json:"input"`
}

type runOutput struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	TaskID    string `json:"task_id,omitempty"`
	AgentID   string `json:"agent_id"`
	Status    string `json:"status"`
	Output    string `json:"output,omitempty"`
	Error     string `json:"error,omitempty"`
}

func identity(ctx context.Context) string {
	info := auth.TokenInfoFromContext(ctx)
	if info == nil {
		return ""
	}
	return info.UserID
}
func toolError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, task.ErrConflict) {
		return errors.New("task changed concurrently; reload and retry")
	}
	return errors.New("operation denied or unavailable")
}
func projectInScope(a agent.Agent, projectID string) bool {
	for _, id := range a.ProjectIDs() {
		if id == projectID {
			return true
		}
	}
	return false
}

// hasProjectScope combines persisted project membership with the active Brain
// relationship. The Brain relationship grants scope only to its own project.
func (s *Server) hasProjectScope(ctx context.Context, a agent.Agent, projectID string) bool {
	if projectInScope(a, projectID) {
		return true
	}
	b, err := s.services.Brains.Get(ctx, projectID)
	return err == nil && b.Active() && b.AgentID() == a.ID() && b.ProjectID() == projectID
}

func (s *Server) isActiveBrain(ctx context.Context, a agent.Agent, projectID string) bool {
	b, err := s.services.Brains.Get(ctx, projectID)
	return err == nil && b.Active() && b.AgentID() == a.ID() && b.ProjectID() == projectID
}

func (s *Server) taskActor(ctx context.Context, a agent.Agent, projectID string) task.Actor {
	actorType := task.ActorAgent
	if s.isActiveBrain(ctx, a, projectID) {
		actorType = task.ActorBrain
	}
	return task.Actor{Type: actorType, ID: a.ID()}
}
func agentView(a agent.Agent, observed agent.Observed, hasObservation bool) agentOutput {
	caps := make([]string, 0, len(a.Capabilities()))
	for _, c := range a.Capabilities() {
		caps = append(caps, string(c))
	}
	status := a.Status()
	if hasObservation {
		status = observed.Status
	}
	return agentOutput{ID: a.ID(), Name: a.Name(), Role: a.Role(), Status: string(status), Runtime: string(a.Runtime()),
		Capabilities: caps, LastSeenAt: observed.LastSeenAt, SessionOpen: observed.SessionOpen}
}
func taskView(t task.Task) taskOutput {
	return taskOutput{ID: t.ID(), ProjectID: t.ProjectID(), Title: t.Title(), Description: t.Description(), Status: string(t.Status()), Priority: t.Priority(), AssignedTo: t.AssignedTo(), HumanReview: t.HumanReview(), Version: t.Version()}
}
func messageView(m conversation.Message) messageOutput {
	return messageOutput{ID: m.ID(), SenderType: string(m.SenderType()), SenderID: m.SenderID(), Content: m.Content(), ContentType: m.ContentType(), CreatedAt: m.CreatedAt()}
}
func (s *Server) actor(ctx context.Context) (agent.Agent, error) {
	id := identity(ctx)
	if id == "" {
		return agent.Agent{}, errors.New("unauthenticated")
	}
	return s.services.Agents.Get(ctx, id)
}
func (s *Server) recordDenial(ctx context.Context, a agent.Agent, projectID, required, reason string) {
	if s.services.Denials != nil {
		if err := s.services.Denials.RecordAuthorizationDenial(ctx, a.ID(), projectID, required, "mcp:"+required, reason); err != nil {
			slog.Error("record MCP authorization denial", "error", err)
		}
	}
}
func (s *Server) authorize(ctx context.Context, a agent.Agent, required, projectID string) error {
	grant, err := capability.New(required)
	if err != nil {
		return err
	}
	err = (capability.Authorizer{}).Authorize(capability.Principal{ID: a.ID(), Capabilities: a.Capabilities(), ProjectIDs: a.ProjectIDs()}, grant, projectID)
	if err != nil {
		s.recordDenial(ctx, a, projectID, required, "capability_or_project_scope_denied")
	}
	return err
}
func (s *Server) authorizeProject(ctx context.Context, a agent.Agent, required, projectID string) error {
	if !s.hasProjectScope(ctx, a, projectID) {
		s.recordDenial(ctx, a, projectID, required, "project_scope_required")
		return capability.ErrDenied
	}
	grant, err := capability.New(required)
	if err != nil {
		return err
	}
	err = (capability.Authorizer{}).Authorize(capability.Principal{ID: a.ID(), Capabilities: a.Capabilities(), ProjectIDs: []string{projectID}}, grant, projectID)
	if err != nil {
		s.recordDenial(ctx, a, projectID, required, "capability_or_project_scope_denied")
	}
	return err
}
func (s *Server) authorizeAssignedTask(ctx context.Context, a agent.Agent, required string, t task.Task) error {
	grant, err := capability.New(required)
	if err != nil {
		return err
	}
	err = (capability.Authorizer{}).Authorize(capability.Principal{ID: a.ID(), Capabilities: a.Capabilities(), ProjectIDs: []string{t.ProjectID()}}, grant, t.ProjectID())
	if err != nil {
		s.recordDenial(ctx, a, t.ProjectID(), required, "assigned_task_capability_denied")
	}
	return err
}
func (s *Server) assignedTask(ctx context.Context, a agent.Agent, id, required string) (task.Task, error) {
	t, err := s.services.Tasks.Get(ctx, id)
	if err != nil {
		return task.Task{}, err
	}
	if t.AssignedTo() != a.ID() {
		s.recordDenial(ctx, a, t.ProjectID(), required, "task_assignment_required")
		return task.Task{}, task.ErrForbidden
	}
	if err := s.authorizeAssignedTask(ctx, a, required, t); err != nil {
		return task.Task{}, task.ErrForbidden
	}
	return t, nil
}
func (s *Server) taskVisible(ctx context.Context, a agent.Agent, t task.Task, required string) error {
	if t.AssignedTo() == a.ID() {
		return s.authorizeAssignedTask(ctx, a, required, t)
	}
	if s.hasProjectScope(ctx, a, t.ProjectID()) {
		return s.authorizeProject(ctx, a, required, t.ProjectID())
	}
	s.recordDenial(ctx, a, t.ProjectID(), required, "project_scope_or_task_assignment_required")
	return task.ErrForbidden
}
func (s *Server) projectVisible(ctx context.Context, a agent.Agent, id string) (project.Project, error) {
	p, err := s.services.Projects.Get(ctx, id)
	if err != nil {
		return project.Project{}, err
	}
	if s.hasProjectScope(ctx, a, id) {
		if err := s.authorizeProject(ctx, a, "project.read", id); err != nil {
			return project.Project{}, task.ErrForbidden
		}
		return p, nil
	}
	s.recordDenial(ctx, a, id, "project.read", "project_membership_required")
	return project.Project{}, task.ErrForbidden
}
func (s *Server) projectGet(ctx context.Context, _ *mcp.CallToolRequest, in projectInput) (*mcp.CallToolResult, projectOutput, error) {
	a, e := s.actor(ctx)
	if e != nil {
		return nil, projectOutput{}, toolError(e)
	}
	p, e := s.projectVisible(ctx, a, strings.TrimSpace(in.ProjectID))
	if e != nil {
		return nil, projectOutput{}, toolError(e)
	}
	return nil, projectOutput{ID: p.ID(), Name: p.Name(), Description: p.Description(), Status: string(p.Status())}, nil
}
func (s *Server) agentList(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, []agentOutput, error) {
	a, e := s.actor(ctx)
	if e != nil {
		return nil, nil, toolError(e)
	}
	if s.authorize(ctx, a, "agent.read", "") != nil {
		return nil, nil, toolError(task.ErrForbidden)
	}
	observed, hasObservation := s.observe(ctx, a.ID())
	return nil, []agentOutput{agentView(a, observed, hasObservation)}, nil
}
func (s *Server) agentGet(ctx context.Context, _ *mcp.CallToolRequest, in agentInput) (*mcp.CallToolResult, agentOutput, error) {
	a, e := s.actor(ctx)
	if e != nil {
		return nil, agentOutput{}, toolError(e)
	}
	if in.AgentID != "" && in.AgentID != a.ID() || s.authorize(ctx, a, "agent.read", "") != nil {
		return nil, agentOutput{}, toolError(task.ErrForbidden)
	}
	observed, hasObservation := s.observe(ctx, a.ID())
	return nil, agentView(a, observed, hasObservation), nil
}
func (s *Server) agentHeartbeat(ctx context.Context, _ *mcp.CallToolRequest, in heartbeatInput) (*mcp.CallToolResult, agentOutput, error) {
	a, err := s.actor(ctx)
	if err != nil {
		return nil, agentOutput{}, toolError(err)
	}
	if in.AgentID != "" && in.AgentID != a.ID() {
		s.recordDenial(ctx, a, "", "agent.status", "heartbeat_identity_mismatch")
		return nil, agentOutput{}, toolError(task.ErrForbidden)
	}
	if s.authorize(ctx, a, "agent.read", "") != nil {
		return nil, agentOutput{}, toolError(task.ErrForbidden)
	}
	if s.services.Heartbeats == nil {
		return nil, agentView(a, agent.Observed{}, false), nil
	}
	observed, err := s.services.Heartbeats.Heartbeat(ctx, heartbeat.Identity{AgentID: a.ID(), TokenID: credentialTokenID(ctx)})
	if err != nil {
		slog.Error("record agent heartbeat", "error", err, "agent_id", a.ID())
		return nil, agentOutput{}, toolError(err)
	}
	return nil, agentView(a, observed, true), nil
}
func (s *Server) observe(ctx context.Context, agentID string) (agent.Observed, bool) {
	if s.services.Heartbeats == nil {
		return agent.Observed{}, false
	}
	observed, err := s.services.Heartbeats.Statuses(ctx, []string{agentID})
	if err != nil {
		slog.Error("derive agent status", "error", err, "agent_id", agentID)
		return agent.Observed{}, false
	}
	value, ok := observed[agentID]
	return value, ok
}
func credentialTokenID(ctx context.Context) string {
	info := auth.TokenInfoFromContext(ctx)
	if info == nil {
		return ""
	}
	value, _ := info.Extra["token_id"].(string)
	return value
}
func (s *Server) taskList(ctx context.Context, _ *mcp.CallToolRequest, in taskListInput) (*mcp.CallToolResult, []taskOutput, error) {
	a, e := s.actor(ctx)
	if e != nil {
		return nil, nil, toolError(e)
	}
	if s.authorize(ctx, a, "task.read", "") != nil {
		return nil, nil, toolError(task.ErrForbidden)
	}
	limit := in.Limit
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > maxToolPageSize || in.Offset < 0 || in.Offset > 10000 {
		return nil, nil, toolError(task.ErrInvalidInput)
	}
	status := task.Status(in.Status)
	var projects []project.Project
	if in.ProjectID != "" {
		p, err := s.services.Projects.Get(ctx, strings.TrimSpace(in.ProjectID))
		if err != nil {
			return nil, nil, toolError(err)
		}
		projects = []project.Project{p}
	} else {
		projects, e = s.services.Projects.List(ctx, 100, 0)
		if e != nil {
			return nil, nil, toolError(e)
		}
	}
	result := make([]taskOutput, 0)
	skip := in.Offset
	for _, p := range projects {
		assignedTo := a.ID()
		if s.hasProjectScope(ctx, a, p.ID()) {
			if err := s.authorizeProject(ctx, a, "task.read", p.ID()); err == nil {
				assignedTo = ""
			}
		}
		projectOffset := 0
		for skip > 0 {
			pageSize := min(skip, maxToolPageSize)
			values, err := s.services.Tasks.List(ctx, p.ID(), status, assignedTo, pageSize, projectOffset)
			if err != nil || len(values) == 0 {
				break
			}
			projectOffset += len(values)
			skip -= len(values)
			if len(values) < pageSize {
				break
			}
		}
		if skip > 0 {
			continue
		}
		values, err := s.services.Tasks.List(ctx, p.ID(), status, assignedTo, min(limit-len(result), maxToolPageSize), projectOffset)
		if err != nil {
			continue
		}
		for _, v := range values {
			result = append(result, taskView(v))
			if len(result) >= limit {
				return nil, result, nil
			}
		}
	}
	return nil, result, nil
}
func (s *Server) taskGet(ctx context.Context, _ *mcp.CallToolRequest, in taskInput) (*mcp.CallToolResult, taskOutput, error) {
	a, e := s.actor(ctx)
	if e != nil {
		return nil, taskOutput{}, toolError(e)
	}
	t, e := s.services.Tasks.Get(ctx, in.TaskID)
	if e != nil {
		return nil, taskOutput{}, toolError(e)
	}
	if e = s.taskVisible(ctx, a, t, "task.read"); e != nil {
		return nil, taskOutput{}, toolError(e)
	}
	return nil, taskView(t), nil
}
func (s *Server) taskClaim(ctx context.Context, _ *mcp.CallToolRequest, in taskActionInput) (*mcp.CallToolResult, taskOutput, error) {
	a, err := s.actor(ctx)
	if err != nil {
		return nil, taskOutput{}, toolError(err)
	}
	value, err := s.services.Tasks.Get(ctx, in.TaskID)
	if err != nil {
		return nil, taskOutput{}, toolError(err)
	}
	if s.isActiveBrain(ctx, a, value.ProjectID()) {
		s.recordDenial(ctx, a, value.ProjectID(), "task.claim", "active_brain_must_delegate")
		return nil, taskOutput{}, toolError(task.ErrForbidden)
	}
	if value.Status() != task.StatusReady {
		return nil, taskOutput{}, toolError(task.ErrForbidden)
	}
	if value.AssignedTo() == "" {
		if !projectInScope(a, value.ProjectID()) || s.authorizeProject(ctx, a, "task.assign", value.ProjectID()) != nil || s.authorizeProject(ctx, a, "task.transition", value.ProjectID()) != nil {
			return nil, taskOutput{}, toolError(task.ErrForbidden)
		}
	} else if value.AssignedTo() != a.ID() {
		return nil, taskOutput{}, toolError(task.ErrForbidden)
	} else if s.authorizeAssignedTask(ctx, a, "task.transition", value) != nil {
		return nil, taskOutput{}, toolError(task.ErrForbidden)
	}
	value, err = s.services.Tasks.Claim(ctx, value.ID(), a.ID(), value.Version(), in.Reason, task.Actor{Type: task.ActorAgent, ID: a.ID()})
	if err != nil {
		return nil, taskOutput{}, toolError(err)
	}
	return nil, taskView(value), nil
}
func (s *Server) taskAssign(ctx context.Context, _ *mcp.CallToolRequest, in taskAssignInput) (*mcp.CallToolResult, taskOutput, error) {
	a, err := s.actor(ctx)
	if err != nil {
		return nil, taskOutput{}, toolError(err)
	}
	value, err := s.services.Tasks.Get(ctx, in.TaskID)
	if err != nil {
		return nil, taskOutput{}, toolError(err)
	}
	projectID := value.ProjectID()
	if !s.hasProjectScope(ctx, a, projectID) || s.authorizeProject(ctx, a, "task.assign", projectID) != nil {
		return nil, taskOutput{}, toolError(task.ErrForbidden)
	}
	assignee, err := s.services.Agents.Get(ctx, strings.TrimSpace(in.AgentID))
	if err != nil || assignee.ID() != a.ID() && !s.hasProjectScope(ctx, assignee, projectID) {
		s.recordDenial(ctx, a, projectID, "task.assign", "assignee_must_have_project_scope")
		return nil, taskOutput{}, toolError(task.ErrForbidden)
	}
	updated, err := s.services.Tasks.AssignAtVersion(ctx, value.ID(), assignee.ID(), in.ExpectedVersion, in.Reason, s.taskActor(ctx, a, projectID))
	if err != nil {
		return nil, taskOutput{}, toolError(err)
	}
	return nil, taskView(updated), nil
}
func (s *Server) taskUpdate(ctx context.Context, _ *mcp.CallToolRequest, in taskUpdateInput) (*mcp.CallToolResult, taskOutput, error) {
	to := task.Status(in.Status)
	if to != task.StatusWorking && to != task.StatusReview && to != task.StatusHumanReview && to != task.StatusBlocked {
		return nil, taskOutput{}, toolError(task.ErrForbidden)
	}
	return s.transition(ctx, in.TaskID, to, in.Reason, "task.transition")
}
func (s *Server) taskComplete(ctx context.Context, _ *mcp.CallToolRequest, in taskActionInput) (*mcp.CallToolResult, taskOutput, error) {
	return s.transition(ctx, in.TaskID, task.StatusDone, in.Reason, "task.complete")
}
func (s *Server) taskFail(ctx context.Context, _ *mcp.CallToolRequest, in taskActionInput) (*mcp.CallToolResult, taskOutput, error) {
	return s.transition(ctx, in.TaskID, task.StatusFailed, in.Reason, "task.transition")
}
func (s *Server) transition(ctx context.Context, id string, to task.Status, reason, required string) (*mcp.CallToolResult, taskOutput, error) {
	a, e := s.actor(ctx)
	if e != nil {
		return nil, taskOutput{}, toolError(e)
	}
	value, e := s.services.Tasks.Get(ctx, id)
	if e != nil {
		return nil, taskOutput{}, toolError(e)
	}
	actor := task.Actor{Type: task.ActorAgent, ID: a.ID()}
	if s.isActiveBrain(ctx, a, value.ProjectID()) {
		brainCapability := required
		switch to {
		case task.StatusWorking:
			brainCapability = "task.assign"
		case task.StatusReview, task.StatusHumanReview, task.StatusBlocked, task.StatusFailed, task.StatusCancelled:
			brainCapability = "task.review"
		case task.StatusDone:
			brainCapability = "task.complete"
		}
		if !s.hasProjectScope(ctx, a, value.ProjectID()) || s.authorizeProject(ctx, a, brainCapability, value.ProjectID()) != nil {
			return nil, taskOutput{}, toolError(task.ErrForbidden)
		}
		actor = task.Actor{Type: task.ActorBrain, ID: a.ID()}
	} else if _, e = s.assignedTask(ctx, a, id, required); e != nil {
		return nil, taskOutput{}, toolError(e)
	}
	v, e := s.services.Tasks.Transition(ctx, id, to, reason, actor)
	if e != nil {
		return nil, taskOutput{}, toolError(e)
	}
	return nil, taskView(v), nil
}
func (s *Server) conversationRead(ctx context.Context, _ *mcp.CallToolRequest, in conversationReadInput) (*mcp.CallToolResult, conversationOutput, error) {
	a, e := s.actor(ctx)
	if e != nil {
		return nil, conversationOutput{}, toolError(e)
	}
	t, e := s.services.Tasks.Get(ctx, in.TaskID)
	if e != nil {
		return nil, conversationOutput{}, toolError(e)
	}
	var value conversation.Conversation
	var msgs []conversation.Message
	var next string
	if s.isActiveBrain(ctx, a, t.ProjectID()) {
		if s.authorizeProject(ctx, a, "task.read", t.ProjectID()) != nil || s.authorizeProject(ctx, a, "conversation.read", t.ProjectID()) != nil {
			return nil, conversationOutput{}, toolError(task.ErrForbidden)
		}
		value, msgs, next, e = s.services.Conversations.GetPageForBrain(ctx, t.ID(), a.ID(), in.Before, in.Limit)
	} else {
		if _, e = s.assignedTask(ctx, a, in.TaskID, "conversation.read"); e != nil {
			return nil, conversationOutput{}, toolError(e)
		}
		if s.authorizeAssignedTask(ctx, a, "task.read", t) != nil {
			return nil, conversationOutput{}, toolError(task.ErrForbidden)
		}
		value, msgs, next, e = s.services.Conversations.GetPageForAgent(ctx, t.ID(), a.ID(), in.Before, in.Limit)
	}
	if e != nil {
		return nil, conversationOutput{}, toolError(e)
	}
	out := conversationOutput{ConversationID: value.ID(), TaskID: t.ID(), Messages: make([]messageOutput, 0, len(msgs)), NextBefore: next}
	for _, m := range msgs {
		out.Messages = append(out.Messages, messageView(m))
	}
	return nil, out, nil
}
func (s *Server) conversationSend(ctx context.Context, _ *mcp.CallToolRequest, in conversationSendInput) (*mcp.CallToolResult, messageOutput, error) {
	a, e := s.actor(ctx)
	if e != nil {
		return nil, messageOutput{}, toolError(e)
	}
	t, e := s.services.Tasks.Get(ctx, in.TaskID)
	if e != nil {
		return nil, messageOutput{}, toolError(e)
	}
	senderType := conversation.SenderAgent
	if s.isActiveBrain(ctx, a, t.ProjectID()) {
		if s.authorizeProject(ctx, a, "task.read", t.ProjectID()) != nil || s.authorizeProject(ctx, a, "conversation.write", t.ProjectID()) != nil {
			return nil, messageOutput{}, toolError(task.ErrForbidden)
		}
		senderType = conversation.SenderBrain
	} else if _, e = s.assignedTask(ctx, a, in.TaskID, "conversation.write"); e != nil {
		return nil, messageOutput{}, toolError(e)
	}
	contentType := strings.TrimSpace(in.ContentType)
	if contentType == "" {
		contentType = "text/plain"
	}
	m, e := s.services.Conversations.Append(ctx, in.TaskID, senderType, a.ID(), conversation.MessageInput{Content: in.Content, ContentType: contentType})
	if e != nil {
		return nil, messageOutput{}, toolError(e)
	}
	return nil, messageView(m), nil
}
func (s *Server) contextGet(ctx context.Context, r *mcp.CallToolRequest, in taskInput) (*mcp.CallToolResult, contextOutput, error) {
	a, e := s.actor(ctx)
	if e != nil {
		return nil, contextOutput{}, toolError(e)
	}
	t, e := s.services.Tasks.Get(ctx, in.TaskID)
	if e != nil {
		return nil, contextOutput{}, toolError(task.ErrForbidden)
	}
	if s.isActiveBrain(ctx, a, t.ProjectID()) {
		if s.authorizeProject(ctx, a, "task.read", t.ProjectID()) != nil || s.authorizeProject(ctx, a, "conversation.read", t.ProjectID()) != nil {
			return nil, contextOutput{}, toolError(task.ErrForbidden)
		}
	} else {
		if _, e = s.assignedTask(ctx, a, in.TaskID, "task.read"); e != nil || s.authorizeAssignedTask(ctx, a, "conversation.read", t) != nil {
			return nil, contextOutput{}, toolError(task.ErrForbidden)
		}
	}
	var projectContext *projectOutput
	if s.hasProjectScope(ctx, a, t.ProjectID()) && s.authorizeProject(ctx, a, "project.read", t.ProjectID()) == nil {
		p, err := s.services.Projects.Get(ctx, t.ProjectID())
		if err != nil {
			return nil, contextOutput{}, toolError(err)
		}
		projectContext = &projectOutput{ID: p.ID(), Name: p.Name(), Description: p.Description(), Status: string(p.Status())}
	}
	var value conversation.Conversation
	var msgs []conversation.Message
	var next string
	if s.isActiveBrain(ctx, a, t.ProjectID()) {
		value, msgs, next, e = s.services.Conversations.GetPageForBrain(ctx, t.ID(), a.ID(), "", 20)
	} else {
		value, msgs, next, e = s.services.Conversations.GetPageForAgent(ctx, t.ID(), a.ID(), "", 20)
	}
	if e != nil {
		return nil, contextOutput{}, toolError(e)
	}
	conv := conversationOutput{ConversationID: value.ID(), TaskID: t.ID(), Messages: make([]messageOutput, 0, len(msgs)), NextBefore: next}
	for _, m := range msgs {
		conv.Messages = append(conv.Messages, messageView(m))
	}
	return nil, contextOutput{Project: projectContext, Task: taskView(t), Conversation: conv}, nil
}
func (s *Server) brainRequest(ctx context.Context, _ *mcp.CallToolRequest, in brainRequestInput) (*mcp.CallToolResult, brainOutput, error) {
	a, e := s.actor(ctx)
	if e != nil {
		return nil, brainOutput{}, toolError(e)
	}
	b, e := s.services.Brains.Get(ctx, in.ProjectID)
	if e != nil || !b.Active() || b.AgentID() != a.ID() || b.ProjectID() != in.ProjectID {
		s.recordDenial(ctx, a, in.ProjectID, "brain.execute", "active_brain_identity_required")
		return nil, brainOutput{}, toolError(task.ErrForbidden)
	}
	if e := s.authorizeProject(ctx, a, "brain.execute", b.ProjectID()); e != nil {
		s.recordDenial(ctx, a, b.ProjectID(), "brain.execute", "active_brain_scope_denied")
		return nil, brainOutput{}, toolError(task.ErrForbidden)
	}
	out, e := s.services.Brains.Execute(ctx, in.ProjectID, in.Objective, in.MaxTokens)
	if e != nil {
		return nil, brainOutput{}, toolError(e)
	}
	return nil, brainOutput{RunID: out.Run.ID(), Content: out.Content}, nil
}
func (s *Server) runGet(ctx context.Context, _ *mcp.CallToolRequest, in runInput) (*mcp.CallToolResult, runOutput, error) {
	a, e := s.actor(ctx)
	if e != nil {
		return nil, runOutput{}, toolError(e)
	}
	if s.authorize(ctx, a, "run.read", "") != nil {
		return nil, runOutput{}, toolError(task.ErrForbidden)
	}
	v, e := s.services.Runs.Get(ctx, in.RunID)
	if e != nil {
		return nil, runOutput{}, toolError(e)
	}
	if !s.hasProjectScope(ctx, a, v.ProjectID()) {
		s.recordDenial(ctx, a, v.ProjectID(), "run.read", "project_membership_required")
		return nil, runOutput{}, toolError(task.ErrForbidden)
	}
	if s.authorizeProject(ctx, a, "run.read", v.ProjectID()) != nil {
		return nil, runOutput{}, toolError(task.ErrForbidden)
	}
	return nil, runOutput{ID: v.ID(), ProjectID: v.ProjectID(), TaskID: v.TaskID(), AgentID: v.AgentID(), Status: string(v.Status()), Output: v.Output(), Error: v.Error()}, nil
}

func (s *Server) shellExecute(ctx context.Context, _ *mcp.CallToolRequest, input shellExecuteInput) (*mcp.CallToolResult, governedtool.ShellOutput, error) {
	a, err := s.actor(ctx)
	if err != nil {
		return nil, governedtool.ShellOutput{}, toolError(err)
	}
	projectID := strings.TrimSpace(input.ProjectID)
	if projectID == "" || projectID != input.ProjectID || s.services.Tools == nil {
		s.recordDenial(ctx, a, projectID, string(capability.ShellExecute), "shell_tool_unavailable_or_invalid_scope")
		return nil, governedtool.ShellOutput{}, toolError(task.ErrForbidden)
	}
	if err := s.authorizeProject(ctx, a, string(capability.ShellExecute), projectID); err != nil {
		return nil, governedtool.ShellOutput{}, toolError(task.ErrForbidden)
	}
	encodedInput, err := json.Marshal(governedtool.ShellInput{
		Executable: input.Executable, Arguments: input.Arguments, WorkingDirectory: input.WorkingDirectory,
	})
	if err != nil {
		return nil, governedtool.ShellOutput{}, toolError(governedtool.ErrInvalidInput)
	}
	encodedOutput, err := s.services.Tools.Execute(ctx, governedtool.Request{
		ToolName: "shell.execute", ProjectID: projectID, ResourceType: "workspace", ResourceID: projectID,
		Principal: governedtool.Principal{
			ID: a.ID(), Type: "agent", Capabilities: a.Capabilities(), ProjectIDs: []string{projectID},
		},
		Input: encodedInput,
	})
	if err != nil {
		return nil, governedtool.ShellOutput{}, toolError(err)
	}
	var output governedtool.ShellOutput
	if err := json.Unmarshal(encodedOutput, &output); err != nil {
		return nil, governedtool.ShellOutput{}, toolError(governedtool.ErrExecution)
	}
	return nil, output, nil
}

// gitExecute authorizes one Git operation against its capability and project
// scope, resolves the project workspace as the repository boundary, and
// dispatches to the governed Git tool set.
func (s *Server) gitExecute(ctx context.Context, a agent.Agent, projectID, taskID, runID, toolName, required string, encodedInput []byte) (json.RawMessage, error) {
	if projectID == "" || strings.TrimSpace(projectID) != projectID || s.services.GitTools == nil {
		s.recordDenial(ctx, a, strings.TrimSpace(projectID), required, "git_tool_unavailable_or_invalid_scope")
		return nil, toolError(task.ErrForbidden)
	}
	projectID = strings.TrimSpace(projectID)
	if err := s.authorizeProject(ctx, a, required, projectID); err != nil {
		return nil, toolError(task.ErrForbidden)
	}
	value, err := s.services.Projects.Get(ctx, projectID)
	if err != nil || strings.TrimSpace(value.WorkspacePath()) == "" {
		s.recordDenial(ctx, a, projectID, required, "repository_boundary_required")
		return nil, toolError(task.ErrForbidden)
	}
	encodedOutput, err := s.services.GitTools.Execute(ctx, governedtool.Request{
		ToolName: toolName, ProjectID: projectID, TaskID: taskID, RunID: runID,
		ResourceType: "repository", ResourceID: value.WorkspacePath(),
		Principal: governedtool.Principal{
			ID: a.ID(), Type: "agent", Capabilities: a.Capabilities(), ProjectIDs: []string{projectID},
		},
		Input: encodedInput,
	})
	if err != nil {
		return nil, toolError(err)
	}
	return encodedOutput, nil
}

func (s *Server) extensionExecute(ctx context.Context, _ *mcp.CallToolRequest, input extensionExecuteMCPInput) (*mcp.CallToolResult, map[string]any, error) {
	a, err := s.actor(ctx)
	if err != nil {
		return nil, nil, toolError(err)
	}
	if input.ProjectID == "" || strings.TrimSpace(input.ProjectID) != input.ProjectID ||
		input.ToolName == "" || strings.TrimSpace(input.ToolName) != input.ToolName ||
		s.services.ExtensionTools == nil {
		return nil, nil, toolError(governedtool.ErrInvalidInput)
	}
	encodedInput, err := json.Marshal(input.Input)
	if err != nil || len(encodedInput) == 0 || len(encodedInput) > 64<<10 {
		return nil, nil, toolError(governedtool.ErrInvalidInput)
	}
	projectIDs := []string(nil)
	if s.hasProjectScope(ctx, a, input.ProjectID) {
		projectIDs = []string{input.ProjectID}
	}
	output, err := s.services.ExtensionTools.Execute(ctx, governedtool.Request{
		ToolName: input.ToolName, ProjectID: input.ProjectID, TaskID: input.TaskID, RunID: input.RunID,
		ResourceType: "extension", ResourceID: input.ToolName,
		Principal: governedtool.Principal{ID: a.ID(), Type: "agent", Capabilities: a.Capabilities(), ProjectIDs: projectIDs},
		Input:     encodedInput,
	})
	if err != nil {
		return nil, nil, toolError(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(output, &decoded); err != nil || decoded == nil {
		return nil, nil, toolError(governedtool.ErrExecution)
	}
	return nil, decoded, nil
}

func (s *Server) gitRead(ctx context.Context, _ *mcp.CallToolRequest, input gitReadMCPInput) (*mcp.CallToolResult, governedtool.GitReadOutput, error) {
	a, err := s.actor(ctx)
	if err != nil {
		return nil, governedtool.GitReadOutput{}, toolError(err)
	}
	encodedInput, err := json.Marshal(governedtool.GitReadInput{
		Query: input.Query, Reference: input.Reference, WorkingDirectory: input.WorkingDirectory,
	})
	if err != nil {
		return nil, governedtool.GitReadOutput{}, toolError(governedtool.ErrInvalidInput)
	}
	encodedOutput, err := s.gitExecute(ctx, a, input.ProjectID, input.TaskID, input.RunID, "git.read", string(capability.GitRead), encodedInput)
	if err != nil {
		return nil, governedtool.GitReadOutput{}, err
	}
	var output governedtool.GitReadOutput
	if err := json.Unmarshal(encodedOutput, &output); err != nil {
		return nil, governedtool.GitReadOutput{}, toolError(governedtool.ErrExecution)
	}
	return nil, output, nil
}

func (s *Server) gitDiff(ctx context.Context, _ *mcp.CallToolRequest, input gitDiffMCPInput) (*mcp.CallToolResult, governedtool.GitDiffOutput, error) {
	a, err := s.actor(ctx)
	if err != nil {
		return nil, governedtool.GitDiffOutput{}, toolError(err)
	}
	encodedInput, err := json.Marshal(governedtool.GitDiffInput{
		Reference: input.Reference, Staged: input.Staged, WorkingDirectory: input.WorkingDirectory,
	})
	if err != nil {
		return nil, governedtool.GitDiffOutput{}, toolError(governedtool.ErrInvalidInput)
	}
	encodedOutput, err := s.gitExecute(ctx, a, input.ProjectID, input.TaskID, input.RunID, "git.diff", string(capability.GitDiff), encodedInput)
	if err != nil {
		return nil, governedtool.GitDiffOutput{}, err
	}
	var output governedtool.GitDiffOutput
	if err := json.Unmarshal(encodedOutput, &output); err != nil {
		return nil, governedtool.GitDiffOutput{}, toolError(governedtool.ErrExecution)
	}
	return nil, output, nil
}

func (s *Server) gitBranch(ctx context.Context, _ *mcp.CallToolRequest, input gitBranchMCPInput) (*mcp.CallToolResult, governedtool.GitBranchOutput, error) {
	a, err := s.actor(ctx)
	if err != nil {
		return nil, governedtool.GitBranchOutput{}, toolError(err)
	}
	encodedInput, err := json.Marshal(governedtool.GitBranchInput{WorkingDirectory: input.WorkingDirectory})
	if err != nil {
		return nil, governedtool.GitBranchOutput{}, toolError(governedtool.ErrInvalidInput)
	}
	encodedOutput, err := s.gitExecute(ctx, a, input.ProjectID, input.TaskID, input.RunID, "git.branch", string(capability.GitBranch), encodedInput)
	if err != nil {
		return nil, governedtool.GitBranchOutput{}, err
	}
	var output governedtool.GitBranchOutput
	if err := json.Unmarshal(encodedOutput, &output); err != nil {
		return nil, governedtool.GitBranchOutput{}, toolError(governedtool.ErrExecution)
	}
	return nil, output, nil
}

func (s *Server) gitCommit(ctx context.Context, _ *mcp.CallToolRequest, input gitCommitMCPInput) (*mcp.CallToolResult, governedtool.GitCommitOutput, error) {
	a, err := s.actor(ctx)
	if err != nil {
		return nil, governedtool.GitCommitOutput{}, toolError(err)
	}
	encodedInput, err := json.Marshal(governedtool.GitCommitInput{Message: input.Message, WorkingDirectory: input.WorkingDirectory})
	if err != nil {
		return nil, governedtool.GitCommitOutput{}, toolError(governedtool.ErrInvalidInput)
	}
	encodedOutput, err := s.gitExecute(ctx, a, input.ProjectID, input.TaskID, input.RunID, "git.commit", string(capability.GitCommit), encodedInput)
	if err != nil {
		return nil, governedtool.GitCommitOutput{}, err
	}
	var output governedtool.GitCommitOutput
	if err := json.Unmarshal(encodedOutput, &output); err != nil {
		return nil, governedtool.GitCommitOutput{}, toolError(governedtool.ErrExecution)
	}
	return nil, output, nil
}

func (s *Server) gitPush(ctx context.Context, _ *mcp.CallToolRequest, input gitPushMCPInput) (*mcp.CallToolResult, governedtool.GitPushOutput, error) {
	a, err := s.actor(ctx)
	if err != nil {
		return nil, governedtool.GitPushOutput{}, toolError(err)
	}
	encodedInput, err := json.Marshal(governedtool.GitPushInput{Remote: input.Remote, Branch: input.Branch, WorkingDirectory: input.WorkingDirectory})
	if err != nil {
		return nil, governedtool.GitPushOutput{}, toolError(governedtool.ErrInvalidInput)
	}
	encodedOutput, err := s.gitExecute(ctx, a, input.ProjectID, input.TaskID, input.RunID, "git.push", string(capability.GitPush), encodedInput)
	if err != nil {
		return nil, governedtool.GitPushOutput{}, err
	}
	var output governedtool.GitPushOutput
	if err := json.Unmarshal(encodedOutput, &output); err != nil {
		return nil, governedtool.GitPushOutput{}, toolError(governedtool.ErrExecution)
	}
	return nil, output, nil
}

func (s *Server) gitMerge(ctx context.Context, _ *mcp.CallToolRequest, input gitMergeMCPInput) (*mcp.CallToolResult, governedtool.GitMergeOutput, error) {
	a, err := s.actor(ctx)
	if err != nil {
		return nil, governedtool.GitMergeOutput{}, toolError(err)
	}
	encodedInput, err := json.Marshal(governedtool.GitMergeInput{Reference: input.Reference, WorkingDirectory: input.WorkingDirectory})
	if err != nil {
		return nil, governedtool.GitMergeOutput{}, toolError(governedtool.ErrInvalidInput)
	}
	encodedOutput, err := s.gitExecute(ctx, a, input.ProjectID, input.TaskID, input.RunID, "git.merge", string(capability.GitMerge), encodedInput)
	if err != nil {
		return nil, governedtool.GitMergeOutput{}, err
	}
	var output governedtool.GitMergeOutput
	if err := json.Unmarshal(encodedOutput, &output); err != nil {
		return nil, governedtool.GitMergeOutput{}, toolError(governedtool.ErrExecution)
	}
	return nil, output, nil
}

// githubExecute authorizes one GitHub operation against its capability and
// project scope, binds it to the project's approved repository, and dispatches
// to the governed GitHub tool set.
func (s *Server) githubExecute(ctx context.Context, a agent.Agent, projectID, taskID, runID, toolName, required string, encodedInput []byte) (json.RawMessage, error) {
	if projectID == "" || strings.TrimSpace(projectID) != projectID || s.services.GitHubTools == nil {
		s.recordDenial(ctx, a, strings.TrimSpace(projectID), required, "github_tool_unavailable_or_invalid_scope")
		return nil, toolError(task.ErrForbidden)
	}
	projectID = strings.TrimSpace(projectID)
	if err := s.authorizeProject(ctx, a, required, projectID); err != nil {
		return nil, toolError(task.ErrForbidden)
	}
	value, err := s.services.Projects.Get(ctx, projectID)
	if err != nil || strings.TrimSpace(value.RepositoryURL()) == "" {
		s.recordDenial(ctx, a, projectID, required, "github_repository_binding_required")
		return nil, toolError(task.ErrForbidden)
	}
	encodedOutput, err := s.services.GitHubTools.Execute(ctx, governedtool.Request{
		ToolName: toolName, ProjectID: projectID, TaskID: taskID, RunID: runID,
		ResourceType: "github.repository", ResourceID: value.RepositoryURL(),
		Principal: governedtool.Principal{
			ID: a.ID(), Type: "agent", Capabilities: a.Capabilities(), ProjectIDs: []string{projectID},
		},
		Input: encodedInput,
	})
	if err != nil {
		return nil, toolError(err)
	}
	return encodedOutput, nil
}

func (s *Server) githubIssueRead(ctx context.Context, _ *mcp.CallToolRequest, input githubIssueReadMCPInput) (*mcp.CallToolResult, governedtool.GitHubIssueReadOutput, error) {
	a, err := s.actor(ctx)
	if err != nil {
		return nil, governedtool.GitHubIssueReadOutput{}, toolError(err)
	}
	encodedInput, err := json.Marshal(governedtool.GitHubIssueReadInput{Action: input.Action, Number: input.Number, State: input.State})
	if err != nil {
		return nil, governedtool.GitHubIssueReadOutput{}, toolError(governedtool.ErrInvalidInput)
	}
	encodedOutput, err := s.githubExecute(ctx, a, input.ProjectID, input.TaskID, input.RunID, "github.issue.read", string(capability.GitHubIssueRead), encodedInput)
	if err != nil {
		return nil, governedtool.GitHubIssueReadOutput{}, err
	}
	var output governedtool.GitHubIssueReadOutput
	if err := json.Unmarshal(encodedOutput, &output); err != nil {
		return nil, governedtool.GitHubIssueReadOutput{}, toolError(governedtool.ErrExecution)
	}
	return nil, output, nil
}

func (s *Server) githubIssueCreate(ctx context.Context, _ *mcp.CallToolRequest, input githubIssueCreateMCPInput) (*mcp.CallToolResult, governedtool.GitHubIssueCreateOutput, error) {
	a, err := s.actor(ctx)
	if err != nil {
		return nil, governedtool.GitHubIssueCreateOutput{}, toolError(err)
	}
	encodedInput, err := json.Marshal(governedtool.GitHubIssueCreateInput{Title: input.Title, Body: input.Body})
	if err != nil {
		return nil, governedtool.GitHubIssueCreateOutput{}, toolError(governedtool.ErrInvalidInput)
	}
	encodedOutput, err := s.githubExecute(ctx, a, input.ProjectID, input.TaskID, input.RunID, "github.issue.create", string(capability.GitHubIssueCreate), encodedInput)
	if err != nil {
		return nil, governedtool.GitHubIssueCreateOutput{}, err
	}
	var output governedtool.GitHubIssueCreateOutput
	if err := json.Unmarshal(encodedOutput, &output); err != nil {
		return nil, governedtool.GitHubIssueCreateOutput{}, toolError(governedtool.ErrExecution)
	}
	return nil, output, nil
}

func (s *Server) githubIssueUpdate(ctx context.Context, _ *mcp.CallToolRequest, input githubIssueUpdateMCPInput) (*mcp.CallToolResult, governedtool.GitHubIssueUpdateOutput, error) {
	a, err := s.actor(ctx)
	if err != nil {
		return nil, governedtool.GitHubIssueUpdateOutput{}, toolError(err)
	}
	encodedInput, err := json.Marshal(governedtool.GitHubIssueUpdateInput{Number: input.Number, Body: input.Body, State: input.State})
	if err != nil {
		return nil, governedtool.GitHubIssueUpdateOutput{}, toolError(governedtool.ErrInvalidInput)
	}
	encodedOutput, err := s.githubExecute(ctx, a, input.ProjectID, input.TaskID, input.RunID, "github.issue.update", string(capability.GitHubIssueUpdate), encodedInput)
	if err != nil {
		return nil, governedtool.GitHubIssueUpdateOutput{}, err
	}
	var output governedtool.GitHubIssueUpdateOutput
	if err := json.Unmarshal(encodedOutput, &output); err != nil {
		return nil, governedtool.GitHubIssueUpdateOutput{}, toolError(governedtool.ErrExecution)
	}
	return nil, output, nil
}

func (s *Server) githubPRRead(ctx context.Context, _ *mcp.CallToolRequest, input githubPRReadMCPInput) (*mcp.CallToolResult, governedtool.GitHubPRReadOutput, error) {
	a, err := s.actor(ctx)
	if err != nil {
		return nil, governedtool.GitHubPRReadOutput{}, toolError(err)
	}
	encodedInput, err := json.Marshal(governedtool.GitHubPRReadInput{Action: input.Action, Number: input.Number, State: input.State})
	if err != nil {
		return nil, governedtool.GitHubPRReadOutput{}, toolError(governedtool.ErrInvalidInput)
	}
	encodedOutput, err := s.githubExecute(ctx, a, input.ProjectID, input.TaskID, input.RunID, "github.pr.read", string(capability.GitHubPRRead), encodedInput)
	if err != nil {
		return nil, governedtool.GitHubPRReadOutput{}, err
	}
	var output governedtool.GitHubPRReadOutput
	if err := json.Unmarshal(encodedOutput, &output); err != nil {
		return nil, governedtool.GitHubPRReadOutput{}, toolError(governedtool.ErrExecution)
	}
	return nil, output, nil
}

func (s *Server) githubPRCreate(ctx context.Context, _ *mcp.CallToolRequest, input githubPRCreateMCPInput) (*mcp.CallToolResult, governedtool.GitHubPRCreateOutput, error) {
	a, err := s.actor(ctx)
	if err != nil {
		return nil, governedtool.GitHubPRCreateOutput{}, toolError(err)
	}
	encodedInput, err := json.Marshal(governedtool.GitHubPRCreateInput{Title: input.Title, Head: input.Head, Base: input.Base, Body: input.Body})
	if err != nil {
		return nil, governedtool.GitHubPRCreateOutput{}, toolError(governedtool.ErrInvalidInput)
	}
	encodedOutput, err := s.githubExecute(ctx, a, input.ProjectID, input.TaskID, input.RunID, "github.pr.create", string(capability.GitHubPRCreate), encodedInput)
	if err != nil {
		return nil, governedtool.GitHubPRCreateOutput{}, err
	}
	var output governedtool.GitHubPRCreateOutput
	if err := json.Unmarshal(encodedOutput, &output); err != nil {
		return nil, governedtool.GitHubPRCreateOutput{}, toolError(governedtool.ErrExecution)
	}
	return nil, output, nil
}

func (s *Server) githubPRReview(ctx context.Context, _ *mcp.CallToolRequest, input githubPRReviewMCPInput) (*mcp.CallToolResult, governedtool.GitHubPRReviewOutput, error) {
	a, err := s.actor(ctx)
	if err != nil {
		return nil, governedtool.GitHubPRReviewOutput{}, toolError(err)
	}
	encodedInput, err := json.Marshal(governedtool.GitHubPRReviewInput{Number: input.Number, Event: input.Event, Body: input.Body})
	if err != nil {
		return nil, governedtool.GitHubPRReviewOutput{}, toolError(governedtool.ErrInvalidInput)
	}
	encodedOutput, err := s.githubExecute(ctx, a, input.ProjectID, input.TaskID, input.RunID, "github.pr.review", string(capability.GitHubPRReview), encodedInput)
	if err != nil {
		return nil, governedtool.GitHubPRReviewOutput{}, err
	}
	var output governedtool.GitHubPRReviewOutput
	if err := json.Unmarshal(encodedOutput, &output); err != nil {
		return nil, governedtool.GitHubPRReviewOutput{}, toolError(governedtool.ErrExecution)
	}
	return nil, output, nil
}

func (s *Server) githubPRMerge(ctx context.Context, _ *mcp.CallToolRequest, input githubPRMergeMCPInput) (*mcp.CallToolResult, governedtool.GitHubPRMergeOutput, error) {
	a, err := s.actor(ctx)
	if err != nil {
		return nil, governedtool.GitHubPRMergeOutput{}, toolError(err)
	}
	encodedInput, err := json.Marshal(governedtool.GitHubPRMergeInput{Number: input.Number, ExpectedHeadSHA: input.ExpectedHeadSHA, MergeMethod: input.MergeMethod})
	if err != nil {
		return nil, governedtool.GitHubPRMergeOutput{}, toolError(governedtool.ErrInvalidInput)
	}
	encodedOutput, err := s.githubExecute(ctx, a, input.ProjectID, input.TaskID, input.RunID, "github.pr.merge", string(capability.GitHubPRMerge), encodedInput)
	if err != nil {
		return nil, governedtool.GitHubPRMergeOutput{}, err
	}
	var output governedtool.GitHubPRMergeOutput
	if err := json.Unmarshal(encodedOutput, &output); err != nil {
		return nil, governedtool.GitHubPRMergeOutput{}, toolError(governedtool.ErrExecution)
	}
	return nil, output, nil
}
