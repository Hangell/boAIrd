package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLocalSessionStoreExpiresAndRevokesIdentifiers(t *testing.T) {
	store := newLocalSessionStore()
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	store.now = func() time.Time { return now }

	identifier, expiresAt, err := store.create("user-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(identifier) < 40 || expiresAt.Sub(now) != localSessionLifetime {
		t.Fatalf("created session = %q, expires at %s; want opaque ID and %s lifetime", identifier, expiresAt, localSessionLifetime)
	}
	userID, ok := store.valid(identifier)
	if !ok || userID != "user-1" {
		t.Fatalf("new session resolved to %q, %t; want user-1", userID, ok)
	}

	now = expiresAt
	if _, ok := store.valid(identifier); ok {
		t.Fatal("expired session was accepted")
	}
	request := httptest.NewRequest("GET", "/api/v1/projects", nil)
	request.AddCookie(&http.Cookie{Name: localAuthCookieName, Value: identifier})
	if _, ok := localConversationPrincipal(request, testLocalAuthTokenBytes(), store); ok {
		t.Fatal("authorization accepted an expired session")
	}
	store.revoke(identifier)
	if _, ok := store.valid(identifier); ok {
		t.Fatal("revoked session was accepted")
	}
	store.revokeUser("user-1")
}
