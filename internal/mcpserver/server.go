// Package mcpserver exposes boAIrd services through authenticated MCP tools.
package mcpserver

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"boAIrd/internal/agent"
	"boAIrd/internal/brain"
	"boAIrd/internal/capability"
	"boAIrd/internal/config"
	"boAIrd/internal/conversation"
	"boAIrd/internal/project"
	"boAIrd/internal/run"
	"boAIrd/internal/task"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const maxToolPageSize = 100

type Services struct {
	Agents        AgentService
	Projects      ProjectService
	Tasks         TaskService
	Conversations ConversationService
	Brains        BrainService
	Runs          RunService
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
	Transition(context.Context, string, task.Status, string, task.Actor) (task.Task, error)
}
type ConversationService interface {
	GetPageForAgent(context.Context, string, string, string, int) (conversation.Conversation, []conversation.Message, string, error)
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
	tokens   []config.MCPAgentToken
}

func New(ctx context.Context, services Services, tokens []config.MCPAgentToken) (*Server, error) {
	if services.Agents == nil || services.Projects == nil || services.Tasks == nil || services.Conversations == nil || services.Brains == nil || services.Runs == nil {
		return nil, errors.New("MCP services are required")
	}
	if len(tokens) == 0 {
		return nil, errors.New("at least one MCP Agent token is required")
	}
	seenAgents := make(map[string]struct{}, len(tokens))
	seenTokens := make(map[[32]byte]struct{}, len(tokens))
	for _, token := range tokens {
		if strings.TrimSpace(token.AgentID) == "" || strings.TrimSpace(token.AgentID) != token.AgentID || token.Token == ([32]byte{}) {
			return nil, errors.New("MCP Agent token registration is invalid")
		}
		if _, exists := seenAgents[token.AgentID]; exists {
			return nil, errors.New("MCP Agent token registrations must be unique")
		}
		if _, exists := seenTokens[token.Token]; exists {
			return nil, errors.New("MCP Agent token registrations must be unique")
		}
		if _, err := services.Agents.Get(ctx, token.AgentID); err != nil {
			return nil, errors.New("MCP token references an unknown Agent")
		}
		seenAgents[token.AgentID] = struct{}{}
		seenTokens[token.Token] = struct{}{}
	}
	return &Server{services: services, tokens: append([]config.MCPAgentToken(nil), tokens...)}, nil
}

func (s *Server) Handler() http.Handler {
	server := mcp.NewServer(&mcp.Implementation{Name: "boAIrd", Version: "0.1.0"}, &mcp.ServerOptions{SupportedProtocolVersions: []string{"2025-11-25"}})
	s.registerTools(server)
	transport := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{SessionTimeout: 30 * time.Minute, MaxRequestBodyBytes: 1 << 20})
	return auth.RequireBearerToken(s.verifyToken, &auth.RequireBearerTokenOptions{AllowMissingExpiration: true})(transport)
}

func (s *Server) verifyToken(ctx context.Context, raw string, _ *http.Request) (*auth.TokenInfo, error) {
	decoded, err := hex.DecodeString(raw)
	if err != nil || len(decoded) != 32 {
		clear(decoded)
		return nil, auth.ErrInvalidToken
	}
	defer clear(decoded)
	matched := 0
	agentID := ""
	for _, configured := range s.tokens {
		equal := subtle.ConstantTimeCompare(decoded, configured.Token[:])
		matched |= equal
		if equal == 1 {
			agentID = configured.AgentID
		}
	}
	if matched != 1 {
		return nil, auth.ErrInvalidToken
	}
	if _, err := s.services.Agents.Get(ctx, agentID); err != nil {
		return nil, auth.ErrInvalidToken
	}
	return &auth.TokenInfo{UserID: agentID}, nil
}

func (s *Server) registerTools(server *mcp.Server) {
	add := func(name, description string) *mcp.Tool { return &mcp.Tool{Name: name, Description: description} }
	mcp.AddTool[projectInput, projectOutput](server, add("boaird.project.get", "Get a project visible to the authenticated Agent."), s.projectGet)
	mcp.AddTool[emptyInput, []agentOutput](server, add("boaird.agent.list", "List only the authenticated Agent."), s.agentList)
	mcp.AddTool[agentInput, agentOutput](server, add("boaird.agent.get", "Get the authenticated Agent; other Agent identities are not exposed."), s.agentGet)
	mcp.AddTool[agentInput, agentOutput](server, add("boaird.agent.status", "Get the authenticated Agent status."), s.agentGet)
	mcp.AddTool[taskListInput, []taskOutput](server, add("boaird.task.list", "List tasks in a project, or only tasks assigned to the authenticated Agent across projects."), s.taskList)
	mcp.AddTool[taskInput, taskOutput](server, add("boaird.task.get", "Get a task assigned to this Agent or in its explicit project scope."), s.taskGet)
	mcp.AddTool[taskActionInput, taskOutput](server, add("boaird.task.claim", "Claim a task already assigned to this Agent by moving READY to WORKING."), s.taskClaim)
	mcp.AddTool[taskUpdateInput, taskOutput](server, add("boaird.task.update", "Move an assigned task to WORKING, REVIEW, or BLOCKED with a reason."), s.taskUpdate)
	mcp.AddTool[taskActionInput, taskOutput](server, add("boaird.task.complete", "Complete an assigned task after review when task.complete and Human Review policy allow it."), s.taskComplete)
	mcp.AddTool[taskActionInput, taskOutput](server, add("boaird.task.fail", "Mark an assigned task failed with a reason."), s.taskFail)
	mcp.AddTool[conversationReadInput, conversationOutput](server, add("boaird.conversation.read", "Read a bounded page of messages for a task assigned to this Agent."), s.conversationRead)
	mcp.AddTool[conversationSendInput, messageOutput](server, add("boaird.conversation.send", "Send a message as the authenticated Agent on an assigned task."), s.conversationSend)
	mcp.AddTool[taskInput, contextOutput](server, add("boaird.context.get", "Get task and project context with a bounded recent conversation page."), s.contextGet)
	mcp.AddTool[brainRequestInput, brainOutput](server, add("boaird.brain.request", "Submit a planning request only when this identity is the configured Brain for the project."), s.brainRequest)
	mcp.AddTool[runInput, runOutput](server, add("boaird.run.get", "Get a run owned by this Agent or visible within its explicit project scope."), s.runGet)
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
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Role         string   `json:"role"`
	Status       string   `json:"status"`
	Runtime      string   `json:"runtime"`
	Capabilities []string `json:"capabilities"`
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
	Project      projectOutput      `json:"project"`
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
func agentView(a agent.Agent) agentOutput {
	caps := make([]string, 0, len(a.Capabilities()))
	for _, c := range a.Capabilities() {
		caps = append(caps, string(c))
	}
	return agentOutput{ID: a.ID(), Name: a.Name(), Role: a.Role(), Status: string(a.Status()), Runtime: string(a.Runtime()), Capabilities: caps}
}
func taskView(t task.Task) taskOutput {
	return taskOutput{ID: t.ID(), ProjectID: t.ProjectID(), Title: t.Title(), Description: t.Description(), Status: string(t.Status()), Priority: t.Priority(), AssignedTo: t.AssignedTo(), HumanReview: t.HumanReview()}
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
func (s *Server) authorize(ctx context.Context, a agent.Agent, required, projectID string) error {
	grant, err := capability.New(required)
	if err != nil {
		return err
	}
	return (capability.Authorizer{}).Authorize(capability.Principal{ID: a.ID(), Capabilities: a.Capabilities(), ProjectIDs: a.ProjectIDs()}, grant, projectID)
}
func (s *Server) authorizeInProject(a agent.Agent, required, projectID string) error {
	grant, err := capability.New(required)
	if err != nil {
		return err
	}
	scope := append(a.ProjectIDs(), projectID)
	return (capability.Authorizer{}).Authorize(capability.Principal{ID: a.ID(), Capabilities: a.Capabilities(), ProjectIDs: scope}, grant, projectID)
}
func (s *Server) assignedTask(ctx context.Context, a agent.Agent, id, required string) (task.Task, error) {
	t, err := s.services.Tasks.Get(ctx, id)
	if err != nil {
		return task.Task{}, err
	}
	if t.AssignedTo() != a.ID() {
		return task.Task{}, task.ErrForbidden
	}
	if err := s.authorizeInProject(a, required, t.ProjectID()); err != nil {
		return task.Task{}, task.ErrForbidden
	}
	return t, nil
}
func (s *Server) taskVisible(ctx context.Context, a agent.Agent, t task.Task, required string) error {
	if t.AssignedTo() == a.ID() {
		return s.authorizeInProject(a, required, t.ProjectID())
	}
	if projectInScope(a, t.ProjectID()) {
		return s.authorize(ctx, a, required, t.ProjectID())
	}
	return task.ErrForbidden
}
func (s *Server) projectVisible(ctx context.Context, a agent.Agent, id string) (project.Project, error) {
	p, err := s.services.Projects.Get(ctx, id)
	if err != nil {
		return project.Project{}, err
	}
	if projectInScope(a, id) {
		if err := s.authorize(ctx, a, "project.read", id); err == nil {
			return p, nil
		}
	}
	if s.authorize(ctx, a, "project.read", "") != nil {
		return project.Project{}, task.ErrForbidden
	}
	values, err := s.services.Tasks.List(ctx, id, "", a.ID(), 100, 0)
	if err != nil || len(values) == 0 || s.authorizeInProject(a, "project.read", id) != nil {
		return project.Project{}, task.ErrForbidden
	}
	return p, nil
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
	return nil, []agentOutput{agentView(a)}, nil
}
func (s *Server) agentGet(ctx context.Context, _ *mcp.CallToolRequest, in agentInput) (*mcp.CallToolResult, agentOutput, error) {
	a, e := s.actor(ctx)
	if e != nil {
		return nil, agentOutput{}, toolError(e)
	}
	if in.AgentID != "" && in.AgentID != a.ID() || s.authorize(ctx, a, "agent.read", "") != nil {
		return nil, agentOutput{}, toolError(task.ErrForbidden)
	}
	return nil, agentView(a), nil
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
		projectOffset := 0
		for skip > 0 {
			pageSize := min(skip, maxToolPageSize)
			values, err := s.services.Tasks.List(ctx, p.ID(), status, a.ID(), pageSize, projectOffset)
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
		values, err := s.services.Tasks.List(ctx, p.ID(), status, a.ID(), min(limit-len(result), maxToolPageSize), projectOffset)
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
	return s.transition(ctx, in.TaskID, task.StatusWorking, in.Reason, "task.transition")
}
func (s *Server) taskUpdate(ctx context.Context, _ *mcp.CallToolRequest, in taskUpdateInput) (*mcp.CallToolResult, taskOutput, error) {
	to := task.Status(in.Status)
	if to != task.StatusWorking && to != task.StatusReview && to != task.StatusBlocked {
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
	if _, e = s.assignedTask(ctx, a, id, required); e != nil {
		return nil, taskOutput{}, toolError(e)
	}
	v, e := s.services.Tasks.Transition(ctx, id, to, reason, task.Actor{Type: task.ActorAgent, ID: a.ID()})
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
	t, e := s.assignedTask(ctx, a, in.TaskID, "conversation.read")
	if e != nil {
		return nil, conversationOutput{}, toolError(e)
	}
	if s.authorize(ctx, a, "task.read", "") != nil {
		return nil, conversationOutput{}, toolError(task.ErrForbidden)
	}
	value, msgs, next, e := s.services.Conversations.GetPageForAgent(ctx, t.ID(), a.ID(), in.Before, in.Limit)
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
	if _, e = s.assignedTask(ctx, a, in.TaskID, "conversation.write"); e != nil {
		return nil, messageOutput{}, toolError(e)
	}
	contentType := strings.TrimSpace(in.ContentType)
	if contentType == "" {
		contentType = "text/plain"
	}
	m, e := s.services.Conversations.Append(ctx, in.TaskID, conversation.SenderAgent, a.ID(), conversation.MessageInput{Content: in.Content, ContentType: contentType})
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
	if e != nil || s.taskVisible(ctx, a, t, "task.read") != nil || s.authorize(ctx, a, "conversation.read", "") != nil {
		return nil, contextOutput{}, toolError(task.ErrForbidden)
	}
	p, e := s.projectVisible(ctx, a, t.ProjectID())
	if e != nil {
		return nil, contextOutput{}, toolError(e)
	}
	value, msgs, next, e := s.services.Conversations.GetPageForAgent(ctx, t.ID(), a.ID(), "", 20)
	if e != nil {
		return nil, contextOutput{}, toolError(e)
	}
	conv := conversationOutput{ConversationID: value.ID(), TaskID: t.ID(), Messages: make([]messageOutput, 0, len(msgs)), NextBefore: next}
	for _, m := range msgs {
		conv.Messages = append(conv.Messages, messageView(m))
	}
	return nil, contextOutput{Project: projectOutput{ID: p.ID(), Name: p.Name(), Description: p.Description(), Status: string(p.Status())}, Task: taskView(t), Conversation: conv}, nil
}
func (s *Server) brainRequest(ctx context.Context, _ *mcp.CallToolRequest, in brainRequestInput) (*mcp.CallToolResult, brainOutput, error) {
	a, e := s.actor(ctx)
	if e != nil {
		return nil, brainOutput{}, toolError(e)
	}
	b, e := s.services.Brains.Get(ctx, in.ProjectID)
	if e != nil || b.AgentID() != a.ID() {
		return nil, brainOutput{}, toolError(task.ErrForbidden)
	}
	if s.authorizeInProject(a, "brain.execute", in.ProjectID) != nil {
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
	if v.AgentID() != a.ID() && (!projectInScope(a, v.ProjectID()) || s.authorize(ctx, a, "run.read", v.ProjectID()) != nil) {
		return nil, runOutput{}, toolError(task.ErrForbidden)
	}
	return nil, runOutput{ID: v.ID(), ProjectID: v.ProjectID(), TaskID: v.TaskID(), AgentID: v.AgentID(), Status: string(v.Status()), Output: v.Output(), Error: v.Error()}, nil
}
