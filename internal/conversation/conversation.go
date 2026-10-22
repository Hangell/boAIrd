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
	id             string
	conversationID string
	senderType     SenderType
	senderID       string
	content        string
	contentType    string
	createdAt      time.Time
}

type Conversation struct {
	id        string
	projectID string
	taskID    string
	messages  []Message
}

func New(id, projectID, taskID string) (Conversation, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(projectID) == "" || strings.TrimSpace(taskID) == "" {
		return Conversation{}, errors.New("conversation, project, and task IDs are required")
	}
	return Conversation{id: id, projectID: projectID, taskID: taskID}, nil
}

func NewMessage(id, conversationID string, senderType SenderType, senderID, content, contentType string, createdAt time.Time) (Message, error) {
	message := Message{
		id: id, conversationID: conversationID, senderType: senderType, senderID: senderID,
		content: content, contentType: contentType, createdAt: createdAt,
	}
	if err := message.validate(); err != nil {
		return Message{}, err
	}
	return message, nil
}

func (m Message) ID() string               { return m.id }
func (m Message) ConversationID() string   { return m.conversationID }
func (m Message) SenderType() SenderType   { return m.senderType }
func (m Message) SenderID() string         { return m.senderID }
func (m Message) Content() string          { return m.content }
func (m Message) ContentType() string      { return m.contentType }
func (m Message) CreatedAt() time.Time     { return m.createdAt }
func (c Conversation) ID() string          { return c.id }
func (c Conversation) ProjectID() string   { return c.projectID }
func (c Conversation) TaskID() string      { return c.taskID }
func (c Conversation) Messages() []Message { return append([]Message(nil), c.messages...) }

func (c Conversation) AddMessage(message Message) (Conversation, error) {
	if err := message.validate(); err != nil {
		return Conversation{}, err
	}
	if message.conversationID != c.id {
		return Conversation{}, errors.New("message belongs to a different conversation")
	}
	c.messages = append(append([]Message(nil), c.messages...), message)
	return c, nil
}

func (m Message) validate() error {
	if strings.TrimSpace(m.id) == "" || strings.TrimSpace(m.conversationID) == "" || strings.TrimSpace(m.senderID) == "" {
		return errors.New("message, conversation, and sender IDs are required")
	}
	if !validSender(m.senderType) {
		return errors.New("invalid message sender type")
	}
	if strings.TrimSpace(m.content) == "" || strings.TrimSpace(m.contentType) == "" {
		return errors.New("message content and content type are required")
	}
	if m.createdAt.IsZero() {
		return errors.New("message creation time is required")
	}
	return nil
}

func validSender(sender SenderType) bool {
	switch sender {
	case SenderUser, SenderAgent, SenderBrain, SenderSystem:
		return true
	default:
		return false
	}
}
