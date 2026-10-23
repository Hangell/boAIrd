package project

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

var (
	ErrNotFound      = errors.New("project not found")
	ErrAlreadyExists = errors.New("project already exists")
	ErrInvalidInput  = errors.New("invalid project input")
)

const (
	LocalAdministratorID   = "local-administrator"
	defaultPageSize        = 50
	maximumPageSize        = 100
	maximumNameSize        = 200
	maximumDescriptionSize = 4000
)

type Repository interface {
	Create(context.Context, Project) error
	Get(context.Context, string) (Project, error)
	List(context.Context, int, int) ([]Project, error)
	Update(context.Context, Project) error
	HasActiveBrain(context.Context, string, string) (bool, error)
	IsMember(context.Context, string, string) (bool, error)
}

type Manager struct {
	repository Repository
}

type CreateInput struct {
	Name                string `json:"name"`
	Description         string `json:"description"`
	WorkspacePath       string `json:"workspace_path"`
	RepositoryURL       string `json:"repository_url"`
	HumanReviewRequired *bool  `json:"human_review_required"`
}

type UpdateInput struct {
	Name                *string `json:"name"`
	Description         *string `json:"description"`
	WorkspacePath       *string `json:"workspace_path"`
	RepositoryURL       *string `json:"repository_url"`
	HumanReviewRequired *bool   `json:"human_review_required"`
	BrainID             *string `json:"brain_id"`
}

func NewManager(repository Repository) (*Manager, error) {
	if repository == nil {
		return nil, errors.New("project repository is required")
	}
	return &Manager{repository: repository}, nil
}

func (m *Manager) Create(ctx context.Context, input CreateInput) (Project, error) {
	name, err := validateName(input.Name)
	if err != nil {
		return Project{}, invalidInput(err)
	}
	workspacePath, err := validateWorkspacePath(input.WorkspacePath)
	if err != nil {
		return Project{}, invalidInput(err)
	}
	if err := validateRepositoryURL(input.RepositoryURL); err != nil {
		return Project{}, invalidInput(err)
	}
	if utf8.RuneCountInString(input.Description) > maximumDescriptionSize {
		return Project{}, invalidInput(fmt.Errorf("description must be at most %d characters", maximumDescriptionSize))
	}
	id, err := newID()
	if err != nil {
		return Project{}, fmt.Errorf("generate project ID: %w", err)
	}
	p, err := New(id, name)
	if err != nil {
		return Project{}, invalidInput(err)
	}
	p = p.SetDescription(strings.TrimSpace(input.Description)).SetWorkspacePath(workspacePath).SetRepositoryURL(strings.TrimSpace(input.RepositoryURL))
	if input.HumanReviewRequired != nil {
		p = p.SetHumanReviewRequired(*input.HumanReviewRequired)
	}
	if err := m.repository.Create(ctx, p); err != nil {
		return Project{}, err
	}
	return p, nil
}

func (m *Manager) Get(ctx context.Context, id string) (Project, error) {
	if strings.TrimSpace(id) == "" {
		return Project{}, invalidInput(errors.New("project ID is required"))
	}
	return m.repository.Get(ctx, id)
}

func (m *Manager) IsMember(ctx context.Context, projectID, userID string) (bool, error) {
	projectID = strings.TrimSpace(projectID)
	userID = strings.TrimSpace(userID)
	if projectID == "" || userID == "" {
		return false, invalidInput(errors.New("project and user IDs are required"))
	}
	return m.repository.IsMember(ctx, projectID, userID)
}

func (m *Manager) List(ctx context.Context, limit, offset int) ([]Project, error) {
	if limit == 0 {
		limit = defaultPageSize
	}
	if limit < 1 || limit > maximumPageSize {
		return nil, invalidInput(fmt.Errorf("limit must be between 1 and %d", maximumPageSize))
	}
	if offset < 0 {
		return nil, invalidInput(errors.New("offset must not be negative"))
	}
	return m.repository.List(ctx, limit, offset)
}

type userScopedProjectRepository interface {
	ListForUser(context.Context, string, int, int) ([]Project, error)
}

// ListForUser returns only projects where the user has current membership.
// Pagination is applied by the repository after membership filtering.
func (m *Manager) ListForUser(ctx context.Context, userID string, limit, offset int) ([]Project, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, invalidInput(errors.New("user ID is required"))
	}
	if limit == 0 {
		limit = defaultPageSize
	}
	if limit < 1 || limit > maximumPageSize {
		return nil, invalidInput(fmt.Errorf("limit must be between 1 and %d", maximumPageSize))
	}
	if offset < 0 {
		return nil, invalidInput(errors.New("offset must not be negative"))
	}
	repository, ok := m.repository.(userScopedProjectRepository)
	if !ok {
		return nil, errors.New("project repository does not support user-scoped listing")
	}
	return repository.ListForUser(ctx, userID, limit, offset)
}

func (m *Manager) Update(ctx context.Context, id string, input UpdateInput) (Project, error) {
	if input.Name == nil && input.Description == nil && input.WorkspacePath == nil && input.RepositoryURL == nil && input.HumanReviewRequired == nil && input.BrainID == nil {
		return Project{}, invalidInput(errors.New("at least one project field must be provided"))
	}
	p, err := m.Get(ctx, id)
	if err != nil {
		return Project{}, err
	}
	if p.Status() == StatusArchived {
		return Project{}, invalidInput(errors.New("archived projects cannot be updated"))
	}
	if input.Name != nil {
		name, err := validateName(*input.Name)
		if err != nil {
			return Project{}, invalidInput(err)
		}
		p, err = p.Rename(name)
		if err != nil {
			return Project{}, invalidInput(err)
		}
	}
	if input.Description != nil {
		if utf8.RuneCountInString(*input.Description) > maximumDescriptionSize {
			return Project{}, invalidInput(fmt.Errorf("description must be at most %d characters", maximumDescriptionSize))
		}
		p = p.SetDescription(strings.TrimSpace(*input.Description))
	}
	if input.WorkspacePath != nil {
		path, err := validateWorkspacePath(*input.WorkspacePath)
		if err != nil {
			return Project{}, invalidInput(err)
		}
		p = p.SetWorkspacePath(path)
	}
	if input.RepositoryURL != nil {
		if err := validateRepositoryURL(*input.RepositoryURL); err != nil {
			return Project{}, invalidInput(err)
		}
		p = p.SetRepositoryURL(strings.TrimSpace(*input.RepositoryURL))
	}
	if input.HumanReviewRequired != nil {
		p = p.SetHumanReviewRequired(*input.HumanReviewRequired)
	}
	if input.BrainID != nil {
		var updated Project
		var updateErr error
		if strings.TrimSpace(*input.BrainID) == "" {
			updated, updateErr = p.WithoutBrain()
		} else {
			active, err := m.repository.HasActiveBrain(ctx, p.ID(), strings.TrimSpace(*input.BrainID))
			if err != nil {
				return Project{}, err
			}
			if !active {
				return Project{}, invalidInput(errors.New("brain_id must reference an active Brain belonging to this project"))
			}
			updated, updateErr = p.WithBrain(*input.BrainID)
		}
		if updateErr != nil {
			return Project{}, invalidInput(updateErr)
		}
		p = updated
	}
	if err := m.repository.Update(ctx, p); err != nil {
		return Project{}, err
	}
	return p, nil
}

func (m *Manager) Archive(ctx context.Context, id string) (Project, error) {
	p, err := m.Get(ctx, id)
	if err != nil {
		return Project{}, err
	}
	if p.Status() != StatusArchived {
		p, err = p.Transition(StatusArchived)
		if err != nil {
			return Project{}, invalidInput(err)
		}
		if err := m.repository.Update(ctx, p); err != nil {
			return Project{}, err
		}
	}
	return p, nil
}

func validateName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("project name is required")
	}
	if utf8.RuneCountInString(name) > maximumNameSize {
		return "", fmt.Errorf("project name must be at most %d characters", maximumNameSize)
	}
	return name, nil
}

func validateWorkspacePath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if strings.ContainsRune(value, '\x00') || !filepath.IsAbs(value) {
		return "", errors.New("workspace_path must be an absolute directory path")
	}
	for _, segment := range strings.Split(value, string(filepath.Separator)) {
		if segment == ".." {
			return "", errors.New("workspace_path must not contain parent traversal")
		}
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(value))
	if err != nil {
		return "", errors.New("workspace_path must resolve to an existing directory")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", errors.New("workspace_path must resolve to an existing directory")
	}
	if !info.IsDir() {
		return "", errors.New("workspace_path must refer to a directory")
	}
	return filepath.Clean(resolved), nil
}

func validateRepositoryURL(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http" && parsed.Scheme != "ssh") {
		return errors.New("repository_url must be an absolute HTTP(S) or SSH URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.RawFragment != "" || strings.ContainsAny(value, "?#") {
		return errors.New("repository_url must not contain user-info, a query, or a fragment")
	}
	return nil
}

func invalidInput(err error) error {
	return fmt.Errorf("%w: %w", ErrInvalidInput, err)
}

func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	encoded := hex.EncodeToString(value[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:], nil
}
