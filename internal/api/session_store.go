package api

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

const localSessionLifetime = 30 * time.Minute
const localSessionIDLength = 43

type localSessionStore struct {
	mu       sync.Mutex
	sessions map[string]time.Time
	now      func() time.Time
}

func newLocalSessionStore() *localSessionStore {
	return &localSessionStore{sessions: make(map[string]time.Time), now: time.Now}
}

func (s *localSessionStore) create() (string, time.Time, error) {
	var identifier [32]byte
	if _, err := rand.Read(identifier[:]); err != nil {
		return "", time.Time{}, err
	}
	value := base64.RawURLEncoding.EncodeToString(identifier[:])
	expiresAt := s.now().Add(localSessionLifetime)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeExpiredLocked(s.now())
	s.sessions[value] = expiresAt
	return value, expiresAt, nil
}

func (s *localSessionStore) valid(value string) bool {
	if len(value) != localSessionIDLength {
		return false
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	expiresAt, ok := s.sessions[value]
	if !ok {
		return false
	}
	if !now.Before(expiresAt) {
		delete(s.sessions, value)
		return false
	}
	return true
}

func (s *localSessionStore) revoke(value string) {
	s.mu.Lock()
	delete(s.sessions, value)
	s.mu.Unlock()
}

func (s *localSessionStore) removeExpiredLocked(now time.Time) {
	for value, expiresAt := range s.sessions {
		if !now.Before(expiresAt) {
			delete(s.sessions, value)
		}
	}
}
