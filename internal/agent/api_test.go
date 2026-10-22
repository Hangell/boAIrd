package agent_test

import (
	"reflect"
	"testing"

	"boAIrd/internal/agent"
)

func TestStatusAndRelationFieldsAreNotPublic(t *testing.T) {
	typeOfAgent := reflect.TypeOf(agent.Agent{})
	for _, name := range []string{"ID", "Name", "Avatar", "Description", "Role", "Runtime", "Provider", "Model", "CredentialID", "Instructions", "LanguagePreferences", "Status", "ProjectIDs", "Capabilities", "Tools"} {
		if _, ok := typeOfAgent.FieldByName(name); ok {
			t.Errorf("Agent.%s must not be publicly mutable", name)
		}
	}
}
