package api

import (
	"errors"
	"net/http"

	"boAIrd/internal/agent"
	"boAIrd/internal/project"
)

type agentResponse struct {
	ID                  string        `json:"id"`
	Name                string        `json:"name"`
	Avatar              string        `json:"avatar"`
	Description         string        `json:"description"`
	Role                string        `json:"role"`
	Runtime             agent.Runtime `json:"runtime"`
	Provider            string        `json:"provider,omitempty"`
	ProviderBaseURL     string        `json:"provider_base_url,omitempty"`
	Model               string        `json:"model,omitempty"`
	CredentialID        string        `json:"credential_id,omitempty"`
	Instructions        string        `json:"instructions"`
	LanguagePreferences []string      `json:"language_preferences"`
	Capabilities        []string      `json:"capabilities"`
	ProjectIDs          []string      `json:"project_ids"`
	Status              agent.Status  `json:"status"`
}

type agentListResponse struct {
	Items  []agentResponse `json:"items"`
	Limit  int             `json:"limit"`
	Offset int             `json:"offset"`
}

func registerAgentRoutes(mux *http.ServeMux, manager AgentManager, projects ProjectManager) {
	mux.HandleFunc("POST /api/v1/agents", func(w http.ResponseWriter, r *http.Request) {
		var input agent.CreateInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "invalid agent request")
			return
		}
		value, err := manager.Create(r.Context(), input)
		if err != nil {
			writeAgentError(w, err)
			return
		}
		response := toAgentResponse(value)
		w.Header().Set("Location", "/api/v1/agents/"+response.ID)
		writeJSON(w, http.StatusCreated, response)
	})

	mux.HandleFunc("GET /api/v1/agents", func(w http.ResponseWriter, r *http.Request) {
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
		values, err := manager.List(r.Context(), limit, offset)
		if err != nil {
			writeAgentError(w, err)
			return
		}
		if limit == 0 {
			limit = 50
		}
		items := make([]agentResponse, 0, len(values))
		for _, value := range values {
			items = append(items, toAgentResponse(value))
		}
		writeJSON(w, http.StatusOK, agentListResponse{Items: items, Limit: limit, Offset: offset})
	})

	mux.HandleFunc("GET /api/v1/agents/{id}", func(w http.ResponseWriter, r *http.Request) {
		value, err := manager.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeAgentError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toAgentResponse(value))
	})

	mux.HandleFunc("PATCH /api/v1/agents/{id}", func(w http.ResponseWriter, r *http.Request) {
		var input agent.UpdateInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "invalid agent request")
			return
		}
		value, err := manager.Update(r.Context(), r.PathValue("id"), input)
		if err != nil {
			writeAgentError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toAgentResponse(value))
	})

	scopeManager, supportsProjectScope := manager.(AgentProjectScopeManager)
	if projects != nil && supportsProjectScope {
		mux.HandleFunc("PUT /api/v1/projects/{projectID}/agents/{agentID}", func(w http.ResponseWriter, r *http.Request) {
			projectID, agentID := r.PathValue("projectID"), r.PathValue("agentID")
			if _, err := projects.Get(r.Context(), projectID); err != nil {
				writeAPIError(w, http.StatusNotFound, "not_found", "project not found")
				return
			}
			if _, err := manager.Get(r.Context(), agentID); err != nil {
				writeAgentError(w, err)
				return
			}
			if err := scopeManager.AddProject(r.Context(), agentID, projectID, project.LocalAdministratorID); err != nil {
				writeAgentError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
		mux.HandleFunc("DELETE /api/v1/projects/{projectID}/agents/{agentID}", func(w http.ResponseWriter, r *http.Request) {
			projectID, agentID := r.PathValue("projectID"), r.PathValue("agentID")
			if _, err := projects.Get(r.Context(), projectID); err != nil {
				writeAPIError(w, http.StatusNotFound, "not_found", "project not found")
				return
			}
			if _, err := manager.Get(r.Context(), agentID); err != nil {
				writeAgentError(w, err)
				return
			}
			if err := scopeManager.RemoveProject(r.Context(), agentID, projectID, project.LocalAdministratorID); err != nil {
				writeAgentError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	}
}

func toAgentResponse(value agent.Agent) agentResponse {
	snapshot := value.Snapshot()
	return agentResponse{
		ID: snapshot.ID, Name: snapshot.Name, Avatar: snapshot.Avatar, Description: snapshot.Description,
		Role: snapshot.Role, Runtime: snapshot.Runtime, Provider: snapshot.Provider, ProviderBaseURL: snapshot.ProviderBaseURL, Model: snapshot.Model,
		CredentialID: snapshot.CredentialID, Instructions: snapshot.Instructions,
		LanguagePreferences: snapshot.LanguagePreferences, Capabilities: snapshot.Capabilities, ProjectIDs: snapshot.ProjectIDs, Status: snapshot.Status,
	}
}

func writeAgentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, agent.ErrInvalidInput):
		writeAPIError(w, http.StatusBadRequest, "invalid_agent", "invalid agent configuration")
	case errors.Is(err, agent.ErrCredentialMismatch):
		writeAPIError(w, http.StatusBadRequest, "invalid_credential_reference", "credential does not match the configured provider")
	case errors.Is(err, agent.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", "agent not found")
	default:
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
