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

func TestAddMessageValidatesConversationReference(t *testing.T) {
	c, _ := New("c1", "p1", "t1")
	m := Message{
		ID: "m1", ConversationID: "other", SenderType: SenderAgent,
		SenderID: "a1", Content: "hello", ContentType: "text/plain", CreatedAt: time.Now(),
	}
	if _, err := c.AddMessage(m); err == nil {
		t.Fatal("expected mismatched conversation reference to be rejected")
	}
	m.ConversationID = c.ID
	if _, err := c.AddMessage(m); err != nil {
		t.Fatal(err)
	}
}
