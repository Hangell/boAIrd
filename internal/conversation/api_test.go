package conversation_test

import (
	"reflect"
	"testing"

	"boAIrd/internal/conversation"
)

func TestConversationAndMessageFieldsAreNotPublic(t *testing.T) {
	for typeName, typeOf := range map[string]reflect.Type{
		"Conversation": reflect.TypeOf(conversation.Conversation{}),
		"Message":      reflect.TypeOf(conversation.Message{}),
	} {
		for _, name := range map[string][]string{
			"Conversation": {"ID", "ProjectID", "TaskID", "Messages"},
			"Message":      {"ID", "ConversationID", "SenderType", "SenderID", "Content", "ContentType", "CreatedAt"},
		}[typeName] {
			if _, ok := typeOf.FieldByName(name); ok {
				t.Errorf("%s.%s must not be publicly mutable", typeName, name)
			}
		}
	}
}
