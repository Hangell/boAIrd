package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"boAIrd/internal/credential"
)

func TestCredentialRepositoryPersistsCiphertextAndListsMetadata(t *testing.T) {
	database, err := OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "boaird.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	repository, err := NewCredentialRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	record := credential.Record{ID: "id-1", Name: "test", Provider: "openai", SecretCiphertext: []byte("ciphertext-bytes-not-plaintext")}
	if err := repository.Create(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	loaded, err := repository.Get(context.Background(), record.ID)
	if err != nil || string(loaded.SecretCiphertext) != string(record.SecretCiphertext) {
		t.Fatalf("Get() = %#v, %v", loaded, err)
	}
	list, err := repository.List(context.Background())
	if err != nil || len(list) != 1 || len(list[0].SecretCiphertext) != 0 {
		t.Fatalf("List() = %#v, %v", list, err)
	}
	if err := repository.Delete(context.Background(), record.ID); err != nil {
		t.Fatal(err)
	}
	if err := repository.Delete(context.Background(), record.ID); err != credential.ErrNotFound {
		t.Fatalf("second Delete() error = %v", err)
	}
}

func TestCredentialManagerPersistsOnlyCiphertext(t *testing.T) {
	database, err := OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "boaird.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	repository, err := NewCredentialRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := credential.NewManager(repository, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	secret := "fixture-fake-provider-secret-123456"
	metadata, err := manager.Create(context.Background(), "test", "openai", secret)
	if err != nil {
		t.Fatal(err)
	}
	var persisted string
	if err := database.QueryRow("SELECT CAST(secret_ciphertext AS TEXT) FROM credentials WHERE id = ?", metadata.ID).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(persisted, secret) {
		t.Fatal("database contains plaintext credential")
	}
	resolved, err := manager.Resolve(context.Background(), metadata.ID)
	if err != nil || resolved != secret {
		t.Fatalf("Resolve() = %q, %v", resolved, err)
	}
}
