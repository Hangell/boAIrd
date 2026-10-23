package user

import (
	"context"
	"errors"
	"testing"
)

type repositoryStub struct {
	users       map[string]User
	memberships map[string]MembershipRole
}

func newRepositoryStub() *repositoryStub {
	return &repositoryStub{
		users:       map[string]User{BootstrapAdministratorID: {id: BootstrapAdministratorID, name: "Administrator", role: RoleAdministrator}},
		memberships: map[string]MembershipRole{},
	}
}

func (s *repositoryStub) Create(_ context.Context, _ string, value User) error {
	if _, exists := s.users[value.ID()]; exists {
		return ErrAlreadyExists
	}
	s.users[value.ID()] = value
	return nil
}

func (s *repositoryStub) List(context.Context) ([]User, error) {
	values := make([]User, 0, len(s.users))
	for _, value := range s.users {
		values = append(values, value)
	}
	return values, nil
}

func (s *repositoryStub) Get(_ context.Context, id string) (User, error) {
	value, ok := s.users[id]
	if !ok {
		return User{}, ErrNotFound
	}
	return value, nil
}

func (s *repositoryStub) Delete(_ context.Context, _, id string) error {
	if _, ok := s.users[id]; !ok {
		return ErrNotFound
	}
	delete(s.users, id)
	return nil
}

func (s *repositoryStub) SetProjectMembership(_ context.Context, _, projectID, userID string, role MembershipRole) error {
	s.memberships[projectID+"/"+userID] = role
	return nil
}

func (s *repositoryStub) RemoveProjectMembership(_ context.Context, _, projectID, userID string) error {
	delete(s.memberships, projectID+"/"+userID)
	return nil
}

func (s *repositoryStub) RecordSessionIssued(context.Context, string, string) error { return nil }

func (s *repositoryStub) ProjectMemberships(_ context.Context, userID string) (map[string]MembershipRole, error) {
	result := map[string]MembershipRole{}
	for key, role := range s.memberships {
		if len(key) > len(userID) && key[len(key)-len(userID):] == userID {
			result[key[:len(key)-len(userID)-1]] = role
		}
	}
	return result, nil
}

type projectExistsStub map[string]bool

func (s projectExistsStub) Exists(_ context.Context, id string) (bool, error) { return s[id], nil }

func newTestService(t *testing.T) *Service {
	t.Helper()
	service, err := NewService(newRepositoryStub(), projectExistsStub{"project-1": true, "project-2": true})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestServiceProtectsBootstrapAdministrator(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	if err := service.Delete(ctx, "local-administrator", BootstrapAdministratorID); !errors.Is(err, ErrBootstrapAdministrator) {
		t.Fatalf("Delete(bootstrap) error = %v, want protected", err)
	}
	if err := service.RevokeProject(ctx, "local-administrator", "project-1", BootstrapAdministratorID); !errors.Is(err, ErrBootstrapAdministrator) {
		t.Fatalf("RevokeProject(bootstrap) error = %v, want protected", err)
	}
}

func TestServiceValidatesUsersAndMembership(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	if _, err := service.Create(ctx, "local-administrator", "alice", "", RoleMember); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Create(empty name) error = %v, want invalid", err)
	}
	if _, err := service.Create(ctx, "local-administrator", "alice", "Alice", Role("owner")); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Create(bad role) error = %v, want invalid", err)
	}
	if err := service.GrantProject(ctx, "local-administrator", "missing-project", "alice", MembershipMember); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("GrantProject(missing project) error = %v, want project not found", err)
	}
	if err := service.GrantProject(ctx, "local-administrator", "project-1", "unknown-user", MembershipMember); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GrantProject(unknown user) error = %v, want not found", err)
	}
}

func TestServiceCreatesAndDeletesMembers(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	if _, err := service.Create(ctx, "local-administrator", "alice", "Alice", RoleMember); err != nil {
		t.Fatal(err)
	}
	if err := service.GrantProject(ctx, "local-administrator", "project-1", "alice", MembershipOwner); err != nil {
		t.Fatal(err)
	}
	memberships, err := service.ProjectsFor(ctx, "alice")
	if err != nil || memberships["project-1"] != MembershipOwner {
		t.Fatalf("memberships = %#v, error = %v", memberships, err)
	}
	if err := service.RevokeProject(ctx, "local-administrator", "project-1", "alice"); err != nil {
		t.Fatal(err)
	}
	memberships, err = service.ProjectsFor(ctx, "alice")
	if err != nil || len(memberships) != 0 {
		t.Fatalf("memberships after revoke = %#v, error = %v", memberships, err)
	}
	if err := service.Delete(ctx, "local-administrator", "alice"); err != nil {
		t.Fatal(err)
	}
}

func TestServiceRejectsMemberAuthorityChanges(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	if _, err := service.Create(ctx, BootstrapAdministratorID, "alice", "Alice", RoleMember); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(ctx, "alice", "bob", "Bob", RoleMember); !errors.Is(err, ErrForbidden) {
		t.Fatalf("member Create() error = %v, want forbidden", err)
	}
	if err := service.RecordSessionIssued(ctx, "alice", "alice"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("member RecordSessionIssued() error = %v, want forbidden", err)
	}
}
