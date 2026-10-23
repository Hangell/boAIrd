package api

import (
	"context"
	"errors"
	"net/http"

	"boAIrd/internal/agent"
	"boAIrd/internal/capability"
	"boAIrd/internal/template"
)

// TemplateManager exposes reusable agent templates and instantiation.
type TemplateManager interface {
	Create(context.Context, template.CreateInput, string) (template.Template, error)
	Get(context.Context, string) (template.Template, error)
	List(context.Context) ([]template.Template, error)
	Update(context.Context, string, template.UpdateInput, string) (template.Template, error)
	Instantiate(context.Context, string, string, template.InstantiateInput, string) (agent.Agent, error)
}

type templateResponse struct {
	ID                    string        `json:"id"`
	Key                   string        `json:"key"`
	Name                  string        `json:"name"`
	Role                  string        `json:"role"`
	Description           string        `json:"description"`
	Instructions          string        `json:"instructions"`
	DefaultRuntime        agent.Runtime `json:"default_runtime"`
	SuggestedTools        []string      `json:"suggested_tools"`
	SuggestedCapabilities []string      `json:"suggested_capabilities"`
	Version               int           `json:"version"`
}

type templateListResponse struct {
	Items []templateResponse `json:"items"`
}

func registerTemplateRoutes(mux *http.ServeMux, manager TemplateManager) {
	mux.HandleFunc("GET /api/v1/templates", func(w http.ResponseWriter, r *http.Request) {
		values, err := manager.List(r.Context())
		if err != nil {
			writeTemplateError(w, err)
			return
		}
		items := make([]templateResponse, 0, len(values))
		for _, value := range values {
			items = append(items, toTemplateResponse(value))
		}
		writeJSON(w, http.StatusOK, templateListResponse{Items: items})
	})

	mux.HandleFunc("POST /api/v1/templates", func(w http.ResponseWriter, r *http.Request) {
		actorID, ok := principalID(r)
		if !ok {
			writeConversationUnauthorized(w)
			return
		}
		var input template.CreateInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "invalid template request")
			return
		}
		value, err := manager.Create(r.Context(), input, actorID)
		if err != nil {
			writeTemplateError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, toTemplateResponse(value))
	})

	mux.HandleFunc("GET /api/v1/templates/{key}", func(w http.ResponseWriter, r *http.Request) {
		value, err := manager.Get(r.Context(), r.PathValue("key"))
		if err != nil {
			writeTemplateError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toTemplateResponse(value))
	})

	mux.HandleFunc("PATCH /api/v1/templates/{key}", func(w http.ResponseWriter, r *http.Request) {
		actorID, ok := principalID(r)
		if !ok {
			writeConversationUnauthorized(w)
			return
		}
		var input template.UpdateInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "invalid template request")
			return
		}
		value, err := manager.Update(r.Context(), r.PathValue("key"), input, actorID)
		if err != nil {
			writeTemplateError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toTemplateResponse(value))
	})

	mux.HandleFunc("POST /api/v1/projects/{projectID}/templates/{key}/instantiate", func(w http.ResponseWriter, r *http.Request) {
		projectID := r.PathValue("projectID")
		if !hasCapabilityForProject(r, "agent.create", projectID) {
			writeAPIError(w, http.StatusForbidden, "forbidden", "operation is not authorized")
			return
		}
		actorID, ok := principalID(r)
		if !ok {
			writeConversationUnauthorized(w)
			return
		}
		var input template.InstantiateInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "invalid instantiate request")
			return
		}
		value, err := manager.Instantiate(r.Context(), projectID, r.PathValue("key"), input, actorID)
		if err != nil {
			writeTemplateError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, toAgentResponse(value, nil))
	})
}

func toTemplateResponse(value template.Template) templateResponse {
	return templateResponse{
		ID: value.ID, Key: value.Key, Name: value.Name, Role: value.Role,
		Description: value.Description, Instructions: value.Instructions, DefaultRuntime: value.DefaultRuntime,
		SuggestedTools:        append([]string(nil), value.SuggestedTools...),
		SuggestedCapabilities: append([]string(nil), value.SuggestedCapabilities...),
		Version:               value.Version,
	}
}

func writeTemplateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, template.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", "agent template not found")
	case errors.Is(err, template.ErrProjectNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", "project not found")
	case errors.Is(err, template.ErrProjectArchived):
		writeAPIError(w, http.StatusConflict, "conflict", "project is archived")
	case errors.Is(err, template.ErrVersionConflict):
		writeAPIError(w, http.StatusConflict, "version_conflict", "agent template was updated concurrently; retry")
	case errors.Is(err, template.ErrInvalidInput):
		writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
	default:
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

// hasCapabilityForProject reports whether the authenticated principal holds the
// required capability within the project boundary.
func hasCapabilityForProject(r *http.Request, required, projectID string) bool {
	principal, ok := r.Context().Value(principalContextKey{}).(capability.Principal)
	if !ok {
		return false
	}
	grant, err := capability.New(required)
	if err != nil {
		return false
	}
	return (capability.Authorizer{}).Authorize(principal, grant, projectID) == nil
}
