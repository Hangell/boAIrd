package conversation

import (
	"testing"
	"time"
)

func TestNewRequiresReferences(t *testing.T) {
	if _, err := New("c1", "p1", ""); err == nil {
		t.Fatal("expected missing task reference to be rejected")
	}
	if _, err := New("c1", "p1", "t1"); err != nil {
		t.Fatal(err)
	}
}

func TestNewMessageValidatesFields(t *testing.T) {
	tests := []struct {
		name           string
		id             string
		conversationID string
		senderType     SenderType
		senderID       string
		content        string
		contentType    string
		createdAt      time.Time
	}{
		{name: "message ID", id: " ", conversationID: "c1", senderType: SenderAgent, senderID: "a1", content: "hello", contentType: "text/plain", createdAt: time.Now()},
		{name: "conversation ID", id: "m1", senderType: SenderAgent, senderID: "a1", content: "hello", contentType: "text/plain", createdAt: time.Now()},
		{name: "sender type", id: "m1", conversationID: "c1", senderType: SenderType("UNKNOWN"), senderID: "a1", content: "hello", contentType: "text/plain", createdAt: time.Now()},
		{name: "sender ID", id: "m1", conversationID: "c1", senderType: SenderAgent, content: "hello", contentType: "text/plain", createdAt: time.Now()},
		{name: "content", id: "m1", conversationID: "c1", senderType: SenderAgent, senderID: "a1", content: " ", contentType: "text/plain", createdAt: time.Now()},
		{name: "content type", id: "m1", conversationID: "c1", senderType: SenderAgent, senderID: "a1", content: "hello", createdAt: time.Now()},
		{name: "creation time", id: "m1", conversationID: "c1", senderType: SenderAgent, senderID: "a1", content: "hello", contentType: "text/plain"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewMessage(test.id, test.conversationID, test.senderType, test.senderID, test.content, test.contentType, test.createdAt); err == nil {
				t.Fatal("expected invalid message to be rejected")
			}
		})
	}
}

func TestAddMessageValidatesConversationReference(t *testing.T) {
	c, _ := New("c1", "p1", "t1")
	m, err := NewMessage("m1", "other", SenderAgent, "a1", "hello", "text/plain", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.AddMessage(m); err == nil {
		t.Fatal("expected mismatched conversation reference to be rejected")
	}
	m, err = NewMessage("m1", c.ID(), SenderAgent, "a1", "hello", "text/plain", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	c, err = c.AddMessage(m)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(c.Messages()); got != 1 {
		t.Fatalf("expected one message, got %d", got)
	}
}

func TestMessagesReturnsDefensiveCopy(t *testing.T) {
	c, _ := New("c1", "p1", "t1")
	m, _ := NewMessage("m1", c.ID(), SenderAgent, "a1", "hello", "text/plain", time.Now())
	c, _ = c.AddMessage(m)

	messages := c.Messages()
	messages[0] = Message{}
	if got := c.Messages()[0].ID(); got != "m1" {
		t.Fatalf("mutating returned slice changed stored message ID to %q", got)
	}
}
