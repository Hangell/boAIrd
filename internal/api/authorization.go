package api

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"boAIrd/internal/capability"
	"boAIrd/internal/project"
	"boAIrd/internal/task"
)

type projectMembershipChecker interface {
	IsMember(context.Context, string, string) (bool, error)
}

type principalContextKey struct{}

type authorizationAuditWriter interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

var administratorCapabilities = []capability.Capability{
	"project.create", "project.read", "project.update", "project.archive",
	"credential.create", "credential.read", "credential.delete",
	"agent.create", "agent.read", "agent.update", "agent.scope.manage",
	"brain.attach", "brain.read", "brain.replace", "brain.execute",
	"run.read", "task.create", "task.read", "task.assign", "task.transition",
	"task.review", "task.complete", "conversation.read", "conversation.write", "mcp.token.manage",
}

func secureAPI(next http.Handler, database DatabaseHealth, projects ProjectManager, tasks TaskManager, runs RunManager, token []byte, sessions *localSessionStore) http.Handler {
	authorizer := capability.Authorizer{}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v1/") || r.URL.Path == "/api/v1/status" || r.URL.Path == "/api/v1/session" && (r.Method == http.MethodPost || r.Method == http.MethodDelete) {
			next.ServeHTTP(w, r)
			return
		}
		if _, ok := localConversationPrincipal(r, token, sessions); !ok {
			recordAuthorizationDenial(r.Context(), database, "", "", "", authorizationResource(r), "authentication_required")
			writeConversationUnauthorized(w)
			return
		}
		if r.Header.Get("Authorization") == "" && isUnsafeMethod(r.Method) {
			if !trustedDashboardOrigin(r.Header.Get("Origin"), r) {
				recordAuthorizationDenial(r.Context(), database, project.LocalAdministratorID, "", "", authorizationResource(r), "cookie_origin_rejected")
				writeAPIError(w, http.StatusForbidden, "forbidden", "browser origin is not authorized")
				return
			}
		}
		required, projectID, recognized := requiredAPICapability(r, projects, tasks, runs)
		if !recognized {
			next.ServeHTTP(w, r)
			return
		}
		principal := capability.Principal{
			ID:           project.LocalAdministratorID,
			Capabilities: append([]capability.Capability(nil), administratorCapabilities...),
		}
		if projectID != "" {
			membership, ok := projects.(projectMembershipChecker)
			if !ok {
				recordAuthorizationDenial(r.Context(), database, principal.ID, projectID, required, authorizationResource(r), "membership_checker_unavailable")
				writeAPIError(w, http.StatusForbidden, "forbidden", "project access is not authorized")
				return
			}
			member, err := membership.IsMember(r.Context(), projectID, principal.ID)
			if err != nil || !member {
				recordAuthorizationDenial(r.Context(), database, principal.ID, projectID, required, authorizationResource(r), "project_membership_required")
				writeAPIError(w, http.StatusForbidden, "forbidden", "project access is not authorized")
				return
			}
			principal.ProjectIDs = []string{projectID}
		}
		value, err := capability.New(required)
		if err != nil || authorizer.Authorize(principal, value, projectID) != nil {
			recordAuthorizationDenial(r.Context(), database, principal.ID, projectID, required, authorizationResource(r), "capability_denied")
			writeAPIError(w, http.StatusForbidden, "forbidden", "operation is not authorized")
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), principalContextKey{}, principal))
		captured := &statusCaptureWriter{ResponseWriter: w}
		next.ServeHTTP(captured, r)
		if captured.status == http.StatusForbidden {
			recordAuthorizationDenial(r.Context(), database, principal.ID, projectID, required, authorizationResource(r), "operation_denied")
		}
	})
}

func authorizationResource(r *http.Request) string {
	return r.Method + " /api/v1"
}

type statusCaptureWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusCaptureWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusCaptureWriter) Write(value []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(value)
}

func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func taskActorFromRequest(r *http.Request) task.Actor {
	principal, ok := r.Context().Value(principalContextKey{}).(capability.Principal)
	if !ok {
		return task.Actor{Type: task.ActorLocalAPI, ID: localTaskActorID}
	}
	capabilities := make([]string, 0, len(principal.Capabilities))
	for _, value := range principal.Capabilities {
		capabilities = append(capabilities, string(value))
	}
	return task.Actor{Type: task.ActorAdministrator, ID: principal.ID, Capabilities: capabilities, ProjectIDs: append([]string(nil), principal.ProjectIDs...)}
}

func requiredAPICapability(r *http.Request, projects ProjectManager, tasks TaskManager, runs RunManager) (string, string, bool) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 || parts[0] != "api" || parts[1] != "v1" {
		return "", "", false
	}
	method := r.Method
	switch {
	case len(parts) == 4 && parts[2] == "mcp" && parts[3] == "tokens" && (method == http.MethodGet || method == http.MethodPost):
		return "mcp.token.manage", "", true
	case len(parts) == 5 && parts[2] == "mcp" && parts[3] == "tokens" && parts[4] != "" && method == http.MethodDelete:
		return "mcp.token.manage", "", true
	case len(parts) == 6 && parts[2] == "mcp" && parts[3] == "tokens" && parts[4] != "" && parts[5] == "rotate" && method == http.MethodPost:
		return "mcp.token.manage", "", true
	case len(parts) == 3 && parts[2] == "projects" && method == http.MethodPost:
		return "project.create", "", true
	case len(parts) == 3 && parts[2] == "projects" && method == http.MethodGet:
		return "project.read", "", true
	case len(parts) >= 4 && parts[2] == "projects":
		projectID := parts[3]
		if len(parts) == 4 {
			switch method {
			case http.MethodGet:
				return "project.read", projectID, true
			case http.MethodPatch:
				return "project.update", projectID, true
			case http.MethodDelete:
				return "project.archive", projectID, true
			}
		}
		if len(parts) == 5 {
			switch parts[4] {
			case "tasks":
				if method == http.MethodPost {
					return "task.create", projectID, true
				}
				if method == http.MethodGet {
					return "task.read", projectID, true
				}
			case "brain":
				if method == http.MethodGet {
					return "brain.read", projectID, true
				}
				if method == http.MethodPost {
					return "brain.attach", projectID, true
				}
				if method == http.MethodPut {
					return "brain.replace", projectID, true
				}
			case "runs":
				if method == http.MethodGet {
					return "run.read", projectID, true
				}
			}
		}
		if len(parts) == 6 && parts[4] == "brain" && parts[5] == "runs" && method == http.MethodPost {
			return "brain.execute", projectID, true
		}
		if len(parts) == 6 && parts[4] == "agents" && (method == http.MethodPut || method == http.MethodDelete) {
			return "agent.scope.manage", projectID, true
		}
	case len(parts) == 3 && parts[2] == "agents":
		switch method {
		case http.MethodGet:
			return "agent.read", "", true
		case http.MethodPost:
			return "agent.create", "", true
		}
	case len(parts) == 4 && parts[2] == "agents":
		if method == http.MethodGet {
			return "agent.read", "", true
		}
		if method == http.MethodPatch {
			return "agent.update", "", true
		}
	case len(parts) == 5 && parts[2] == "agents" && parts[4] == "status-events" && method == http.MethodGet:
		return "agent.read", "", true
	case len(parts) == 3 && parts[2] == "credentials":
		switch method {
		case http.MethodGet:
			return "credential.read", "", true
		case http.MethodPost:
			return "credential.create", "", true
		}
	case len(parts) == 4 && parts[2] == "credentials" && method == http.MethodDelete:
		return "credential.delete", "", true
	case len(parts) == 4 && parts[2] == "tasks" && method == http.MethodGet:
		if tasks == nil {
			return "", "", true
		}
		value, err := tasks.Get(r.Context(), parts[3])
		if err != nil {
			return "", "", true
		}
		return "task.read", value.ProjectID(), true
	case len(parts) == 5 && parts[2] == "tasks":
		if tasks == nil {
			return "", "", true
		}
		value, err := tasks.Get(r.Context(), parts[3])
		if err != nil {
			return "", "", true
		}
		switch {
		case parts[4] == "messages" && method == http.MethodGet:
			return "conversation.read", value.ProjectID(), true
		case parts[4] == "messages" && method == http.MethodPost:
			return "conversation.write", value.ProjectID(), true
		case parts[4] == "assignment" && method == http.MethodPut:
			return "task.assign", value.ProjectID(), true
		case parts[4] == "transitions" && method == http.MethodPost:
			return "task.transition", value.ProjectID(), true
		}
	case len(parts) == 4 && parts[2] == "runs" && method == http.MethodGet:
		if runs == nil {
			return "", "", true
		}
		value, err := runs.Get(r.Context(), parts[3])
		if err != nil {
			return "", "", true
		}
		return "run.read", value.ProjectID(), true
	}
	return "", "", false
}

func recordAuthorizationDenial(ctx context.Context, database DatabaseHealth, principalID, projectID, required, resource, reason string) {
	principalID = truncateAuditValue(principalID, 200)
	projectID = truncateAuditValue(projectID, 200)
	required = truncateAuditValue(required, 100)
	resource = truncateAuditValue(resource, 256)
	reason = truncateAuditValue(reason, 100)
	writer, ok := database.(authorizationAuditWriter)
	if !ok {
		slog.WarnContext(ctx, "authorization denied", "principal_id", principalID, "resource", resource, "reason", reason)
		return
	}
	if _, err := writer.ExecContext(ctx, "INSERT INTO authorization_denials(principal_id, project_id, required_capability, resource, reason) VALUES (?, ?, ?, ?, ?)", principalID, projectID, required, resource, reason); err != nil {
		slog.ErrorContext(ctx, "record authorization denial", "error", fmt.Errorf("write denial audit: %w", err))
	}
}

func truncateAuditValue(value string, maximum int) string {
	if len(value) > maximum {
		return value[:maximum]
	}
	return value
}
