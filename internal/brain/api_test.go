package brain_test

import (
	"reflect"
	"testing"

	"boAIrd/internal/brain"
)

func TestAuthorityFieldsAreNotPublic(t *testing.T) {
	typeOfBrain := reflect.TypeOf(brain.Brain{})
	for _, name := range []string{"ID", "ProjectID", "AgentID", "Autonomy", "Active"} {
		if _, ok := typeOfBrain.FieldByName(name); ok {
			t.Errorf("Brain.%s must not be publicly mutable", name)
		}
	}
}
