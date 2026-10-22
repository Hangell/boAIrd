package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"boAIrd/internal/agent"
	"boAIrd/internal/task"
)

const localTaskActorID = "local-api"

type taskAPIManager interface {
	Create(context.Context, string, task.CreateInput, task.Actor) (task.Task, error)
	Get(context.Context, string) (task.Task, error)
	List(context.Context, string, task.Status, string, int, int) ([]task.Task, error)
	Assign(context.Context, string, string, string, task.Actor) (task.Task, error)
	Transition(context.Context, string, task.Status, string, task.Actor) (task.Task, error)
	ListAuditEvents(context.Context, string) ([]task.AuditEvent, error)
}

type createTaskRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    string `json:"priority"`
	ParentID    string `json:"parent_id"`
	Reason      string `json:"reason"`
}

type assignTaskRequest struct {
	AgentID string `json:"agent_id"`
	Reason  string `json:"reason"`
}

type transitionTaskRequest struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type taskResponse struct {
	ID            string            `json:"id"`
	ProjectID     string            `json:"project_id"`
	Title         string            `json:"title"`
	Description   string            `json:"description"`
	Status        task.Status       `json:"status"`
	Priority      string            `json:"priority"`
	CreatedBy     string            `json:"created_by"`
	CreatedByType task.ActorType    `json:"created_by_type"`
	AssignedTo    string            `json:"assigned_to,omitempty"`
	HumanReview   bool              `json:"human_review_required"`
	ParentID      string            `json:"parent_id,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	Version       int64             `json:"version"`
	AuditEvents   []task.AuditEvent `json:"audit_events,omitempty"`
}

type taskPageResponse struct {
	Items  []taskResponse `json:"items"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

func registerTaskRoutes(mux *http.ServeMux, manager taskAPIManager) {
	mux.HandleFunc("POST /api/v1/projects/{id}/tasks", func(w http.ResponseWriter, r *http.Request) {
		var request createTaskRequest
		if err := decodeJSON(w, r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		value, err := manager.Create(r.Context(), r.PathValue("id"), task.CreateInput{
			Title: request.Title, Description: request.Description, Priority: request.Priority, ParentID: request.ParentID, Reason: request.Reason,
		}, taskActorFromRequest(r))
		if err != nil {
			writeTaskError(w, err)
			return
		}
		w.Header().Set("Location", "/api/v1/tasks/"+value.ID())
		writeJSON(w, http.StatusCreated, toTaskResponse(value, nil))
	})

	mux.HandleFunc("GET /api/v1/projects/{id}/tasks", func(w http.ResponseWriter, r *http.Request) {
		limit, err := queryInteger(r, "limit", 50)
		if err != nil || limit < 1 || limit > 100 {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "limit must be an integer between 1 and 100")
			return
		}
		offset, err := queryInteger(r, "offset", 0)
		if err != nil || offset < 0 {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "offset must be a non-negative integer")
			return
		}
		values, err := manager.List(r.Context(), r.PathValue("id"), task.Status(r.URL.Query().Get("status")), r.URL.Query().Get("assigned_to"), limit, offset)
		if err != nil {
			writeTaskError(w, err)
			return
		}
		items := make([]taskResponse, 0, len(values))
		for _, value := range values {
			items = append(items, toTaskResponse(value, nil))
		}
		writeJSON(w, http.StatusOK, taskPageResponse{Items: items, Limit: limit, Offset: offset})
	})

	mux.HandleFunc("GET /api/v1/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		value, err := manager.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeTaskError(w, err)
			return
		}
		events, err := manager.ListAuditEvents(r.Context(), value.ID())
		if err != nil {
			writeTaskError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toTaskResponse(value, events))
	})

	mux.HandleFunc("PUT /api/v1/tasks/{id}/assignment", func(w http.ResponseWriter, r *http.Request) {
		var request assignTaskRequest
		if err := decodeJSON(w, r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		value, err := manager.Assign(r.Context(), r.PathValue("id"), request.AgentID, request.Reason, taskActorFromRequest(r))
		if err != nil {
			writeTaskError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toTaskResponse(value, nil))
	})

	mux.HandleFunc("POST /api/v1/tasks/{id}/transitions", func(w http.ResponseWriter, r *http.Request) {
		var request transitionTaskRequest
		if err := decodeJSON(w, r, &request); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		value, err := manager.Transition(r.Context(), r.PathValue("id"), task.Status(request.Status), request.Reason, taskActorFromRequest(r))
		if err != nil {
			writeTaskError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toTaskResponse(value, nil))
	})
}

func toTaskResponse(value task.Task, events []task.AuditEvent) taskResponse {
	snapshot := value.Snapshot()
	return taskResponse{ID: snapshot.ID, ProjectID: snapshot.ProjectID, Title: snapshot.Title, Description: snapshot.Description,
		Status: snapshot.Status, Priority: snapshot.Priority, CreatedBy: snapshot.CreatedBy, CreatedByType: snapshot.CreatedByType,
		AssignedTo: snapshot.AssignedTo, HumanReview: snapshot.HumanReview, ParentID: snapshot.ParentID,
		CreatedAt: snapshot.CreatedAt, UpdatedAt: snapshot.UpdatedAt, Version: snapshot.Version, AuditEvents: events}
}

func writeTaskError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, task.ErrNotFound), errors.Is(err, agent.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", "task or assigned agent not found")
	case errors.Is(err, task.ErrProjectNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", "project not found")
	case errors.Is(err, task.ErrForbidden):
		writeAPIError(w, http.StatusForbidden, "forbidden", "task action is not authorized")
	case errors.Is(err, task.ErrConflict):
		writeAPIError(w, http.StatusConflict, "task_conflict", "task changed concurrently; reload and retry")
	case errors.Is(err, task.ErrInvalidInput), errors.Is(err, task.ErrInvalidTransition), errors.Is(err, task.ErrProjectArchived), errors.Is(err, agent.ErrInvalidInput):
		writeAPIError(w, http.StatusBadRequest, "invalid_task", err.Error())
	default:
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
