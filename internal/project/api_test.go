package project_test

import (
	"reflect"
	"testing"

	"boAIrd/internal/project"
)

func TestLifecycleAndBrainFieldsAreNotPublic(t *testing.T) {
	typeOfProject := reflect.TypeOf(project.Project{})
	for _, name := range []string{"ID", "Name", "Status", "BrainID", "WorkspacePath", "RepositoryURL"} {
		if _, ok := typeOfProject.FieldByName(name); ok {
			t.Errorf("Project.%s must not be publicly mutable", name)
		}
	}
}
