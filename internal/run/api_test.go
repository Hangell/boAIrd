package run_test

import (
	"reflect"
	"testing"

	"boAIrd/internal/run"
)

func TestLifecycleAndReferenceFieldsAreNotPublic(t *testing.T) {
	typeOfRun := reflect.TypeOf(run.Run{})
	for _, name := range []string{"ID", "TaskID", "Status", "StartedAt", "FinishedAt", "ToolsUsed"} {
		if _, ok := typeOfRun.FieldByName(name); ok {
			t.Errorf("Run.%s must not be publicly mutable", name)
		}
	}
}
