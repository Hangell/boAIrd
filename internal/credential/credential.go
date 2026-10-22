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

type Manager struct {
	repository Repository
	aead       cipher.AEAD
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
	return &Manager{repository: repository, aead: aead}, nil
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
