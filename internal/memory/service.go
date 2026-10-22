package memory

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"boAIrd/internal/project"
)

type Repository interface {
	CreateNext(context.Context, Summary) (Summary, error)
	List(context.Context, string) ([]Summary, error)
	DeleteOlderThanVersion(context.Context, string, Category, int) (int, error)
}

type ProjectReader interface {
	Get(context.Context, string) (project.Project, error)
}

type Service struct {
	repository Repository
	projects   ProjectReader
	config     Config
}

type RecordInput struct {
	Category      Category
	Content       string
	SourceIDs     []string
	CreatedByType string
	CreatedBy     string
}

type Config struct {
	MaxSummariesPerCategory int
	MaxSelectionBytes       int
}

type Selection struct {
	Items           []Summary
	Omitted         []OmittedItem
	SelectedBytes   int
	RawHistoryBytes int
}

type OmittedItem struct {
	ID     string
	Reason string
}

const (
	defaultMaxSummariesPerCategory = 20
	defaultMaxSelectionBytes       = 64 << 10
)

func NewService(repository Repository, projects ProjectReader, config Config) (*Service, error) {
	if repository == nil || projects == nil {
		return nil, errors.New("memory repository and project reader are required")
	}
	if config.MaxSummariesPerCategory == 0 {
		config.MaxSummariesPerCategory = defaultMaxSummariesPerCategory
	}
	if config.MaxSelectionBytes == 0 {
		config.MaxSelectionBytes = defaultMaxSelectionBytes
	}
	if config.MaxSummariesPerCategory < 1 || config.MaxSelectionBytes < 1 {
		return nil, errors.New("memory limits must be positive")
	}
	return &Service{repository: repository, projects: projects, config: config}, nil
}

// Record sanitizes and persists one versioned summary, linking it to source records.
func (s *Service) Record(ctx context.Context, projectID string, input RecordInput) (Summary, error) {
	if _, err := s.ensureProjectActive(ctx, projectID); err != nil {
		return Summary{}, err
	}
	input.Category = Category(strings.TrimSpace(string(input.Category)))
	input.CreatedBy = strings.TrimSpace(input.CreatedBy)
	if input.CreatedBy == "" {
		return Summary{}, fmt.Errorf("%w: creator is required", ErrInvalidInput)
	}
	content, err := SanitizeContent(input.Content)
	if err != nil {
		return Summary{}, err
	}
	if content == "" {
		return Summary{}, fmt.Errorf("%w: content is required", ErrInvalidInput)
	}
	id, err := newID()
	if err != nil {
		return Summary{}, fmt.Errorf("generate memory ID: %w", err)
	}
	value, err := New(id, projectID, input.Category, 1, content, input.CreatedByType, input.CreatedBy, input.SourceIDs, time.Now().UTC())
	if err != nil {
		return Summary{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	return s.repository.CreateNext(ctx, value)
}

// List returns every summary for a project, ordered by category then version.
func (s *Service) List(ctx context.Context, projectID string) ([]Summary, error) {
	if _, err := s.ensureProjectActive(ctx, projectID); err != nil {
		return nil, err
	}
	return s.repository.List(ctx, projectID)
}

// Decisions returns the canonical project decisions, newest first.
func (s *Service) Decisions(ctx context.Context, projectID string) ([]Summary, error) {
	values, err := s.List(ctx, projectID)
	if err != nil {
		return nil, err
	}
	decisions := make([]Summary, 0, len(values))
	for _, value := range values {
		if value.Canonical() {
			decisions = append(decisions, value)
		}
	}
	sort.Slice(decisions, func(i, j int) bool {
		if decisions[i].Version() != decisions[j].Version() {
			return decisions[i].Version() > decisions[j].Version()
		}
		return decisions[i].CreatedAt().After(decisions[j].CreatedAt())
	})
	return decisions, nil
}

// Select returns a bounded, reproducible selection of relevant summaries. Canonical
// decisions are always considered first; the remaining items are ordered by version.
func (s *Service) Select(ctx context.Context, projectID, query string, maxBytes int) (Selection, error) {
	values, err := s.List(ctx, projectID)
	if err != nil {
		return Selection{}, err
	}
	if maxBytes <= 0 {
		maxBytes = s.config.MaxSelectionBytes
	}
	rawBytes := 0
	for _, value := range values {
		rawBytes += len(value.Content())
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].Canonical() != values[j].Canonical() {
			return values[i].Canonical()
		}
		return values[i].Version() > values[j].Version()
	})
	tokens := queryTokens(query)
	selection := Selection{RawHistoryBytes: rawBytes}
	for _, value := range values {
		if !relevant(value, tokens) {
			selection.Omitted = append(selection.Omitted, OmittedItem{ID: value.ID(), Reason: "not relevant to query"})
			continue
		}
		if selection.SelectedBytes+len(value.Content()) > maxBytes {
			selection.Omitted = append(selection.Omitted, OmittedItem{ID: value.ID(), Reason: "exceeds selection size limit"})
			continue
		}
		selection.Items = append(selection.Items, value)
		selection.SelectedBytes += len(value.Content())
	}
	return selection, nil
}

// Prune applies the retention policy by deleting the oldest summaries of each
// non-canonical category beyond the configured limit. Canonical decisions are
// never pruned, and raw source history is never touched.
func (s *Service) Prune(ctx context.Context, projectID string) (int, error) {
	if _, err := s.ensureProjectActive(ctx, projectID); err != nil {
		return 0, err
	}
	values, err := s.repository.List(ctx, projectID)
	if err != nil {
		return 0, err
	}
	counts := make(map[Category][]Summary)
	for _, value := range values {
		if value.Canonical() {
			continue
		}
		counts[value.Category()] = append(counts[value.Category()], value)
	}
	pruned := 0
	for category, summaries := range counts {
		keep := s.config.MaxSummariesPerCategory
		if len(summaries) <= keep {
			continue
		}
		sort.Slice(summaries, func(i, j int) bool { return summaries[i].Version() > summaries[j].Version() })
		oldestKeptVersion := summaries[keep-1].Version()
		deleted, err := s.repository.DeleteOlderThanVersion(ctx, projectID, category, oldestKeptVersion)
		if err != nil {
			return pruned, err
		}
		pruned += deleted
	}
	return pruned, nil
}

func (s *Service) ensureProjectActive(ctx context.Context, projectID string) (project.Project, error) {
	value, err := s.projects.Get(ctx, projectID)
	if err != nil {
		if errors.Is(err, project.ErrNotFound) {
			return project.Project{}, ErrProjectNotFound
		}
		return project.Project{}, err
	}
	if value.Status() == project.StatusArchived {
		return project.Project{}, ErrProjectArchived
	}
	return value, nil
}

func relevant(value Summary, tokens []string) bool {
	if value.Canonical() || len(tokens) == 0 {
		return true
	}
	haystack := strings.ToLower(string(value.Category()) + " " + value.Content())
	for _, token := range tokens {
		if strings.Contains(haystack, token) {
			return true
		}
	}
	return false
}

func queryTokens(query string) []string {
	fields := strings.Fields(strings.ToLower(query))
	tokens := make([]string, 0, len(fields))
	for _, field := range fields {
		if len(field) >= 2 {
			tokens = append(tokens, field)
		}
	}
	return tokens
}

func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}
