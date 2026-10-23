package api

import (
	"context"
	"net/http"
	"time"

	"boAIrd/internal/audit"
	"boAIrd/internal/logging"
)

// AuditReader lists immutable, project-scoped security audit events.
type AuditReader interface {
	ListEvents(context.Context, string, int, int) ([]audit.Entry, error)
}

type auditEntryResponse struct {
	ID           string    `json:"id"`
	ProjectID    string    `json:"project_id"`
	TaskID       string    `json:"task_id,omitempty"`
	ActorType    string    `json:"actor_type"`
	ActorID      string    `json:"actor_id"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	Result       string    `json:"result,omitempty"`
	Reason       string    `json:"reason,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type auditListResponse struct {
	Items  []auditEntryResponse `json:"items"`
	Limit  int                  `json:"limit"`
	Offset int                  `json:"offset"`
}

func registerAuditRoutes(mux *http.ServeMux, reader AuditReader) {
	mux.HandleFunc("GET /api/v1/projects/{projectID}/audit", func(w http.ResponseWriter, r *http.Request) {
		limit, err := queryInteger(r, "limit", 0)
		if err != nil || (r.URL.Query().Has("limit") && limit == 0) {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "limit must be an integer between 1 and 100")
			return
		}
		offset, err := queryInteger(r, "offset", 0)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "offset must be a non-negative integer")
			return
		}
		if limit == 0 {
			limit = 50
		}
		values, err := reader.ListEvents(r.Context(), r.PathValue("projectID"), limit, offset)
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		items := make([]auditEntryResponse, 0, len(values))
		for _, value := range values {
			items = append(items, auditEntryResponse{
				ID: value.ID, ProjectID: value.ProjectID, TaskID: value.TaskID,
				ActorType: value.ActorType, ActorID: value.ActorID, Action: value.Action,
				ResourceType: value.ResourceType, ResourceID: value.ResourceID, Result: value.Result,
				Reason: logging.RedactSensitiveText(value.Reason), CreatedAt: value.CreatedAt,
			})
		}
		writeJSON(w, http.StatusOK, auditListResponse{Items: items, Limit: limit, Offset: offset})
	})
}
