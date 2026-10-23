package storage

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"boAIrd/internal/agent"
	"boAIrd/internal/credential"
	"boAIrd/internal/project"
)

func TestCredentialRepositoryPersistsCiphertextAndListsMetadata(t *testing.T) {
	database, err := openTestDatabase(t, context.Background(), filepath.Join(t.TempDir(), "boaird.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	repository, err := NewCredentialRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	record := credential.Record{ID: "id-1", Name: "test", Provider: "openai", SecretCiphertext: []byte("ciphertext-bytes-not-plaintext")}
	if err := repository.Create(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	loaded, err := repository.Get(context.Background(), record.ID)
	if err != nil || string(loaded.SecretCiphertext) != string(record.SecretCiphertext) {
		t.Fatalf("Get() = %#v, %v", loaded, err)
	}
	list, err := repository.List(context.Background())
	if err != nil || len(list) != 1 || len(list[0].SecretCiphertext) != 0 {
		t.Fatalf("List() = %#v, %v", list, err)
	}
	if err := repository.Delete(context.Background(), record.ID); err != nil {
		t.Fatal(err)
	}
	if err := repository.Delete(context.Background(), record.ID); err != credential.ErrNotFound {
		t.Fatalf("second Delete() error = %v", err)
	}
}

func TestCredentialManagerPersistsOnlyCiphertext(t *testing.T) {
	database, err := openTestDatabase(t, context.Background(), filepath.Join(t.TempDir(), "boaird.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	repository, err := NewCredentialRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := credential.NewManager(repository, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	secret := "fixture-fake-provider-secret-123456"
	metadata, err := manager.Create(context.Background(), "test", "openai", secret)
	if err != nil {
		t.Fatal(err)
	}
	var persisted []byte
	if err := database.QueryRow("SELECT secret_ciphertext FROM credentials WHERE id = ?", metadata.ID).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(persisted), secret) {
		t.Fatal("database contains plaintext credential")
	}
	ctx := context.Background()
	projectRepository, err := NewProjectRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	projects, err := project.NewManager(projectRepository)
	if err != nil {
		t.Fatal(err)
	}
	p, err := projects.Create(ctx, project.CreateInput{Name: "Credential scope"})
	if err != nil {
		t.Fatal(err)
	}
	agentRepository, err := NewAgentRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	agents, err := agent.NewManager(agentRepository)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := agents.Create(ctx, agent.CreateInput{Name: "Worker", Role: "worker", Runtime: agent.RuntimeAPI, Provider: "openai", Model: "test-model", CredentialID: metadata.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := agents.AddProject(ctx, worker.ID(), p.ID(), project.LocalAdministratorID); err != nil {
		t.Fatal(err)
	}
	scope := credential.Scope{AgentID: worker.ID(), ProjectID: p.ID()}
	resolved, err := manager.Resolve(ctx, metadata.ID, scope)
	if err != nil || resolved != secret {
		t.Fatalf("Resolve() = %q, %v", resolved, err)
	}
	if _, err := manager.Resolve(ctx, metadata.ID, credential.Scope{AgentID: "other-agent", ProjectID: p.ID()}); err != credential.ErrCredentialUnavailable {
		t.Fatalf("cross-agent Resolve() error = %v, want unavailable", err)
	}
	if _, err := manager.Resolve(ctx, metadata.ID, credential.Scope{AgentID: worker.ID(), ProjectID: "other-project"}); err != credential.ErrCredentialUnavailable {
		t.Fatalf("cross-project Resolve() error = %v, want unavailable", err)
	}
	if _, err := database.ExecContext(ctx, "UPDATE credentials SET provider='anthropic' WHERE id=?", metadata.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Resolve(ctx, metadata.ID, scope); err != credential.ErrCredentialUnavailable {
		t.Fatalf("provider mismatch Resolve() error = %v, want unavailable", err)
	}
	if _, err := database.ExecContext(ctx, "UPDATE credentials SET provider='openai' WHERE id=?", metadata.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO credentials(id, name, provider, secret_ciphertext) VALUES ('replacement', 'Replacement', 'openai', ?)`, make([]byte, 28)); err != nil {
		t.Fatal(err)
	}
	replacementID := "replacement"
	if _, err := agents.Update(ctx, worker.ID(), agent.UpdateInput{CredentialID: &replacementID}); err != nil {
		t.Fatal(err)
	}
	var scopeAuditCount int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE project_id=? AND resource_type='agent' AND resource_id=? AND event_type='agent.credential.changed' AND actor_id=?`,
		p.ID(), worker.ID(), project.LocalAdministratorID).Scan(&scopeAuditCount); err != nil {
		t.Fatal(err)
	}
	if scopeAuditCount != 1 {
		t.Fatalf("credential scope change audit count = %d, want 1", scopeAuditCount)
	}
	if err := agents.RemoveProject(ctx, worker.ID(), p.ID(), project.LocalAdministratorID); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Resolve(ctx, replacementID, scope); err != credential.ErrCredentialUnavailable {
		t.Fatalf("Resolve() after scope removal error = %v, want unavailable", err)
	}
}

func TestGitHubIntegrationCredentialBindingIsScopedAndAuditedAtomically(t *testing.T) {
	ctx := context.Background()
	database, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "boaird.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	projectRepository, err := NewProjectRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	projects, err := project.NewManager(projectRepository)
	if err != nil {
		t.Fatal(err)
	}
	firstProject, err := projects.Create(ctx, project.CreateInput{Name: "GitHub integration one"})
	if err != nil {
		t.Fatal(err)
	}
	secondProject, err := projects.Create(ctx, project.CreateInput{Name: "GitHub integration two"})
	if err != nil {
		t.Fatal(err)
	}
	agentRepository, err := NewAgentRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	agents, err := agent.NewManager(agentRepository)
	if err != nil {
		t.Fatal(err)
	}
	boundAgent, err := agents.Create(ctx, agent.CreateInput{Name: "Bound MCP", Role: "worker", Runtime: agent.RuntimeMCP, Capabilities: []string{"github.issue.read"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := agents.AddProject(ctx, boundAgent.ID(), firstProject.ID(), project.LocalAdministratorID); err != nil {
		t.Fatal(err)
	}
	unboundAgent, err := agents.Create(ctx, agent.CreateInput{Name: "Unbound MCP", Role: "worker", Runtime: agent.RuntimeMCP, Capabilities: []string{"github.issue.read"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := agents.AddProject(ctx, unboundAgent.ID(), firstProject.ID(), project.LocalAdministratorID); err != nil {
		t.Fatal(err)
	}
	credentialRepository, err := NewCredentialRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := credential.NewManager(credentialRepository, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	secret := "ghp_private_test_secret_123456"
	githubCredential, err := manager.Create(ctx, "GitHub", credential.ProviderGitHub, secret)
	if err != nil {
		t.Fatal(err)
	}
	openAICredential, err := manager.Create(ctx, "OpenAI", "openai", "sk-openai-test-secret-123456")
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.BindGitHub(ctx, firstProject.ID(), boundAgent.ID(), githubCredential.ID, project.LocalAdministratorID); err != nil {
		t.Fatalf("BindGitHub() error = %v", err)
	}
	resolved, err := manager.ResolveGitHub(ctx, credential.Scope{AgentID: boundAgent.ID(), ProjectID: firstProject.ID()})
	if err != nil || resolved != secret {
		t.Fatalf("ResolveGitHub() = %q, %v; want the bound token", resolved, err)
	}
	if _, err := manager.ResolveGitHub(ctx, credential.Scope{AgentID: unboundAgent.ID(), ProjectID: firstProject.ID()}); !errors.Is(err, credential.ErrCredentialUnavailable) {
		t.Fatalf("unbound Agent ResolveGitHub() error = %v, want unavailable", err)
	}
	if _, err := manager.ResolveGitHub(ctx, credential.Scope{AgentID: boundAgent.ID(), ProjectID: secondProject.ID()}); !errors.Is(err, credential.ErrCredentialUnavailable) {
		t.Fatalf("cross-project ResolveGitHub() error = %v, want unavailable", err)
	}
	if err := manager.BindGitHub(ctx, firstProject.ID(), boundAgent.ID(), openAICredential.ID, project.LocalAdministratorID); !errors.Is(err, credential.ErrCredentialUnavailable) {
		t.Fatalf("BindGitHub(OpenAI credential) error = %v, want unavailable", err)
	}
	if err := manager.BindGitHub(ctx, secondProject.ID(), boundAgent.ID(), githubCredential.ID, project.LocalAdministratorID); !errors.Is(err, credential.ErrCredentialUnavailable) {
		t.Fatalf("cross-project BindGitHub() error = %v, want unavailable", err)
	}
	if _, err := database.ExecContext(ctx, `CREATE TRIGGER reject_github_binding_audit BEFORE INSERT ON audit_events
		WHEN NEW.event_type = 'credential.integration.bound' BEGIN SELECT RAISE(ABORT, 'audit unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if err := manager.BindGitHub(ctx, firstProject.ID(), unboundAgent.ID(), githubCredential.ID, project.LocalAdministratorID); err == nil {
		t.Fatal("BindGitHub() succeeded while audit insertion was rejected")
	}
	if _, err := database.ExecContext(ctx, "DROP TRIGGER reject_github_binding_audit"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ResolveGitHub(ctx, credential.Scope{AgentID: unboundAgent.ID(), ProjectID: firstProject.ID()}); !errors.Is(err, credential.ErrCredentialUnavailable) {
		t.Fatalf("binding survived failed audit transaction: %v", err)
	}
	if err := manager.UnbindGitHub(ctx, firstProject.ID(), boundAgent.ID(), project.LocalAdministratorID); err != nil {
		t.Fatalf("UnbindGitHub() error = %v", err)
	}
	if _, err := manager.ResolveGitHub(ctx, credential.Scope{AgentID: boundAgent.ID(), ProjectID: firstProject.ID()}); !errors.Is(err, credential.ErrCredentialUnavailable) {
		t.Fatalf("ResolveGitHub() after revocation error = %v, want unavailable", err)
	}
	var events int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events
		WHERE event_type IN ('credential.integration.bound', 'credential.integration.unbound')`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 2 {
		t.Fatalf("integration binding audit events = %d, want 2", events)
	}
	var auditText string
	if err := database.QueryRowContext(ctx, `SELECT group_concat(event_type || ' ' || resource_id || ' ' || reason, ' ')
		FROM audit_events WHERE event_type LIKE 'credential.integration.%'`).Scan(&auditText); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(auditText, secret) || strings.Contains(auditText, githubCredential.ID) {
		t.Fatalf("integration audit contains secret or credential ID: %q", auditText)
	}
}
