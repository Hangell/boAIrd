package api

import (
	"errors"
	"net/http"
	"time"

	"boAIrd/internal/mcptoken"
)

type mcpTokenRequest struct {
	AgentID          string `json:"agent_id,omitempty"`
	ExpiresInSeconds int64  `json:"expires_in_seconds"`
}

type mcpTokenIssuedResponse struct {
	Token string `json:"token"`
	mcptoken.Metadata
}

type mcpTokenListResponse struct {
	Items []mcptoken.Metadata `json:"items"`
}

func registerMCPTokenRoutes(mux *http.ServeMux, manager MCPTokenManager) {
	mux.HandleFunc("GET /api/v1/mcp/tokens", func(w http.ResponseWriter, r *http.Request) {
		values, err := manager.List(r.Context())
		if err != nil {
			writeMCPTokenError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, mcpTokenListResponse{Items: values})
	})
	mux.HandleFunc("POST /api/v1/mcp/tokens", func(w http.ResponseWriter, r *http.Request) {
		var input mcpTokenRequest
		if err := decodeJSON(w, r, &input); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "invalid MCP token request")
			return
		}
		if !validMCPTokenLifetime(input.ExpiresInSeconds) {
			writeAPIError(w, http.StatusBadRequest, "invalid_mcp_token_request", "expires_in_seconds must be between 1 and 31536000")
			return
		}
		metadata, token, err := manager.Create(r.Context(), input.AgentID, time.Duration(input.ExpiresInSeconds)*time.Second, taskActorFromRequest(r).ID)
		if err != nil {
			writeMCPTokenError(w, err)
			return
		}
		w.Header().Set("Location", "/api/v1/mcp/tokens/"+metadata.ID)
		writeJSON(w, http.StatusCreated, mcpTokenIssuedResponse{Token: token, Metadata: metadata})
	})
	mux.HandleFunc("POST /api/v1/mcp/tokens/{id}/rotate", func(w http.ResponseWriter, r *http.Request) {
		var input mcpTokenRequest
		if err := decodeJSON(w, r, &input); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "invalid MCP token request")
			return
		}
		if !validMCPTokenLifetime(input.ExpiresInSeconds) {
			writeAPIError(w, http.StatusBadRequest, "invalid_mcp_token_request", "expires_in_seconds must be between 1 and 31536000")
			return
		}
		metadata, token, err := manager.Rotate(r.Context(), r.PathValue("id"), time.Duration(input.ExpiresInSeconds)*time.Second, taskActorFromRequest(r).ID)
		if err != nil {
			writeMCPTokenError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, mcpTokenIssuedResponse{Token: token, Metadata: metadata})
	})
	mux.HandleFunc("DELETE /api/v1/mcp/tokens/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := manager.Revoke(r.Context(), r.PathValue("id"), taskActorFromRequest(r).ID); err != nil {
			writeMCPTokenError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func validMCPTokenLifetime(value int64) bool { return value >= 1 && value <= 365*24*60*60 }

func writeMCPTokenError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, mcptoken.ErrInvalidInput):
		writeAPIError(w, http.StatusBadRequest, "invalid_mcp_token_request", "invalid MCP token request")
	case errors.Is(err, mcptoken.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", "MCP token not found")
	case errors.Is(err, mcptoken.ErrInactive):
		writeAPIError(w, http.StatusConflict, "mcp_token_inactive", "MCP token is no longer active")
	default:
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
