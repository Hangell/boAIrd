package api

import (
	"context"
	"errors"
	"net/http"

	"boAIrd/internal/credential"
)

type createCredentialRequest struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Secret   string `json:"secret"`
}

type credentialListResponse struct {
	Items []credential.Metadata `json:"items"`
}

type githubCredentialBindingManager interface {
	BindGitHub(context.Context, string, string, string, string) error
	UnbindGitHub(context.Context, string, string, string) error
}

type githubCredentialBindingRequest struct {
	CredentialID string `json:"credential_id"`
}

func registerCredentialRoutes(mux *http.ServeMux, manager CredentialManager) {
	mux.HandleFunc("POST /api/v1/credentials", func(w http.ResponseWriter, r *http.Request) {
		var input createCredentialRequest
		if err := decodeJSON(w, r, &input); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "invalid credential request")
			return
		}
		value, err := manager.Create(r.Context(), input.Name, input.Provider, input.Secret)
		if err != nil {
			writeCredentialError(w, err)
			return
		}
		w.Header().Set("Location", "/api/v1/credentials/"+value.ID)
		writeJSON(w, http.StatusCreated, value)
	})

	mux.HandleFunc("GET /api/v1/credentials", func(w http.ResponseWriter, r *http.Request) {
		values, err := manager.List(r.Context())
		if err != nil {
			writeCredentialError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, credentialListResponse{Items: values})
	})

	mux.HandleFunc("DELETE /api/v1/credentials/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := manager.Delete(r.Context(), r.PathValue("id")); err != nil {
			writeCredentialError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	if bindings, ok := manager.(githubCredentialBindingManager); ok {
		mux.HandleFunc("PUT /api/v1/projects/{projectID}/agents/{agentID}/github-credential", func(w http.ResponseWriter, r *http.Request) {
			var input githubCredentialBindingRequest
			if err := decodeJSON(w, r, &input); err != nil {
				writeAPIError(w, http.StatusBadRequest, "invalid_request", "invalid GitHub credential binding request")
				return
			}
			if err := bindings.BindGitHub(r.Context(), r.PathValue("projectID"), r.PathValue("agentID"), input.CredentialID, taskActorFromRequest(r).ID); err != nil {
				writeCredentialError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})

		mux.HandleFunc("DELETE /api/v1/projects/{projectID}/agents/{agentID}/github-credential", func(w http.ResponseWriter, r *http.Request) {
			if err := bindings.UnbindGitHub(r.Context(), r.PathValue("projectID"), r.PathValue("agentID"), taskActorFromRequest(r).ID); err != nil {
				writeCredentialError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	}
}

func writeCredentialError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, credential.ErrInvalidInput):
		writeAPIError(w, http.StatusBadRequest, "invalid_credential", "invalid credential data")
	case errors.Is(err, credential.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", "credential not found")
	case errors.Is(err, credential.ErrCredentialUnavailable):
		writeAPIError(w, http.StatusNotFound, "not_found", "GitHub credential or Agent binding is unavailable")
	default:
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
