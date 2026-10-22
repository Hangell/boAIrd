package api

import (
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"

	"boAIrd/internal/project"
)

func localConversationPrincipal(r *http.Request, expectedToken []byte, sessions *localSessionStore) (string, bool) {
	if len(expectedToken) != 32 {
		return "", false
	}
	authorization := r.Header.Get("Authorization")
	if authorization == "" {
		cookie, err := r.Cookie(localAuthCookieName)
		if err != nil {
			return "", false
		}
		if sessions == nil || !sessions.valid(cookie.Value) {
			return "", false
		}
		return project.LocalAdministratorID, true
	}
	return localBearerPrincipal(authorization, expectedToken)
}

func localBearerPrincipal(authorization string, expectedToken []byte) (string, bool) {
	if len(expectedToken) != 32 {
		return "", false
	}
	scheme, token, ok := strings.Cut(authorization, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || len(token) != 64 {
		return "", false
	}
	providedToken, err := hex.DecodeString(token)
	if err != nil || len(providedToken) != len(expectedToken) {
		return "", false
	}
	if subtle.ConstantTimeCompare(expectedToken, providedToken) != 1 {
		return "", false
	}
	return project.LocalAdministratorID, true
}

func writeConversationUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="boAIrd"`)
	writeAPIError(w, http.StatusUnauthorized, "unauthorized", "a valid local API bearer token is required")
}
