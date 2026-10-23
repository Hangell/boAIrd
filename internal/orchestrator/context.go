package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"boAIrd/internal/agent"
	"boAIrd/internal/brain"
	agentcontext "boAIrd/internal/context"
	"boAIrd/internal/conversation"
	"boAIrd/internal/project"
	"boAIrd/internal/run"
	"boAIrd/internal/task"
)

type runContextReader interface {
	List(context.Context, string, string, string, int, int) ([]run.Run, error)
}

func (s *Service) selectExecutionContext(ctx context.Context, p project.Project, b brain.Brain, work task.Task, worker agent.Agent, attempt int, instructions string) (agentcontext.Result, error) {
	unavailable := []agentcontext.Source{agentcontext.SourceFile, agentcontext.SourceRepository}
	candidates := []agentcontext.Candidate{
		{
			ID: "project:" + p.ID(), ProjectID: p.ID(), Source: agentcontext.SourceProject,
			Trust: agentcontext.TrustAdmin, Priority: 90,
			Content: strings.TrimSpace(p.Name() + "\n" + p.Description()),
		},
		{
			ID: "task:" + work.ID(), ProjectID: work.ProjectID(), TaskID: work.ID(), Source: agentcontext.SourceTask,
			Trust: taskTrust(work.CreatedByType()), Priority: 100,
			Content: fmt.Sprintf("Title: %s\nDescription: %s\nPriority: %s\nAttempt: %d of %d", work.Title(), work.Description(), work.Priority(), attempt, s.config.MaxAttempts),
		},
	}
	if instructions = strings.TrimSpace(instructions); instructions != "" {
		candidates = append(candidates, agentcontext.Candidate{
			ID: "brain-review-instructions:" + work.ID(), ProjectID: work.ProjectID(), TaskID: work.ID(),
			Source: agentcontext.SourceBrain, Trust: agentcontext.TrustBrain, Priority: 80, Content: instructions,
		})
	}
	brainAgent, err := s.agents.Get(ctx, b.AgentID())
	if err != nil {
		unavailable = append(unavailable, agentcontext.SourceBrain)
	} else if brainInstructions := strings.TrimSpace(brainAgent.Instructions()); brainInstructions != "" {
		candidates = append(candidates, agentcontext.Candidate{
			ID: "brain-instructions:" + brainAgent.ID(), ProjectID: work.ProjectID(),
			Source: agentcontext.SourceBrain, Trust: agentcontext.TrustBrain, Priority: 80, Content: brainInstructions,
		})
	}
	if agentInstructions := strings.TrimSpace(worker.Instructions()); agentInstructions != "" {
		candidates = append(candidates, agentcontext.Candidate{
			ID: "agent-instructions:" + worker.ID(), ProjectID: work.ProjectID(), TaskID: work.ID(),
			Source: agentcontext.SourceAgent, Trust: agentcontext.TrustAgent, Priority: 70, Content: agentInstructions,
		})
	}
	var memoryOmissions []agentcontext.Omission
	if s.memory == nil {
		unavailable = append(unavailable, agentcontext.SourceMemory)
	} else {
		selected, err := s.memory.Select(ctx, work.ProjectID(), work.Title()+"\n"+work.Description(), s.config.ContextMaxBytes)
		if err != nil {
			unavailable = append(unavailable, agentcontext.SourceMemory)
		} else {
			for _, value := range selected.Items {
				candidates = append(candidates, agentcontext.Candidate{
					ID: "memory:" + value.ID(), ProjectID: work.ProjectID(), Source: agentcontext.SourceMemory,
					Trust: memoryTrust(value.CreatedByType()), Priority: 65,
					Content: fmt.Sprintf("Category: %s\nVersion: %d\nSources: %s\n%s", value.Category(), value.Version(), strings.Join(value.SourceIDs(), ", "), value.Content()),
				})
			}
			for _, item := range selected.Omitted {
				memoryOmissions = append(memoryOmissions, agentcontext.Omission{ID: item.ID, Source: agentcontext.SourceMemory, Reason: item.Reason})
			}
		}
	}

	if s.contextReader == nil {
		unavailable = append(unavailable, agentcontext.SourceConversation)
	} else {
		_, messages, _, err := s.contextReader.GetPageForAgent(ctx, work.ID(), worker.ID(), "", 20)
		if err != nil {
			unavailable = append(unavailable, agentcontext.SourceConversation)
		} else {
			for _, message := range messages {
				candidates = append(candidates, agentcontext.Candidate{
					ID: message.ID(), ProjectID: work.ProjectID(), TaskID: work.ID(),
					Source: agentcontext.SourceConversation, Trust: messageTrust(message.SenderType()),
					Priority: 50, Content: message.Content(),
				})
			}
		}
	}
	if history, ok := s.runs.(runContextReader); !ok {
		unavailable = append(unavailable, agentcontext.SourceRun)
	} else {
		values, err := history.List(ctx, work.ProjectID(), work.ID(), worker.ID(), 10, 0)
		if err != nil {
			unavailable = append(unavailable, agentcontext.SourceRun)
		} else {
			for _, value := range values {
				content := strings.TrimSpace(value.Output())
				if content == "" {
					content = strings.TrimSpace(value.Error())
				}
				if content == "" {
					continue
				}
				candidates = append(candidates, agentcontext.Candidate{
					ID: "run:" + value.ID(), ProjectID: work.ProjectID(), TaskID: work.ID(),
					Source: agentcontext.SourceRun, Trust: agentcontext.TrustUntrusted,
					Priority: 40, Content: content,
				})
			}
		}
	}

	selection, err := s.contextEngine.Select(agentcontext.Request{
		ProjectID: work.ProjectID(), TaskID: work.ID(), Candidates: candidates, UnavailableSources: unavailable,
	})
	if err != nil {
		return agentcontext.Result{}, err
	}
	selection.Summary.Omitted = append(selection.Summary.Omitted, memoryOmissions...)
	for _, item := range selection.Summary.Selected {
		if item.ID == "task:"+work.ID() {
			return selection, nil
		}
	}
	return agentcontext.Result{}, fmt.Errorf("task details did not fit the configured context budget")
}

func memoryTrust(creatorType string) agentcontext.Trust {
	switch strings.ToLower(strings.TrimSpace(creatorType)) {
	case "administrator":
		return agentcontext.TrustAdmin
	case "brain":
		return agentcontext.TrustBrain
	case "agent", "worker":
		return agentcontext.TrustAgent
	default:
		return agentcontext.TrustUntrusted
	}
}

func taskTrust(actor task.ActorType) agentcontext.Trust {
	switch actor {
	case task.ActorAdministrator:
		return agentcontext.TrustAdmin
	case task.ActorBrain:
		return agentcontext.TrustBrain
	case task.ActorAgent:
		return agentcontext.TrustAgent
	default:
		return agentcontext.TrustUntrusted
	}
}

func messageTrust(sender conversation.SenderType) agentcontext.Trust {
	switch sender {
	case conversation.SenderUser:
		return agentcontext.TrustAdmin
	case conversation.SenderBrain:
		return agentcontext.TrustBrain
	case conversation.SenderAgent:
		return agentcontext.TrustAgent
	case conversation.SenderSystem:
		return agentcontext.TrustProjectPolicy
	default:
		return agentcontext.TrustUntrusted
	}
}
