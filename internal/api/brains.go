package api

import (
	"errors"
	"net/http"

	"boAIrd/internal/agent"
	"boAIrd/internal/brain"
	"boAIrd/internal/project"
)

type createBrainRequest struct {
	AgentID  string             `json:"agent_id"`
	Autonomy brain.AutonomyMode `json:"autonomy"`
}
type executeBrainRequest struct {
	Objective string `json:"objective"`
	MaxTokens int    `json:"max_tokens,omitempty"`
}
type replaceBrainRequest struct {
	AgentID  string             `json:"agent_id"`
	Autonomy brain.AutonomyMode `json:"autonomy"`
	Reason   string             `json:"reason"`
}
type brainResponse struct {
	ID        string             `json:"id"`
	ProjectID string             `json:"project_id"`
	AgentID   string             `json:"agent_id"`
	Autonomy  brain.AutonomyMode `json:"autonomy"`
	Active    bool               `json:"active"`
}
type brainExecutionResponse struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Content  string `json:"content,omitempty"`
}

func registerBrainRoutes(mux *http.ServeMux, manager BrainManager, localAuthToken []byte, sessions *localSessionStore) {
	mux.HandleFunc("POST /api/v1/projects/{id}/brain", func(w http.ResponseWriter, r *http.Request) {
		var input createBrainRequest
		if err := decodeJSON(w, r, &input); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		value, err := manager.Create(r.Context(), r.PathValue("id"), brain.CreateInput{AgentID: input.AgentID, Autonomy: input.Autonomy})
		if err != nil {
			writeBrainError(w, err)
			return
		}
		w.Header().Set("Location", "/api/v1/projects/"+value.ProjectID()+"/brain")
		writeJSON(w, http.StatusCreated, toBrainResponse(value))
	})
	if replacer, ok := manager.(BrainReplacementManager); ok {
		mux.HandleFunc("PUT /api/v1/projects/{id}/brain", func(w http.ResponseWriter, r *http.Request) {
			actorID, authorized := localConversationPrincipal(r, localAuthToken, sessions)
			if !authorized {
				writeConversationUnauthorized(w)
				return
			}
			var input replaceBrainRequest
			if err := decodeJSON(w, r, &input); err != nil {
				writeAPIError(w, http.StatusBadRequest, "invalid_request", "invalid Brain replacement request")
				return
			}
			value, err := replacer.Replace(r.Context(), r.PathValue("id"), brain.CreateInput{AgentID: input.AgentID, Autonomy: input.Autonomy}, actorID, input.Reason)
			if err != nil {
				writeBrainError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, toBrainResponse(value))
		})
	}
	mux.HandleFunc("GET /api/v1/projects/{id}/brain", func(w http.ResponseWriter, r *http.Request) {
		value, err := manager.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeBrainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toBrainResponse(value))
	})
	mux.HandleFunc("POST /api/v1/projects/{id}/brain/runs", func(w http.ResponseWriter, r *http.Request) {
		var input executeBrainRequest
		if err := decodeJSON(w, r, &input); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		result, err := manager.Execute(r.Context(), r.PathValue("id"), input.Objective, input.MaxTokens)
		if err != nil {
			writeBrainError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, brainExecutionResponse{ID: result.Run.ID(), Status: string(result.Run.Status()), Provider: result.Run.Provider(), Model: result.Run.Model(), Content: result.Content})
	})
}

func toBrainResponse(value brain.Brain) brainResponse {
	return brainResponse{ID: value.ID(), ProjectID: value.ProjectID(), AgentID: value.AgentID(), Autonomy: value.Autonomy(), Active: value.Active()}
}

func writeBrainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, brain.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", "Brain not found")
	case errors.Is(err, brain.ErrProjectNotFound), errors.Is(err, project.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", "project not found")
	case errors.Is(err, agent.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", "agent not found")
	case errors.Is(err, brain.ErrUnauthorizedAction), errors.Is(err, brain.ErrInvalidAgent), errors.Is(err, brain.ErrProjectUnavailable):
		writeAPIError(w, http.StatusForbidden, "not_allowed", "Brain action is not allowed")
	case errors.Is(err, brain.ErrInvalidInput), errors.Is(err, agent.ErrInvalidInput):
		writeAPIError(w, http.StatusBadRequest, "invalid_brain", err.Error())
	case errors.Is(err, brain.ErrAlreadyExists):
		writeAPIError(w, http.StatusConflict, "already_configured", "project already has a Brain")
	case errors.Is(err, brain.ErrConfigurationChanged):
		writeAPIError(w, http.StatusConflict, "configuration_changed", "Brain configuration changed; reload and retry")
	default:
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
