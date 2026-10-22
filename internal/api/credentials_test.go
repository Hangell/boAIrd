package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"boAIrd/internal/credential"
)

type credentialStub struct{ value credential.Metadata }

func (s credentialStub) Create(_ context.Context, name, provider, _ string) (credential.Metadata, error) {
	return credential.Metadata{ID: "cred-1", Name: name, Provider: provider}, nil
}
func (s credentialStub) List(context.Context) ([]credential.Metadata, error) {
	return []credential.Metadata{s.value}, nil
}
func (s credentialStub) Delete(context.Context, string) error { return nil }

type failedCredentialStub struct{ credentialStub }

func (failedCredentialStub) Create(context.Context, string, string, string) (credential.Metadata, error) {
	return credential.Metadata{}, credential.ErrInvalidInput
}

func TestCredentialEndpointsNeverReturnSubmittedSecret(t *testing.T) {
	secret := "fake-api-key-123456789"
	manager := credentialStub{value: credential.Metadata{ID: "cred-1", Name: "test", Provider: "openai"}}
	handler := newHandler(fakeDatabase{}, nil, manager, nil, nil, nil, nil, nil, testLocalAuthTokenBytes())
	request := authorizedTestRequest(http.MethodPost, "/api/v1/credentials", strings.NewReader(`{"name":"test","provider":"openai","secret":"`+secret+`"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), secret) || !strings.Contains(response.Body.String(), "cred-1") {
		t.Fatalf("unsafe create response: %s", response.Body.String())
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedTestRequest(http.MethodGet, "/api/v1/credentials", nil))
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), secret) {
		t.Fatalf("unsafe list response: %d %s", response.Code, response.Body.String())
	}
}

func TestCredentialEndpointDoesNotEchoInvalidSecret(t *testing.T) {
	handler := newHandler(fakeDatabase{}, nil, failedCredentialStub{}, nil, nil, nil, nil, nil, testLocalAuthTokenBytes())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authorizedTestRequest(http.MethodPost, "/api/v1/credentials", strings.NewReader(`{"name":"x","provider":"openai","secret":"bad-secret"}`)))
	if response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), "bad-secret") {
		t.Fatalf("unexpected error response: %d %s", response.Code, response.Body.String())
	}
}
