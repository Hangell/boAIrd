package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"boAIrd/internal/project"
)

const maximumProjectRequestSize = 1 << 20

type projectResponse struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	Description         string         `json:"description"`
	Status              project.Status `json:"status"`
	BrainID             string         `json:"brain_id,omitempty"`
	WorkspacePath       string         `json:"workspace_path,omitempty"`
	RepositoryURL       string         `json:"repository_url,omitempty"`
	HumanReviewRequired bool           `json:"human_review_required"`
}

type projectListResponse struct {
	Items  []projectResponse `json:"items"`
	Limit  int               `json:"limit"`
	Offset int               `json:"offset"`
}

type apiError struct {
	Error apiErrorDetail `json:"error"`
}

type apiErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type userScopedProjectLister interface {
	ListForUser(context.Context, string, int, int) ([]project.Project, error)
}

func registerProjectRoutes(mux *http.ServeMux, manager ProjectManager, users UserManager) {
	mux.HandleFunc("POST /api/v1/projects", func(w http.ResponseWriter, r *http.Request) {
		var input project.CreateInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		value, err := manager.Create(r.Context(), input)
		if err != nil {
			writeProjectError(w, err)
			return
		}
		w.Header().Set("Location", "/api/v1/projects/"+value.ID())
		writeJSON(w, http.StatusCreated, toProjectResponse(value))
	})

	mux.HandleFunc("GET /api/v1/projects", func(w http.ResponseWriter, r *http.Request) {
		limit, err := queryInteger(r, "limit", 0)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		if r.URL.Query().Has("limit") && limit == 0 {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "query parameter limit must be between 1 and 100")
			return
		}
		offset, err := queryInteger(r, "offset", 0)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		if limit == 0 {
			limit = 50
		}
		var values []project.Project
		if principalIsAdministrator(r) {
			values, err = manager.List(r.Context(), limit, offset)
		} else {
			principal, ok := principalID(r)
			if !ok || users == nil {
				writeAPIError(w, http.StatusForbidden, "forbidden", "project access is not authorized")
				return
			}
			lister, supported := manager.(userScopedProjectLister)
			if !supported {
				writeAPIError(w, http.StatusForbidden, "forbidden", "project access is not authorized")
				return
			}
			values, err = lister.ListForUser(r.Context(), principal, limit, offset)
		}
		if err != nil {
			writeProjectError(w, err)
			return
		}
		items := make([]projectResponse, 0, len(values))
		for _, value := range values {
			items = append(items, toProjectResponse(value))
		}
		writeJSON(w, http.StatusOK, projectListResponse{Items: items, Limit: limit, Offset: offset})
	})

	mux.HandleFunc("GET /api/v1/projects/{id}", func(w http.ResponseWriter, r *http.Request) {
		value, err := manager.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeProjectError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toProjectResponse(value))
	})

	mux.HandleFunc("PATCH /api/v1/projects/{id}", func(w http.ResponseWriter, r *http.Request) {
		var input project.UpdateInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		value, err := manager.Update(r.Context(), r.PathValue("id"), input)
		if err != nil {
			writeProjectError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toProjectResponse(value))
	})

	mux.HandleFunc("DELETE /api/v1/projects/{id}", func(w http.ResponseWriter, r *http.Request) {
		value, err := manager.Archive(r.Context(), r.PathValue("id"))
		if err != nil {
			writeProjectError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, toProjectResponse(value))
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	return decodeJSONWithLimit(w, r, target, maximumProjectRequestSize)
}

func decodeJSONWithLimit(w http.ResponseWriter, r *http.Request, target any, maximumSize int64) (resultErr error) {
	defer func() {
		if err := r.Body.Close(); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close request body: %w", err))
		}
	}()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maximumSize))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode request body: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain one JSON value")
		}
		return fmt.Errorf("decode trailing request data: %w", err)
	}
	return nil
}

func queryInteger(r *http.Request, key string, fallback int) (int, error) {
	value := r.URL.Query().Get(key)
	if value == "" {
		if r.URL.Query().Has(key) {
			return 0, fmt.Errorf("query parameter %s must be an integer", key)
		}
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("query parameter %s must be an integer", key)
	}
	return parsed, nil
}

func toProjectResponse(value project.Project) projectResponse {
	snapshot := value.Snapshot()
	return projectResponse{
		ID: snapshot.ID, Name: snapshot.Name, Description: snapshot.Description, Status: snapshot.Status,
		BrainID: snapshot.BrainID, WorkspacePath: snapshot.WorkspacePath, RepositoryURL: snapshot.RepositoryURL,
		HumanReviewRequired: snapshot.HumanReviewRequired,
	}
}

func writeProjectError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, project.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", "project not found")
	case errors.Is(err, project.ErrAlreadyExists):
		writeAPIError(w, http.StatusConflict, "already_exists", "project already exists")
	case errors.Is(err, project.ErrInvalidInput):
		writeAPIError(w, http.StatusBadRequest, "invalid_project", err.Error())
	default:
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, apiError{Error: apiErrorDetail{Code: code, Message: message}})
}
