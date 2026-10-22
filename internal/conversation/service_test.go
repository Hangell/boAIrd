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
	administrator := task.Actor{Type: task.ActorAdministrator, ID: project.LocalAdministratorID, Capabilities: []string{"task.create", "task.assign"}, ProjectIDs: []string{p.ID()}}
	work, err := tasks.Create(ctx, p.ID(), task.CreateInput{Title: "Discuss implementation", Reason: "requested"}, administrator)
	if err != nil {
		t.Fatal(err)
	}
	work, err = tasks.Assign(ctx, work.ID(), worker.ID(), "assign worker", administrator)
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
	userMessage, err := service.Append(ctx, work.ID(), conversation.SenderUser, project.LocalAdministratorID, conversation.MessageInput{Content: "human reply", ContentType: "text/plain"})
	if err != nil {
		t.Fatalf("authorized local administrator append: %v", err)
	}

	value, messages, nextBefore, err := service.GetPage(ctx, work.ID(), project.LocalAdministratorID, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if value.TaskID() != work.ID() || value.ProjectID() != p.ID() || len(messages) != 1 || nextBefore == "" {
		t.Fatalf("conversation retrieval = task %q project %q messages %+v", value.TaskID(), value.ProjectID(), messages)
	}
	older, olderMessages, olderNext, err := service.GetPage(ctx, work.ID(), project.LocalAdministratorID, nextBefore, 1)
	if err != nil {
		t.Fatal(err)
	}
	if older.ID() != value.ID() || len(olderMessages) != 1 || olderNext != "" {
		t.Fatalf("older page = conversation %q messages %+v cursor %q", older.ID(), olderMessages, olderNext)
	}
	messageIDs := map[string]bool{messages[0].ID(): true, olderMessages[0].ID(): true}
	if len(messageIDs) != 2 || !messageIDs[message.ID()] || !messageIDs[userMessage.ID()] {
		t.Fatalf("cursor pages returned message IDs %v, want both messages", messageIDs)
	}
	var agentMessage conversation.Message
	if messages[0].SenderID() == worker.ID() {
		agentMessage = messages[0]
	} else {
		agentMessage = olderMessages[0]
	}
	if string(agentMessage.Metadata()) != string(message.Metadata()) {
		t.Fatalf("metadata changed during persistence: %s", agentMessage.Metadata())
	}
	if _, _, _, err := service.GetPage(ctx, work.ID(), "unverified-human", "", 10); !errors.Is(err, conversation.ErrForbidden) {
		t.Fatalf("unverified user read error = %v, want forbidden", err)
	}
	if _, agentMessages, _, err := service.GetPageForAgent(ctx, work.ID(), worker.ID(), "", 10); err != nil || len(agentMessages) != 2 {
		t.Fatalf("assigned agent page = %d messages, error %v; want two messages", len(agentMessages), err)
	}
	if _, _, _, err := service.GetPageForAgent(ctx, work.ID(), "another-agent", "", 10); !errors.Is(err, conversation.ErrForbidden) {
		t.Fatalf("unassigned agent read error = %v, want forbidden", err)
	}
	if _, _, _, err := service.GetPage(ctx, work.ID(), project.LocalAdministratorID, "missing-cursor", 10); !errors.Is(err, conversation.ErrInvalidCursor) {
		t.Fatalf("invalid cursor error = %v, want invalid cursor", err)
	}
	if _, err := service.Append(ctx, work.ID(), conversation.SenderAgent, "another-agent", conversation.MessageInput{Content: "no", ContentType: "text/plain"}); !errors.Is(err, conversation.ErrForbidden) {
		t.Fatalf("unassigned agent append error = %v, want forbidden", err)
	}
}

var _ conversation.BrainReader = (*storage.BrainRepository)(nil)
