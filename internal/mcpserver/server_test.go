package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type agentStub struct{ values map[string]agent.Agent }

func (s agentStub) Get(_ context.Context, id string) (agent.Agent, error) {
	v, ok := s.values[id]
	if !ok {
		return agent.Agent{}, agent.ErrNotFound
	}
	return v, nil
}
func (s agentStub) List(context.Context, int, int) ([]agent.Agent, error) { return nil, nil }

type projectStub struct{ values map[string]project.Project }

func (s projectStub) Get(_ context.Context, id string) (project.Project, error) {
	value, ok := s.values[id]
	if !ok {
		return project.Project{}, project.ErrNotFound
	}
	return value, nil
}
func (projectStub) List(context.Context, int, int) ([]project.Project, error) { return nil, nil }

type taskStub struct {
	values       map[string]task.Task
	actorRecords map[string]task.Actor
}

func (s taskStub) Get(_ context.Context, id string) (task.Task, error) {
	value, ok := s.values[id]
	if !ok {
		return task.Task{}, task.ErrNotFound
	}
	return value, nil
}
func (s taskStub) List(_ context.Context, projectID string, status task.Status, assignedTo string, limit, offset int) ([]task.Task, error) {
	values := make([]task.Task, 0)
	for _, value := range s.values {
		if value.ProjectID() == projectID && (status == "" || value.Status() == status) && (assignedTo == "" || value.AssignedTo() == assignedTo) {
			values = append(values, value)
		}
	}
	if offset >= len(values) {
		return []task.Task{}, nil
	}
	values = values[offset:]
	if len(values) > limit {
		values = values[:limit]
	}
	return values, nil
}
func (s taskStub) AssignAtVersion(_ context.Context, id, agentID string, expectedVersion int64, reason string, actor task.Actor) (task.Task, error) {
	value, ok := s.values[id]
	if !ok {
		return task.Task{}, task.ErrNotFound
	}
	if value.Version() != expectedVersion {
		return task.Task{}, task.ErrConflict
	}
	var err error
	value, err = value.Assign(agentID)
	if err != nil {
		return task.Task{}, err
	}
	value, err = value.RecordMutation(actor, reason)
	if err != nil {
		return task.Task{}, err
	}
	value, err = value.AdvanceVersion()
	if err != nil {
		return task.Task{}, err
	}
	s.values[id] = value
	if s.actorRecords != nil {
		s.actorRecords[id] = actor
	}
	return value, nil
}
func (s taskStub) Claim(_ context.Context, id, agentID string, expectedVersion int64, reason string, actor task.Actor) (task.Task, error) {
	value, ok := s.values[id]
	if !ok {
		return task.Task{}, task.ErrNotFound
	}
	if value.Version() != expectedVersion || value.Status() != task.StatusReady || value.AssignedTo() != "" && value.AssignedTo() != agentID {
		return task.Task{}, task.ErrConflict
	}
	var err error
	if value.AssignedTo() == "" {
		value, err = value.Assign(agentID)
		if err != nil {
			return task.Task{}, err
		}
	}
	value, err = value.Transition(task.StatusWorking)
	if err != nil {
		return task.Task{}, err
	}
	value, err = value.RecordMutation(actor, reason)
	if err != nil {
		return task.Task{}, err
	}
	value, err = value.AdvanceVersion()
	if err != nil {
		return task.Task{}, err
	}
	s.values[id] = value
	return value, nil
}
func (s taskStub) Transition(_ context.Context, id string, to task.Status, reason string, actor task.Actor) (task.Task, error) {
	value, ok := s.values[id]
	if !ok {
		return task.Task{}, task.ErrNotFound
	}
	next, err := value.Transition(to)
	if err != nil {
		return task.Task{}, err
	}
	next, err = next.RecordMutation(actor, reason)
	if err != nil {
		return task.Task{}, err
	}
	s.values[id] = next
	if s.actorRecords != nil {
		s.actorRecords[id] = actor
	}
	return next, nil
}

type conversationStub struct {
	brainReads *[]string
	agentReads *[]string
	senders    *[]conversation.SenderType
}

func (s conversationStub) GetPageForAgent(_ context.Context, taskID, _ string, _ string, _ int) (conversation.Conversation, []conversation.Message, string, error) {
	if s.agentReads != nil {
		*s.agentReads = append(*s.agentReads, taskID)
	}
	value, _ := conversation.New("conversation-"+taskID, "project-one", taskID)
	return value, nil, "", nil
}
func (s conversationStub) GetPageForBrain(_ context.Context, taskID, _ string, _ string, _ int) (conversation.Conversation, []conversation.Message, string, error) {
	if s.brainReads != nil {
		*s.brainReads = append(*s.brainReads, taskID)
	}
	value, _ := conversation.New("conversation-"+taskID, "project-one", taskID)
	return value, nil, "", nil
}
func (s conversationStub) Append(_ context.Context, taskID string, senderType conversation.SenderType, senderID string, input conversation.MessageInput) (conversation.Message, error) {
	if s.senders != nil {
		*s.senders = append(*s.senders, senderType)
	}
	return conversation.NewMessage("message-"+taskID, "conversation-"+taskID, senderType, senderID, input.Content, input.ContentType, time.Now().UTC())
}

type brainStub struct{ values map[string]brain.Brain }

func (s brainStub) Get(_ context.Context, projectID string) (brain.Brain, error) {
	value, ok := s.values[projectID]
	if !ok {
		return brain.Brain{}, brain.ErrNotFound
	}
	return value, nil
}
func (brainStub) Execute(context.Context, string, string, int) (brain.Execution, error) {
	return brain.Execution{}, brain.ErrNotFound
}

type runStub struct{}

func (runStub) Get(context.Context, string) (run.Run, error) { return run.Run{}, run.ErrNotFound }

type shellExecutorStub struct {
	request governedtool.Request
	calls   int
}

func (s *shellExecutorStub) Execute(_ context.Context, request governedtool.Request) (json.RawMessage, error) {
	s.request = request
	s.calls++
	return json.RawMessage(`{"stdout":"ok","exit_code":0}`), nil
}

type gitExecutorStub struct {
	request governedtool.Request
	calls   int
}

func (s *gitExecutorStub) Execute(_ context.Context, request governedtool.Request) (json.RawMessage, error) {
	s.request = request
	s.calls++
	return json.RawMessage(`{"stdout":"ok","truncated":false}`), nil
}

type mcpExtensionAuditor struct{}

func (mcpExtensionAuditor) RecordToolExecution(context.Context, governedtool.Event) error { return nil }

type staticVerifier map[string]string

func (s staticVerifier) Verify(_ context.Context, raw string) (mcptoken.Identity, error) {
	value, ok := s[raw]
	if !ok {
		return mcptoken.Identity{}, errors.New("invalid token")
	}
	return mcptoken.Identity{AgentID: value, TokenID: "token-" + value}, nil
}

type denialStub struct{ records []string }

func (s *denialStub) RecordAuthorizationDenial(_ context.Context, _, projectID, required, _, reason string) error {
	s.records = append(s.records, projectID+":"+required+":"+reason)
	return nil
}

type bearerRoundTripper struct {
	token func() string
	base  http.RoundTripper
}

func (r bearerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	copy := req.Clone(req.Context())
	copy.Header = req.Header.Clone()
	copy.Header.Set("Authorization", "Bearer "+r.token())
	return r.base.RoundTrip(copy)
}

func TestHandlerAuthenticatesAndBindsSessionToAgent(t *testing.T) {
	first, err := testAgentWithCapabilities(t, "agent-one", []string{"agent.read", "project.read", "task.read", "task.transition", "task.assign", "task.complete", "conversation.read", "conversation.write", "brain.execute", "run.read"})
	if err != nil {
		t.Fatal(err)
	}
	first, err = first.AddProject("project-a")
	if err != nil {
		t.Fatal(err)
	}
	second := testAgent(t, "agent-two")
	firstToken := "0101010101010101010101010101010101010101010101010101010101010101"
	secondToken := "0202020202020202020202020202020202020202020202020202020202020202"
	assigned, err := task.New("task-a", "project-a", "Assigned task", "details", first.ID())
	if err != nil {
		t.Fatal(err)
	}
	assigned, err = assigned.Assign(first.ID())
	if err != nil {
		t.Fatal(err)
	}
	other, err := task.New("task-b", "project-b", "Other task", "details", second.ID())
	if err != nil {
		t.Fatal(err)
	}
	other, err = other.Assign(second.ID())
	if err != nil {
		t.Fatal(err)
	}
	other, err = other.Transition(task.StatusReady)
	if err != nil {
		t.Fatal(err)
	}
	claimable, err := task.New("task-c", "project-a", "Claimable task", "details", second.ID())
	if err != nil {
		t.Fatal(err)
	}
	claimable, err = claimable.Transition(task.StatusReady)
	if err != nil {
		t.Fatal(err)
	}
	outOfScope, err := task.New("task-d", "project-b", "Out of scope task", "details", second.ID())
	if err != nil {
		t.Fatal(err)
	}
	outOfScope, err = outOfScope.Transition(task.StatusReady)
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(Services{Agents: agentStub{map[string]agent.Agent{first.ID(): first, second.ID(): second}}, Projects: projectStub{}, Tasks: taskStub{values: map[string]task.Task{"task-a": assigned, "task-b": other, "task-c": claimable, "task-d": outOfScope}}, Conversations: conversationStub{}, Brains: brainStub{}, Runs: runStub{}}, staticVerifier{firstToken: first.ID(), secondToken: second.ID()})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	current := firstToken
	httpClient := &http.Client{Transport: bearerRoundTripper{token: func() string { return current }, base: http.DefaultTransport}}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: httpServer.URL, HTTPClient: httpClient, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Errorf("close MCP session: %v", err)
		}
	})
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	wantTools := []string{"boaird.project.get", "boaird.agent.list", "boaird.agent.get", "boaird.agent.status", "boaird.agent.heartbeat", "boaird.task.list", "boaird.task.get", "boaird.task.assign", "boaird.task.claim", "boaird.task.update", "boaird.task.complete", "boaird.task.fail", "boaird.conversation.read", "boaird.conversation.send", "boaird.context.get", "boaird.brain.request", "boaird.run.get"}
	if len(result.Tools) != len(wantTools) {
		t.Fatalf("tool count = %d, want %d", len(result.Tools), len(wantTools))
	}
	gotTools := make(map[string]bool, len(result.Tools))
	for _, value := range result.Tools {
		gotTools[value.Name] = true
	}
	for _, value := range wantTools {
		if !gotTools[value] {
			t.Errorf("missing MCP tool %q", value)
		}
	}
	visible, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "boaird.task.get", Arguments: map[string]any{"task_id": "task-a"}})
	if err != nil || visible.IsError {
		t.Fatalf("assigned task.get result = %+v, error %v", visible, err)
	}
	claimed, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "boaird.task.claim", Arguments: map[string]any{"task_id": "task-c", "reason": "claim authorized project work"}})
	if err != nil || claimed.IsError {
		t.Fatalf("authorized self-claim result = %+v, error %v", claimed, err)
	}
	claimedJSON, err := json.Marshal(claimed.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(claimedJSON), `"status":"WORKING"`) || !strings.Contains(string(claimedJSON), `"assigned_to":"agent-one"`) {
		t.Fatalf("self-claim result = %s, want task assigned to agent-one and WORKING", claimedJSON)
	}
	for _, taskID := range []string{"task-b", "task-d"} {
		deniedClaim, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "boaird.task.claim", Arguments: map[string]any{"task_id": taskID, "reason": "attempt unauthorized claim"}})
		if err != nil || deniedClaim == nil || !deniedClaim.IsError {
			t.Fatalf("claim of unauthorized task %q result = %+v, error %v; want tool error", taskID, deniedClaim, err)
		}
	}
	denied, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "boaird.task.get", Arguments: map[string]any{"task_id": "task-b"}})
	if err != nil || denied == nil || !denied.IsError {
		t.Fatalf("cross-Agent task.get result = %+v, error %v; want tool error", denied, err)
	}
	missing, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "boaird.task.get", Arguments: map[string]any{"task_id": "missing"}})
	if err != nil || missing == nil || !missing.IsError {
		t.Fatalf("missing task result = %+v, error %v; want tool error", missing, err)
	}
	encoded, err := json.Marshal(missing.Content)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "task not found") {
		t.Fatalf("tool leaked internal error: %s", encoded)
	}
	current = secondToken
	if _, err := session.ListTools(context.Background(), nil); err == nil || !strings.Contains(strings.ToLower(err.Error()), "forbidden") {
		t.Fatalf("session accepted a different Agent token: %v", err)
	}
}

func TestAuthenticatedMCPBrainUsesBrainTaskPolicyActor(t *testing.T) {
	capabilities := []string{"agent.read", "task.read", "task.assign", "task.review", "task.complete", "task.transition", "conversation.read", "conversation.write", "brain.execute"}
	brainAgent, err := testAgentWithCapabilities(t, "brain-agent", capabilities)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := testAgentWithCapabilities(t, "worker-agent", []string{"agent.read", "task.read", "conversation.read", "conversation.write"})
	if err != nil {
		t.Fatal(err)
	}
	worker, err = worker.AddProject("project-one")
	if err != nil {
		t.Fatal(err)
	}
	otherWorker, err := testAgentWithCapabilities(t, "other-worker", []string{"agent.read", "task.read", "conversation.read", "conversation.write"})
	if err != nil {
		t.Fatal(err)
	}
	activeBrain, err := brain.New("brain-one", "project-one", brainAgent.ID(), brain.AutonomySemiAuto)
	if err != nil {
		t.Fatal(err)
	}
	readyTask, err := task.New("task-one", "project-one", "Task", "Details", "administrator")
	if err != nil {
		t.Fatal(err)
	}
	readyTask, err = readyTask.Transition(task.StatusReady)
	if err != nil {
		t.Fatal(err)
	}
	delegatedTask := newAssignedTask(t, "task-other-project", "project-two", otherWorker.ID())
	values := map[string]task.Task{readyTask.ID(): readyTask, delegatedTask.ID(): delegatedTask}
	actorRecords := make(map[string]task.Actor)
	brainReads := []string{}
	agentReads := []string{}
	senders := []conversation.SenderType{}
	conversationService := conversationStub{brainReads: &brainReads, agentReads: &agentReads, senders: &senders}
	brainToken := strings.Repeat("01", 32)
	server, err := New(Services{
		Agents:        agentStub{values: map[string]agent.Agent{brainAgent.ID(): brainAgent, worker.ID(): worker, otherWorker.ID(): otherWorker}},
		Projects:      projectStub{},
		Tasks:         taskStub{values: values, actorRecords: actorRecords},
		Conversations: conversationService,
		Brains:        brainStub{values: map[string]brain.Brain{"project-one": activeBrain}},
		Runs:          runStub{},
	}, staticVerifier{brainToken: brainAgent.ID(), "worker-token": worker.ID()})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "brain-test", Version: "1"}, nil)
	httpClient := &http.Client{Transport: bearerRoundTripper{token: func() string { return brainToken }, base: http.DefaultTransport}}
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: httpServer.URL, HTTPClient: httpClient, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer func() {
		if err := session.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}()
	assigned, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "boaird.task.assign", Arguments: map[string]any{
		"task_id": readyTask.ID(), "agent_id": worker.ID(), "expected_version": float64(readyTask.Version()), "reason": "delegate task",
	}})
	if err != nil || assigned.IsError {
		t.Fatalf("Brain task.assign result = %+v, error %v", assigned, err)
	}
	if actor := actorRecords[readyTask.ID()]; actor.Type != task.ActorBrain || actor.ID != brainAgent.ID() {
		t.Fatalf("assignment actor = %+v, want authenticated active Brain", actorRecords[readyTask.ID()])
	}
	contextResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "boaird.context.get", Arguments: map[string]any{"task_id": readyTask.ID()}})
	if err != nil || contextResult.IsError {
		t.Fatalf("Brain context.get for delegated task = %+v, error %v", contextResult, err)
	}
	if len(brainReads) != 1 || brainReads[0] != readyTask.ID() || len(agentReads) != 0 {
		t.Fatalf("context.get conversation reads: Brain=%v Agent=%v", brainReads, agentReads)
	}
	readResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "boaird.conversation.read", Arguments: map[string]any{"task_id": readyTask.ID()}})
	if err != nil || readResult.IsError {
		t.Fatalf("Brain conversation.read for delegated task = %+v, error %v", readResult, err)
	}
	if len(brainReads) != 2 || brainReads[1] != readyTask.ID() {
		t.Fatalf("Brain conversation read path = %v, want delegated task", brainReads)
	}
	sendResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "boaird.conversation.send", Arguments: map[string]any{"task_id": readyTask.ID(), "content": "Brain review note"}})
	if err != nil || sendResult.IsError {
		t.Fatalf("Brain conversation.send for delegated task = %+v, error %v", sendResult, err)
	}
	if len(senders) != 1 || senders[0] != conversation.SenderBrain {
		t.Fatalf("persisted sender types = %v, want BRAIN", senders)
	}
	for _, name := range []string{"boaird.context.get", "boaird.conversation.read", "boaird.conversation.send"} {
		crossProject, callErr := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: map[string]any{"task_id": delegatedTask.ID(), "content": "must be denied"}})
		if callErr != nil || crossProject == nil || !crossProject.IsError {
			t.Fatalf("Brain %s cross-project result = %+v, error %v; want denial", name, crossProject, callErr)
		}
	}
	workerClient := mcp.NewClient(&mcp.Implementation{Name: "worker-test", Version: "1"}, nil)
	workerHTTPClient := &http.Client{Transport: bearerRoundTripper{token: func() string { return "worker-token" }, base: http.DefaultTransport}}
	workerSession, err := workerClient.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: httpServer.URL, HTTPClient: workerHTTPClient, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatalf("Worker Connect() error = %v", err)
	}
	defer func() {
		if err := workerSession.Close(); err != nil {
			t.Errorf("close Worker session: %v", err)
		}
	}()
	workerContext, err := workerSession.CallTool(context.Background(), &mcp.CallToolParams{Name: "boaird.context.get", Arguments: map[string]any{"task_id": readyTask.ID()}})
	if err != nil || workerContext.IsError {
		t.Fatalf("assigned Worker context.get = %+v, error %v", workerContext, err)
	}
	workerRead, err := workerSession.CallTool(context.Background(), &mcp.CallToolParams{Name: "boaird.conversation.read", Arguments: map[string]any{"task_id": readyTask.ID()}})
	if err != nil || workerRead.IsError {
		t.Fatalf("assigned Worker conversation.read = %+v, error %v", workerRead, err)
	}
	workerSend, err := workerSession.CallTool(context.Background(), &mcp.CallToolParams{Name: "boaird.conversation.send", Arguments: map[string]any{"task_id": readyTask.ID(), "content": "Worker update"}})
	if err != nil || workerSend.IsError {
		t.Fatalf("assigned Worker conversation.send = %+v, error %v", workerSend, err)
	}
	if len(agentReads) != 2 || agentReads[0] != readyTask.ID() || agentReads[1] != readyTask.ID() {
		t.Fatalf("Worker conversation reads = %v, want exact assigned task twice", agentReads)
	}
	if len(senders) != 2 || senders[1] != conversation.SenderAgent {
		t.Fatalf("persisted sender types = %v, want BRAIN then AGENT", senders)
	}
	for _, name := range []string{"boaird.context.get", "boaird.conversation.read", "boaird.conversation.send"} {
		workerDenied, callErr := workerSession.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: map[string]any{"task_id": delegatedTask.ID(), "content": "must be denied"}})
		if callErr != nil || workerDenied == nil || !workerDenied.IsError {
			t.Fatalf("Worker %s for unassigned task result = %+v, error %v; want denial", name, workerDenied, callErr)
		}
	}
	stale, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "boaird.task.assign", Arguments: map[string]any{
		"task_id": readyTask.ID(), "agent_id": brainAgent.ID(), "expected_version": float64(readyTask.Version()), "reason": "stale retry",
	}})
	if err != nil || stale == nil || !stale.IsError {
		t.Fatalf("stale assignment retry result = %+v, error %v; want conflict", stale, err)
	}
	claim, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "boaird.task.claim", Arguments: map[string]any{
		"task_id": readyTask.ID(), "reason": "Brain must delegate instead of claiming as worker",
	}})
	if err != nil || claim == nil || !claim.IsError {
		t.Fatalf("Brain task.claim result = %+v, error %v; want denial", claim, err)
	}
	updated, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "boaird.task.update", Arguments: map[string]any{
		"task_id": readyTask.ID(), "status": string(task.StatusWorking), "reason": "start delegated task",
	}})
	if err != nil || updated.IsError {
		t.Fatalf("Brain task.update result = %+v, error %v", updated, err)
	}
	if actor := actorRecords[readyTask.ID()]; actor.Type != task.ActorBrain || actor.ID != brainAgent.ID() {
		t.Fatalf("transition actor = %+v, want authenticated active Brain", actorRecords[readyTask.ID()])
	}
}

func TestShellToolIsDiscoveredOnlyWhenConfiguredAndUsesAuthenticatedScope(t *testing.T) {
	a, err := testAgentWithCapabilities(t, "shell-agent", []string{"shell.execute"})
	if err != nil {
		t.Fatal(err)
	}
	a, err = a.AddProject("project-one")
	if err != nil {
		t.Fatal(err)
	}
	executor := &shellExecutorStub{}
	server, err := New(Services{
		Agents: agentStub{values: map[string]agent.Agent{a.ID(): a}}, Projects: projectStub{}, Tasks: taskStub{},
		Conversations: conversationStub{}, Brains: brainStub{}, Runs: runStub{}, Tools: executor,
	}, staticVerifier{"secret": a.ID()})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "shell-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint: httpServer.URL, HTTPClient: &http.Client{Transport: bearerRoundTripper{token: func() string { return "secret" }, base: http.DefaultTransport}}, DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, value := range tools.Tools {
		if value.Name == "boaird.shell.execute" {
			found = true
		}
	}
	if !found {
		t.Fatal("configured shell tool was not advertised")
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "boaird.shell.execute", Arguments: map[string]any{
		"project_id": "project-one", "executable": "go", "arguments": []string{"version"},
	}})
	if err != nil || result.IsError {
		t.Fatalf("shell.execute result = %+v, error = %v", result, err)
	}
	if executor.calls != 1 || executor.request.Principal.ID != a.ID() || executor.request.ProjectID != "project-one" || executor.request.ToolName != "shell.execute" {
		t.Fatalf("executor calls = %d, request = %#v", executor.calls, executor.request)
	}
}

func TestGitToolsAreDiscoveredAndUseAuthenticatedRepositoryScope(t *testing.T) {
	a, err := testAgentWithCapabilities(t, "git-agent", []string{"git.read", "git.push"})
	if err != nil {
		t.Fatal(err)
	}
	a, err = a.AddProject("project-one")
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	value, err := project.Restore(project.Snapshot{ID: "project-one", Name: "Project One", Status: project.StatusConfiguring, WorkspacePath: workspace})
	if err != nil {
		t.Fatal(err)
	}
	executor := &gitExecutorStub{}
	server, err := New(Services{
		Agents: agentStub{values: map[string]agent.Agent{a.ID(): a}}, Projects: projectStub{values: map[string]project.Project{"project-one": value}},
		Tasks: taskStub{}, Conversations: conversationStub{}, Brains: brainStub{}, Runs: runStub{}, GitTools: executor,
	}, staticVerifier{"secret": a.ID()})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "git-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint: httpServer.URL, HTTPClient: &http.Client{Transport: bearerRoundTripper{token: func() string { return "secret" }, base: http.DefaultTransport}}, DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[string]bool, len(tools.Tools))
	for _, tool := range tools.Tools {
		got[tool.Name] = true
	}
	for _, name := range []string{"boaird.git.read", "boaird.git.diff", "boaird.git.branch", "boaird.git.commit", "boaird.git.push", "boaird.git.merge"} {
		if !got[name] {
			t.Errorf("missing MCP tool %q", name)
		}
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "boaird.git.read", Arguments: map[string]any{
		"project_id": "project-one", "task_id": "task-1", "run_id": "run-1", "query": "status",
	}})
	if err != nil || result.IsError {
		t.Fatalf("git.read result = %+v, error = %v", result, err)
	}
	if executor.calls != 1 || executor.request.Principal.ID != a.ID() || executor.request.ProjectID != "project-one" ||
		executor.request.ToolName != "git.read" || executor.request.TaskID != "task-1" || executor.request.RunID != "run-1" ||
		executor.request.ResourceType != "repository" || executor.request.ResourceID != workspace {
		t.Fatalf("executor calls = %d, request = %#v", executor.calls, executor.request)
	}
	// A missing git capability must be denied before reaching the executor.
	denied, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "boaird.git.commit", Arguments: map[string]any{
		"project_id": "project-one", "message": "nope",
	}})
	if err != nil || denied == nil || !denied.IsError {
		t.Fatalf("git.commit without git.commit capability result = %+v, error = %v; want denial", denied, err)
	}
	if executor.calls != 1 {
		t.Fatalf("git.commit reached the executor despite missing capability; calls = %d", executor.calls)
	}
}

func TestExtensionToolUsesAuthenticatedIdentityAndEnforcedProjectScope(t *testing.T) {
	a, err := testAgentWithCapabilities(t, "extension-agent", []string{"extension.search"})
	if err != nil {
		t.Fatal(err)
	}
	a, err = a.AddProject("project-one")
	if err != nil {
		t.Fatal(err)
	}
	projectOne, err := project.Restore(project.Snapshot{ID: "project-one", Name: "One", Status: project.StatusConfiguring})
	if err != nil {
		t.Fatal(err)
	}
	projectTwo, err := project.Restore(project.Snapshot{ID: "project-two", Name: "Two", Status: project.StatusConfiguring})
	if err != nil {
		t.Fatal(err)
	}
	var executions int
	registry := governedtool.NewRegistry()
	tool := governedtool.Definition[json.RawMessage, json.RawMessage]{
		Metadata: governedtool.Contract{
			Name: "example.search", Description: "Search", InputSchema: json.RawMessage(`{"type":"object"}`),
			OutputSchema: json.RawMessage(`{"type":"object"}`), RequiredCapabilities: []capability.Capability{"extension.search"},
			Limits: governedtool.Limits{Timeout: time.Second, MaxInputBytes: 1024, MaxOutputBytes: 1024},
		},
		Validate: func(value json.RawMessage) error { return nil }, ValidateOutput: func(json.RawMessage) error { return nil },
		Run: func(_ context.Context, value json.RawMessage) (json.RawMessage, error) {
			executions++
			return json.RawMessage(`{"ok":true}`), nil
		},
	}
	if err := registry.Register(tool); err != nil {
		t.Fatal(err)
	}
	executor, err := governedtool.NewEnforcer(registry, mcpExtensionAuditor{}, governedtool.ScopeAuthorizerFunc(func(_ context.Context, principal governedtool.Principal, request governedtool.Request) error {
		for _, id := range principal.ProjectIDs {
			if id == request.ProjectID && request.ResourceType == "extension" && request.ResourceID == request.ToolName {
				return nil
			}
		}
		return governedtool.ErrDenied
	}))
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(Services{
		Agents:   agentStub{values: map[string]agent.Agent{a.ID(): a}},
		Projects: projectStub{values: map[string]project.Project{"project-one": projectOne, "project-two": projectTwo}},
		Tasks:    taskStub{}, Conversations: conversationStub{}, Brains: brainStub{}, Runs: runStub{}, ExtensionTools: executor,
	}, staticVerifier{"secret": a.ID()})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "extension-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint: httpServer.URL, HTTPClient: &http.Client{Transport: bearerRoundTripper{token: func() string { return "secret" }, base: http.DefaultTransport}}, DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, value := range tools.Tools {
		if value.Name == "boaird.extension.execute" {
			found = true
		}
	}
	if !found {
		t.Fatal("extension execution gateway was not advertised")
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "boaird.extension.execute", Arguments: map[string]any{
		"project_id": "project-one", "tool_name": "example.search", "input": map[string]any{"query": "hello"},
	}})
	if err != nil || result == nil || result.IsError {
		var encoded []byte
		isError := false
		if result != nil {
			encoded, _ = json.Marshal(result.Content)
			isError = result.IsError
		}
		t.Fatalf("authorized extension result = %s, isError=%v, error = %v", encoded, isError, err)
	}
	if executions != 1 {
		t.Fatalf("extension executions = %d, want 1", executions)
	}
	denied, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "boaird.extension.execute", Arguments: map[string]any{
		"project_id": "project-two", "tool_name": "example.search", "input": map[string]any{"query": "hello"},
	}})
	if err != nil || denied == nil || !denied.IsError {
		t.Fatalf("cross-project extension result = %+v, error = %v; want denial", denied, err)
	}
	if executions != 1 {
		t.Fatalf("cross-project request reached extension adapter; executions = %d", executions)
	}
}

func TestHandlerRejectsMissingOrInvalidBearerToken(t *testing.T) {
	a := testAgent(t, "agent-one")
	token := "0101010101010101010101010101010101010101010101010101010101010101"
	server, err := New(Services{Agents: agentStub{map[string]agent.Agent{a.ID(): a}}, Projects: projectStub{}, Tasks: taskStub{}, Conversations: conversationStub{}, Brains: brainStub{}, Runs: runStub{}}, staticVerifier{token: a.ID()})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	for _, token := range []string{"", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"} {
		req, err := http.NewRequest(http.MethodPost, httpServer.URL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25"}}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if err := resp.Body.Close(); err != nil {
			t.Errorf("close unauthorized response body: %v", err)
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("token %q status = %d, want 401", token, resp.StatusCode)
		}
	}
}

func TestRevokedTokenCannotReuseAnExistingSession(t *testing.T) {
	value := testAgent(t, "agent-one")
	rawToken := strings.Repeat("01", 32)
	verifier := staticVerifier{rawToken: value.ID()}
	server, err := New(Services{Agents: agentStub{map[string]agent.Agent{value.ID(): value}}, Projects: projectStub{}, Tasks: taskStub{}, Conversations: conversationStub{}, Brains: brainStub{}, Runs: runStub{}}, verifier)
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	httpClient := &http.Client{Transport: bearerRoundTripper{token: func() string { return rawToken }, base: http.DefaultTransport}}
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: httpServer.URL, HTTPClient: httpClient, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Errorf("close MCP session: %v", err)
		}
	})
	if _, err := session.ListTools(context.Background(), nil); err != nil {
		t.Fatalf("initial authenticated request failed: %v", err)
	}
	delete(verifier, rawToken)
	if _, err := session.ListTools(context.Background(), nil); err == nil || !strings.Contains(strings.ToLower(err.Error()), "unauthorized") {
		t.Fatalf("revoked token reused an existing session: %v", err)
	}
}

func TestNewRejectsTokenForUnregisteredAgent(t *testing.T) {
	services := Services{Agents: agentStub{values: map[string]agent.Agent{}}, Projects: projectStub{}, Tasks: taskStub{}, Conversations: conversationStub{}, Brains: brainStub{}, Runs: runStub{}}
	token := "0101010101010101010101010101010101010101010101010101010101010101"
	server, err := New(services, staticVerifier{token: "missing-agent"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.verifyToken(context.Background(), token, nil); err == nil {
		t.Fatal("verifyToken() accepted an unregistered Agent")
	}
}

func TestActiveBrainProvidesOnlyItsProjectScope(t *testing.T) {
	a := testAgent(t, "brain-agent")
	active, err := brain.New("brain-one", "project-one", a.ID(), brain.AutonomyManual)
	if err != nil {
		t.Fatal(err)
	}
	inactive := active.Deactivate()
	s := &Server{services: Services{
		Brains: brainStub{values: map[string]brain.Brain{"project-one": active, "project-two": inactive}},
	}}
	if !s.hasProjectScope(context.Background(), a, "project-one") {
		t.Fatal("active Brain does not have scope for its project")
	}
	if s.hasProjectScope(context.Background(), a, "project-two") {
		t.Fatal("inactive Brain granted project scope")
	}
	if s.hasProjectScope(context.Background(), a, "project-three") {
		t.Fatal("Brain granted scope to an unrelated project")
	}
	if err := s.authorizeProject(context.Background(), a, "project.read", "project-one"); err != nil {
		t.Fatalf("authorize active Brain with project.read: %v", err)
	}
	if err := s.authorizeProject(context.Background(), a, "project.delete", "project-one"); err == nil {
		t.Fatal("active Brain bypassed the required capability")
	}
}

func TestAssignedTaskScopeRemainsTaskSpecific(t *testing.T) {
	a := testAgent(t, "worker-agent")
	other, err := testAgentWithCapabilities(t, "other-agent", []string{"agent.read", "project.read", "task.read"})
	if err != nil {
		t.Fatal(err)
	}
	assigned := newAssignedTask(t, "assigned", "project-one", a.ID())
	unassigned := newAssignedTask(t, "unassigned", "project-one", other.ID())
	unscoped := &Server{services: Services{Brains: brainStub{}}}
	if err := unscoped.taskVisible(context.Background(), a, assigned, "task.read"); err != nil {
		t.Fatalf("assigned task should be visible without project scope: %v", err)
	}
	if err := unscoped.taskVisible(context.Background(), a, unassigned, "task.read"); err == nil {
		t.Fatal("unscoped Agent read an unassigned task")
	}
	brainValue, err := brain.New("brain-one", "project-one", a.ID(), brain.AutonomyManual)
	if err != nil {
		t.Fatal(err)
	}
	denials := &denialStub{}
	s := &Server{services: Services{Brains: brainStub{values: map[string]brain.Brain{"project-one": brainValue}}, Denials: denials}}
	if err := s.taskVisible(context.Background(), a, assigned, "task.read"); err != nil {
		t.Fatalf("assigned task should remain visible: %v", err)
	}
	if err := s.taskVisible(context.Background(), a, unassigned, "task.read"); err != nil {
		t.Fatalf("active Brain should read an unassigned task in its project: %v", err)
	}
	crossProject := newAssignedTask(t, "cross-project", "project-two", other.ID())
	if err := s.taskVisible(context.Background(), a, crossProject, "task.read"); err == nil {
		t.Fatal("active Brain granted access to a different project")
	}
	if len(denials.records) != 1 || denials.records[0] != "project-two:task.read:project_scope_or_task_assignment_required" {
		t.Fatalf("cross-project authorization denials = %#v", denials.records)
	}
}

func testAgentWithCapabilities(t *testing.T, id string, capabilities []string) (agent.Agent, error) {
	t.Helper()
	return agent.NewConfigured(id, agent.Configuration{Name: id, Role: "worker", Runtime: agent.RuntimeMCP, Capabilities: capabilities})
}

func newAssignedTask(t *testing.T, id, projectID, assignedTo string) task.Task {
	t.Helper()
	value, err := task.New(id, projectID, id, "details", assignedTo)
	if err != nil {
		t.Fatal(err)
	}
	value, err = value.Assign(assignedTo)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func testAgent(t *testing.T, id string) agent.Agent {
	t.Helper()
	value, err := agent.NewConfigured(id, agent.Configuration{Name: id, Role: "worker", Runtime: agent.RuntimeMCP, Capabilities: []string{"agent.read", "project.read", "task.read", "task.transition", "task.complete", "conversation.read", "conversation.write", "brain.execute", "run.read"}})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func TestToolErrorReportsTaskConflictRetryGuidance(t *testing.T) {
	err := toolError(task.ErrConflict)
	if err == nil || err.Error() != "task changed concurrently; reload and retry" {
		t.Fatalf("toolError(ErrConflict) = %v, want safe retry guidance", err)
	}
}

type heartbeatStub struct {
	identities []heartbeat.Identity
	statuses   map[string]agent.Observed
}

func (s *heartbeatStub) Heartbeat(_ context.Context, identity heartbeat.Identity) (agent.Observed, error) {
	s.identities = append(s.identities, identity)
	return s.statuses[identity.AgentID], nil
}

func (s *heartbeatStub) Statuses(_ context.Context, agentIDs []string) (map[string]agent.Observed, error) {
	observed := make(map[string]agent.Observed, len(agentIDs))
	for _, id := range agentIDs {
		if value, ok := s.statuses[id]; ok {
			observed[id] = value
		}
	}
	return observed, nil
}

func TestHeartbeatBindsAuthenticatedIdentityAndRejectsSpoofing(t *testing.T) {
	first := testAgent(t, "agent-one")
	second := testAgent(t, "agent-two")
	rawToken := strings.Repeat("01", 32)
	denials := &denialStub{}
	heartbeats := &heartbeatStub{statuses: map[string]agent.Observed{
		first.ID():  {Status: agent.StatusOnline, SessionOpen: true},
		second.ID(): {Status: agent.StatusIdle},
	}}
	server, err := New(Services{
		Agents:   agentStub{map[string]agent.Agent{first.ID(): first, second.ID(): second}},
		Projects: projectStub{}, Tasks: taskStub{}, Conversations: conversationStub{}, Brains: brainStub{}, Runs: runStub{},
		Denials: denials, Heartbeats: heartbeats,
	}, staticVerifier{rawToken: first.ID()})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	httpClient := &http.Client{Transport: bearerRoundTripper{token: func() string { return rawToken }, base: http.DefaultTransport}}
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: httpServer.URL, HTTPClient: httpClient, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Errorf("close MCP session: %v", err)
		}
	})

	// Connecting is authenticated MCP activity, so liveness is refreshed from
	// the verified credential alone.
	if len(heartbeats.identities) == 0 {
		t.Fatal("authenticated MCP activity did not refresh Agent liveness")
	}
	if got := heartbeats.identities[0]; got.AgentID != first.ID() || got.TokenID != "token-"+first.ID() {
		t.Fatalf("recorded identity = %+v, want the verified Agent and credential", got)
	}

	status, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "boaird.agent.status"})
	if err != nil || status.IsError {
		t.Fatalf("agent.status result = %+v, error %v", status, err)
	}
	observed := decodeAgentOutput(t, status)
	if observed.Status != string(agent.StatusOnline) || !observed.SessionOpen {
		t.Fatalf("agent.status output = %+v, want observed ONLINE session", observed)
	}

	beat, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "boaird.agent.heartbeat"})
	if err != nil || beat.IsError || decodeAgentOutput(t, beat).Status != string(agent.StatusOnline) {
		t.Fatalf("agent.heartbeat result = %+v, error %v", beat, err)
	}

	spoofed, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "boaird.agent.heartbeat", Arguments: map[string]any{"agent_id": second.ID()}})
	if err != nil || spoofed == nil || !spoofed.IsError {
		t.Fatalf("spoofed heartbeat result = %+v, error %v; want tool error", spoofed, err)
	}
	for _, identity := range heartbeats.identities {
		if identity.AgentID == second.ID() {
			t.Fatal("spoofed identity was recorded as activity for another Agent")
		}
	}
	recorded := strings.Join(denials.records, ",")
	if !strings.Contains(recorded, "heartbeat_identity_mismatch") {
		t.Fatalf("denials = %q, want heartbeat_identity_mismatch", recorded)
	}
}

func decodeAgentOutput(t *testing.T, result *mcp.CallToolResult) agentOutput {
	t.Helper()
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var value agentOutput
	if err := json.Unmarshal(encoded, &value); err != nil {
		t.Fatalf("decode agent output %s: %v", encoded, err)
	}
	return value
}
