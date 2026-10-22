package task_test

import (
	"reflect"
	"testing"

	"boAIrd/internal/task"
)

func TestInvariantFieldsAreNotPublic(t *testing.T) {
	typeOfTask := reflect.TypeOf(task.Task{})
	for _, name := range []string{"ID", "Status", "HumanReview", "AssignedTo", "ParentID"} {
		if _, ok := typeOfTask.FieldByName(name); ok {
			t.Errorf("Task.%s must not be publicly mutable", name)
		}
	}
}
