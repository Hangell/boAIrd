package storage

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"boAIrd/internal/user"
)

func TestUserRepositoryPersistsUsersAndMembership(t *testing.T) {
	database, err := openTestDatabase(t, context.Background(), filepath.Join(t.TempDir(), "users.db"))
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
	if err := repository.Create(ctx, user.BootstrapAdministratorID, value); err != nil {
		t.Fatal(err)
	}
	if err := repository.Create(ctx, user.BootstrapAdministratorID, value); !errors.Is(err, user.ErrAlreadyExists) {
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
	if err := repository.SetProjectMembership(ctx, user.BootstrapAdministratorID, "project-1", "alice", user.MembershipMember); err != nil {
		t.Fatal(err)
	}
	memberships, err := repository.ProjectMemberships(ctx, "alice")
	if err != nil || memberships["project-1"] != user.MembershipMember {
		t.Fatalf("memberships = %#v, error = %v", memberships, err)
	}
	if err := repository.RemoveProjectMembership(ctx, user.BootstrapAdministratorID, "project-1", "alice"); err != nil {
		t.Fatal(err)
	}
	memberships, err = repository.ProjectMemberships(ctx, "alice")
	if err != nil || len(memberships) != 0 {
		t.Fatalf("memberships after removal = %#v, error = %v", memberships, err)
	}
	if err := repository.Delete(ctx, user.BootstrapAdministratorID, "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Get(ctx, "alice"); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("Get() after delete error = %v, want not found", err)
	}
}

func TestUserRepositoryDeleteRemovesMemberships(t *testing.T) {
	database, err := openTestDatabase(t, context.Background(), filepath.Join(t.TempDir(), "users.db"))
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
	if err := repository.Create(ctx, user.BootstrapAdministratorID, value); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO projects(id, name, status, human_review_required) VALUES ('project-1', 'Project One', 'CREATED', 1)`); err != nil {
		t.Fatal(err)
	}
	if err := repository.SetProjectMembership(ctx, user.BootstrapAdministratorID, "project-1", "bob", user.MembershipMember); err != nil {
		t.Fatal(err)
	}
	if err := repository.Delete(ctx, user.BootstrapAdministratorID, "bob"); err != nil {
		t.Fatal(err)
	}
	member, err := repository.IsUserProjectMember(ctx, "project-1", "bob")
	if err != nil || member {
		t.Fatalf("membership after user delete = %t, error = %v; want removed", member, err)
	}
}

type deleteBeforeMembershipRepository struct {
	user.Repository
}

func (r deleteBeforeMembershipRepository) SetProjectMembership(ctx context.Context, actorID, projectID, userID string, role user.MembershipRole) error {
	if err := r.Delete(ctx, actorID, userID); err != nil {
		return err
	}
	recreated, err := user.Restore(userID, "Recreated Alice", user.RoleMember)
	if err != nil {
		return err
	}
	if err := r.Create(ctx, actorID, recreated); !errors.Is(err, user.ErrAlreadyExists) {
		return fmt.Errorf("recreate deleted user ID: got %v, want retired ID conflict", err)
	}
	return r.Repository.SetProjectMembership(ctx, actorID, projectID, userID, role)
}

func TestGrantProjectRevalidatesUserInsideMembershipTransaction(t *testing.T) {
	database, err := openTestDatabase(t, context.Background(), filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	userRepository, err := NewUserRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	projectRepository, err := NewProjectRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	value, err := user.Restore("alice", "Alice", user.RoleMember)
	if err != nil {
		t.Fatal(err)
	}
	if err := userRepository.Create(ctx, user.BootstrapAdministratorID, value); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO projects(id, name, status, human_review_required) VALUES ('project-1', 'Project One', 'CREATED', 1)`); err != nil {
		t.Fatal(err)
	}
	service, err := user.NewService(deleteBeforeMembershipRepository{Repository: userRepository}, projectRepository)
	if err != nil {
		t.Fatal(err)
	}
	err = service.GrantProject(ctx, user.BootstrapAdministratorID, "project-1", "alice", user.MembershipMember)
	if !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("GrantProject() error = %v, want not found after concurrent deletion", err)
	}
	value, err = user.Restore("alice", "Recreated Alice", user.RoleMember)
	if err != nil {
		t.Fatal(err)
	}
	if err := userRepository.Create(ctx, user.BootstrapAdministratorID, value); !errors.Is(err, user.ErrAlreadyExists) {
		t.Fatalf("recreate deleted user ID error = %v, want retired ID conflict", err)
	}
	if _, err := database.Exec(`UPDATE user_identity_tombstones SET deleted_at = CURRENT_TIMESTAMP WHERE user_id = 'alice'`); err == nil {
		t.Fatal("user identity tombstone was mutable")
	}
	if _, err := database.Exec(`DELETE FROM user_identity_tombstones WHERE user_id = 'alice'`); err == nil {
		t.Fatal("user identity tombstone was deletable")
	}
	member, err := userRepository.IsUserProjectMember(ctx, "project-1", "alice")
	if err != nil || member {
		t.Fatalf("recreated user inherited membership = %t, error = %v", member, err)
	}
}

func TestUserAuthorityChangesAreAuditedAndTransactional(t *testing.T) {
	database, err := openTestDatabase(t, context.Background(), filepath.Join(t.TempDir(), "users.db"))
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
	if err := repository.Create(ctx, user.BootstrapAdministratorID, value); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO projects(id, name, status, human_review_required) VALUES ('project-1', 'Project One', 'CREATED', 1)`); err != nil {
		t.Fatal(err)
	}
	if err := repository.SetProjectMembership(ctx, user.BootstrapAdministratorID, "project-1", "alice", user.MembershipMember); err != nil {
		t.Fatal(err)
	}
	if err := repository.RecordSessionIssued(ctx, user.BootstrapAdministratorID, "alice"); err != nil {
		t.Fatal(err)
	}
	if err := repository.RemoveProjectMembership(ctx, user.BootstrapAdministratorID, "project-1", "alice"); err != nil {
		t.Fatal(err)
	}
	if err := repository.Delete(ctx, user.BootstrapAdministratorID, "alice"); err != nil {
		t.Fatal(err)
	}
	events, err := database.Query(`SELECT event_type, actor_id, target_user_id, result FROM user_authority_audit_events ORDER BY sequence`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = events.Close() }()
	var eventTypes []string
	for events.Next() {
		var eventType, actorID, targetID, result string
		if err := events.Scan(&eventType, &actorID, &targetID, &result); err != nil {
			t.Fatal(err)
		}
		if actorID != user.BootstrapAdministratorID || targetID != "alice" || result != "success" {
			t.Fatalf("unexpected authority event identity/outcome: actor=%q target=%q result=%q", actorID, targetID, result)
		}
		eventTypes = append(eventTypes, eventType)
	}
	if err := events.Err(); err != nil {
		t.Fatal(err)
	}
	if len(eventTypes) != 5 || eventTypes[0] != "user.created" || eventTypes[1] != "user.project.granted" || eventTypes[2] != "user.session.issued" || eventTypes[3] != "user.project.revoked" || eventTypes[4] != "user.deleted" {
		t.Fatalf("authority event types = %v", eventTypes)
	}
	if _, err := database.Exec(`UPDATE user_authority_audit_events SET reason='changed'`); err == nil {
		t.Fatal("user authority audit events were mutable")
	}
	if _, err := database.Exec(`DELETE FROM user_authority_audit_events`); err == nil {
		t.Fatal("user authority audit events were deletable")
	}
	rollbackValue, err := user.Restore("rollback-user", "Rollback User", user.RoleMember)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Create(ctx, strings.Repeat("a", 201), rollbackValue); err == nil {
		t.Fatal("create succeeded despite audit failure")
	}
	if _, err := repository.Get(ctx, "rollback-user"); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("user after failed audit = error %v, want not found", err)
	}
}

func TestPostgresConcurrentMembershipChangesKeepAuditCausal(t *testing.T) {
	database, err := openTestDatabase(t, context.Background(), filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	if database.dialect != dialectPostgres {
		t.Skip("membership row locking is PostgreSQL-specific")
	}
	repository, err := NewUserRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	value, err := user.Restore("concurrent-member", "Concurrent member", user.RoleMember)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Create(ctx, user.BootstrapAdministratorID, value); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO projects(id, name, status, human_review_required) VALUES ('concurrent-project', 'Concurrent project', 'CREATED', 1)`); err != nil {
		t.Fatal(err)
	}
	roles := []user.MembershipRole{user.MembershipMember, user.MembershipOwner}
	start := make(chan struct{})
	errorsByWriter := make(chan error, len(roles))
	var writers sync.WaitGroup
	for _, role := range roles {
		writers.Add(1)
		go func(role user.MembershipRole) {
			defer writers.Done()
			<-start
			errorsByWriter <- repository.SetProjectMembership(ctx, user.BootstrapAdministratorID, "concurrent-project", value.ID(), role)
		}(role)
	}
	close(start)
	writers.Wait()
	close(errorsByWriter)
	for err := range errorsByWriter {
		if err != nil {
			t.Fatalf("concurrent SetProjectMembership() error = %v", err)
		}
	}
	rows, err := database.QueryContext(ctx, `SELECT previous_role, next_role FROM user_authority_audit_events
		WHERE project_id = ? AND event_type IN ('user.project.granted', 'user.project.updated') ORDER BY sequence`, "concurrent-project")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var events [][2]string
	for rows.Next() {
		var previous, next string
		if err := rows.Scan(&previous, &next); err != nil {
			t.Fatal(err)
		}
		events = append(events, [2]string{previous, next})
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0][0] != "" || events[1][0] != events[0][1] {
		t.Fatalf("membership audit chain = %v, want first grant then previous role matching committed next role", events)
	}

	var revokes sync.WaitGroup
	start = make(chan struct{})
	errorsByWriter = make(chan error, 2)
	for range 2 {
		revokes.Add(1)
		go func() {
			defer revokes.Done()
			<-start
			errorsByWriter <- repository.RemoveProjectMembership(ctx, user.BootstrapAdministratorID, "concurrent-project", value.ID())
		}()
	}
	close(start)
	revokes.Wait()
	close(errorsByWriter)
	for err := range errorsByWriter {
		if err != nil {
			t.Fatalf("concurrent RemoveProjectMembership() error = %v", err)
		}
	}
	var revokeCount int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_authority_audit_events
		WHERE project_id = ? AND event_type = 'user.project.revoked'`, "concurrent-project").Scan(&revokeCount); err != nil {
		t.Fatal(err)
	}
	if revokeCount != 1 {
		t.Fatalf("concurrent revoke audit count = %d, want 1", revokeCount)
	}
}
