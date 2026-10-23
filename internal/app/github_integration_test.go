package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"boAIrd/internal/agent"
	"boAIrd/internal/brain"
	"boAIrd/internal/capability"
	"boAIrd/internal/conversation"
	"boAIrd/internal/credential"
	"boAIrd/internal/mcpserver"
	"boAIrd/internal/mcptoken"
	"boAIrd/internal/project"
	"boAIrd/internal/run"
	"boAIrd/internal/storage"
	"boAIrd/internal/task"
	governedtool "boAIrd/internal/tool"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type githubIntegrationTaskService struct{}

func (githubIntegrationTaskService) Get(context.Context, string) (task.Task, error) {
	return task.Task{}, task.ErrNotFound
}
func (githubIntegrationTaskService) List(context.Context, string, task.Status, string, int, int) ([]task.Task, error) {
	return nil, nil
}
func (githubIntegrationTaskService) AssignAtVersion(context.Context, string, string, int64, string, task.Actor) (task.Task, error) {
	return task.Task{}, task.ErrNotFound
}
func (githubIntegrationTaskService) Claim(context.Context, string, string, int64, string, task.Actor) (task.Task, error) {
	return task.Task{}, task.ErrNotFound
}
func (githubIntegrationTaskService) Transition(context.Context, string, task.Status, string, task.Actor) (task.Task, error) {
	return task.Task{}, task.ErrNotFound
}

type githubIntegrationConversationService struct{}

func (githubIntegrationConversationService) GetPageForAgent(context.Context, string, string, string, int) (conversation.Conversation, []conversation.Message, string, error) {
	return conversation.Conversation{}, nil, "", conversation.ErrNotFound
}
func (githubIntegrationConversationService) GetPageForBrain(context.Context, string, string, string, int) (conversation.Conversation, []conversation.Message, string, error) {
	return conversation.Conversation{}, nil, "", conversation.ErrNotFound
}
func (githubIntegrationConversationService) Append(context.Context, string, conversation.SenderType, string, conversation.MessageInput) (conversation.Message, error) {
	return conversation.Message{}, conversation.ErrNotFound
}

type githubIntegrationBrainService struct{}

func (githubIntegrationBrainService) Get(context.Context, string) (brain.Brain, error) {
	return brain.Brain{}, brain.ErrNotFound
}
func (githubIntegrationBrainService) Execute(context.Context, string, string, int) (brain.Execution, error) {
	return brain.Execution{}, brain.ErrNotFound
}

type githubIntegrationRunService struct{}

func (githubIntegrationRunService) Get(context.Context, string) (run.Run, error) {
	return run.Run{}, run.ErrNotFound
}

type githubBearerTransport struct {
	token string
	base  http.RoundTripper
}

func (t githubBearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	cloned := request.Clone(request.Context())
	cloned.Header = request.Header.Clone()
	cloned.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(cloned)
}

func TestMCPGitHubUsesBoundCredentialFromRealRepository(t *testing.T) {
	ctx := context.Background()
	database, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "boaird.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	projectRepository, err := storage.NewProjectRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	projectManager, err := project.NewManager(projectRepository)
	if err != nil {
		t.Fatal(err)
	}
	projectValue, err := projectManager.Create(ctx, project.CreateInput{Name: "GitHub MCP integration", RepositoryURL: "https://github.com/acme/widgets"})
	if err != nil {
		t.Fatal(err)
	}
	agentRepository, err := storage.NewAgentRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	agentManager, err := agent.NewManager(agentRepository)
	if err != nil {
		t.Fatal(err)
	}
	createMCPAgent := func(name string) agent.Agent {
		t.Helper()
		value, err := agentManager.Create(ctx, agent.CreateInput{
			Name: name, Role: "worker", Runtime: agent.RuntimeMCP,
			Capabilities: []string{string(capability.GitHubIssueRead)},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := agentManager.AddProject(ctx, value.ID(), projectValue.ID(), project.LocalAdministratorID); err != nil {
			t.Fatal(err)
		}
		return value
	}
	boundAgent := createMCPAgent("Bound GitHub MCP")
	unboundAgent := createMCPAgent("Unbound GitHub MCP")
	credentialRepository, err := storage.NewCredentialRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	credentialManager, err := credential.NewManager(credentialRepository, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	secret := "ghp_mcp_integration_secret_123456"
	metadata, err := credentialManager.Create(ctx, "GitHub integration", credential.ProviderGitHub, secret)
	if err != nil {
		t.Fatal(err)
	}
	if err := credentialManager.BindGitHub(ctx, projectValue.ID(), boundAgent.ID(), metadata.ID, project.LocalAdministratorID); err != nil {
		t.Fatalf("BindGitHub() error = %v", err)
	}
	tokenRepository, err := storage.NewMCPTokenRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	tokenManager, err := mcptoken.NewService(tokenRepository, agentManager)
	if err != nil {
		t.Fatal(err)
	}
	_, boundToken, err := tokenManager.Create(ctx, boundAgent.ID(), time.Hour, project.LocalAdministratorID)
	if err != nil {
		t.Fatal(err)
	}
	_, unboundToken, err := tokenManager.Create(ctx, unboundAgent.ID(), time.Hour, project.LocalAdministratorID)
	if err != nil {
		t.Fatal(err)
	}
	githubRequests := 0
	githubAuthorization := ""
	githubServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		githubRequests++
		githubAuthorization = request.Header.Get("Authorization")
		if request.Method != http.MethodGet || request.URL.Path != "/repos/acme/widgets/issues" {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`[{"number":1,"title":"authorized issue","state":"open"}]`))
	}))
	t.Cleanup(githubServer.Close)
	auditRepository, err := storage.NewAuditRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	registry := governedtool.NewRegistry()
	githubTools, err := governedtool.NewGitHubTools(governedtool.GitHubConfig{
		Enabled:    true,
		APIBaseURL: githubServer.URL,
		Client:     githubServer.Client(),
		ResolveRepository: func(ctx context.Context, projectID string) (string, error) {
			value, err := projectManager.Get(ctx, projectID)
			if err != nil {
				return "", err
			}
			return value.RepositoryURL(), nil
		},
		ResolveCredential: func(ctx context.Context, agentID, projectID, provider string) (string, error) {
			if provider != credential.ProviderGitHub {
				return "", credential.ErrCredentialUnavailable
			}
			return credentialManager.ResolveGitHub(ctx, credential.Scope{AgentID: agentID, ProjectID: projectID})
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range githubTools {
		if err := registry.Register(value); err != nil {
			t.Fatal(err)
		}
	}
	githubExecutor, err := governedtool.NewEnforcer(registry, auditRepository, githubScopeAuthorizer{projects: projectManager})
	if err != nil {
		t.Fatal(err)
	}
	mcpServer, err := mcpserver.New(mcpserver.Services{
		Agents: agentManager, Projects: projectManager, Tasks: githubIntegrationTaskService{},
		Conversations: githubIntegrationConversationService{}, Brains: githubIntegrationBrainService{},
		Runs: githubIntegrationRunService{}, GitHubTools: githubExecutor,
	}, tokenManager)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(mcpServer.Handler())
	t.Cleanup(server.Close)
	callTool := func(token string) (*mcp.CallToolResult, error) {
		t.Helper()
		client := mcp.NewClient(&mcp.Implementation{Name: "github-integration-test", Version: "1"}, nil)
		session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
			Endpoint:             server.URL,
			HTTPClient:           &http.Client{Transport: githubBearerTransport{token: token, base: http.DefaultTransport}},
			DisableStandaloneSSE: true,
		}, nil)
		if err != nil {
			return nil, err
		}
		defer func() { _ = session.Close() }()
		return session.CallTool(ctx, &mcp.CallToolParams{
			Name:      "boaird.github.issue.read",
			Arguments: map[string]any{"project_id": projectValue.ID(), "action": "list"},
		})
	}
	result, err := callTool(boundToken)
	if err != nil || result == nil || result.IsError {
		t.Fatalf("bound MCP GitHub call result=%+v error=%v", result, err)
	}
	if githubRequests != 1 || githubAuthorization != "Bearer "+secret {
		t.Fatalf("GitHub request count=%d authorization=%q; bound credential was not used", githubRequests, githubAuthorization)
	}
	encodedOutput, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodedOutput), secret) || strings.Contains(string(encodedOutput), metadata.ID) {
		t.Fatalf("MCP result exposed credential material: %s", encodedOutput)
	}
	unboundResult, unboundErr := callTool(unboundToken)
	if (unboundErr == nil && (unboundResult == nil || !unboundResult.IsError)) || githubRequests != 1 {
		t.Fatalf("unbound MCP Agent result=%+v error=%v GitHub requests=%d; wanted denial before HTTP", unboundResult, unboundErr, githubRequests)
	}
	auditEntries, err := auditRepository.ListEvents(ctx, projectValue.ID(), 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	encodedAudit, err := json.Marshal(auditEntries)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodedAudit), secret) || strings.Contains(string(encodedAudit), metadata.ID) {
		t.Fatalf("audit events exposed credential material: %s", encodedAudit)
	}
	if !strings.Contains(string(encodedAudit), "credential.integration.bound") || !strings.Contains(string(encodedAudit), "tool.executed") {
		t.Fatalf("audit events missing binding or tool execution evidence: %s", encodedAudit)
	}
	if _, err := credentialManager.ResolveGitHub(ctx, credential.Scope{AgentID: unboundAgent.ID(), ProjectID: projectValue.ID()}); err != credential.ErrCredentialUnavailable {
		t.Fatalf("unbound Agent credential resolution error = %v", err)
	}
}
