package api

import (
	"net/http"
	"net/url"
	"strings"
)

var dashboardOrigins = map[string]struct{}{
	"http://localhost:4200":  {},
	"http://127.0.0.1:4200":  {},
	"https://localhost:4200": {},
	"https://127.0.0.1:4200": {},
}

func trustedDashboardOrigin(origin string, r *http.Request) bool {
	if _, allowed := dashboardOrigins[origin]; allowed {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return parsed.Scheme == scheme && parsed.Host == r.Host
}

func corsAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		_, allowed := dashboardOrigins[origin]
		if origin != "" && allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Add("Vary", "Origin")
		}
		if r.Method == http.MethodOptions && origin != "" {
			if !allowed {
				http.Error(w, "origin is not allowed", http.StatusForbidden)
				return
			}
			requestedHeaders := strings.ToLower(r.Header.Get("Access-Control-Request-Headers"))
			for _, header := range strings.Split(requestedHeaders, ",") {
				header = strings.TrimSpace(header)
				if header != "" && header != "authorization" && header != "content-type" {
					http.Error(w, "request header is not allowed", http.StatusForbidden)
					return
				}
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
