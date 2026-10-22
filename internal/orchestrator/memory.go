package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"boAIrd/internal/brain"
	"boAIrd/internal/memory"
	"boAIrd/internal/project"
	"boAIrd/internal/run"
	"boAIrd/internal/task"
)

func (s *Service) recordTaskMemory(ctx context.Context, p project.Project, b brain.Brain, work task.Task) {
	if s.memory == nil || s.summarizer == nil || work.AssignedTo() == "" {
		return
	}
	history, ok := s.runs.(runContextReader)
	if !ok {
		slog.WarnContext(ctx, "skip project memory summary: Run history reader unavailable", "project_id", p.ID(), "task_id", work.ID())
		return
	}
	runs, err := history.List(ctx, p.ID(), work.ID(), work.AssignedTo(), 20, 0)
	if err != nil {
		slog.WarnContext(ctx, "skip project memory summary: read Runs", "project_id", p.ID(), "task_id", work.ID(), "error", err)
		return
	}
	sourceIDs := []string{work.ID()}
	var source strings.Builder
	fmt.Fprintf(&source, "Task: %s\nDescription: %s\n", work.Title(), work.Description())
	for _, value := range runs {
		if value.Status() != run.StatusSuccess || strings.TrimSpace(value.Output()) == "" {
			continue
		}
		sourceIDs = append(sourceIDs, value.ID())
		fmt.Fprintf(&source, "\nCompleted Run %s output:\n%s\n", value.ID(), value.Output())
		break
	}
	if s.contextReader != nil {
		_, messages, _, err := s.contextReader.GetPageForAgent(ctx, work.ID(), work.AssignedTo(), "", 20)
		if err != nil {
			slog.WarnContext(ctx, "skip project memory summary: read task conversation", "project_id", p.ID(), "task_id", work.ID(), "error", err)
			return
		}
		for _, message := range messages {
			sourceIDs = append(sourceIDs, message.ID())
			fmt.Fprintf(&source, "\nConversation (%s):\n%s\n", message.SenderType(), message.Content())
		}
	}
	objective := strings.TrimSpace(source.String())
	if objective == "" || len(objective) > maximumInputBytes {
		slog.WarnContext(ctx, "skip project memory summary: source is empty or exceeds the summary limit", "project_id", p.ID(), "task_id", work.ID())
		return
	}
	summaryCtx, cancel := context.WithTimeout(ctx, min(s.config.ExecutionTimeout, 45*time.Second))
	defer cancel()
	generated, err := s.summarizer.Summarize(summaryCtx, p.ID(), objective, 1024)
	if err != nil {
		slog.WarnContext(ctx, "generate project memory summary", "project_id", p.ID(), "task_id", work.ID(), "error", err)
		return
	}
	if strings.TrimSpace(generated.Content) == "" {
		slog.WarnContext(ctx, "skip project memory summary: Brain returned empty content", "project_id", p.ID(), "task_id", work.ID())
		return
	}
	if _, err := s.memory.Record(ctx, p.ID(), memory.RecordInput{
		Category: memory.CategoryTaskHistory, Content: generated.Content, SourceIDs: sourceIDs,
		CreatedByType: "brain", CreatedBy: b.AgentID(),
	}); err != nil {
		slog.WarnContext(ctx, "persist project memory summary", "project_id", p.ID(), "task_id", work.ID(), "error", err)
		return
	}
	if _, err := s.memory.Prune(ctx, p.ID()); err != nil {
		slog.WarnContext(ctx, "prune project memory summaries", "project_id", p.ID(), "task_id", work.ID(), "error", err)
	}
}
