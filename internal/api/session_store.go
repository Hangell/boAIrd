package api

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

const localSessionLifetime = 30 * time.Minute
const localSessionIDLength = 43

type sessionEntry struct {
	userID    string
	expiresAt time.Time
}

type localSessionStore struct {
	mu       sync.Mutex
	sessions map[string]sessionEntry
	now      func() time.Time
}

func newLocalSessionStore() *localSessionStore {
	return &localSessionStore{sessions: make(map[string]sessionEntry), now: time.Now}
}

func (s *localSessionStore) create(userID string) (string, time.Time, error) {
	var identifier [32]byte
	if _, err := rand.Read(identifier[:]); err != nil {
		return "", time.Time{}, err
	}
	value := base64.RawURLEncoding.EncodeToString(identifier[:])
	expiresAt := s.now().Add(localSessionLifetime)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeExpiredLocked(s.now())
	s.sessions[value] = sessionEntry{userID: userID, expiresAt: expiresAt}
	return value, expiresAt, nil
}

// valid returns the user ID bound to a live session.
func (s *localSessionStore) valid(value string) (string, bool) {
	if len(value) != localSessionIDLength {
		return "", false
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.sessions[value]
	if !ok {
		return "", false
	}
	if !now.Before(entry.expiresAt) {
		delete(s.sessions, value)
		return "", false
	}
	return entry.userID, true
}

func (s *localSessionStore) revoke(value string) {
	s.mu.Lock()
	delete(s.sessions, value)
	s.mu.Unlock()
}

// revokeUser removes every session bound to a user, enforcing immediate
// revocation when a user is deleted or loses project membership.
func (s *localSessionStore) revokeUser(userID string) {
	s.mu.Lock()
	for value, entry := range s.sessions {
		if entry.userID == userID {
			delete(s.sessions, value)
		}
	}
	s.mu.Unlock()
}

func (s *localSessionStore) removeExpiredLocked(now time.Time) {
	for value, entry := range s.sessions {
		if !now.Before(entry.expiresAt) {
			delete(s.sessions, value)
		}
	}
}
