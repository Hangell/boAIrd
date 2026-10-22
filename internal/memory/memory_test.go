package memory

import (
	"strings"
	"testing"
	"time"
)

func TestNewValidatesInput(t *testing.T) {
	created := time.Now().UTC()
	tests := []struct {
		name        string
		projectID   string
		category    Category
		version     int
		content     string
		createdBy   string
		createdAt   time.Time
		wantErrText string
	}{
		{
			name: "missing project id", projectID: "  ", category: CategoryKnownProblem,
			version: 1, content: "content", createdBy: "brain", createdAt: created,
			wantErrText: "required",
		},
		{
			name: "missing creator", projectID: "p1", category: CategoryKnownProblem,
			version: 1, content: "content", createdBy: "  ", createdAt: created,
			wantErrText: "required",
		},
		{
			name: "unknown category", projectID: "p1", category: Category("unknown"),
			version: 1, content: "content", createdBy: "brain", createdAt: created,
			wantErrText: "unknown memory category",
		},
		{
			name: "zero version", projectID: "p1", category: CategoryKnownProblem,
			version: 0, content: "content", createdBy: "brain", createdAt: created,
			wantErrText: "version must be positive",
		},
		{
			name: "empty content", projectID: "p1", category: CategoryKnownProblem,
			version: 1, content: "   ", createdBy: "brain", createdAt: created,
			wantErrText: "must contain",
		},
		{
			name: "oversized content", projectID: "p1", category: CategoryKnownProblem,
			version: 1, content: strings.Repeat("a", MaximumContentBytes+1), createdBy: "brain", createdAt: created,
			wantErrText: "must contain",
		},
		{
			name: "missing creation time", projectID: "p1", category: CategoryKnownProblem,
			version: 1, content: "content", createdBy: "brain", createdAt: time.Time{},
			wantErrText: "creation time",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New("id", tt.projectID, tt.category, tt.version, tt.content, "brain", tt.createdBy, nil, tt.createdAt)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tt.wantErrText) {
				t.Fatalf("error = %q, want substring %q", err, tt.wantErrText)
			}
		})
	}
}

func TestCanonical(t *testing.T) {
	canonical := []Category{CategoryArchitectureDecision, CategoryProjectDecision}
	ephemeral := []Category{CategoryImportantContext, CategoryKnownProblem, CategoryCurrentState, CategoryTaskHistory}
	created := time.Now().UTC()
	for _, category := range canonical {
		value, err := New("id", "p1", category, 1, "content", "brain", "brain", nil, created)
		if err != nil {
			t.Fatal(err)
		}
		if !value.Canonical() {
			t.Fatalf("category %q should be canonical", category)
		}
	}
	for _, category := range ephemeral {
		value, err := New("id", "p1", category, 1, "content", "brain", "brain", nil, created)
		if err != nil {
			t.Fatal(err)
		}
		if value.Canonical() {
			t.Fatalf("category %q should not be canonical", category)
		}
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	created := time.Now().UTC()
	value, err := New("id", "p1", CategoryArchitectureDecision, 3, "content", "brain", "brain", []string{"s1", "s2", "s1", ""}, created)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Restore(value.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if restored.ID() != value.ID() || restored.ProjectID() != value.ProjectID() ||
		restored.Category() != value.Category() || restored.Version() != value.Version() ||
		restored.Content() != value.Content() || restored.CreatedBy() != value.CreatedBy() ||
		!restored.CreatedAt().Equal(value.CreatedAt()) {
		t.Fatalf("round trip mismatch: %#v vs %#v", restored, value)
	}
	if len(restored.SourceIDs()) != 2 || restored.SourceIDs()[0] != "s1" || restored.SourceIDs()[1] != "s2" {
		t.Fatalf("source IDs = %v, want deduplicated [s1 s2]", restored.SourceIDs())
	}
}

func TestSanitizeContentRedactsSecrets(t *testing.T) {
	cleaned, err := SanitizeContent("decided to use key sk-1234567890abcdefgh for the worker")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(cleaned, "sk-1234567890abcdefgh") {
		t.Fatalf("sanitized content leaked secret: %q", cleaned)
	}
	if !strings.Contains(cleaned, "[REDACTED]") {
		t.Fatalf("expected redaction marker in %q", cleaned)
	}
}

func TestSanitizeContentFailsClosedWhenRedactionIncomplete(t *testing.T) {
	// A bearer token followed by more token characters leaves a recognizable
	// bearer pattern behind after the first redaction pass.
	_, err := SanitizeContent("Bearer abc def")
	if err == nil {
		t.Fatal("expected fail-closed error for incomplete redaction")
	}
	if !strings.Contains(err.Error(), ErrSanitization.Error()) {
		t.Fatalf("error = %q, want to wrap ErrSanitization", err)
	}
}
