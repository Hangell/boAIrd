package api

import (
	"net/http"

	"boAIrd/internal/project"
)

const localAuthCookieName = "boaird_local_auth"

func registerSessionRoutes(mux *http.ServeMux, database DatabaseHealth, expectedToken []byte, sessions *localSessionStore) {
	mux.HandleFunc("POST /api/v1/session", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := localBearerPrincipal(r.Header.Get("Authorization"), expectedToken); !ok {
			recordAuthorizationDenial(r.Context(), database, "", "", "", "POST /api/v1/session", "authentication_required")
			writeConversationUnauthorized(w)
			return
		}
		identifier, expiresAt, err := sessions.create()
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "internal_error", "could not create a browser session")
			return
		}
		cookie := &http.Cookie{Name: localAuthCookieName, Value: identifier, Path: "/api/v1", Expires: expiresAt, MaxAge: int(localSessionLifetime.Seconds()), HttpOnly: true, SameSite: http.SameSiteStrictMode}
		if r.TLS != nil {
			cookie.Secure = true
		}
		http.SetCookie(w, cookie)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/v1/session", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := localConversationPrincipal(r, expectedToken, sessions); !ok {
			recordAuthorizationDenial(r.Context(), database, "", "", "", "GET /api/v1/session", "authentication_required")
			writeConversationUnauthorized(w)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /api/v1/session", func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie(localAuthCookieName); err == nil {
			if !trustedDashboardOrigin(r.Header.Get("Origin"), r) {
				recordAuthorizationDenial(r.Context(), database, project.LocalAdministratorID, "", "", "DELETE /api/v1/session", "cookie_origin_rejected")
				writeAPIError(w, http.StatusForbidden, "forbidden", "browser origin is not authorized")
				return
			}
			sessions.revoke(cookie.Value)
		}
		cookie := &http.Cookie{Name: localAuthCookieName, Value: "", Path: "/api/v1", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode}
		if r.TLS != nil {
			cookie.Secure = true
		}
		http.SetCookie(w, cookie)
		w.WriteHeader(http.StatusNoContent)
	})
}
