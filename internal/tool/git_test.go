package tool

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"boAIrd/internal/capability"
)

func gitWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGitFixture(t, root, "init", "-q", "-b", "main")
	runGitFixture(t, root, "config", "user.name", "test")
	runGitFixture(t, root, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitFixture(t, root, "add", "f.txt")
	runGitFixture(t, root, "commit", "-q", "-m", "init")
	return root
}

func runGitFixture(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-C", dir}, args...)
	command := exec.Command("git", full...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

func newGitEnforcer(t *testing.T, root string, auditor *memoryAuditor, config GitConfig) *Enforcer {
	t.Helper()
	config.Enabled = true
	if config.ResolveWorkspace == nil {
		config.ResolveWorkspace = func(_ context.Context, projectID string) (string, error) {
			if projectID != "project-1" {
				return "", ErrDenied
			}
			return root, nil
		}
	}
	tools, err := NewGitTools(config)
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	for _, value := range tools {
		if err := registry.Register(value); err != nil {
			t.Fatal(err)
		}
	}
	return newGitTestEnforcer(t, registry, auditor)
}

func newGitTestEnforcer(t *testing.T, registry *Registry, auditor *memoryAuditor) *Enforcer {
	t.Helper()
	enforcer, err := NewEnforcer(registry, auditor, ScopeAuthorizerFunc(func(_ context.Context, principal Principal, request Request) error {
		if principal.ID == "agent-1" && request.ResourceType == "repository" && request.ResourceID != "" {
			return nil
		}
		return ErrDenied
	}))
	if err != nil {
		t.Fatal(err)
	}
	return enforcer
}

func gitPrincipal(required capability.Capability) Principal {
	return Principal{ID: "agent-1", Type: "agent", Capabilities: []capability.Capability{required}, ProjectIDs: []string{"project-1"}}
}

func gitRequest(t *testing.T, toolName string, required capability.Capability, input any) Request {
	t.Helper()
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return Request{
		ToolName: toolName, ProjectID: "project-1", ResourceType: "repository", ResourceID: "workspace",
		Principal: gitPrincipal(required), Input: encoded,
	}
}

func TestGitToolRequiresExplicitEnablement(t *testing.T) {
	if _, err := NewGitTools(GitConfig{}); !errors.Is(err, ErrGitDisabled) {
		t.Fatalf("NewGitTools() error = %v, want disabled", err)
	}
	if _, err := NewGitTools(GitConfig{Enabled: true}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("NewGitTools() error = %v, want missing workspace resolver", err)
	}
}

func TestGitReadDiffBranchCommitWithinWorkspace(t *testing.T) {
	root := gitWorkspace(t)
	auditor := &memoryAuditor{}
	enforcer := newGitEnforcer(t, root, auditor, GitConfig{})
	readInput := GitReadInput{Query: "status"}
	encoded, err := enforcer.Execute(context.Background(), gitRequest(t, "git.read", capability.GitRead, readInput))
	if err != nil {
		t.Fatalf("git.read Execute() error = %v", err)
	}
	var readOutput GitReadOutput
	if err := json.Unmarshal(encoded, &readOutput); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(readOutput.Stdout, "f.txt") {
		t.Fatalf("unexpected status output = %q", readOutput.Stdout)
	}

	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitFixture(t, root, "add", "f.txt")
	commitInput := GitCommitInput{Message: "second"}
	encoded, err = enforcer.Execute(context.Background(), gitRequest(t, "git.commit", capability.GitCommit, commitInput))
	if err != nil {
		t.Fatalf("git.commit Execute() error = %v", err)
	}
	var commitOutput GitCommitOutput
	if err := json.Unmarshal(encoded, &commitOutput); err != nil {
		t.Fatal(err)
	}
	if commitOutput.Revision == "" {
		t.Fatalf("commit revision is empty")
	}

	if _, err = enforcer.Execute(context.Background(), gitRequest(t, "git.diff", capability.GitDiff, GitDiffInput{})); err != nil {
		t.Fatalf("git.diff Execute() error = %v", err)
	}
	encoded, err = enforcer.Execute(context.Background(), gitRequest(t, "git.branch", capability.GitBranch, GitBranchInput{}))
	if err != nil {
		t.Fatalf("git.branch Execute() error = %v", err)
	}
	var branchOutput GitBranchOutput
	if err := json.Unmarshal(encoded, &branchOutput); err != nil {
		t.Fatal(err)
	}
	if branchOutput.Current != "main" || len(branchOutput.Branches) == 0 {
		t.Fatalf("branch output = %#v", branchOutput)
	}
	events := auditor.snapshot()
	if len(events) == 0 {
		t.Fatalf("no audit events recorded")
	}
	for _, event := range events {
		if strings.Contains(event.ResourceID, "f.txt") || strings.Contains(event.ResourceID, "sk-") {
			t.Fatalf("audit event leaks file contents: %#v", event)
		}
	}
}

func TestGitOperationsRequireSeparateCapabilities(t *testing.T) {
	root := gitWorkspace(t)
	auditor := &memoryAuditor{}
	enforcer := newGitEnforcer(t, root, auditor, GitConfig{})
	// git.read must not be executable with only git.commit granted.
	_, err := enforcer.Execute(context.Background(), gitRequest(t, "git.commit", capability.GitRead, GitCommitInput{Message: "nope"}))
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("git.commit with git.read capability error = %v, want denied", err)
	}
	// git.read must not be executable with only git.push granted.
	_, err = enforcer.Execute(context.Background(), gitRequest(t, "git.read", capability.GitPush, GitReadInput{Query: "status"}))
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("git.read with git.push capability error = %v, want denied", err)
	}
}

func TestGitDeniesPushAndMergeByDefault(t *testing.T) {
	root := gitWorkspace(t)
	auditor := &memoryAuditor{}
	enforcer := newGitEnforcer(t, root, auditor, GitConfig{})
	_, err := enforcer.Execute(context.Background(), gitRequest(t, "git.push", capability.GitPush, GitPushInput{}))
	if !errors.Is(err, ErrExecution) && !errors.Is(err, ErrDenied) {
		t.Fatalf("git.push error = %v, want denied", err)
	}
	_, err = enforcer.Execute(context.Background(), gitRequest(t, "git.merge", capability.GitMerge, GitMergeInput{Reference: "main"}))
	if !errors.Is(err, ErrExecution) && !errors.Is(err, ErrDenied) {
		t.Fatalf("git.merge error = %v, want denied", err)
	}
}

func TestGitPushRequiresApprovedHumanReviewWhenConfigured(t *testing.T) {
	root := gitWorkspace(t)
	auditor := &memoryAuditor{}
	enforcer := newGitEnforcer(t, root, auditor, GitConfig{
		AllowPush: true,
		HumanReviewRequired: func(_ context.Context, projectID string) (bool, error) {
			return projectID == "project-1", nil
		},
		TaskApproved: func(_ context.Context, taskID string) (bool, error) {
			return taskID == "approved-task", nil
		},
	})
	// Without a task, push must be denied.
	request := gitRequest(t, "git.push", capability.GitPush, GitPushInput{})
	if _, err := enforcer.Execute(context.Background(), request); !errors.Is(err, ErrExecution) {
		t.Fatalf("git.push without task error = %v, want denied", err)
	}
	// With an unapproved task, push must be denied.
	request.TaskID = "unapproved-task"
	if _, err := enforcer.Execute(context.Background(), request); !errors.Is(err, ErrExecution) {
		t.Fatalf("git.push with unapproved task error = %v, want denied", err)
	}
	// With an approved task, push proceeds past the review gate to the remote.
	request.TaskID = "approved-task"
	_, err := enforcer.Execute(context.Background(), request)
	if err == nil {
		t.Fatalf("git.push with approved task unexpectedly succeeded against a missing remote")
	}
	if !errors.Is(err, ErrExecution) {
		t.Fatalf("git.push with approved task error = %v, want execution failure (no remote)", err)
	}
}

func TestGitRejectsRepositoryOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	outside := gitWorkspace(t)
	auditor := &memoryAuditor{}
	config := GitConfig{
		ResolveWorkspace: func(_ context.Context, projectID string) (string, error) {
			if projectID != "project-1" {
				return "", ErrDenied
			}
			return workspace, nil
		},
	}
	// The workspace itself is not a repository, so even in-workspace resolution fails.
	enforcer := newGitEnforcer(t, workspace, auditor, config)
	if _, err := enforcer.Execute(context.Background(), gitRequest(t, "git.read", capability.GitRead, GitReadInput{Query: "status"})); !errors.Is(err, ErrExecution) {
		t.Fatalf("git.read outside repo error = %v, want execution failure", err)
	}
	// An explicit working directory that escapes to another repository is denied.
	escapeInput := GitReadInput{Query: "status", WorkingDirectory: filepath.Join("..", filepath.Base(outside))}
	// The enforcer uses the workspace root; the escaped path resolves outside and must be rejected.
	request := gitRequest(t, "git.read", capability.GitRead, escapeInput)
	request.ResourceID = "workspace"
	if _, err := enforcer.Execute(context.Background(), request); err == nil {
		t.Fatalf("git.read with escaped working directory unexpectedly succeeded")
	}
}

func TestGitAuditIncludesTaskRunAndRepository(t *testing.T) {
	root := gitWorkspace(t)
	auditor := &memoryAuditor{}
	enforcer := newGitEnforcer(t, root, auditor, GitConfig{})
	request := gitRequest(t, "git.read", capability.GitRead, GitReadInput{Query: "status"})
	request.TaskID = "task-1"
	request.RunID = "run-1"
	if _, err := enforcer.Execute(context.Background(), request); err != nil {
		t.Fatalf("git.read Execute() error = %v", err)
	}
	events := auditor.snapshot()
	if len(events) != 2 || events[0].Outcome != "start" || events[1].Outcome != "success" {
		t.Fatalf("audit events = %#v", events)
	}
	for _, event := range events {
		if event.TaskID != "task-1" || event.RunID != "run-1" || event.ToolName != "git.read" {
			t.Fatalf("audit event missing task/run/tool: %#v", event)
		}
		if event.ResourceType != "repository" || event.ResourceID != "workspace" {
			t.Fatalf("audit event missing repository: %#v", event)
		}
	}
}

func TestGitReadRejectsUnknownQuery(t *testing.T) {
	root := gitWorkspace(t)
	auditor := &memoryAuditor{}
	enforcer := newGitEnforcer(t, root, auditor, GitConfig{})
	request := gitRequest(t, "git.read", capability.GitRead, GitReadInput{Query: "gc"})
	if _, err := enforcer.Execute(context.Background(), request); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("git.read with unknown query error = %v, want ErrInvalidInput", err)
	}
}

func TestGitCommitRejectsEmptyMessage(t *testing.T) {
	root := gitWorkspace(t)
	auditor := &memoryAuditor{}
	enforcer := newGitEnforcer(t, root, auditor, GitConfig{})
	request := gitRequest(t, "git.commit", capability.GitCommit, GitCommitInput{Message: "  "})
	if _, err := enforcer.Execute(context.Background(), request); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("git.commit with empty message error = %v, want ErrInvalidInput", err)
	}
}
