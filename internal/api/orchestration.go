package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"boAIrd/internal/agent"
	"boAIrd/internal/brain"
	"boAIrd/internal/orchestrator"
	"boAIrd/internal/project"
	"boAIrd/internal/task"
)

type orchestrationManager interface {
	Dispatch(context.Context, string) (orchestrator.DispatchReport, error)
	Review(context.Context, string, orchestrator.ReviewDecision, string) (task.Task, error)
}

type reviewTaskRequest struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

func registerOrchestrationRoutes(mux *http.ServeMux, manager orchestrationManager) {
	mux.HandleFunc("POST /api/v1/projects/{id}/orchestration/dispatch", func(w http.ResponseWriter, r *http.Request) {
		report, err := manager.Dispatch(r.Context(), r.PathValue("id"))
		if err != nil {
			writeOrchestrationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, report)
	})

	mux.HandleFunc("POST /api/v1/tasks/{id}/review", func(w http.ResponseWriter, r *http.Request) {
		var request reviewTaskRequest
		if err := decodeJSON(w, r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		decision := orchestrator.ReviewDecision(strings.ToUpper(strings.TrimSpace(request.Decision)))
		value, err := manager.Review(r.Context(), r.PathValue("id"), decision, request.Reason)
		if err != nil {
			writeOrchestrationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toTaskResponse(value, nil))
	})
}

func writeOrchestrationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, brain.ErrHumanApprovalRequired):
		writeAPIError(w, http.StatusConflict, "human_approval_required", "project policy requires human approval for this decision")
	case errors.Is(err, task.ErrNotFound), errors.Is(err, agent.ErrNotFound), errors.Is(err, project.ErrNotFound), errors.Is(err, brain.ErrNotFound), errors.Is(err, brain.ErrProjectNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", "project, task, or Brain not found")
	case errors.Is(err, task.ErrForbidden):
		writeAPIError(w, http.StatusForbidden, "forbidden", "orchestration action is not authorized")
	case errors.Is(err, task.ErrConflict), errors.Is(err, orchestrator.ErrTaskNotInReview):
		writeAPIError(w, http.StatusConflict, "orchestration_conflict", "task state changed concurrently; reload and retry")
	case errors.Is(err, orchestrator.ErrProjectUnavailable), errors.Is(err, task.ErrProjectArchived):
		writeAPIError(w, http.StatusConflict, "project_unavailable", "project cannot be orchestrated")
	case errors.Is(err, orchestrator.ErrInvalidRequest), errors.Is(err, task.ErrInvalidInput), errors.Is(err, task.ErrInvalidTransition):
		writeAPIError(w, http.StatusBadRequest, "invalid_orchestration", err.Error())
	default:
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
