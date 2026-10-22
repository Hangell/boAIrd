// Package conversation defines provider-independent task conversations.
package conversation

import (
	"errors"
	"strings"
	"time"
)

type SenderType string

const (
	SenderUser   SenderType = "USER"
	SenderAgent  SenderType = "AGENT"
	SenderBrain  SenderType = "BRAIN"
	SenderSystem SenderType = "SYSTEM"
)

type Message struct {
	ID             string
	ConversationID string
	SenderType     SenderType
	SenderID       string
	Content        string
	ContentType    string
	CreatedAt      time.Time
}

type Conversation struct {
	ID        string
	ProjectID string
	TaskID    string
	Messages  []Message
}

func New(id, projectID, taskID string) (Conversation, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(projectID) == "" || strings.TrimSpace(taskID) == "" {
		return Conversation{}, errors.New("conversation, project, and task IDs are required")
	}
	return Conversation{ID: id, ProjectID: projectID, TaskID: taskID}, nil
}

func (c Conversation) AddMessage(message Message) (Conversation, error) {
	if strings.TrimSpace(message.ID) == "" || strings.TrimSpace(message.SenderID) == "" {
		return Conversation{}, errors.New("message ID and sender ID are required")
	}
	if message.ConversationID != c.ID {
		return Conversation{}, errors.New("message belongs to a different conversation")
	}
	if !validSender(message.SenderType) {
		return Conversation{}, errors.New("invalid message sender type")
	}
	if strings.TrimSpace(message.Content) == "" || strings.TrimSpace(message.ContentType) == "" {
		return Conversation{}, errors.New("message content and content type are required")
	}
	if message.CreatedAt.IsZero() {
		return Conversation{}, errors.New("message creation time is required")
	}
	c.Messages = append(append([]Message(nil), c.Messages...), message)
	return c, nil
}

func validSender(sender SenderType) bool {
	switch sender {
	case SenderUser, SenderAgent, SenderBrain, SenderSystem:
		return true
	default:
		return false
	}
}
