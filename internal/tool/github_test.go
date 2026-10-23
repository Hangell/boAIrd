package tool

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"boAIrd/internal/capability"
)

func newGitHubTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, []string) {
	t.Helper()
	var authHeaders []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		authHeaders = append(authHeaders, request.Header.Get("Authorization"))
		handler(writer, request)
	}))
	t.Cleanup(server.Close)
	return server, authHeaders
}

func newGitHubEnforcer(t *testing.T, config GitHubConfig, auditor *memoryAuditor) *Enforcer {
	t.Helper()
	config.Enabled = true
	if config.ResolveRepository == nil {
		config.ResolveRepository = func(_ context.Context, projectID string) (string, error) {
			if projectID != "project-1" {
				return "", ErrDenied
			}
			return "https://github.com/acme/widgets", nil
		}
	}
	if config.ResolveCredential == nil {
		config.ResolveCredential = func(_ context.Context, agentID, projectID, credentialID string) (string, error) {
			if agentID != "agent-1" || projectID != "project-1" || credentialID != "cred-1" {
				return "", ErrDenied
			}
			return "ghp_test_token_1234567890", nil
		}
	}
	tools, err := NewGitHubTools(config)
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	for _, value := range tools {
		if err := registry.Register(value); err != nil {
			t.Fatal(err)
		}
	}
	enforcer, err := NewEnforcer(registry, auditor, ScopeAuthorizerFunc(func(_ context.Context, principal Principal, request Request) error {
		if principal.ID == "agent-1" && request.ResourceType == "github.repository" && request.ResourceID != "" {
			return nil
		}
		return ErrDenied
	}))
	if err != nil {
		t.Fatal(err)
	}
	return enforcer
}

func githubRequest(t *testing.T, toolName string, required capability.Capability, input any) Request {
	t.Helper()
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return Request{
		ToolName: toolName, ProjectID: "project-1", ResourceType: "github.repository", ResourceID: "https://github.com/acme/widgets",
		Principal: Principal{ID: "agent-1", Type: "agent", Capabilities: []capability.Capability{required}, ProjectIDs: []string{"project-1"}},
		Input:     encoded,
	}
}

func TestGitHubToolRequiresExplicitEnablement(t *testing.T) {
	if _, err := NewGitHubTools(GitHubConfig{}); !errors.Is(err, ErrGitHubDisabled) {
		t.Fatalf("NewGitHubTools() error = %v, want disabled", err)
	}
	if _, err := NewGitHubTools(GitHubConfig{Enabled: true}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("NewGitHubTools() error = %v, want missing resolvers", err)
	}
}

func TestGitHubIssueReadCreateUpdate(t *testing.T) {
	server, authHeaders := newGitHubTestServer(t, func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/issues"):
			_, _ = writer.Write([]byte(`[{"number":1,"title":"first","state":"open","html_url":"https://github.com/acme/widgets/issues/1"}]`))
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/issues"):
			_, _ = writer.Write([]byte(`{"number":2,"title":"second","state":"open","html_url":"https://github.com/acme/widgets/issues/2"}`))
		case request.Method == http.MethodPatch && strings.Contains(request.URL.Path, "/issues/2"):
			_, _ = writer.Write([]byte(`{"number":2,"state":"closed","html_url":"https://github.com/acme/widgets/issues/2"}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	})
	auditor := &memoryAuditor{}
	enforcer := newGitHubEnforcer(t, GitHubConfig{APIBaseURL: server.URL, Client: server.Client()}, auditor)

	encoded, err := enforcer.Execute(context.Background(), githubRequest(t, "github.issue.read", capability.GitHubIssueRead, GitHubIssueReadInput{Action: "list", State: "open", CredentialID: "cred-1"}))
	if err != nil {
		t.Fatalf("issue.read error = %v", err)
	}
	var readOutput GitHubIssueReadOutput
	if err := json.Unmarshal(encoded, &readOutput); err != nil || readOutput.Count != 1 || readOutput.Issues[0].Number != 1 {
		t.Fatalf("issue.read output = %#v, error = %v", readOutput, err)
	}

	encoded, err = enforcer.Execute(context.Background(), githubRequest(t, "github.issue.create", capability.GitHubIssueCreate, GitHubIssueCreateInput{Title: "second", CredentialID: "cred-1"}))
	if err != nil {
		t.Fatalf("issue.create error = %v", err)
	}
	var createOutput GitHubIssueCreateOutput
	if err := json.Unmarshal(encoded, &createOutput); err != nil || createOutput.Number != 2 {
		t.Fatalf("issue.create output = %#v, error = %v", createOutput, err)
	}

	encoded, err = enforcer.Execute(context.Background(), githubRequest(t, "github.issue.update", capability.GitHubIssueUpdate, GitHubIssueUpdateInput{Number: 2, State: "closed", CredentialID: "cred-1"}))
	if err != nil {
		t.Fatalf("issue.update error = %v", err)
	}
	var updateOutput GitHubIssueUpdateOutput
	if err := json.Unmarshal(encoded, &updateOutput); err != nil || updateOutput.State != "closed" {
		t.Fatalf("issue.update output = %#v, error = %v", updateOutput, err)
	}

	for _, header := range authHeaders {
		if header != "Bearer ghp_test_token_1234567890" {
			t.Fatalf("authorization header = %q, want managed credential token", header)
		}
	}
	events := auditor.snapshot()
	if len(events) != 6 {
		t.Fatalf("audit events = %d, want 6 (start+success per call)", len(events))
	}
	for _, event := range events {
		if strings.Contains(event.ResourceID, "ghp_") || strings.Contains(event.ResourceID, "token") {
			t.Fatalf("audit event leaks credential material: %#v", event)
		}
	}
}

func TestGitHubOperationsRequireSeparateCapabilities(t *testing.T) {
	server, _ := newGitHubTestServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`[]`))
	})
	enforcer := newGitHubEnforcer(t, GitHubConfig{APIBaseURL: server.URL, Client: server.Client()}, &memoryAuditor{})
	// issue.read with pr.read capability must be denied.
	if _, err := enforcer.Execute(context.Background(), githubRequest(t, "github.issue.read", capability.GitHubPRRead, GitHubIssueReadInput{Action: "list", CredentialID: "cred-1"})); !errors.Is(err, ErrDenied) {
		t.Fatalf("issue.read with pr.read capability error = %v, want denied", err)
	}
	// pr.merge with pr.review capability must be denied.
	if _, err := enforcer.Execute(context.Background(), githubRequest(t, "github.pr.merge", capability.GitHubPRReview, GitHubPRMergeInput{Number: 1, CredentialID: "cred-1"})); !errors.Is(err, ErrDenied) {
		t.Fatalf("pr.merge with pr.review capability error = %v, want denied", err)
	}
}

func TestGitHubMergeDeniedByDefaultAndHumanReviewGate(t *testing.T) {
	server, _ := newGitHubTestServer(t, func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"merged":true,"message":"merged"}`))
	})
	auditor := &memoryAuditor{}
	// Default: merge denied without AllowMerge.
	enforcer := newGitHubEnforcer(t, GitHubConfig{APIBaseURL: server.URL, Client: server.Client()}, auditor)
	if _, err := enforcer.Execute(context.Background(), githubRequest(t, "github.pr.merge", capability.GitHubPRMerge, GitHubPRMergeInput{Number: 1, CredentialID: "cred-1"})); !errors.Is(err, ErrExecution) {
		t.Fatalf("pr.merge with AllowMerge=false error = %v, want denied", err)
	}
	// Allowed but project requires review: without a task it is denied.
	enforcer = newGitHubEnforcer(t, GitHubConfig{
		APIBaseURL: server.URL, Client: server.Client(), AllowMerge: true,
		HumanReviewRequired: func(_ context.Context, projectID string) (bool, error) { return true, nil },
		TaskApproved: func(_ context.Context, projectID, taskID string) (bool, error) {
			return projectID == "project-1" && taskID == "approved-task", nil
		},
	}, auditor)
	request := githubRequest(t, "github.pr.merge", capability.GitHubPRMerge, GitHubPRMergeInput{Number: 1, CredentialID: "cred-1"})
	if _, err := enforcer.Execute(context.Background(), request); !errors.Is(err, ErrExecution) {
		t.Fatalf("pr.merge without task error = %v, want denied", err)
	}
	request.TaskID = "unapproved-task"
	if _, err := enforcer.Execute(context.Background(), request); !errors.Is(err, ErrExecution) {
		t.Fatalf("pr.merge with unapproved task error = %v, want denied", err)
	}
	request.TaskID = "approved-task"
	encoded, err := enforcer.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("pr.merge with approved task error = %v", err)
	}
	var output GitHubPRMergeOutput
	if err := json.Unmarshal(encoded, &output); err != nil || !output.Merged {
		t.Fatalf("pr.merge output = %#v, error = %v", output, err)
	}
}

func TestGitHubRejectsNonGitHubRepository(t *testing.T) {
	server, _ := newGitHubTestServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
	})
	enforcer := newGitHubEnforcer(t, GitHubConfig{
		APIBaseURL: server.URL, Client: server.Client(),
		ResolveRepository: func(_ context.Context, _ string) (string, error) { return "https://gitlab.com/acme/widgets", nil },
	}, &memoryAuditor{})
	if _, err := enforcer.Execute(context.Background(), githubRequest(t, "github.issue.read", capability.GitHubIssueRead, GitHubIssueReadInput{Action: "list", CredentialID: "cred-1"})); !errors.Is(err, ErrDenied) {
		t.Fatalf("non-GitHub repository error = %v, want denied", err)
	}
}

func TestGitHubRedactsCredentialFromErrors(t *testing.T) {
	server, _ := newGitHubTestServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte(`{"message":"Bad credentials"}`))
	})
	enforcer := newGitHubEnforcer(t, GitHubConfig{APIBaseURL: server.URL, Client: server.Client()}, &memoryAuditor{})
	_, err := enforcer.Execute(context.Background(), githubRequest(t, "github.issue.read", capability.GitHubIssueRead, GitHubIssueReadInput{Action: "list", CredentialID: "cred-1"}))
	if err == nil {
		t.Fatal("github.issue.read against 401 unexpectedly succeeded")
	}
	if strings.Contains(err.Error(), "ghp_test_token") || strings.Contains(err.Error(), "1234567890") {
		t.Fatalf("error leaks credential material: %v", err)
	}
}

func TestGitHubAuditIncludesTaskRunAndRepository(t *testing.T) {
	server, _ := newGitHubTestServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`[]`))
	})
	auditor := &memoryAuditor{}
	enforcer := newGitHubEnforcer(t, GitHubConfig{APIBaseURL: server.URL, Client: server.Client()}, auditor)
	request := githubRequest(t, "github.issue.read", capability.GitHubIssueRead, GitHubIssueReadInput{Action: "list", CredentialID: "cred-1"})
	request.TaskID = "task-1"
	request.RunID = "run-1"
	if _, err := enforcer.Execute(context.Background(), request); err != nil {
		t.Fatalf("issue.read error = %v", err)
	}
	events := auditor.snapshot()
	if len(events) != 2 || events[0].Outcome != "start" || events[1].Outcome != "success" {
		t.Fatalf("audit events = %#v", events)
	}
	for _, event := range events {
		if event.TaskID != "task-1" || event.RunID != "run-1" || event.ToolName != "github.issue.read" {
			t.Fatalf("audit event missing task/run/tool: %#v", event)
		}
		if event.ResourceType != "github.repository" || event.ResourceID != "acme/widgets" {
			t.Fatalf("audit event missing canonical repository: %#v", event)
		}
	}
}
