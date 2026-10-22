package credential

import (
	"context"
	"errors"
	"strings"
	"testing"

	"boAIrd/internal/provider"
)

var _ provider.CredentialResolver = (*Manager)(nil)

type memoryRepository struct{ records map[string]Record }

func (r *memoryRepository) Create(_ context.Context, record Record) error {
	if r.records == nil {
		r.records = make(map[string]Record)
	}
	r.records[record.ID] = record
	return nil
}
func (r *memoryRepository) Get(_ context.Context, id string) (Record, error) {
	record, ok := r.records[id]
	if !ok {
		return Record{}, ErrNotFound
	}
	return record, nil
}
func (r *memoryRepository) List(context.Context) ([]Record, error) {
	values := make([]Record, 0, len(r.records))
	for _, record := range r.records {
		record.SecretCiphertext = nil
		values = append(values, record)
	}
	return values, nil
}
func (r *memoryRepository) Delete(_ context.Context, id string) error {
	if _, ok := r.records[id]; !ok {
		return ErrNotFound
	}
	delete(r.records, id)
	return nil
}

func TestManagerEncryptsResolvesAndReturnsMetadataOnly(t *testing.T) {
	repository := &memoryRepository{}
	manager, err := NewManager(repository, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	secret := "fake-provider-key-12345"
	metadata, err := manager.Create(context.Background(), "OpenAI", "openai", secret)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(repository.records[metadata.ID].SecretCiphertext), secret) {
		t.Fatal("ciphertext contains plaintext secret")
	}
	resolved, err := manager.Resolve(context.Background(), metadata.ID)
	if err != nil || resolved != secret {
		t.Fatalf("Resolve() = %q, %v", resolved, err)
	}
	listed, err := manager.List(context.Background())
	if err != nil || len(listed) != 1 || listed[0] != metadata {
		t.Fatalf("List() = %#v, %v", listed, err)
	}
	if _, err := manager.Resolve(context.Background(), "missing"); !errors.Is(err, ErrCredentialUnavailable) {
		t.Fatalf("Resolve(missing) error = %v", err)
	}
}

func TestManagerRejectsInvalidKeyAndTamperedCiphertext(t *testing.T) {
	if _, err := NewManager(&memoryRepository{}, []byte("short")); !errors.Is(err, ErrInvalidMasterKey) {
		t.Fatalf("NewManager() error = %v", err)
	}
	repository := &memoryRepository{}
	manager, err := NewManager(repository, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := manager.Create(context.Background(), "test", "openai", "fake-secret-value")
	if err != nil {
		t.Fatal(err)
	}
	record := repository.records[metadata.ID]
	record.SecretCiphertext[len(record.SecretCiphertext)-1] ^= 0xff
	repository.records[metadata.ID] = record
	if resolved, err := manager.Resolve(context.Background(), metadata.ID); resolved != "" || !errors.Is(err, ErrCredentialUnavailable) {
		t.Fatalf("Resolve(tampered) = %q, %v", resolved, err)
	}
}
