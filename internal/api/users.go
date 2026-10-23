package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"boAIrd/internal/user"
)

// UserManager is the subset of the user service exposed to the HTTP API.
type UserManager interface {
	Create(context.Context, string, string, user.Role) (user.User, error)
	List(context.Context) ([]user.User, error)
	Get(context.Context, string) (user.User, error)
	Delete(context.Context, string) error
	GrantProject(context.Context, string, string, user.MembershipRole) error
	RevokeProject(context.Context, string, string) error
}

type userOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

type userCreateInput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role,omitempty"`
}

type memberGrantInput struct {
	Role string `json:"role"`
}

func registerUserRoutes(mux *http.ServeMux, manager UserManager, sessions *localSessionStore) {
	mux.HandleFunc("GET /api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		values, err := manager.List(r.Context())
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "internal_error", "could not list users")
			return
		}
		output := make([]userOutput, 0, len(values))
		for _, value := range values {
			output = append(output, userOutput{ID: value.ID(), Name: value.Name(), Role: string(value.Role())})
		}
		writeJSON(w, http.StatusOK, output)
	})
	mux.HandleFunc("POST /api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		var input userCreateInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "request body is invalid")
			return
		}
		role := user.Role(strings.TrimSpace(input.Role))
		if role == "" {
			role = user.RoleMember
		}
		value, err := manager.Create(r.Context(), input.ID, input.Name, role)
		if err != nil {
			writeUserError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, userOutput{ID: value.ID(), Name: value.Name(), Role: string(value.Role())})
	})
	mux.HandleFunc("GET /api/v1/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		value, err := manager.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeUserError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, userOutput{ID: value.ID(), Name: value.Name(), Role: string(value.Role())})
	})
	mux.HandleFunc("DELETE /api/v1/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := manager.Delete(r.Context(), r.PathValue("id")); err != nil {
			writeUserError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("PUT /api/v1/projects/{id}/members/{userID}", func(w http.ResponseWriter, r *http.Request) {
		var input memberGrantInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "request body is invalid")
			return
		}
		role := user.MembershipRole(strings.ToUpper(strings.TrimSpace(input.Role)))
		if role != user.MembershipOwner && role != user.MembershipMember {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "role must be ADMIN or MEMBER")
			return
		}
		if err := manager.GrantProject(r.Context(), r.PathValue("id"), r.PathValue("userID"), role); err != nil {
			writeUserError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /api/v1/projects/{id}/members/{userID}", func(w http.ResponseWriter, r *http.Request) {
		if err := manager.RevokeProject(r.Context(), r.PathValue("id"), r.PathValue("userID")); err != nil {
			writeUserError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v1/users/{id}/sessions", func(w http.ResponseWriter, r *http.Request) {
		value, err := manager.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeUserError(w, err)
			return
		}
		identifier, expiresAt, err := sessions.create(value.ID())
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "internal_error", "could not create a user session")
			return
		}
		cookie := &http.Cookie{Name: localAuthCookieName, Value: identifier, Path: "/api/v1", Expires: expiresAt, MaxAge: int(localSessionLifetime.Seconds()), HttpOnly: true, SameSite: http.SameSiteStrictMode}
		if r.TLS != nil {
			cookie.Secure = true
		}
		http.SetCookie(w, cookie)
		w.WriteHeader(http.StatusNoContent)
	})
}

func writeUserError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, user.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", "user not found")
	case errors.Is(err, user.ErrAlreadyExists):
		writeAPIError(w, http.StatusConflict, "conflict", "user already exists")
	case errors.Is(err, user.ErrInvalidInput):
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "user input is invalid")
	case errors.Is(err, user.ErrBootstrapAdministrator):
		writeAPIError(w, http.StatusForbidden, "forbidden", "the bootstrap administrator cannot be modified")
	case errors.Is(err, user.ErrProjectNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", "project not found")
	default:
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "could not update user")
	}
}
