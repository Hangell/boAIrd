package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeDatabase struct {
	err error
}

func (f fakeDatabase) PingContext(context.Context) error {
	return f.err
}

func TestHealthEndpoint(t *testing.T) {
	handler := NewHandler(fakeDatabase{err: errors.New("database is down")}, nil)
	recorder := request(t, handler, http.MethodGet, "/health")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got, want := strings.TrimSpace(recorder.Body.String()), `{"status":"ok"}`; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestHandlerRegistersOptionalMCPEndpointOutsideRESTAuthentication(t *testing.T) {
	mcpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp" {
			t.Errorf("MCP path = %q, want /mcp", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	handler := NewHandlerWithConversations(nil, nil, nil, nil, nil, nil, nil, nil, []byte(testLocalAuthToken), mcpHandler)
	recorder := request(t, handler, http.MethodPost, "/mcp")
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("MCP endpoint status = %d, want 204", recorder.Code)
	}
}

func TestStatusEndpointReportsSQLiteState(t *testing.T) {
	tests := []struct {
		name       string
		database   fakeDatabase
		statusCode int
		body       string
	}{
		{
			name:       "connected",
			database:   fakeDatabase{},
			statusCode: http.StatusOK,
			body:       `{"status":"ok","database":"connected"}`,
		},
		{
			name:       "disconnected",
			database:   fakeDatabase{err: errors.New("database is down")},
			statusCode: http.StatusServiceUnavailable,
			body:       `{"status":"unavailable","database":"disconnected"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := request(t, NewHandler(tt.database, nil), http.MethodGet, "/api/v1/status")
			if recorder.Code != tt.statusCode {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.statusCode)
			}
			if got := strings.TrimSpace(recorder.Body.String()); got != tt.body {
				t.Fatalf("body = %q, want %q", got, tt.body)
			}
		})
	}
}

func TestServeShutsDownWhenContextIsCancelled(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	address := listener.Addr().String()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	go func() {
		done <- serve(ctx, listener, address, NewHandler(fakeDatabase{}, nil), logger)
	}()

	client := &http.Client{Timeout: 2 * time.Second}
	var response *http.Response
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		response, err = client.Get("http://" + address + "/health")
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		cancel()
		t.Fatalf("request health endpoint: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		cancel()
		t.Fatalf("health status = %d, want %d", response.StatusCode, http.StatusOK)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve after cancellation: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not shut down after context cancellation")
	}

	if _, err := client.Get("http://" + address + "/health"); err == nil {
		t.Fatal("server accepted a request after shutdown")
	}
}

func TestServeReportsListenerStartupFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	err = Serve(context.Background(), listener.Addr().String(), NewHandler(fakeDatabase{}, nil), logger)
	if err == nil {
		t.Fatal("Serve() succeeded while the address was already in use")
	}
	if !strings.Contains(err.Error(), "listen on") {
		t.Fatalf("Serve() error = %q, want a listener startup error", err)
	}
}

func request(t *testing.T, handler http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest(method, target, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+testLocalAuthToken)
	handler.ServeHTTP(recorder, request)
	return recorder
}
