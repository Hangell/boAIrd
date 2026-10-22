package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"boAIrd/internal/agent"
	"boAIrd/internal/brain"
	"boAIrd/internal/credential"
	"boAIrd/internal/project"
	"boAIrd/internal/run"
	"boAIrd/internal/task"
)

const statusCheckTimeout = 2 * time.Second

// DatabaseHealth is the database behavior required by the status endpoint.
type DatabaseHealth interface {
	PingContext(context.Context) error
}

type statusResponse struct {
	Status   string `json:"status"`
	Database string `json:"database"`
}

type ProjectManager interface {
	Create(context.Context, project.CreateInput) (project.Project, error)
	Get(context.Context, string) (project.Project, error)
	List(context.Context, int, int) ([]project.Project, error)
	Update(context.Context, string, project.UpdateInput) (project.Project, error)
	Archive(context.Context, string) (project.Project, error)
}

type CredentialManager interface {
	Create(context.Context, string, string, string) (credential.Metadata, error)
	List(context.Context) ([]credential.Metadata, error)
	Delete(context.Context, string) error
}

type AgentManager interface {
	Create(context.Context, agent.CreateInput) (agent.Agent, error)
	Get(context.Context, string) (agent.Agent, error)
	List(context.Context, int, int) ([]agent.Agent, error)
	Update(context.Context, string, agent.UpdateInput) (agent.Agent, error)
}

type BrainManager interface {
	Create(context.Context, string, brain.CreateInput) (brain.Brain, error)
	Get(context.Context, string) (brain.Brain, error)
	Execute(context.Context, string, string, int) (brain.Execution, error)
}

type RunManager interface {
	Get(context.Context, string) (run.Run, error)
	List(context.Context, string, string, string, int, int) ([]run.Run, error)
	ListAuditEvents(context.Context, string) ([]run.AuditEvent, error)
	ListAuditEventsForRuns(context.Context, []string) (map[string][]run.AuditEvent, error)
}

type TaskManager interface {
	Create(context.Context, string, task.CreateInput, task.Actor) (task.Task, error)
	Get(context.Context, string) (task.Task, error)
	List(context.Context, string, task.Status, string, int, int) ([]task.Task, error)
	Assign(context.Context, string, string, string, task.Actor) (task.Task, error)
	Transition(context.Context, string, task.Status, string, task.Actor) (task.Task, error)
	ListAuditEvents(context.Context, string) ([]task.AuditEvent, error)
}

// NewHandler returns the HTTP handler for the configured service endpoints.
func NewHandler(database DatabaseHealth, projects ProjectManager, credentials ...CredentialManager) http.Handler {
	var credentialManager CredentialManager
	if len(credentials) > 0 {
		credentialManager = credentials[0]
	}
	return newHandler(database, projects, credentialManager, nil, nil, nil, nil)
}

// NewHandlerWithAgents returns a handler with project, credential, and agent routes.
func NewHandlerWithAgents(database DatabaseHealth, projects ProjectManager, credentials CredentialManager, agents AgentManager) http.Handler {
	return newHandler(database, projects, credentials, agents, nil, nil, nil)
}

func NewHandlerWithBrain(database DatabaseHealth, projects ProjectManager, credentials CredentialManager, agents AgentManager, brains BrainManager, runs RunManager) http.Handler {
	return newHandler(database, projects, credentials, agents, brains, runs, nil)
}

func NewHandlerWithTasks(database DatabaseHealth, projects ProjectManager, credentials CredentialManager, agents AgentManager, brains BrainManager, runs RunManager, tasks TaskManager) http.Handler {
	return newHandler(database, projects, credentials, agents, brains, runs, tasks)
}

func newHandler(database DatabaseHealth, projects ProjectManager, credentials CredentialManager, agents AgentManager, brains BrainManager, runs RunManager, tasks TaskManager) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/v1/status", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), statusCheckTimeout)
		defer cancel()

		if err := database.PingContext(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, statusResponse{
				Status:   "unavailable",
				Database: "disconnected",
			})
			return
		}
		writeJSON(w, http.StatusOK, statusResponse{
			Status:   "ok",
			Database: "connected",
		})
	})
	if projects != nil {
		registerProjectRoutes(mux, projects)
	}
	if credentials != nil {
		registerCredentialRoutes(mux, credentials)
	}
	if agents != nil {
		registerAgentRoutes(mux, agents)
	}
	if brains != nil {
		registerBrainRoutes(mux, brains)
	}
	if runs != nil {
		registerRunRoutes(mux, runs)
	}
	if tasks != nil {
		registerTaskRoutes(mux, tasks)
	}
	return mux
}

// Serve listens on address and gracefully shuts down when ctx is cancelled.
func Serve(ctx context.Context, address string, handler http.Handler, logger *slog.Logger) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", address, err)
	}
	return serve(ctx, listener, address, handler, logger)
}

func serve(ctx context.Context, listener net.Listener, address string, handler http.Handler, logger *slog.Logger) error {
	server := &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- server.Serve(listener)
	}()

	logger.Info("HTTP server listening", "address", address)
	select {
	case err := <-serveErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil
	case <-ctx.Done():
		logger.Info("shutting down HTTP server", "reason", ctx.Err())
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			closeErr := server.Close()
			return errors.Join(fmt.Errorf("gracefully shut down HTTP server: %w", err), closeErr)
		}
		if err := <-serveErrors; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("finish HTTP server shutdown: %w", err)
		}
		return nil
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(append(body, '\n')); err != nil {
		// A client may disconnect after the response headers are sent.
		slog.Error("write HTTP response", "error", err)
	}
}
