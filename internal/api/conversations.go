package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"boAIrd/internal/conversation"
	"boAIrd/internal/task"
)

const maximumConversationRequestSize = (1 << 20) + (64 << 10) + (4 << 10)

type conversationAPIManager interface {
	GetPage(context.Context, string, string, string, int) (conversation.Conversation, []conversation.Message, string, error)
	Append(context.Context, string, conversation.SenderType, string, conversation.MessageInput) (conversation.Message, error)
}

type createMessageRequest struct {
	Content     string          `json:"content"`
	ContentType string          `json:"content_type"`
	Metadata    json.RawMessage `json:"metadata"`
}

type conversationMessageResponse struct {
	ID             string                  `json:"id"`
	ConversationID string                  `json:"conversation_id"`
	SenderType     conversation.SenderType `json:"sender_type"`
	SenderID       string                  `json:"sender_id"`
	Content        string                  `json:"content"`
	ContentType    string                  `json:"content_type"`
	Metadata       json.RawMessage         `json:"metadata"`
	CreatedAt      time.Time               `json:"created_at"`
}

type conversationResponse struct {
	ID         string                        `json:"id"`
	ProjectID  string                        `json:"project_id"`
	TaskID     string                        `json:"task_id"`
	Messages   []conversationMessageResponse `json:"messages"`
	NextBefore string                        `json:"next_before,omitempty"`
}

func registerConversationRoutes(mux *http.ServeMux, manager conversationAPIManager, localAuthToken []byte, sessions *localSessionStore) {
	mux.HandleFunc("GET /api/v1/tasks/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		userID, ok := localConversationPrincipal(r, localAuthToken, sessions)
		if !ok {
			writeConversationUnauthorized(w)
			return
		}
		limit, err := queryInteger(r, "limit", 0)
		if err != nil || (r.URL.Query().Has("limit") && limit == 0) || limit > 100 {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "limit must be an integer between 1 and 100")
			return
		}
		value, messages, nextBefore, err := manager.GetPage(r.Context(), r.PathValue("id"), userID, r.URL.Query().Get("before"), limit)
		if err != nil {
			writeConversationError(w, err)
			return
		}
		items := make([]conversationMessageResponse, 0, len(messages))
		for _, message := range messages {
			items = append(items, toConversationMessage(message))
		}
		writeJSON(w, http.StatusOK, conversationResponse{ID: value.ID(), ProjectID: value.ProjectID(), TaskID: value.TaskID(), Messages: items, NextBefore: nextBefore})
	})

	mux.HandleFunc("POST /api/v1/tasks/{id}/messages", func(w http.ResponseWriter, r *http.Request) {
		userID, ok := localConversationPrincipal(r, localAuthToken, sessions)
		if !ok {
			writeConversationUnauthorized(w)
			return
		}
		var request createMessageRequest
		if err := decodeJSONWithLimit(w, r, &request, maximumConversationRequestSize); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		message, err := manager.Append(r.Context(), r.PathValue("id"), conversation.SenderUser, userID,
			conversation.MessageInput{Content: request.Content, ContentType: request.ContentType, Metadata: request.Metadata})
		if err != nil {
			writeConversationError(w, err)
			return
		}
		w.Header().Set("Location", "/api/v1/tasks/"+r.PathValue("id")+"/messages")
		writeJSON(w, http.StatusCreated, toConversationMessage(message))
	})
}

func toConversationMessage(value conversation.Message) conversationMessageResponse {
	return conversationMessageResponse{ID: value.ID(), ConversationID: value.ConversationID(), SenderType: value.SenderType(),
		SenderID: value.SenderID(), Content: value.Content(), ContentType: value.ContentType(), Metadata: value.Metadata(), CreatedAt: value.CreatedAt()}
}

func writeConversationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, conversation.ErrNotFound), errors.Is(err, task.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", "task conversation not found")
	case errors.Is(err, task.ErrProjectNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", "project not found")
	case errors.Is(err, conversation.ErrForbidden):
		writeAPIError(w, http.StatusForbidden, "forbidden", "sender is not authorized for this task")
	case errors.Is(err, conversation.ErrInvalidInput):
		writeAPIError(w, http.StatusBadRequest, "invalid_message", err.Error())
	case errors.Is(err, conversation.ErrInvalidCursor):
		writeAPIError(w, http.StatusBadRequest, "invalid_cursor", "message cursor does not belong to this conversation")
	case errors.Is(err, conversation.ErrProjectArchived):
		writeAPIError(w, http.StatusConflict, "task_unavailable", "task project is unavailable")
	default:
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
