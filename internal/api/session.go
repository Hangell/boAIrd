package api

import (
	"net/http"
	"strings"
)

const localAuthCookieName = "boaird_local_auth"

func registerSessionRoutes(mux *http.ServeMux, database DatabaseHealth, expectedToken []byte) {
	mux.HandleFunc("POST /api/v1/session", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := localConversationPrincipal(r, expectedToken); !ok || r.Header.Get("Authorization") == "" {
			recordAuthorizationDenial(r.Context(), database, "", "", "", "POST /api/v1/session", "authentication_required")
			writeConversationUnauthorized(w)
			return
		}
		_, token, _ := strings.Cut(r.Header.Get("Authorization"), " ")
		cookie := &http.Cookie{Name: localAuthCookieName, Value: token, Path: "/api/v1", HttpOnly: true, SameSite: http.SameSiteStrictMode}
		if r.TLS != nil {
			cookie.Secure = true
		}
		http.SetCookie(w, cookie)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/v1/session", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := localConversationPrincipal(r, expectedToken); !ok {
			recordAuthorizationDenial(r.Context(), database, "", "", "", "GET /api/v1/session", "authentication_required")
			writeConversationUnauthorized(w)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /api/v1/session", func(w http.ResponseWriter, r *http.Request) {
		cookie := &http.Cookie{Name: localAuthCookieName, Value: "", Path: "/api/v1", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode}
		if r.TLS != nil {
			cookie.Secure = true
		}
		http.SetCookie(w, cookie)
		w.WriteHeader(http.StatusNoContent)
	})
}
