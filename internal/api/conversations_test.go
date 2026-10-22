package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"boAIrd/internal/conversation"
)

type conversationAPIStub struct {
	senderType conversation.SenderType
	senderID   string
	message    conversation.Message
}

func (s *conversationAPIStub) Get(context.Context, string) (conversation.Conversation, []conversation.Message, error) {
	value, _ := conversation.New("c1", "p1", "t1")
	return value, []conversation.Message{s.message}, nil
}
func (s *conversationAPIStub) Append(_ context.Context, _ string, senderType conversation.SenderType, senderID string, input conversation.MessageInput) (conversation.Message, error) {
	s.senderType, s.senderID = senderType, senderID
	return conversation.NewMessage("m1", "c1", senderType, senderID, input.Content, input.ContentType, time.Now().UTC())
}

func TestCreateConversationMessageUsesServerOwnedIdentity(t *testing.T) {
	manager := &conversationAPIStub{}
	mux := http.NewServeMux()
	registerConversationRoutes(mux, manager)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/t1/messages", strings.NewReader(`{"content":"hello","content_type":"text/plain"}`))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if manager.senderType != conversation.SenderUser || manager.senderID != localConversationSenderID {
		t.Fatalf("sender = %s/%s, want server-owned local identity", manager.senderType, manager.senderID)
	}
}

func TestCreateConversationMessageRejectsCallerIdentity(t *testing.T) {
	manager := &conversationAPIStub{}
	mux := http.NewServeMux()
	registerConversationRoutes(mux, manager)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/t1/messages", strings.NewReader(`{"content":"hello","content_type":"text/plain","sender_id":"admin"}`))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if manager.senderID != "" {
		t.Fatalf("manager received caller-controlled sender: %q", manager.senderID)
	}
}
