package mcpserver

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"boAIrd/internal/agent"
	"boAIrd/internal/brain"
	"boAIrd/internal/config"
	"boAIrd/internal/conversation"
	"boAIrd/internal/project"
	"boAIrd/internal/run"
	"boAIrd/internal/task"

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

type projectStub struct{}

func (projectStub) Get(context.Context, string) (project.Project, error) {
	return project.Project{}, project.ErrNotFound
}
func (projectStub) List(context.Context, int, int) ([]project.Project, error) { return nil, nil }

type taskStub struct{ values map[string]task.Task }

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
func (s taskStub) Transition(_ context.Context, id string, to task.Status, _ string, _ task.Actor) (task.Task, error) {
	value, ok := s.values[id]
	if !ok {
		return task.Task{}, task.ErrNotFound
	}
	next, err := value.Transition(to)
	if err != nil {
		return task.Task{}, err
	}
	s.values[id] = next
	return next, nil
}

type conversationStub struct{}

func (conversationStub) GetPageForAgent(context.Context, string, string, string, int) (conversation.Conversation, []conversation.Message, string, error) {
	return conversation.Conversation{}, nil, "", nil
}
func (conversationStub) Append(context.Context, string, conversation.SenderType, string, conversation.MessageInput) (conversation.Message, error) {
	return conversation.Message{}, nil
}

type brainStub struct{}

func (brainStub) Get(context.Context, string) (brain.Brain, error) {
	return brain.Brain{}, brain.ErrNotFound
}
func (brainStub) Execute(context.Context, string, string, int) (brain.Execution, error) {
	return brain.Execution{}, brain.ErrNotFound
}

type runStub struct{}

func (runStub) Get(context.Context, string) (run.Run, error) { return run.Run{}, run.ErrNotFound }

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
	first := testAgent(t, "agent-one")
	second := testAgent(t, "agent-two")
	tokens := []config.MCPAgentToken{{AgentID: first.ID(), Token: decodeToken(t, "0101010101010101010101010101010101010101010101010101010101010101")}, {AgentID: second.ID(), Token: decodeToken(t, "0202020202020202020202020202020202020202020202020202020202020202")}}
	assigned, err := task.New("task-a", "project-a", "Assigned task", "details", first.ID())
	if err != nil {
		t.Fatal(err)
	}
	assigned, err = assigned.Assign(first.ID())
	if err != nil {
		t.Fatal(err)
	}
	other, err := task.New("task-b", "project-a", "Other task", "details", second.ID())
	if err != nil {
		t.Fatal(err)
	}
	other, err = other.Assign(second.ID())
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(context.Background(), Services{Agents: agentStub{map[string]agent.Agent{first.ID(): first, second.ID(): second}}, Projects: projectStub{}, Tasks: taskStub{map[string]task.Task{"task-a": assigned, "task-b": other}}, Conversations: conversationStub{}, Brains: brainStub{}, Runs: runStub{}}, tokens)
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	current := "0101010101010101010101010101010101010101010101010101010101010101"
	httpClient := &http.Client{Transport: bearerRoundTripper{token: func() string { return current }, base: http.DefaultTransport}}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: httpServer.URL, HTTPClient: httpClient, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer session.Close()
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	wantTools := []string{"boaird.project.get", "boaird.agent.list", "boaird.agent.get", "boaird.agent.status", "boaird.task.list", "boaird.task.get", "boaird.task.claim", "boaird.task.update", "boaird.task.complete", "boaird.task.fail", "boaird.conversation.read", "boaird.conversation.send", "boaird.context.get", "boaird.brain.request", "boaird.run.get"}
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
	current = "0202020202020202020202020202020202020202020202020202020202020202"
	if _, err := session.ListTools(context.Background(), nil); err == nil || !strings.Contains(strings.ToLower(err.Error()), "forbidden") {
		t.Fatalf("session accepted a different Agent token: %v", err)
	}
}

func TestHandlerRejectsMissingOrInvalidBearerToken(t *testing.T) {
	a := testAgent(t, "agent-one")
	server, err := New(context.Background(), Services{Agents: agentStub{map[string]agent.Agent{a.ID(): a}}, Projects: projectStub{}, Tasks: taskStub{}, Conversations: conversationStub{}, Brains: brainStub{}, Runs: runStub{}}, []config.MCPAgentToken{{AgentID: a.ID(), Token: decodeToken(t, "0101010101010101010101010101010101010101010101010101010101010101")}})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
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
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("token %q status = %d, want 401", token, resp.StatusCode)
		}
	}
}

func TestNewRejectsTokenForUnregisteredAgent(t *testing.T) {
	services := Services{Agents: agentStub{values: map[string]agent.Agent{}}, Projects: projectStub{}, Tasks: taskStub{}, Conversations: conversationStub{}, Brains: brainStub{}, Runs: runStub{}}
	_, err := New(context.Background(), services, []config.MCPAgentToken{{AgentID: "missing-agent", Token: decodeToken(t, "0101010101010101010101010101010101010101010101010101010101010101")}})
	if err == nil {
		t.Fatal("New() accepted a token for an unregistered Agent")
	}
}

func testAgent(t *testing.T, id string) agent.Agent {
	t.Helper()
	value, err := agent.NewConfigured(id, agent.Configuration{Name: id, Role: "worker", Runtime: agent.RuntimeMCP, Capabilities: []string{"agent.read", "project.read", "task.read", "task.transition", "task.complete", "conversation.read", "conversation.write", "brain.execute", "run.read"}})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func decodeToken(t *testing.T, value string) [32]byte {
	t.Helper()
	var out [32]byte
	decoded, err := hex.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	copy(out[:], decoded)
	return out
}
