// Package mcptoken manages expiring bearer credentials for MCP Agents.
package mcptoken

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"boAIrd/internal/agent"
	"boAIrd/internal/config"
)

const (
	maxLifetime       = 365 * 24 * time.Hour
	bootstrapLifetime = 90 * 24 * time.Hour
)

var (
	ErrInvalidInput = errors.New("invalid MCP token input")
	ErrNotFound     = errors.New("MCP token not found")
	ErrInactive     = errors.New("MCP token is not active")
)

type Record struct {
	ID        string
	AgentID   string
	Digest    [32]byte
	IssuedAt  time.Time
	ExpiresAt time.Time
	RevokedAt *time.Time
}

// Identity is the authenticated Agent behind a bearer token. The token ID binds
// a runtime session to the exact credential that opened it.
type Identity struct {
	AgentID string
	TokenID string
}

type Metadata struct {
	ID        string     `json:"id"`
	AgentID   string     `json:"agent_id"`
	IssuedAt  time.Time  `json:"issued_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	Status    string     `json:"status"`
}

type Repository interface {
	Create(context.Context, Record, string) error
	Rotate(context.Context, string, Record, string, time.Time) error
	Revoke(context.Context, string, string, time.Time) error
	Get(context.Context, string) (Record, error)
	List(context.Context) ([]Record, error)
	VerificationRecords(context.Context) ([]Record, error)
	Bootstrap(context.Context, []Record, string) (bool, error)
}

type AgentReader interface {
	Get(context.Context, string) (agent.Agent, error)
}

type Service struct {
	repository Repository
	agents     AgentReader
	now        func() time.Time
}

func NewService(repository Repository, agents AgentReader) (*Service, error) {
	if repository == nil || agents == nil {
		return nil, errors.New("MCP token repository and Agent reader are required")
	}
	return &Service{repository: repository, agents: agents, now: time.Now}, nil
}

func (s *Service) Create(ctx context.Context, agentID string, lifetime time.Duration, actorID string) (Metadata, string, error) {
	if err := validateLifetime(lifetime); err != nil || !validIdentity(agentID) || !validIdentity(actorID) {
		return Metadata{}, "", ErrInvalidInput
	}
	if err := s.validateAgent(ctx, agentID); err != nil {
		return Metadata{}, "", ErrInvalidInput
	}
	value, secret, err := newRecord(agentID, s.now(), lifetime)
	if err != nil {
		return Metadata{}, "", err
	}
	if err := s.repository.Create(ctx, value, actorID); err != nil {
		return Metadata{}, "", fmt.Errorf("persist MCP token: %w", err)
	}
	return metadata(value, s.now()), secret, nil
}

func (s *Service) Rotate(ctx context.Context, id string, lifetime time.Duration, actorID string) (Metadata, string, error) {
	if err := validateLifetime(lifetime); err != nil || !validIdentity(id) || !validIdentity(actorID) {
		return Metadata{}, "", ErrInvalidInput
	}
	previous, err := s.repository.Get(ctx, id)
	if err != nil {
		return Metadata{}, "", err
	}
	if err := s.validateAgent(ctx, previous.AgentID); err != nil {
		return Metadata{}, "", ErrInvalidInput
	}
	now := s.now().UTC()
	if !active(previous, now) {
		return Metadata{}, "", ErrInactive
	}
	value, secret, err := newRecord(previous.AgentID, now, lifetime)
	if err != nil {
		return Metadata{}, "", err
	}
	if err := s.repository.Rotate(ctx, id, value, actorID, now); err != nil {
		return Metadata{}, "", err
	}
	return metadata(value, now), secret, nil
}

func (s *Service) Revoke(ctx context.Context, id, actorID string) error {
	if !validIdentity(id) || !validIdentity(actorID) {
		return ErrInvalidInput
	}
	return s.repository.Revoke(ctx, id, actorID, s.now().UTC())
}

func (s *Service) List(ctx context.Context) ([]Metadata, error) {
	values, err := s.repository.List(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	result := make([]Metadata, 0, len(values))
	for _, value := range values {
		result = append(result, metadata(value, now))
	}
	return result, nil
}

// Verify checks the full verifier set so token comparisons do not reveal the matching record position.
func (s *Service) Verify(ctx context.Context, raw string) (Identity, error) {
	if len(raw) != 64 {
		return Identity{}, ErrInactive
	}
	decoded, err := hex.DecodeString(raw)
	if err != nil || len(decoded) != 32 {
		clear(decoded)
		return Identity{}, ErrInactive
	}
	defer clear(decoded)
	digest := sha256.Sum256(decoded)
	values, err := s.repository.VerificationRecords(ctx)
	if err != nil {
		return Identity{}, err
	}
	now := s.now().UTC()
	matched := 0
	identity := Identity{}
	for _, value := range values {
		equal := subtle.ConstantTimeCompare(digest[:], value.Digest[:])
		valid := equal & boolToInt(active(value, now))
		matched |= valid
		if valid == 1 {
			identity = Identity{AgentID: value.AgentID, TokenID: value.ID}
		}
	}
	if matched != 1 {
		return Identity{}, ErrInactive
	}
	if err := s.validateAgent(ctx, identity.AgentID); err != nil {
		return Identity{}, ErrInactive
	}
	return identity, nil
}

// Bootstrap imports legacy environment tokens once, assigning each a finite lifetime.
func (s *Service) Bootstrap(ctx context.Context, tokens []config.MCPAgentToken, actorID string) (bool, error) {
	if !validIdentity(actorID) {
		return false, ErrInvalidInput
	}
	now := s.now().UTC()
	values := make([]Record, 0, len(tokens))
	for _, token := range tokens {
		if !validIdentity(token.AgentID) || token.Token == ([32]byte{}) {
			return false, ErrInvalidInput
		}
		if err := s.validateAgent(ctx, token.AgentID); err != nil {
			return false, ErrInvalidInput
		}
		id, err := randomID()
		if err != nil {
			return false, err
		}
		values = append(values, Record{ID: id, AgentID: token.AgentID, Digest: sha256.Sum256(token.Token[:]), IssuedAt: now, ExpiresAt: now.Add(bootstrapLifetime)})
	}
	return s.repository.Bootstrap(ctx, values, actorID)
}

func (s *Service) validateAgent(ctx context.Context, id string) error {
	value, err := s.agents.Get(ctx, id)
	if err != nil {
		return err
	}
	if value.Runtime() != agent.RuntimeMCP {
		return agent.ErrInvalidInput
	}
	return nil
}

func newRecord(agentID string, now time.Time, lifetime time.Duration) (Record, string, error) {
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return Record{}, "", fmt.Errorf("generate MCP token: %w", err)
	}
	raw := hex.EncodeToString(secret[:])
	digest := sha256.Sum256(secret[:])
	clear(secret[:])
	id, err := randomID()
	if err != nil {
		return Record{}, "", err
	}
	return Record{ID: id, AgentID: agentID, Digest: digest, IssuedAt: now.UTC(), ExpiresAt: now.UTC().Add(lifetime)}, raw, nil
}

func randomID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate MCP token ID: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}

func validateLifetime(value time.Duration) error {
	if value <= 0 || value > maxLifetime {
		return ErrInvalidInput
	}
	return nil
}

func validIdentity(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && len(value) <= 200
}

func active(value Record, now time.Time) bool {
	return value.RevokedAt == nil && now.Before(value.ExpiresAt)
}

func metadata(value Record, now time.Time) Metadata {
	status := "active"
	if value.RevokedAt != nil {
		status = "revoked"
	} else if !now.Before(value.ExpiresAt) {
		status = "expired"
	}
	return Metadata{ID: value.ID, AgentID: value.AgentID, IssuedAt: value.IssuedAt, ExpiresAt: value.ExpiresAt, RevokedAt: value.RevokedAt, Status: status}
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
