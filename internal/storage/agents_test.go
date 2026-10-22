package storage

import (
	"context"
	"path/filepath"
	"testing"

	"boAIrd/internal/agent"
)

func TestAgentRepositoryPersistsConfigurationAndRelations(t *testing.T) {
	database, err := OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "boaird.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Exec(`INSERT INTO credentials(id, name, provider, secret_ciphertext) VALUES ('cred-1', 'OpenAI', 'openai', ?)`, make([]byte, 28)); err != nil {
		t.Fatal(err)
	}
	repository, err := NewAgentRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := agent.NewManager(repository)
	if err != nil {
		t.Fatal(err)
	}
	value, err := manager.Create(context.Background(), agent.CreateInput{
		Name: "Backend Go", Role: "developer", Runtime: agent.RuntimeAPI, Provider: "OpenAI", Model: "gpt-test",
		CredentialID: "cred-1", Instructions: "Implement Go APIs", LanguagePreferences: []string{"pt-BR", "en-US"},
		Capabilities: []string{"filesystem.read", "github.issue.create"},
	})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := manager.Get(context.Background(), value.ID())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Provider() != "openai" || loaded.Model() != "gpt-test" || loaded.CredentialID() != "cred-1" || loaded.Status() != agent.StatusIdle {
		t.Fatalf("persisted configuration mismatch: %+v", loaded.Snapshot())
	}
	if got := len(loaded.Capabilities()); got != 2 {
		t.Fatalf("capability count = %d, want 2", got)
	}
	languages := loaded.LanguagePreferences()
	if len(languages) != 2 || languages[0] != "pt-BR" || languages[1] != "en-US" {
		t.Fatalf("language preferences = %#v", languages)
	}
	newName := "Backend Go Senior"
	updated, err := manager.Update(context.Background(), value.ID(), agent.UpdateInput{Name: &newName})
	if err != nil || updated.Name() != newName || updated.Provider() != "openai" {
		t.Fatalf("Update() = %+v, %v", updated.Snapshot(), err)
	}
	if got := updated.LanguagePreferences(); len(got) != 2 || got[0] != "pt-BR" || got[1] != "en-US" {
		t.Fatalf("language preferences after update = %#v", got)
	}
	listed, err := manager.List(context.Background(), 0, 0)
	if err != nil || len(listed) != 1 || listed[0].Name() != newName {
		t.Fatalf("List() = %#v, %v", listed, err)
	}
}

func TestAgentRepositoryEnforcesCredentialReferences(t *testing.T) {
	database, err := OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "boaird.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	repository, err := NewAgentRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := agent.NewManager(repository)
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.Create(context.Background(), agent.CreateInput{Name: "API agent", Role: "worker", Runtime: agent.RuntimeAPI, Provider: "openai", Model: "gpt-test", CredentialID: "missing"})
	if err != agent.ErrCredentialMismatch {
		t.Fatalf("create with missing credential reference error = %v", err)
	}
	if _, err := database.Exec(`INSERT INTO credentials(id, name, provider, secret_ciphertext) VALUES ('cred-1', 'OpenAI', 'openai', ?)`, make([]byte, 28)); err != nil {
		t.Fatal(err)
	}
	_, err = manager.Create(context.Background(), agent.CreateInput{Name: "Mismatch", Role: "worker", Runtime: agent.RuntimeAPI, Provider: "anthropic", Model: "claude", CredentialID: "cred-1"})
	if err != agent.ErrCredentialMismatch {
		t.Fatalf("create with mismatched provider credential error = %v", err)
	}
}
