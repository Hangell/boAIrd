// Package user models authenticated human identities and their project
// membership, kept separate from Agent and Brain identity.
package user

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

var (
	ErrNotFound               = errors.New("user not found")
	ErrAlreadyExists          = errors.New("user already exists")
	ErrInvalidInput           = errors.New("invalid user input")
	ErrBootstrapAdministrator = errors.New("the bootstrap administrator cannot be modified")
	ErrProjectNotFound        = errors.New("project not found")
	ErrForbidden              = errors.New("administrator role required")
)

// Role is a global user role.
type Role string

const (
	RoleAdministrator Role = "administrator"
	RoleMember        Role = "member"
)

// MembershipRole is the project-scoped role stored in project_members.
type MembershipRole string

const (
	MembershipOwner  MembershipRole = "ADMIN"
	MembershipMember MembershipRole = "MEMBER"
)

// BootstrapAdministratorID is the seeded administrator identity.
const BootstrapAdministratorID = "local-administrator"

// User is an authenticated human identity.
type User struct {
	id   string
	name string
	role Role
}

func (u User) ID() string   { return u.id }
func (u User) Name() string { return u.name }
func (u User) Role() Role   { return u.role }

// Restore rebuilds a User from persisted fields after validation.
func Restore(id, name string, role Role) (User, error) {
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	if id == "" || len(id) > 200 || strings.ContainsRune(id, '\x00') {
		return User{}, ErrInvalidInput
	}
	if name == "" || utf8.RuneCountInString(name) > 200 || strings.ContainsRune(name, '\x00') {
		return User{}, ErrInvalidInput
	}
	if role != RoleAdministrator && role != RoleMember {
		return User{}, ErrInvalidInput
	}
	return User{id: id, name: name, role: role}, nil
}

// Repository persists users and project membership.
type Repository interface {
	Create(context.Context, string, User) error
	List(context.Context) ([]User, error)
	Get(context.Context, string) (User, error)
	Delete(context.Context, string, string) error
	SetProjectMembership(context.Context, string, string, string, MembershipRole) error
	RemoveProjectMembership(context.Context, string, string, string) error
	ProjectMemberships(context.Context, string) (map[string]MembershipRole, error)
	RecordSessionIssued(context.Context, string, string) error
}

// ProjectReader validates that a project exists before membership changes.
type ProjectReader interface {
	Exists(context.Context, string) (bool, error)
}

// Service applies user and membership rules on top of persistence.
type Service struct {
	repository Repository
	projects   ProjectReader
}

func NewService(repository Repository, projects ProjectReader) (*Service, error) {
	if repository == nil {
		return nil, errors.New("user repository is required")
	}
	if projects == nil {
		return nil, errors.New("project reader is required")
	}
	return &Service{repository: repository, projects: projects}, nil
}

// Create validates and persists a new user identity.
func (s *Service) Create(ctx context.Context, actorID, id, name string, role Role) (User, error) {
	if err := s.ensureAdministrator(ctx, actorID); err != nil {
		return User{}, err
	}
	value, err := Restore(id, name, role)
	if err != nil {
		return User{}, err
	}
	if err := s.repository.Create(ctx, actorID, value); err != nil {
		return User{}, err
	}
	return value, nil
}

func (s *Service) List(ctx context.Context) ([]User, error) {
	return s.repository.List(ctx)
}

func (s *Service) Get(ctx context.Context, id string) (User, error) {
	if strings.TrimSpace(id) == "" {
		return User{}, ErrInvalidInput
	}
	return s.repository.Get(ctx, id)
}

// Delete removes a user identity and its memberships. The bootstrap
// administrator cannot be removed, preserving administrator authority.
func (s *Service) Delete(ctx context.Context, actorID, id string) error {
	if err := s.ensureAdministrator(ctx, actorID); err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	if id == BootstrapAdministratorID {
		return ErrBootstrapAdministrator
	}
	if id == "" {
		return ErrInvalidInput
	}
	return s.repository.Delete(ctx, actorID, id)
}

// GrantProject assigns or updates a user's project membership.
func (s *Service) GrantProject(ctx context.Context, actorID, projectID, userID string, role MembershipRole) error {
	if err := s.ensureAdministrator(ctx, actorID); err != nil {
		return err
	}
	if role != MembershipOwner && role != MembershipMember {
		return ErrInvalidInput
	}
	if err := s.ensureProject(ctx, projectID); err != nil {
		return err
	}
	if err := s.ensureUser(ctx, userID); err != nil {
		return err
	}
	return s.repository.SetProjectMembership(ctx, actorID, projectID, userID, role)
}

// RevokeProject removes a user's membership from a project. The bootstrap
// administrator's membership cannot be revoked.
func (s *Service) RevokeProject(ctx context.Context, actorID, projectID, userID string) error {
	if err := s.ensureAdministrator(ctx, actorID); err != nil {
		return err
	}
	userID = strings.TrimSpace(userID)
	if userID == BootstrapAdministratorID {
		return ErrBootstrapAdministrator
	}
	if err := s.ensureProject(ctx, projectID); err != nil {
		return err
	}
	return s.repository.RemoveProjectMembership(ctx, actorID, projectID, userID)
}

// RecordSessionIssued appends a non-secret audit record for an issued session.
func (s *Service) RecordSessionIssued(ctx context.Context, actorID, userID string) error {
	if err := s.ensureAdministrator(ctx, actorID); err != nil {
		return err
	}
	if strings.TrimSpace(userID) == "" {
		return ErrInvalidInput
	}
	return s.repository.RecordSessionIssued(ctx, actorID, userID)
}

func (s *Service) ensureAdministrator(ctx context.Context, actorID string) error {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return ErrInvalidInput
	}
	actor, err := s.repository.Get(ctx, actorID)
	if err != nil {
		return err
	}
	if actor.Role() != RoleAdministrator {
		return ErrForbidden
	}
	return nil
}

// ProjectsFor returns the user's current project memberships.
func (s *Service) ProjectsFor(ctx context.Context, userID string) (map[string]MembershipRole, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, ErrInvalidInput
	}
	return s.repository.ProjectMemberships(ctx, userID)
}

func (s *Service) ensureProject(ctx context.Context, projectID string) error {
	exists, err := s.projects.Exists(ctx, projectID)
	if err != nil {
		return fmt.Errorf("check project: %w", err)
	}
	if !exists {
		return ErrProjectNotFound
	}
	return nil
}

func (s *Service) ensureUser(ctx context.Context, userID string) error {
	if _, err := s.repository.Get(ctx, userID); err != nil {
		return err
	}
	return nil
}
