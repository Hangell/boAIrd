// Package credential stores provider credentials encrypted at rest.
package credential

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInvalidInput          = errors.New("invalid credential input")
	ErrInvalidMasterKey      = errors.New("invalid credential master key")
	ErrNotFound              = errors.New("credential not found")
	ErrCredentialUnavailable = errors.New("credential unavailable")
)

const ProviderGitHub = "github"

const (
	maxNameBytes     = 200
	maxProviderBytes = 100
	maxSecretBytes   = 16 << 10
)

type Record struct {
	ID               string
	Name             string
	Provider         string
	SecretCiphertext []byte
}

type Metadata struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
}

// Scope identifies the Agent and project authorized to use a credential.
type Scope struct {
	AgentID   string
	ProjectID string
}

type Repository interface {
	Create(context.Context, Record) error
	Get(context.Context, string) (Record, error)
	GetForUse(context.Context, string, string, string) (Record, error)
	List(context.Context) ([]Record, error)
	Delete(context.Context, string) error
}

// IntegrationRepository stores administrator-granted tool credentials apart
// from an Agent's runtime-provider credential.
type IntegrationRepository interface {
	BindIntegration(context.Context, string, string, string, string, string) error
	UnbindIntegration(context.Context, string, string, string, string) error
	GetIntegrationForUse(context.Context, string, string, string) (Record, error)
}

type Manager struct {
	repository            Repository
	integrationRepository IntegrationRepository
	aead                  cipher.AEAD
}

func NewManager(repository Repository, masterKey []byte) (*Manager, error) {
	if repository == nil {
		return nil, errors.New("credential repository is required")
	}
	if len(masterKey) != 32 {
		return nil, ErrInvalidMasterKey
	}
	keyCopy := append([]byte(nil), masterKey...)
	defer clear(keyCopy)
	block, err := aes.NewCipher(keyCopy)
	if err != nil {
		return nil, ErrInvalidMasterKey
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("initialize credential encryption: %w", err)
	}
	integrationRepository, _ := repository.(IntegrationRepository)
	return &Manager{repository: repository, integrationRepository: integrationRepository, aead: aead}, nil
}

func (m *Manager) Create(ctx context.Context, name, provider, secret string) (Metadata, error) {
	name = strings.TrimSpace(name)
	provider = strings.TrimSpace(provider)
	if name == "" || len(name) > maxNameBytes {
		return Metadata{}, fmt.Errorf("%w: name must contain 1 to %d bytes", ErrInvalidInput, maxNameBytes)
	}
	if provider == "" || len(provider) > maxProviderBytes {
		return Metadata{}, fmt.Errorf("%w: provider must contain 1 to %d bytes", ErrInvalidInput, maxProviderBytes)
	}
	if strings.TrimSpace(secret) == "" || len(secret) > maxSecretBytes {
		return Metadata{}, fmt.Errorf("%w: secret must contain 1 to %d bytes", ErrInvalidInput, maxSecretBytes)
	}
	id, err := newID()
	if err != nil {
		return Metadata{}, errors.New("generate credential ID")
	}
	ciphertext, err := m.encrypt(id, provider, secret)
	if err != nil {
		return Metadata{}, errors.New("encrypt credential")
	}
	record := Record{ID: id, Name: name, Provider: provider, SecretCiphertext: ciphertext}
	if err := m.repository.Create(ctx, record); err != nil {
		return Metadata{}, fmt.Errorf("store credential: %w", err)
	}
	return metadata(record), nil
}

func (m *Manager) List(ctx context.Context) ([]Metadata, error) {
	records, err := m.repository.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list credentials: %w", err)
	}
	result := make([]Metadata, 0, len(records))
	for _, record := range records {
		result = append(result, metadata(record))
	}
	return result, nil
}

func (m *Manager) Delete(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("%w: ID is required", ErrInvalidInput)
	}
	if err := m.repository.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete credential: %w", err)
	}
	return nil
}

// Resolve decrypts one secret only when a provider adapter requests its credential ID.
func (m *Manager) Resolve(ctx context.Context, id string, scope Scope) (string, error) {
	return m.resolve(ctx, id, scope, "")
}

// ResolveForProvider decrypts a credential only when it is scoped to the
// requested provider as well as the Agent and project.
func (m *Manager) ResolveForProvider(ctx context.Context, id string, scope Scope, provider string) (string, error) {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return "", ErrCredentialUnavailable
	}
	return m.resolve(ctx, id, scope, provider)
}

// BindGitHub grants an MCP Agent use of one GitHub credential in one project.
// The binding operation is audited atomically by the repository.
func (m *Manager) BindGitHub(ctx context.Context, projectID, agentID, credentialID, actorID string) error {
	if m.integrationRepository == nil || ctx == nil || !validBindingIdentifier(projectID) ||
		!validBindingIdentifier(agentID) || !validBindingIdentifier(credentialID) || !validBindingIdentifier(actorID) {
		return ErrCredentialUnavailable
	}
	return m.integrationRepository.BindIntegration(ctx, agentID, projectID, ProviderGitHub, credentialID, actorID)
}

// UnbindGitHub removes the project's explicit GitHub credential grant.
func (m *Manager) UnbindGitHub(ctx context.Context, projectID, agentID, actorID string) error {
	if m.integrationRepository == nil || ctx == nil || !validBindingIdentifier(projectID) ||
		!validBindingIdentifier(agentID) || !validBindingIdentifier(actorID) {
		return ErrCredentialUnavailable
	}
	return m.integrationRepository.UnbindIntegration(ctx, agentID, projectID, ProviderGitHub, actorID)
}

// ResolveGitHub decrypts only the GitHub credential explicitly bound to this
// MCP Agent and project. MCP input never supplies the credential identifier.
func (m *Manager) ResolveGitHub(ctx context.Context, scope Scope) (string, error) {
	if m.integrationRepository == nil || ctx == nil || !validBindingIdentifier(scope.AgentID) || !validBindingIdentifier(scope.ProjectID) {
		return "", ErrCredentialUnavailable
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	record, err := m.integrationRepository.GetIntegrationForUse(ctx, scope.AgentID, scope.ProjectID, ProviderGitHub)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "", err
		}
		return "", ErrCredentialUnavailable
	}
	if !strings.EqualFold(record.Provider, ProviderGitHub) {
		return "", ErrCredentialUnavailable
	}
	plaintext, err := m.decrypt(record.ID, record.Provider, record.SecretCiphertext)
	if err != nil {
		return "", ErrCredentialUnavailable
	}
	return plaintext, nil
}

func validBindingIdentifier(value string) bool {
	return strings.TrimSpace(value) != "" && strings.TrimSpace(value) == value &&
		!strings.ContainsRune(value, '\x00') && len(value) <= 128
}

func (m *Manager) resolve(ctx context.Context, id string, scope Scope, expectedProvider string) (string, error) {
	if ctx == nil || strings.TrimSpace(id) == "" || strings.TrimSpace(scope.AgentID) == "" || strings.TrimSpace(scope.ProjectID) == "" {
		return "", ErrCredentialUnavailable
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	record, err := m.repository.GetForUse(ctx, id, scope.AgentID, scope.ProjectID)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "", err
		}
		return "", ErrCredentialUnavailable
	}
	if expectedProvider != "" && !strings.EqualFold(record.Provider, expectedProvider) {
		return "", ErrCredentialUnavailable
	}
	plaintext, err := m.decrypt(record.ID, record.Provider, record.SecretCiphertext)
	if err != nil {
		return "", ErrCredentialUnavailable
	}
	return plaintext, nil
}

func (m *Manager) encrypt(id, provider, secret string) ([]byte, error) {
	plaintext := []byte(secret)
	defer clear(plaintext)
	nonce := make([]byte, m.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	sealed := m.aead.Seal(nil, nonce, plaintext, associatedData(id, provider))
	return append(nonce, sealed...), nil
}

func (m *Manager) decrypt(id, provider string, ciphertext []byte) (string, error) {
	nonceSize := m.aead.NonceSize()
	if len(ciphertext) < nonceSize+m.aead.Overhead() {
		return "", ErrCredentialUnavailable
	}
	plaintext, err := m.aead.Open(nil, ciphertext[:nonceSize], ciphertext[nonceSize:], associatedData(id, provider))
	if err != nil {
		return "", ErrCredentialUnavailable
	}
	defer clear(plaintext)
	return string(plaintext), nil
}

func associatedData(id, provider string) []byte {
	return []byte("boaird/credential/v1\x00" + id + "\x00" + provider)
}

func metadata(record Record) Metadata {
	return Metadata{ID: record.ID, Name: record.Name, Provider: record.Provider}
}

func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}
