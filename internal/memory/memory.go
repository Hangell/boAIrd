// Package memory defines provider-independent project memory summaries.
package memory

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"boAIrd/internal/logging"
)

type Category string

const (
	CategoryArchitectureDecision Category = "architecture_decision"
	CategoryProjectDecision      Category = "project_decision"
	CategoryImportantContext     Category = "important_context"
	CategoryKnownProblem         Category = "known_problem"
	CategoryCurrentState         Category = "current_state"
	CategoryTaskHistory          Category = "task_history"
)

var (
	ErrNotFound        = errors.New("project memory not found")
	ErrInvalidInput    = errors.New("invalid project memory input")
	ErrProjectNotFound = errors.New("project not found")
	ErrProjectArchived = errors.New("project is archived")
	ErrSanitization    = errors.New("project memory sanitization failed")
)

const MaximumContentBytes = 64 << 10

// Summary is one progressive Brain summary with provenance links to its sources.
type Summary struct {
	id            string
	projectID     string
	category      Category
	version       int
	content       string
	sourceIDs     []string
	createdByType string
	createdBy     string
	createdAt     time.Time
}

type Snapshot struct {
	ID            string
	ProjectID     string
	Category      Category
	Version       int
	Content       string
	SourceIDs     []string
	CreatedByType string
	CreatedBy     string
	CreatedAt     time.Time
}

func New(id, projectID string, category Category, version int, content, createdByType, createdBy string, sourceIDs []string, createdAt time.Time) (Summary, error) {
	id = strings.TrimSpace(id)
	projectID = strings.TrimSpace(projectID)
	content = strings.TrimSpace(content)
	createdBy = strings.TrimSpace(createdBy)
	if id == "" || projectID == "" || createdBy == "" {
		return Summary{}, errors.New("summary ID, project ID, and creator are required")
	}
	if !validCategory(category) {
		return Summary{}, fmt.Errorf("unknown memory category %q", category)
	}
	if version < 1 {
		return Summary{}, errors.New("memory version must be positive")
	}
	if content == "" || len(content) > MaximumContentBytes {
		return Summary{}, fmt.Errorf("memory content must contain 1 to %d bytes", MaximumContentBytes)
	}
	if createdAt.IsZero() {
		return Summary{}, errors.New("memory creation time is required")
	}
	return Summary{
		id: id, projectID: projectID, category: category, version: version, content: content,
		sourceIDs: cleanStrings(sourceIDs), createdByType: createdByType, createdBy: createdBy, createdAt: createdAt.UTC(),
	}, nil
}

func Restore(snapshot Snapshot) (Summary, error) {
	return New(snapshot.ID, snapshot.ProjectID, snapshot.Category, snapshot.Version, snapshot.Content,
		snapshot.CreatedByType, snapshot.CreatedBy, snapshot.SourceIDs, snapshot.CreatedAt)
}

func (s Summary) ID() string            { return s.id }
func (s Summary) ProjectID() string     { return s.projectID }
func (s Summary) Category() Category    { return s.category }
func (s Summary) Version() int          { return s.version }
func (s Summary) Content() string       { return s.content }
func (s Summary) SourceIDs() []string   { return copyStrings(s.sourceIDs) }
func (s Summary) CreatedByType() string { return s.createdByType }
func (s Summary) CreatedBy() string     { return s.createdBy }
func (s Summary) CreatedAt() time.Time  { return s.createdAt }

// Canonical reports whether the summary holds a project decision that must survive retention pruning.
func (s Summary) Canonical() bool {
	return s.category == CategoryArchitectureDecision || s.category == CategoryProjectDecision
}

func (s Summary) Snapshot() Snapshot {
	return Snapshot{
		ID: s.id, ProjectID: s.projectID, Category: s.category, Version: s.version, Content: s.content,
		SourceIDs: copyStrings(s.sourceIDs), CreatedByType: s.createdByType, CreatedBy: s.createdBy, CreatedAt: s.createdAt,
	}
}

func copyStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	return append([]string(nil), values...)
}

// SanitizeContent redacts secrets and fails closed if redaction is incomplete.
func SanitizeContent(content string) (string, error) {
	cleaned, err := logging.SanitizeSensitiveText(strings.TrimSpace(content))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrSanitization, err)
	}
	return cleaned, nil
}

func validCategory(category Category) bool {
	switch category {
	case CategoryArchitectureDecision, CategoryProjectDecision, CategoryImportantContext,
		CategoryKnownProblem, CategoryCurrentState, CategoryTaskHistory:
		return true
	default:
		return false
	}
}

func cleanStrings(values []string) []string {
	cleaned := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		cleaned = append(cleaned, value)
	}
	return cleaned
}
