package tool

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"boAIrd/internal/capability"
)

func TestShellToolRequiresExplicitEnablementAndAllowlist(t *testing.T) {
	if _, err := NewShellTool(ShellConfig{}); !errors.Is(err, ErrShellDisabled) {
		t.Fatalf("NewShellTool() error = %v, want disabled", err)
	}
	if _, err := NewShellTool(ShellConfig{Enabled: true, ResolveWorkspace: func(context.Context, string) (string, error) { return t.TempDir(), nil }}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("NewShellTool() error = %v, want missing allowlist", err)
	}
}

func TestShellToolUsesStructuredArgumentsBoundsOutputAndAudits(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(t.TempDir(), "injected")
	script := writeShellFixture(t, "#!/bin/sh\nprintf '%s\\n' \"$@\"\nprintf 'token=sk-1234567890abcdefgh\\n' >&2\n")
	auditor := &memoryAuditor{}
	enforcer, err := newShellEnforcer(t, script, root, auditor, 128, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	argument := "$(touch " + marker + ")"
	request := shellRequest(t, script, []string{argument}, root)
	encoded, err := enforcer.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var output ShellOutput
	if err := json.Unmarshal(encoded, &output); err != nil {
		t.Fatal(err)
	}
	if output.Stdout != argument+"\n" || output.Truncated {
		t.Fatalf("output = %#v", output)
	}
	if strings.Contains(output.Stderr, "sk-1234567890abcdefgh") {
		t.Fatalf("stderr contains an unredacted token: %q", output.Stderr)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("structured argument executed as shell text; marker stat error = %v", err)
	}
	events := auditor.snapshot()
	if len(events) != 2 || events[0].Outcome != "start" || events[1].Outcome != "success" || events[1].ToolName != "shell.execute" || events[1].ProjectID != "project-1" {
		t.Fatalf("audit events = %#v", events)
	}
	for _, event := range events {
		if strings.Contains(event.ResourceID, argument) {
			t.Fatalf("audit event contains command arguments: %#v", event)
		}
	}
}

func TestShellToolTruncatesOutput(t *testing.T) {
	root := t.TempDir()
	script := writeShellFixture(t, "#!/bin/sh\nprintf '"+strings.Repeat("A", 200)+"'\n")
	enforcer, err := newShellEnforcer(t, script, root, &memoryAuditor{}, 32, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := enforcer.Execute(context.Background(), shellRequest(t, script, nil, root))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var output ShellOutput
	if err := json.Unmarshal(encoded, &output); err != nil {
		t.Fatal(err)
	}
	if len(output.Stdout) != 32 || !output.Truncated {
		t.Fatalf("output length = %d, truncated = %t", len(output.Stdout), output.Truncated)
	}
}

func TestShellToolRejectsUnapprovedExecutableAndWorkspaceEscape(t *testing.T) {
	root := t.TempDir()
	script := writeShellFixture(t, "#!/bin/sh\nprintf executed\n")
	enforcer, err := newShellEnforcer(t, script, root, &memoryAuditor{}, 1024, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	request := shellRequest(t, script, nil, root)
	var value map[string]any
	if err := json.Unmarshal(request.Input, &value); err != nil {
		t.Fatal(err)
	}
	value["executable"] = "sh"
	request.Input, _ = json.Marshal(value)
	if _, err := enforcer.Execute(context.Background(), request); !errors.Is(err, ErrExecution) {
		t.Fatalf("unapproved executable error = %v, want execution failure", err)
	}

	request = shellRequest(t, script, nil, root)
	request.Input = json.RawMessage(`{"executable":"` + filepath.Base(script) + `","working_directory":"../"}`)
	if _, err := enforcer.Execute(context.Background(), request); !errors.Is(err, ErrExecution) {
		t.Fatalf("workspace traversal error = %v, want execution failure", err)
	}

	outside := t.TempDir()
	link := filepath.Join(root, "outside")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	request = shellRequest(t, script, []string{filepath.Join("outside", "file")}, root)
	if _, err := enforcer.Execute(context.Background(), request); !errors.Is(err, ErrExecution) {
		t.Fatalf("symlink escape error = %v, want execution failure", err)
	}
	request = shellRequest(t, script, []string{"outside"}, root)
	if _, err := enforcer.Execute(context.Background(), request); !errors.Is(err, ErrExecution) {
		t.Fatalf("bare symlink escape error = %v, want execution failure", err)
	}
}

func TestShellToolTimesOutAndKillsProcessGroup(t *testing.T) {
	root := t.TempDir()
	script := writeShellFixture(t, "#!/bin/sh\n/bin/sleep 5\n")
	enforcer, err := newShellEnforcer(t, script, root, &memoryAuditor{}, 1024, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err = enforcer.Execute(context.Background(), shellRequest(t, script, nil, root))
	if !errors.Is(err, ErrTimedOut) {
		t.Fatalf("Execute() error = %v, want timeout", err)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("timed out process took %s to stop", time.Since(start))
	}
}

func newShellEnforcer(t *testing.T, script, root string, auditor *memoryAuditor, outputLimit int, timeout time.Duration) (*Enforcer, error) {
	t.Helper()
	value, err := NewShellTool(ShellConfig{
		Enabled: true, AllowedExecutables: []string{script}, Timeout: timeout, MaxOutputBytes: outputLimit,
		ResolveWorkspace: func(_ context.Context, projectID string) (string, error) {
			if projectID != "project-1" {
				return "", ErrDenied
			}
			return root, nil
		},
	})
	if err != nil {
		return nil, err
	}
	registry := NewRegistry()
	if err := registry.Register(value); err != nil {
		return nil, err
	}
	return NewEnforcer(registry, auditor, ScopeAuthorizerFunc(func(_ context.Context, principal Principal, request Request) error {
		if principal.ID == "agent-1" && request.ResourceType == "workspace" && request.ResourceID == request.ProjectID {
			return nil
		}
		return ErrDenied
	}))
}

func shellRequest(t *testing.T, executable string, arguments []string, root string) Request {
	t.Helper()
	request := Request{
		ToolName: "shell.execute", ProjectID: "project-1", ResourceType: "workspace", ResourceID: "project-1",
		Principal: Principal{ID: "agent-1", Type: "agent", Capabilities: []capability.Capability{capability.ShellExecute}, ProjectIDs: []string{"project-1"}},
	}
	request.Input, _ = json.Marshal(ShellInput{Executable: filepath.Base(executable), Arguments: arguments})
	return request
}

func writeShellFixture(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture")
	if err := os.WriteFile(path, []byte(contents), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}
