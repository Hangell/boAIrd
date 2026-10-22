package orchestrator_test

import (
	"context"
	"errors"
	"testing"

	"boAIrd/internal/agent"
	"boAIrd/internal/brain"
	"boAIrd/internal/orchestrator"
	"boAIrd/internal/project"
	"boAIrd/internal/run"
	"boAIrd/internal/task"
)

type stubProjects struct {
	value project.Project
	err   error
}

func (s stubProjects) Get(context.Context, string) (project.Project, error) { return s.value, s.err }

type stubBrains struct {
	value brain.Brain
	err   error
}

func (s stubBrains) GetActiveByProject(context.Context, string) (brain.Brain, error) {
	return s.value, s.err
}

type stubAgents struct{ values map[string]agent.Agent }

func (s stubAgents) Get(_ context.Context, id string) (agent.Agent, error) {
	value, ok := s.values[id]
	if !ok {
		return agent.Agent{}, agent.ErrNotFound
	}
	return value, nil
}

type stubTasks struct{ listCalls int }

func (s *stubTasks) Get(context.Context, string) (task.Task, error) {
	return task.Task{}, task.ErrNotFound
}
func (s *stubTasks) List(context.Context, string, task.Status, string, int, int) ([]task.Task, error) {
	s.listCalls++
	return nil, nil
}
func (s *stubTasks) Transition(context.Context, string, task.Status, string, task.Actor) (task.Task, error) {
	return task.Task{}, task.ErrNotFound
}

type stubRuns struct{}

func (stubRuns) Create(context.Context, run.Run) error                            { return nil }
func (stubRuns) Update(context.Context, run.Run) error                            { return nil }
func (stubRuns) CountWorkerAttempts(context.Context, string, string) (int, error) { return 0, nil }

func newBrainAuthService(t *testing.T, capabilities []string) (*orchestrator.Service, *fakeExecutor, *stubTasks) {
	t.Helper()
	projectValue, err := project.Restore(project.Snapshot{ID: "p1", Name: "P", Status: project.StatusReady, BrainID: "b1"})
	if err != nil {
		t.Fatal(err)
	}
	brainValue, err := brain.New("b1", "p1", "brain-agent", brain.AutonomyAutonomous)
	if err != nil {
		t.Fatal(err)
	}
	brainAgent, err := agent.NewConfigured("brain-agent", agent.Configuration{
		Name: "Brain", Role: "coordinator", Runtime: agent.RuntimeMCP, Capabilities: capabilities,
	})
	if err != nil {
		t.Fatal(err)
	}
	executor := &fakeExecutor{}
	tasks := &stubTasks{}
	service, err := orchestrator.New(orchestrator.Dependencies{
		Tasks: tasks, Runs: stubRuns{}, Agents: stubAgents{values: map[string]agent.Agent{"brain-agent": brainAgent}},
		Projects: stubProjects{value: projectValue}, Brains: stubBrains{value: brainValue}, Executor: executor,
	})
	if err != nil {
		t.Fatal(err)
	}
	return service, executor, tasks
}

func TestDispatchFailsClosedWithoutBrainExecuteCapability(t *testing.T) {
	service, executor, tasks := newBrainAuthService(t, []string{"task.assign", "task.review", "task.complete"})

	if _, err := service.Dispatch(context.Background(), "p1"); !errors.Is(err, brain.ErrUnauthorizedAction) {
		t.Fatalf("dispatch error = %v, want brain.ErrUnauthorizedAction", err)
	}
	if len(executor.Calls()) != 0 {
		t.Fatal("executor was invoked without the brain.execute capability")
	}
	if tasks.listCalls != 0 {
		t.Fatal("tasks were listed before the Brain was authorized")
	}
}

func TestDispatchAllowsBrainWithExecuteCapability(t *testing.T) {
	service, _, tasks := newBrainAuthService(t, []string{"brain.execute", "task.assign", "task.review", "task.complete"})

	report, err := service.Dispatch(context.Background(), "p1")
	if err != nil {
		t.Fatalf("dispatch error = %v", err)
	}
	if len(report.Outcomes) != 0 || tasks.listCalls != 1 {
		t.Fatalf("dispatch report = %+v, list calls = %d", report, tasks.listCalls)
	}
}
