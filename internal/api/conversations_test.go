package api

import (
	"context"
	"encoding/hex"
	"encoding/json"
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
	userID     string
	before     string
	limit      int
}

func (s *conversationAPIStub) GetPage(_ context.Context, _ string, userID, before string, limit int) (conversation.Conversation, []conversation.Message, string, error) {
	s.userID, s.before, s.limit = userID, before, limit
	value, _ := conversation.New("c1", "p1", "t1")
	return value, []conversation.Message{s.message}, "m-oldest", nil
}
func (s *conversationAPIStub) Append(_ context.Context, _ string, senderType conversation.SenderType, senderID string, input conversation.MessageInput) (conversation.Message, error) {
	s.senderType, s.senderID = senderType, senderID
	return conversation.NewMessage("m1", "c1", senderType, senderID, input.Content, input.ContentType, time.Now().UTC())
}

func TestCreateConversationMessageUsesServerOwnedIdentity(t *testing.T) {
	manager := &conversationAPIStub{}
	mux := http.NewServeMux()
	registerConversationRoutes(mux, manager, testConversationToken(t), newLocalSessionStore())
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/t1/messages", strings.NewReader(`{"content":"hello","content_type":"text/plain"}`))
	request.Header.Set("Authorization", "Bearer "+testConversationTokenHex)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if manager.senderType != conversation.SenderUser || manager.senderID != "local-administrator" {
		t.Fatalf("sender = %s/%s, want server-owned local identity", manager.senderType, manager.senderID)
	}
}

func TestCreateConversationMessageRejectsCallerIdentity(t *testing.T) {
	manager := &conversationAPIStub{}
	mux := http.NewServeMux()
	registerConversationRoutes(mux, manager, testConversationToken(t), newLocalSessionStore())
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/t1/messages", strings.NewReader(`{"content":"hello","content_type":"text/plain","sender_id":"admin"}`))
	request.Header.Set("Authorization", "Bearer "+testConversationTokenHex)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if manager.senderID != "" {
		t.Fatalf("manager received caller-controlled sender: %q", manager.senderID)
	}
}

func TestConversationRoutesRequireBearerAuthentication(t *testing.T) {
	manager := &conversationAPIStub{}
	mux := http.NewServeMux()
	registerConversationRoutes(mux, manager, testConversationToken(t), newLocalSessionStore())
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		request := httptest.NewRequest(method, "/api/v1/tasks/t1/messages", strings.NewReader(`{"content":"hello","content_type":"text/plain"}`))
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized || response.Header().Get("WWW-Authenticate") == "" {
			t.Fatalf("%s status = %d, want 401 with bearer challenge", method, response.Code)
		}
	}
}

func TestConversationRoutesRejectIncorrectBearerToken(t *testing.T) {
	manager := &conversationAPIStub{}
	mux := http.NewServeMux()
	registerConversationRoutes(mux, manager, testConversationToken(t), newLocalSessionStore())
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/t1/messages", strings.NewReader(`{"content":"hello","content_type":"text/plain"}`))
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("0", 64))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
	if manager.senderID != "" {
		t.Fatalf("unauthenticated sender reached conversation service: %q", manager.senderID)
	}
}

func TestGetConversationMessagesUsesCursorAndLimit(t *testing.T) {
	manager := &conversationAPIStub{}
	mux := http.NewServeMux()
	registerConversationRoutes(mux, manager, testConversationToken(t), newLocalSessionStore())
	request := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/t1/messages?limit=25&before=m-last", nil)
	request.Header.Set("Authorization", "Bearer "+testConversationTokenHex)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if manager.userID != "local-administrator" || manager.before != "m-last" || manager.limit != 25 {
		t.Fatalf("GetPage() user=%q before=%q limit=%d", manager.userID, manager.before, manager.limit)
	}
	var page conversationResponse
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.NextBefore != "m-oldest" {
		t.Fatalf("next_before = %q, want m-oldest", page.NextBefore)
	}
}

const testConversationTokenHex = "0101010101010101010101010101010101010101010101010101010101010101"

func testConversationToken(t *testing.T) []byte {
	t.Helper()
	token, err := hex.DecodeString(testConversationTokenHex)
	if err != nil {
		t.Fatal(err)
	}
	return token
}
