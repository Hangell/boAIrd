// Package conversation defines provider-independent task conversations.
package conversation

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"strings"
	"time"

	"boAIrd/internal/logging"
)

const (
	maximumContentSize  = 1 << 20
	maximumMetadataSize = 64 << 10
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
	metadata       []byte
	createdAt      time.Time
}

type Conversation struct {
	id        string
	projectID string
	taskID    string
	messages  []Message
}

func New(id, projectID, taskID string) (Conversation, error) {
	id, projectID, taskID = strings.TrimSpace(id), strings.TrimSpace(projectID), strings.TrimSpace(taskID)
	if id == "" || projectID == "" || taskID == "" {
		return Conversation{}, errors.New("conversation, project, and task IDs are required")
	}
	return Conversation{id: id, projectID: projectID, taskID: taskID}, nil
}

func NewMessage(id, conversationID string, senderType SenderType, senderID, content, contentType string, createdAt time.Time) (Message, error) {
	return NewMessageWithMetadata(id, conversationID, senderType, senderID, content, contentType, []byte(`{}`), createdAt)
}

func NewMessageWithMetadata(id, conversationID string, senderType SenderType, senderID, content, contentType string, metadata []byte, createdAt time.Time) (Message, error) {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil {
		return Message{}, errors.New("message content type must be a valid media type")
	}
	if (mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")) && !json.Valid([]byte(content)) {
		return Message{}, errors.New("JSON message content must be valid JSON")
	}
	cleanMetadata, err := sanitizeMetadata(metadata)
	if err != nil {
		return Message{}, err
	}
	message := Message{
		id: strings.TrimSpace(id), conversationID: strings.TrimSpace(conversationID), senderType: senderType, senderID: strings.TrimSpace(senderID),
		content: logging.RedactSensitiveText(content), contentType: strings.TrimSpace(contentType), metadata: cleanMetadata, createdAt: createdAt.UTC(),
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
func (m Message) Metadata() []byte         { return append([]byte(nil), m.metadata...) }
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
	if strings.TrimSpace(m.content) == "" || len(m.content) > maximumContentSize || strings.TrimSpace(m.contentType) == "" || len(m.contentType) > 128 {
		return errors.New("message content and content type are required")
	}
	if len(m.metadata) == 0 || len(m.metadata) > maximumMetadataSize || !json.Valid(m.metadata) {
		return errors.New("message metadata must be valid JSON within the size limit")
	}
	if m.createdAt.IsZero() {
		return errors.New("message creation time is required")
	}
	return nil
}

func sanitizeMetadata(metadata []byte) ([]byte, error) {
	if len(metadata) == 0 {
		metadata = []byte(`{}`)
	}
	if len(metadata) > maximumMetadataSize {
		return nil, errors.New("message metadata exceeds the size limit")
	}
	var value map[string]any
	if err := json.Unmarshal(metadata, &value); err != nil || value == nil {
		return nil, errors.New("message metadata must be a JSON object")
	}
	redactMetadata(value)
	clean, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode message metadata: %w", err)
	}
	return clean, nil
}

func redactMetadata(value map[string]any) {
	for key, child := range value {
		name := strings.ToLower(strings.NewReplacer("-", "_", " ", "_").Replace(key))
		if containsSensitiveMarker(name) {
			value[key] = "[REDACTED]"
			continue
		}
		value[key] = redactMetadataValue(child)
	}
}

func redactMetadataValue(value any) any {
	switch typed := value.(type) {
	case string:
		return logging.RedactSensitiveText(typed)
	case map[string]any:
		redactMetadata(typed)
		return typed
	case []any:
		for i := range typed {
			typed[i] = redactMetadataValue(typed[i])
		}
		return typed
	default:
		return value
	}
}

func containsSensitiveMarker(name string) bool {
	for _, marker := range []string{"secret", "token", "api_key", "apikey", "authorization", "password", "cookie", "private_key", "client_key", "credential"} {
		if strings.Contains(name, marker) && !(marker == "credential" && strings.HasSuffix(name, "_id")) {
			return true
		}
	}
	return false
}

func validSender(sender SenderType) bool {
	switch sender {
	case SenderUser, SenderAgent, SenderBrain, SenderSystem:
		return true
	default:
		return false
	}
}
