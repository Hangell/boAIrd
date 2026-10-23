package api

import (
	"context"
	"errors"
	"net/http"

	"boAIrd/internal/agent"
	"boAIrd/internal/profile"
	"boAIrd/internal/project"
)

// ProfileManager exposes built-in profile definitions and project overrides.
type ProfileManager interface {
	Effective(context.Context, string, string) (profile.Effective, error)
	ListEffective(context.Context, string) ([]profile.Effective, error)
	Save(context.Context, string, string, profile.SaveInput) (profile.Effective, error)
}

type profileDefinitionResponse struct {
	Key                   string        `json:"key"`
	Name                  string        `json:"name"`
	Description           string        `json:"description"`
	Instructions          string        `json:"instructions"`
	DefaultRuntime        agent.Runtime `json:"default_runtime"`
	SuggestedTools        []string      `json:"suggested_tools"`
	SuggestedCapabilities []string      `json:"suggested_capabilities"`
}

type effectiveProfileResponse struct {
	Key                   string        `json:"key"`
	Name                  string        `json:"name"`
	Description           string        `json:"description"`
	Instructions          string        `json:"instructions"`
	DefaultRuntime        agent.Runtime `json:"default_runtime"`
	Capabilities          []string      `json:"capabilities"`
	Tools                 []string      `json:"tools"`
	SuggestedTools        []string      `json:"suggested_tools"`
	SuggestedCapabilities []string      `json:"suggested_capabilities"`
	Customized            bool          `json:"customized"`
}

type profileListResponse struct {
	Items []profileDefinitionResponse `json:"items"`
}

type effectiveProfileListResponse struct {
	Items []effectiveProfileResponse `json:"items"`
}

func registerProfileRoutes(mux *http.ServeMux, manager ProfileManager) {
	mux.HandleFunc("GET /api/v1/profiles", func(w http.ResponseWriter, r *http.Request) {
		items := make([]profileDefinitionResponse, 0, len(profile.Builtins()))
		for _, definition := range profile.Builtins() {
			items = append(items, toProfileDefinitionResponse(definition))
		}
		writeJSON(w, http.StatusOK, profileListResponse{Items: items})
	})

	mux.HandleFunc("GET /api/v1/projects/{projectID}/profiles", func(w http.ResponseWriter, r *http.Request) {
		values, err := manager.ListEffective(r.Context(), r.PathValue("projectID"))
		if err != nil {
			writeProfileError(w, err)
			return
		}
		items := make([]effectiveProfileResponse, 0, len(values))
		for _, value := range values {
			items = append(items, toEffectiveProfileResponse(value))
		}
		writeJSON(w, http.StatusOK, effectiveProfileListResponse{Items: items})
	})

	mux.HandleFunc("GET /api/v1/projects/{projectID}/profiles/{key}", func(w http.ResponseWriter, r *http.Request) {
		value, err := manager.Effective(r.Context(), r.PathValue("projectID"), r.PathValue("key"))
		if err != nil {
			writeProfileError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toEffectiveProfileResponse(value))
	})

	mux.HandleFunc("PUT /api/v1/projects/{projectID}/profiles/{key}", func(w http.ResponseWriter, r *http.Request) {
		var input profile.SaveInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "invalid profile request")
			return
		}
		input.ActorID = project.LocalAdministratorID
		value, err := manager.Save(r.Context(), r.PathValue("projectID"), r.PathValue("key"), input)
		if err != nil {
			writeProfileError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toEffectiveProfileResponse(value))
	})
}

func toProfileDefinitionResponse(definition profile.Definition) profileDefinitionResponse {
	return profileDefinitionResponse{
		Key: definition.Key, Name: definition.Name, Description: definition.Description,
		Instructions: definition.Instructions, DefaultRuntime: definition.DefaultRuntime,
		SuggestedTools:        append([]string(nil), definition.SuggestedTools...),
		SuggestedCapabilities: append([]string(nil), definition.SuggestedCapabilities...),
	}
}

func toEffectiveProfileResponse(value profile.Effective) effectiveProfileResponse {
	return effectiveProfileResponse{
		Key: value.Key, Name: value.Name, Description: value.Description,
		Instructions: value.Instructions, DefaultRuntime: value.DefaultRuntime,
		Capabilities:          append([]string(nil), value.Capabilities...),
		Tools:                 append([]string(nil), value.Tools...),
		SuggestedTools:        append([]string(nil), value.SuggestedTools...),
		SuggestedCapabilities: append([]string(nil), value.SuggestedCapabilities...),
		Customized:            value.Customized,
	}
}

func writeProfileError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, profile.ErrUnknownProfile), errors.Is(err, profile.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", "profile not found")
	case errors.Is(err, profile.ErrProjectNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", "project not found")
	case errors.Is(err, profile.ErrProjectArchived):
		writeAPIError(w, http.StatusConflict, "conflict", "project is archived")
	case errors.Is(err, profile.ErrInvalidInput):
		writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
	default:
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
