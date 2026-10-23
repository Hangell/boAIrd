package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"boAIrd/internal/user"
)

func TestUserRepositoryPersistsUsersAndMembership(t *testing.T) {
	database, err := OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	repository, err := NewUserRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	value, err := user.Restore("alice", "Alice", user.RoleMember)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Create(ctx, value); err != nil {
		t.Fatal(err)
	}
	if err := repository.Create(ctx, value); !errors.Is(err, user.ErrAlreadyExists) {
		t.Fatalf("duplicate Create() error = %v, want already exists", err)
	}
	got, err := repository.Get(ctx, "alice")
	if err != nil || got.Name() != "Alice" || got.Role() != user.RoleMember {
		t.Fatalf("Get() = %#v, error = %v", got, err)
	}
	values, err := repository.List(ctx)
	if err != nil || len(values) != 2 {
		t.Fatalf("List() = %#v, error = %v; want bootstrap administrator plus alice", values, err)
	}
	if _, err := database.Exec(`INSERT INTO projects(id, name, status, human_review_required) VALUES ('project-1', 'Project One', 'CREATED', 1)`); err != nil {
		t.Fatal(err)
	}
	if err := repository.SetProjectMembership(ctx, "project-1", "alice", user.MembershipMember); err != nil {
		t.Fatal(err)
	}
	memberships, err := repository.ProjectMemberships(ctx, "alice")
	if err != nil || memberships["project-1"] != user.MembershipMember {
		t.Fatalf("memberships = %#v, error = %v", memberships, err)
	}
	if err := repository.RemoveProjectMembership(ctx, "project-1", "alice"); err != nil {
		t.Fatal(err)
	}
	memberships, err = repository.ProjectMemberships(ctx, "alice")
	if err != nil || len(memberships) != 0 {
		t.Fatalf("memberships after removal = %#v, error = %v", memberships, err)
	}
	if err := repository.Delete(ctx, "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Get(ctx, "alice"); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("Get() after delete error = %v, want not found", err)
	}
}

func TestUserRepositoryDeleteRemovesMemberships(t *testing.T) {
	database, err := OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	repository, err := NewUserRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	value, err := user.Restore("bob", "Bob", user.RoleMember)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Create(ctx, value); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO projects(id, name, status, human_review_required) VALUES ('project-1', 'Project One', 'CREATED', 1)`); err != nil {
		t.Fatal(err)
	}
	if err := repository.SetProjectMembership(ctx, "project-1", "bob", user.MembershipMember); err != nil {
		t.Fatal(err)
	}
	if err := repository.Delete(ctx, "bob"); err != nil {
		t.Fatal(err)
	}
	member, err := repository.IsUserProjectMember(ctx, "project-1", "bob")
	if err != nil || member {
		t.Fatalf("membership after user delete = %t, error = %v; want removed", member, err)
	}
}
