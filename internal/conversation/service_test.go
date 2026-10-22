package conversation_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"boAIrd/internal/agent"
	"boAIrd/internal/brain"
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
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})

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
	brainAgent, err := agents.Create(ctx, agent.CreateInput{Name: "Brain", Role: "brain", Runtime: agent.RuntimeMCP, Capabilities: []string{"brain.execute"}})
	if err != nil {
		t.Fatal(err)
	}
	activeBrain, err := brain.New("brain-one", p.ID(), brainAgent.ID(), brain.AutonomyManual)
	if err != nil {
		t.Fatal(err)
	}
	if err := brainRepository.Create(ctx, activeBrain); err != nil {
		t.Fatal(err)
	}
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
	if _, _, _, err := service.GetPageForAgent(ctx, work.ID(), brainAgent.ID(), "", 10); !errors.Is(err, conversation.ErrForbidden) {
		t.Fatalf("active Brain used Worker assignment read path = %v, want forbidden", err)
	}
	brainMessage, err := service.Append(ctx, work.ID(), conversation.SenderBrain, brainAgent.ID(), conversation.MessageInput{Content: "review delegated work", ContentType: "text/plain"})
	if err != nil {
		t.Fatalf("active Brain append = %v", err)
	}
	brainConversation, brainMessages, _, err := service.GetPageForBrain(ctx, work.ID(), brainAgent.ID(), "", 10)
	if err != nil || brainConversation.TaskID() != work.ID() {
		t.Fatalf("active Brain read = conversation %q messages %d error %v", brainConversation.TaskID(), len(brainMessages), err)
	}
	foundBrainMessage := false
	for _, value := range brainMessages {
		if value.ID() == brainMessage.ID() && value.SenderType() == conversation.SenderBrain && value.SenderID() == brainAgent.ID() {
			foundBrainMessage = true
		}
	}
	if !foundBrainMessage {
		t.Fatalf("Brain-authored message missing or identity changed: %+v", brainMessages)
	}
	otherProject, err := projects.Create(ctx, project.CreateInput{Name: "Other conversation"})
	if err != nil {
		t.Fatal(err)
	}
	otherAdministrator := task.Actor{Type: task.ActorAdministrator, ID: project.LocalAdministratorID, Capabilities: []string{"task.create", "task.assign"}, ProjectIDs: []string{otherProject.ID()}}
	otherTask, err := tasks.Create(ctx, otherProject.ID(), task.CreateInput{Title: "Other task", Reason: "requested"}, otherAdministrator)
	if err != nil {
		t.Fatal(err)
	}
	otherTask, err = tasks.Assign(ctx, otherTask.ID(), worker.ID(), "assign worker", otherAdministrator)
	if err != nil {
		t.Fatal(err)
	}
	otherBrain, err := agents.Create(ctx, agent.CreateInput{Name: "Other Brain", Role: "brain", Runtime: agent.RuntimeMCP, Capabilities: []string{"brain.execute"}})
	if err != nil {
		t.Fatal(err)
	}
	otherActiveBrain, err := brain.New("brain-two", otherProject.ID(), otherBrain.ID(), brain.AutonomyManual)
	if err != nil {
		t.Fatal(err)
	}
	if err := brainRepository.Create(ctx, otherActiveBrain); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := service.GetPageForBrain(ctx, otherTask.ID(), brainAgent.ID(), "", 10); !errors.Is(err, conversation.ErrForbidden) {
		t.Fatalf("Brain from another project read conversation = %v, want forbidden", err)
	}
	if _, _, _, err := service.GetPage(ctx, work.ID(), project.LocalAdministratorID, "missing-cursor", 10); !errors.Is(err, conversation.ErrInvalidCursor) {
		t.Fatalf("invalid cursor error = %v, want invalid cursor", err)
	}
	if _, err := service.Append(ctx, work.ID(), conversation.SenderAgent, "another-agent", conversation.MessageInput{Content: "no", ContentType: "text/plain"}); !errors.Is(err, conversation.ErrForbidden) {
		t.Fatalf("unassigned agent append error = %v, want forbidden", err)
	}
}

var _ conversation.BrainReader = (*storage.BrainRepository)(nil)
