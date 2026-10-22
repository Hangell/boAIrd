package memory_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"boAIrd/internal/memory"
	"boAIrd/internal/storage"
)

func newTestService(t *testing.T) (*memory.Service, *storage.MemoryRepository) {
	t.Helper()
	ctx := context.Background()
	database, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "boaird.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	if _, err := database.Exec(`INSERT INTO projects(id, name, status, human_review_required) VALUES ('p1', 'Active', 'CREATED', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO projects(id, name, status, human_review_required) VALUES ('p2', 'Other', 'CREATED', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO projects(id, name, status, human_review_required) VALUES ('p3', 'Archived', 'ARCHIVED', 1)`); err != nil {
		t.Fatal(err)
	}
	projects, err := storage.NewProjectRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := storage.NewMemoryRepository(database)
	if err != nil {
		t.Fatal(err)
	}
	service, err := memory.NewService(repository, projects, memory.Config{MaxSummariesPerCategory: 2, MaxSelectionBytes: 1 << 10})
	if err != nil {
		t.Fatal(err)
	}
	return service, repository
}

func TestRecordSanitizesSecrets(t *testing.T) {
	ctx := context.Background()
	service, _ := newTestService(t)
	raw := "store key sk-1234567890abcdefgh and password=hunter2"
	value, err := service.Record(ctx, "p1", memory.RecordInput{
		Category: memory.CategoryKnownProblem, Content: raw, SourceIDs: []string{"src-1"},
		CreatedByType: "brain", CreatedBy: "brain-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(value.Content(), "sk-1234567890abcdefgh") || strings.Contains(value.Content(), "hunter2") {
		t.Fatalf("stored memory leaked secret: %q", value.Content())
	}
	if !strings.Contains(value.Content(), "[REDACTED]") {
		t.Fatalf("expected redaction marker in %q", value.Content())
	}
	if value.Version() != 1 {
		t.Fatalf("version = %d, want 1", value.Version())
	}
}

func TestRecordProvenanceAndVersioning(t *testing.T) {
	ctx := context.Background()
	service, _ := newTestService(t)
	first, err := service.Record(ctx, "p1", memory.RecordInput{
		Category: memory.CategoryKnownProblem, Content: "first note", SourceIDs: []string{"s1", "s2"},
		CreatedByType: "brain", CreatedBy: "brain-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Record(ctx, "p1", memory.RecordInput{
		Category: memory.CategoryKnownProblem, Content: "second note", SourceIDs: []string{"s2", "s3"},
		CreatedByType: "worker", CreatedBy: "worker-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Version() != 1 || second.Version() != 2 {
		t.Fatalf("versions = %d, %d, want 1, 2", first.Version(), second.Version())
	}
	if len(first.SourceIDs()) != 2 || first.SourceIDs()[0] != "s1" || first.SourceIDs()[1] != "s2" {
		t.Fatalf("first source IDs = %v", first.SourceIDs())
	}
	if second.CreatedBy() != "worker-1" || second.CreatedByType() != "worker" {
		t.Fatalf("second provenance = %s/%s", second.CreatedByType(), second.CreatedBy())
	}
	if second.CreatedAt().IsZero() {
		t.Fatal("createdAt not populated")
	}
}

func TestDecisionsReturnsCanonicalNewestFirst(t *testing.T) {
	ctx := context.Background()
	service, _ := newTestService(t)
	if _, err := service.Record(ctx, "p1", memory.RecordInput{Category: memory.CategoryArchitectureDecision, Content: "first decision", CreatedByType: "brain", CreatedBy: "brain-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Record(ctx, "p1", memory.RecordInput{Category: memory.CategoryKnownProblem, Content: "ephemeral note", CreatedByType: "brain", CreatedBy: "brain-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Record(ctx, "p1", memory.RecordInput{Category: memory.CategoryArchitectureDecision, Content: "second decision", CreatedByType: "brain", CreatedBy: "brain-1"}); err != nil {
		t.Fatal(err)
	}
	decisions, err := service.Decisions(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 2 {
		t.Fatalf("decisions = %d, want 2", len(decisions))
	}
	if decisions[0].Version() != 2 || decisions[1].Version() != 1 {
		t.Fatalf("decision order = %d, %d, want 2, 1", decisions[0].Version(), decisions[1].Version())
	}
}

func TestPruneKeepsCanonicalAndLimitsEphemeral(t *testing.T) {
	ctx := context.Background()
	service, _ := newTestService(t)
	for i := 0; i < 3; i++ {
		if _, err := service.Record(ctx, "p1", memory.RecordInput{Category: memory.CategoryKnownProblem, Content: "problem", CreatedByType: "brain", CreatedBy: "brain-1"}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ {
		if _, err := service.Record(ctx, "p1", memory.RecordInput{Category: memory.CategoryArchitectureDecision, Content: "decision", CreatedByType: "brain", CreatedBy: "brain-1"}); err != nil {
			t.Fatal(err)
		}
	}
	pruned, err := service.Prune(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if pruned != 1 {
		t.Fatalf("pruned = %d, want 1", pruned)
	}
	values, err := service.List(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	var problems, decisions int
	for _, value := range values {
		if value.Category() == memory.CategoryKnownProblem {
			problems++
		}
		if value.Category() == memory.CategoryArchitectureDecision {
			decisions++
		}
	}
	if problems != 2 {
		t.Fatalf("known_problem summaries = %d, want 2", problems)
	}
	if decisions != 3 {
		t.Fatalf("canonical decisions = %d, want 3", decisions)
	}
}

func TestSelectRespectsSizeLimit(t *testing.T) {
	ctx := context.Background()
	service, _ := newTestService(t)
	if _, err := service.Record(ctx, "p1", memory.RecordInput{Category: memory.CategoryArchitectureDecision, Content: "decision content", CreatedByType: "brain", CreatedBy: "brain-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Record(ctx, "p1", memory.RecordInput{Category: memory.CategoryKnownProblem, Content: strings.Repeat("problem ", 100), CreatedByType: "brain", CreatedBy: "brain-1"}); err != nil {
		t.Fatal(err)
	}
	selection, err := service.Select(ctx, "p1", "", 128)
	if err != nil {
		t.Fatal(err)
	}
	if selection.SelectedBytes > 128 {
		t.Fatalf("selected bytes = %d, exceeds limit 128", selection.SelectedBytes)
	}
	if selection.RawHistoryBytes <= selection.SelectedBytes {
		t.Fatalf("raw bytes %d should exceed selected bytes %d", selection.RawHistoryBytes, selection.SelectedBytes)
	}
	canonicalIncluded := false
	for _, item := range selection.Items {
		if item.Canonical() {
			canonicalIncluded = true
		}
	}
	if !canonicalIncluded {
		t.Fatal("canonical decision was omitted from selection")
	}
}

func TestRecordFailsForUnknownOrArchivedProject(t *testing.T) {
	ctx := context.Background()
	service, _ := newTestService(t)
	if _, err := service.Record(ctx, "missing", memory.RecordInput{Category: memory.CategoryKnownProblem, Content: "note", CreatedByType: "brain", CreatedBy: "brain-1"}); err == nil {
		t.Fatal("expected project not found error")
	} else if !strings.Contains(err.Error(), memory.ErrProjectNotFound.Error()) {
		t.Fatalf("error = %q, want ErrProjectNotFound", err)
	}
	if _, err := service.Record(ctx, "p3", memory.RecordInput{Category: memory.CategoryKnownProblem, Content: "note", CreatedByType: "brain", CreatedBy: "brain-1"}); err == nil {
		t.Fatal("expected archived project error")
	} else if !strings.Contains(err.Error(), memory.ErrProjectArchived.Error()) {
		t.Fatalf("error = %q, want ErrProjectArchived", err)
	}
}

func TestProjectIsolation(t *testing.T) {
	ctx := context.Background()
	service, _ := newTestService(t)
	if _, err := service.Record(ctx, "p1", memory.RecordInput{Category: memory.CategoryKnownProblem, Content: "p1 note", CreatedByType: "brain", CreatedBy: "brain-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Record(ctx, "p2", memory.RecordInput{Category: memory.CategoryKnownProblem, Content: "p2 note", CreatedByType: "brain", CreatedBy: "brain-1"}); err != nil {
		t.Fatal(err)
	}
	p1, err := service.List(ctx, "p1")
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range p1 {
		if value.ProjectID() != "p1" || strings.Contains(value.Content(), "p2 note") {
			t.Fatalf("project isolation violated: %#v", value)
		}
	}
	if len(p1) != 1 {
		t.Fatalf("p1 summaries = %d, want 1", len(p1))
	}
}
