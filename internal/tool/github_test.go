package tool

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
		config.ResolveCredential = func(_ context.Context, agentID, projectID, provider string) (string, error) {
			if agentID != "agent-1" || projectID != "project-1" || provider != "github" {
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
		case request.Method == http.MethodGet && strings.Contains(request.URL.Path, "/issues/2"):
			_, _ = writer.Write([]byte(`{"number":2,"title":"second","state":"open","html_url":"https://github.com/acme/widgets/issues/2"}`))
		case request.Method == http.MethodPatch && strings.Contains(request.URL.Path, "/issues/2"):
			_, _ = writer.Write([]byte(`{"number":2,"state":"closed","html_url":"https://github.com/acme/widgets/issues/2"}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	})
	auditor := &memoryAuditor{}
	enforcer := newGitHubEnforcer(t, GitHubConfig{APIBaseURL: server.URL, Client: server.Client()}, auditor)

	encoded, err := enforcer.Execute(context.Background(), githubRequest(t, "github.issue.read", capability.GitHubIssueRead, GitHubIssueReadInput{Action: "list", State: "open"}))
	if err != nil {
		t.Fatalf("issue.read error = %v", err)
	}
	var readOutput GitHubIssueReadOutput
	if err := json.Unmarshal(encoded, &readOutput); err != nil || readOutput.Count != 1 || readOutput.Issues[0].Number != 1 {
		t.Fatalf("issue.read output = %#v, error = %v", readOutput, err)
	}

	encoded, err = enforcer.Execute(context.Background(), githubRequest(t, "github.issue.create", capability.GitHubIssueCreate, GitHubIssueCreateInput{Title: "second"}))
	if err != nil {
		t.Fatalf("issue.create error = %v", err)
	}
	var createOutput GitHubIssueCreateOutput
	if err := json.Unmarshal(encoded, &createOutput); err != nil || createOutput.Number != 2 {
		t.Fatalf("issue.create output = %#v, error = %v", createOutput, err)
	}

	encoded, err = enforcer.Execute(context.Background(), githubRequest(t, "github.issue.update", capability.GitHubIssueUpdate, GitHubIssueUpdateInput{Number: 2, State: "closed"}))
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
	if _, err := enforcer.Execute(context.Background(), githubRequest(t, "github.issue.read", capability.GitHubPRRead, GitHubIssueReadInput{Action: "list"})); !errors.Is(err, ErrDenied) {
		t.Fatalf("issue.read with pr.read capability error = %v, want denied", err)
	}
	// pr.merge with pr.review capability must be denied.
	if _, err := enforcer.Execute(context.Background(), githubRequest(t, "github.pr.merge", capability.GitHubPRReview, GitHubPRMergeInput{Number: 1})); !errors.Is(err, ErrDenied) {
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
	if _, err := enforcer.Execute(context.Background(), githubRequest(t, "github.pr.merge", capability.GitHubPRMerge, GitHubPRMergeInput{Number: 1, ExpectedHeadSHA: strings.Repeat("a", 40)})); !errors.Is(err, ErrExecution) {
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
	request := githubRequest(t, "github.pr.merge", capability.GitHubPRMerge, GitHubPRMergeInput{Number: 1, ExpectedHeadSHA: strings.Repeat("a", 40)})
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
	if _, err := enforcer.Execute(context.Background(), githubRequest(t, "github.issue.read", capability.GitHubIssueRead, GitHubIssueReadInput{Action: "list"})); !errors.Is(err, ErrDenied) {
		t.Fatalf("non-GitHub repository error = %v, want denied", err)
	}
}

func TestGitHubSSHRepositoryRequiresExactHost(t *testing.T) {
	if owner, repo, err := parseGitHubRepository("git@github.com:acme/widgets.git"); err != nil || owner != "acme" || repo != "widgets" {
		t.Fatalf("parse valid SSH repository = (%q, %q), %v", owner, repo, err)
	}
	for _, value := range []string{"git@evilgithub.com:acme/widgets.git", "git@github.com.evil.invalid:acme/widgets.git"} {
		if _, _, err := parseGitHubRepository(value); err == nil {
			t.Errorf("parseGitHubRepository(%q) unexpectedly accepted a non-GitHub host", value)
		}
	}
}

func TestGitHubHTTPSRepositoryRejectsCredentialsAndURLDecorations(t *testing.T) {
	if owner, repo, err := parseGitHubRepository("https://github.com/acme/widgets.git"); err != nil || owner != "acme" || repo != "widgets" {
		t.Fatalf("parse valid HTTPS repository = (%q, %q), %v", owner, repo, err)
	}
	for _, value := range []string{
		"https://user:secret@github.com/acme/widgets",
		"https://github.com/acme/widgets?x=y",
		"https://github.com/acme/widgets?",
		"https://github.com/acme/widgets#fragment",
		"https://github.com/acme/widgets#",
		"https://github.com:443/acme/widgets",
		"https://github.com/acme/widgets/",
		"https://github.com/acme%2fwidgets",
		"https://github.com/acme/../widgets",
	} {
		if _, _, err := parseGitHubRepository(value); err == nil {
			t.Errorf("parseGitHubRepository(%q) unexpectedly accepted an ambiguous URL", value)
		}
	}
}

func TestGitHubHTTPClientDoesNotFollowRedirects(t *testing.T) {
	redirectTargetCalls := 0
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		redirectTargetCalls++
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(`{"number":1}`))
	}))
	t.Cleanup(target.Close)
	redirector := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, target.URL+"/repos/other/repo/issues/1", http.StatusFound)
	}))
	t.Cleanup(redirector.Close)
	client := githubClient{baseURL: redirector.URL, client: githubHTTPClient(GitHubConfig{Timeout: time.Second, Client: redirector.Client()})}
	if _, err := client.call(context.Background(), http.MethodGet, "/repos/acme/widgets/issues/1", "github-token", nil); !errors.Is(err, ErrGitHubPolicy) {
		t.Fatalf("redirected GitHub call error = %v, want policy rejection", err)
	}
	if redirectTargetCalls != 0 {
		t.Fatalf("redirect target received %d requests, want none", redirectTargetCalls)
	}
}

func TestGitHubHumanReviewLookupErrorsFailClosed(t *testing.T) {
	serverCalls := 0
	server, _ := newGitHubTestServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		serverCalls++
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"merged":true,"message":"merged"}`))
	})
	enforcer := newGitHubEnforcer(t, GitHubConfig{
		APIBaseURL: server.URL, Client: server.Client(), AllowMerge: true,
		HumanReviewRequired: func(context.Context, string) (bool, error) { return false, errors.New("policy read failed") },
		TaskApproved: func(context.Context, string, string) (bool, error) {
			t.Fatal("task approval must not be checked after policy lookup fails")
			return false, nil
		},
	}, &memoryAuditor{})
	request := githubRequest(t, "github.pr.merge", capability.GitHubPRMerge, GitHubPRMergeInput{Number: 1, ExpectedHeadSHA: strings.Repeat("a", 40)})
	if _, err := enforcer.Execute(context.Background(), request); !errors.Is(err, ErrExecution) {
		t.Fatalf("merge after Human Review lookup error = %v, want denial", err)
	}
	if serverCalls != 0 {
		t.Fatalf("GitHub API received %d merge requests after policy lookup failure", serverCalls)
	}
}

func TestGitHubIssueCapabilitiesExcludePullRequests(t *testing.T) {
	patchCalls := 0
	server, _ := newGitHubTestServer(t, func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/widgets/issues":
			_, _ = writer.Write([]byte(`[{"number":1,"title":"issue","state":"open"},{"number":2,"title":"pull request","state":"open","pull_request":{"url":"https://api.github.com/repos/acme/widgets/pulls/2"}}]`))
		case request.Method == http.MethodGet && request.URL.Path == "/repos/acme/widgets/issues/2":
			_, _ = writer.Write([]byte(`{"number":2,"title":"pull request","state":"open","pull_request":{"url":"https://api.github.com/repos/acme/widgets/pulls/2"}}`))
		case request.Method == http.MethodPatch && request.URL.Path == "/repos/acme/widgets/issues/2":
			patchCalls++
			_, _ = writer.Write([]byte(`{"number":2,"state":"closed"}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	})
	enforcer := newGitHubEnforcer(t, GitHubConfig{APIBaseURL: server.URL, Client: server.Client()}, &memoryAuditor{})
	encoded, err := enforcer.Execute(context.Background(), githubRequest(t, "github.issue.read", capability.GitHubIssueRead, GitHubIssueReadInput{Action: "list"}))
	if err != nil {
		t.Fatalf("list issues = %v", err)
	}
	var output GitHubIssueReadOutput
	if err := json.Unmarshal(encoded, &output); err != nil || output.Count != 1 || output.Issues[0].Number != 1 {
		t.Fatalf("issue list = %#v, error = %v; want PR excluded", output, err)
	}
	if _, err := enforcer.Execute(context.Background(), githubRequest(t, "github.issue.read", capability.GitHubIssueRead, GitHubIssueReadInput{Action: "get", Number: 2})); !errors.Is(err, ErrExecution) {
		t.Fatalf("read pull request through issue capability = %v, want denial", err)
	}
	if _, err := enforcer.Execute(context.Background(), githubRequest(t, "github.issue.update", capability.GitHubIssueUpdate, GitHubIssueUpdateInput{Number: 2, State: "closed"})); !errors.Is(err, ErrExecution) {
		t.Fatalf("update pull request through issue capability = %v, want denial", err)
	}
	if patchCalls != 0 {
		t.Fatalf("issue.update issued %d PATCH requests against a pull request", patchCalls)
	}
}

func TestGitHubMergePinsExpectedHeadSHA(t *testing.T) {
	expectedSHA := strings.Repeat("b", 40)
	server, _ := newGitHubTestServer(t, func(writer http.ResponseWriter, request *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode merge body: %v", err)
		}
		if body["sha"] != expectedSHA {
			t.Errorf("merge SHA = %q, want %q", body["sha"], expectedSHA)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"merged":true,"message":"merged"}`))
	})
	request := githubRequest(t, "github.pr.merge", capability.GitHubPRMerge, GitHubPRMergeInput{Number: 2, ExpectedHeadSHA: expectedSHA})
	_, err := runGitHubPRMerge(context.Background(), request, GitHubConfig{
		AllowMerge:          true,
		APIBaseURL:          server.URL,
		Client:              server.Client(),
		HumanReviewRequired: func(context.Context, string) (bool, error) { return false, nil },
	}, "acme", "widgets", "github-token", GitHubPRMergeInput{Number: 2, ExpectedHeadSHA: expectedSHA})
	if err != nil {
		t.Fatalf("merge with expected SHA = %v", err)
	}
}

func TestGitHubPullRequestReadExposesExpectedHeadSHA(t *testing.T) {
	const headSHA = "0123456789abcdef0123456789abcdef01234567"
	server, _ := newGitHubTestServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"number":2,"title":"change","state":"open","head":{"ref":"feature","sha":"` + headSHA + `"},"base":{"ref":"main","sha":"fedcba9876543210fedcba9876543210fedcba98"}}`))
	})
	enforcer := newGitHubEnforcer(t, GitHubConfig{APIBaseURL: server.URL, Client: server.Client()}, &memoryAuditor{})
	encoded, err := enforcer.Execute(context.Background(), githubRequest(t, "github.pr.read", capability.GitHubPRRead, GitHubPRReadInput{Action: "get", Number: 2}))
	if err != nil {
		t.Fatalf("github.pr.read = %v", err)
	}
	var output GitHubPRReadOutput
	if err := json.Unmarshal(encoded, &output); err != nil || output.Count != 1 || output.PullRequests[0].Head.SHA != headSHA {
		t.Fatalf("PR read output = %#v, error = %v; want head SHA %q", output, err, headSHA)
	}
}

func TestGitHubMergeRejectsChangedHead(t *testing.T) {
	server, _ := newGitHubTestServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusConflict)
		_, _ = writer.Write([]byte(`{"message":"Head sha does not match"}`))
	})
	input := GitHubPRMergeInput{Number: 4, ExpectedHeadSHA: strings.Repeat("c", 40)}
	request := githubRequest(t, "github.pr.merge", capability.GitHubPRMerge, input)
	_, err := runGitHubPRMerge(context.Background(), request, GitHubConfig{
		AllowMerge:          true,
		APIBaseURL:          server.URL,
		Client:              server.Client(),
		HumanReviewRequired: func(context.Context, string) (bool, error) { return false, nil },
	}, "acme", "widgets", "github-token", input)
	if !errors.Is(err, ErrGitHubPolicy) {
		t.Fatalf("merge after PR head changed = %v, want conflict rejection", err)
	}
}

func TestGitHubRedactsCredentialFromErrors(t *testing.T) {
	server, _ := newGitHubTestServer(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte(`{"message":"Bad credentials"}`))
	})
	enforcer := newGitHubEnforcer(t, GitHubConfig{APIBaseURL: server.URL, Client: server.Client()}, &memoryAuditor{})
	_, err := enforcer.Execute(context.Background(), githubRequest(t, "github.issue.read", capability.GitHubIssueRead, GitHubIssueReadInput{Action: "list"}))
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
	request := githubRequest(t, "github.issue.read", capability.GitHubIssueRead, GitHubIssueReadInput{Action: "list"})
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
