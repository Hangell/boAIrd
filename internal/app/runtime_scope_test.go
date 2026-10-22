package app

import (
	"context"
	"testing"

	"boAIrd/internal/agent"
	"boAIrd/internal/runtime"
	"boAIrd/internal/task"
)

type runtimeTaskReaderStub struct {
	value task.Task
	err   error
}

func (r runtimeTaskReaderStub) Get(context.Context, string) (task.Task, error) {
	return r.value, r.err
}

func TestRuntimeExecutionScopeAllowsOnlyAssignedWorkingTask(t *testing.T) {
	worker, err := agent.NewConfigured("worker", agent.Configuration{Name: "Worker", Role: "worker", Runtime: agent.RuntimeRemote})
	if err != nil {
		t.Fatal(err)
	}
	work, err := task.New("task-1", "project-a", "Work", "", "worker")
	if err != nil {
		t.Fatal(err)
	}
	work, err = work.Assign("worker")
	if err != nil {
		t.Fatal(err)
	}
	work, err = work.Transition(task.StatusReady)
	if err != nil {
		t.Fatal(err)
	}
	work, err = work.Transition(task.StatusWorking)
	if err != nil {
		t.Fatal(err)
	}
	scope := runtimeExecutionScope{tasks: runtimeTaskReaderStub{value: work}}

	if err := scope.Authorize(context.Background(), worker, runtime.Request{
		AgentID: worker.ID(), ProjectID: "project-a", TaskID: "task-1",
	}); err != nil {
		t.Fatalf("authorize assigned task: %v", err)
	}
	if err := scope.Authorize(context.Background(), worker, runtime.Request{
		AgentID: worker.ID(), ProjectID: "project-b", TaskID: "task-1",
	}); err == nil {
		t.Fatal("cross-project task scope was authorized")
	}
}
