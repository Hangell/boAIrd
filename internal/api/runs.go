package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"boAIrd/internal/project"
	"boAIrd/internal/run"
)

type runAPIManager interface {
	Get(context.Context, string) (run.Run, error)
	List(context.Context, string, string, string, int, int) ([]run.Run, error)
	ListAuditEvents(context.Context, string) ([]run.AuditEvent, error)
	ListAuditEventsForRuns(context.Context, []string) (map[string][]run.AuditEvent, error)
}

type runResponse struct {
	ID          string           `json:"id"`
	ProjectID   string           `json:"project_id"`
	TaskID      string           `json:"task_id,omitempty"`
	AgentID     string           `json:"agent_id"`
	Runtime     string           `json:"runtime"`
	Provider    string           `json:"provider"`
	Model       string           `json:"model"`
	Status      run.Status       `json:"status"`
	StartedAt   *time.Time       `json:"started_at,omitempty"`
	FinishedAt  *time.Time       `json:"finished_at,omitempty"`
	Input       string           `json:"input"`
	Output      string           `json:"output"`
	Error       string           `json:"error,omitempty"`
	ToolsUsed   []string         `json:"tools_used"`
	AuditEvents []run.AuditEvent `json:"audit_events"`
}

type runPageResponse struct {
	Items  []runResponse `json:"items"`
	Limit  int           `json:"limit"`
	Offset int           `json:"offset"`
}

func registerRunRoutes(mux *http.ServeMux, manager runAPIManager) {
	mux.HandleFunc("GET /api/v1/projects/{id}/runs", func(w http.ResponseWriter, r *http.Request) {
		limit, err := queryInteger(r, "limit", 0)
		if err != nil || (r.URL.Query().Has("limit") && limit == 0) {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "limit must be an integer between 1 and 100")
			return
		}
		offset, err := queryInteger(r, "offset", 0)
		if err != nil || offset < 0 {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "offset must be a non-negative integer")
			return
		}
		values, err := manager.List(r.Context(), r.PathValue("id"), r.URL.Query().Get("task_id"), r.URL.Query().Get("agent_id"), limit, offset)
		if err != nil {
			writeRunError(w, err)
			return
		}
		if limit == 0 {
			limit = 50
		}
		runIDs := make([]string, 0, len(values))
		for _, value := range values {
			runIDs = append(runIDs, value.ID())
		}
		eventsByRun, err := manager.ListAuditEventsForRuns(r.Context(), runIDs)
		if err != nil {
			writeRunError(w, err)
			return
		}
		items := make([]runResponse, 0, len(values))
		for _, value := range values {
			items = append(items, toRunResponse(value, eventsByRun[value.ID()]))
		}
		writeJSON(w, http.StatusOK, runPageResponse{Items: items, Limit: limit, Offset: offset})
	})

	mux.HandleFunc("GET /api/v1/runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		value, err := manager.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeRunError(w, err)
			return
		}
		events, err := manager.ListAuditEvents(r.Context(), value.ID())
		if err != nil {
			writeRunError(w, err)
			return
		}
		response := toRunResponse(value, events)
		writeJSON(w, http.StatusOK, response)
	})
}

func toRunResponse(value run.Run, events []run.AuditEvent) runResponse {
	snapshot := value.Snapshot()
	response := runResponse{
		ID: snapshot.ID, ProjectID: snapshot.ProjectID, TaskID: snapshot.TaskID, AgentID: snapshot.AgentID,
		Runtime: snapshot.Runtime, Provider: snapshot.Provider, Model: snapshot.Model, Status: snapshot.Status,
		Input: snapshot.Input, Output: snapshot.Output, Error: snapshot.Error,
		ToolsUsed: append([]string{}, snapshot.ToolsUsed...), AuditEvents: events,
	}
	if !snapshot.StartedAt.IsZero() {
		started := snapshot.StartedAt
		response.StartedAt = &started
	}
	if !snapshot.FinishedAt.IsZero() {
		finished := snapshot.FinishedAt
		response.FinishedAt = &finished
	}
	return response
}

func writeRunError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, run.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", "run not found")
	case errors.Is(err, project.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", "project not found")
	case errors.Is(err, run.ErrInvalidQuery):
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "invalid run query")
	default:
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
