package conversation_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"boAIrd/internal/agent"
	"boAIrd/internal/conversation"
	"boAIrd/internal/project"
	"boAIrd/internal/storage"
	"boAIrd/internal/task"
)

func TestConversationPersistsStructuredMessagesWithScopedActorsAndRedaction(t *testing.T) {
	ctx := context.Background()
	database, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "conversation.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	projectRepository, _ := storage.NewProjectRepository(database)
	projects, err := project.NewManager(projectRepository)
	if err != nil {
		t.Fatal(err)
	}
	p, err := projects.Create(ctx, project.CreateInput{Name: "Conversation"})
	if err != nil {
		t.Fatal(err)
	}

	agentRepository, _ := storage.NewAgentRepository(database)
	agents, err := agent.NewManager(agentRepository)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := agents.Create(ctx, agent.CreateInput{Name: "Worker", Role: "worker", Runtime: agent.RuntimeLocal})
	if err != nil {
		t.Fatal(err)
	}

	taskRepository, _ := storage.NewTaskRepository(database)
	brainRepository, _ := storage.NewBrainRepository(database)
	tasks, err := task.NewService(taskRepository, projects, agents, brainRepository)
	if err != nil {
		t.Fatal(err)
	}
	work, err := tasks.Create(ctx, p.ID(), task.CreateInput{Title: "Discuss implementation", Reason: "requested"}, task.Actor{Type: task.ActorLocalAPI, ID: "local-api"})
	if err != nil {
		t.Fatal(err)
	}
	work, err = tasks.Assign(ctx, work.ID(), worker.ID(), "assign worker", task.Actor{Type: task.ActorLocalAPI, ID: "local-api"})
	if err != nil {
		t.Fatal(err)
	}

	conversationRepository, err := storage.NewConversationRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	service, err := conversation.NewService(conversationRepository, tasks, projects, agents, brainRepository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Append(ctx, work.ID(), conversation.SenderUser, "unverified-human", conversation.MessageInput{Content: "hello", ContentType: "text/plain"}); !errors.Is(err, conversation.ErrForbidden) {
		t.Fatalf("unverified user append error = %v, want forbidden", err)
	}

	structured := `{"kind":"status","text":"Bearer abcdef1234567890","secret":"top-secret"}`
	metadata := json.RawMessage(`{"api_key":"sk-proj-abcdefghijklmnopqrstuvwxyz","tags":["safe",{"nested":{"password":"private"}}]}`)
	message, err := service.Append(ctx, work.ID(), conversation.SenderAgent, worker.ID(), conversation.MessageInput{
		Content: structured, ContentType: "application/json", Metadata: []byte(metadata),
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(message.Content(), "abcdef1234567890") || strings.Contains(message.Content(), "top-secret") {
		t.Fatalf("message content was not redacted: %s", message.Content())
	}
	if !json.Valid([]byte(message.Content())) {
		t.Fatalf("redaction corrupted structured message JSON: %s", message.Content())
	}
	if strings.Contains(string(message.Metadata()), "sk-proj-abcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("message metadata was not redacted: %s", message.Metadata())
	}
	if strings.Contains(string(message.Metadata()), "private") {
		t.Fatalf("nested message metadata was not redacted: %s", message.Metadata())
	}

	value, messages, err := service.Get(ctx, work.ID())
	if err != nil {
		t.Fatal(err)
	}
	if value.TaskID() != work.ID() || value.ProjectID() != p.ID() || len(messages) != 1 || messages[0].SenderID() != worker.ID() {
		t.Fatalf("conversation retrieval = task %q project %q messages %+v", value.TaskID(), value.ProjectID(), messages)
	}
	if string(messages[0].Metadata()) != string(message.Metadata()) {
		t.Fatalf("metadata changed during persistence: %s", messages[0].Metadata())
	}
	if _, err := service.Append(ctx, work.ID(), conversation.SenderAgent, "another-agent", conversation.MessageInput{Content: "no", ContentType: "text/plain"}); !errors.Is(err, conversation.ErrForbidden) {
		t.Fatalf("unassigned agent append error = %v, want forbidden", err)
	}
}

var _ conversation.BrainReader = (*storage.BrainRepository)(nil)
