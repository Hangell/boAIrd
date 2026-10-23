package api

import (
	"context"
	"errors"
	"net/http"

	"boAIrd/internal/extension"
	"boAIrd/internal/project"
)

// ExtensionManager is the subset of the extension service exposed to the API.
type ExtensionManager interface {
	Install(context.Context, extension.Manifest, string) (extension.Extension, error)
	List(context.Context) ([]extension.Extension, error)
	Get(context.Context, string) (extension.Extension, error)
	Activate(context.Context, string, string) (extension.Extension, error)
	Deactivate(context.Context, string, string) error
}

type extensionOutput struct {
	Name    string                      `json:"name"`
	Version string                      `json:"version"`
	Status  string                      `json:"status"`
	Tools   []extension.ToolDeclaration `json:"tools"`
}

func registerExtensionRoutes(mux *http.ServeMux, manager ExtensionManager) {
	mux.HandleFunc("GET /api/v1/extensions", func(w http.ResponseWriter, r *http.Request) {
		values, err := manager.List(r.Context())
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "internal_error", "could not list extensions")
			return
		}
		output := make([]extensionOutput, 0, len(values))
		for _, value := range values {
			output = append(output, extensionView(value))
		}
		writeJSON(w, http.StatusOK, output)
	})
	mux.HandleFunc("POST /api/v1/extensions", func(w http.ResponseWriter, r *http.Request) {
		var manifest extension.Manifest
		if err := decodeJSON(w, r, &manifest); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "request body is invalid")
			return
		}
		value, err := manager.Install(r.Context(), manifest, project.LocalAdministratorID)
		if err != nil {
			writeExtensionError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, extensionView(value))
	})
	mux.HandleFunc("GET /api/v1/extensions/{name}", func(w http.ResponseWriter, r *http.Request) {
		value, err := manager.Get(r.Context(), r.PathValue("name"))
		if err != nil {
			writeExtensionError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, extensionView(value))
	})
	mux.HandleFunc("PUT /api/v1/extensions/{name}/activate", func(w http.ResponseWriter, r *http.Request) {
		value, err := manager.Activate(r.Context(), r.PathValue("name"), project.LocalAdministratorID)
		if err != nil {
			writeExtensionError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, extensionView(value))
	})
	mux.HandleFunc("DELETE /api/v1/extensions/{name}", func(w http.ResponseWriter, r *http.Request) {
		if err := manager.Deactivate(r.Context(), r.PathValue("name"), project.LocalAdministratorID); err != nil {
			writeExtensionError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func extensionView(value extension.Extension) extensionOutput {
	return extensionOutput{Name: value.Name(), Version: value.Version(), Status: string(value.Status()), Tools: value.Tools()}
}

func writeExtensionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, extension.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", "extension not found")
	case errors.Is(err, extension.ErrAlreadyExists):
		writeAPIError(w, http.StatusConflict, "conflict", "extension already exists")
	case errors.Is(err, extension.ErrInvalidManifest):
		writeAPIError(w, http.StatusBadRequest, "invalid_manifest", "extension manifest is invalid")
	case errors.Is(err, extension.ErrWrongStatus):
		writeAPIError(w, http.StatusConflict, "wrong_status", "extension is not in the required status")
	default:
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "could not update extension")
	}
}
