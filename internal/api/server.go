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

// NewHandler returns the HTTP handler for the initial service endpoints.
func NewHandler(database DatabaseHealth) http.Handler {
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
