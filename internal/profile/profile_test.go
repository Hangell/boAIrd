package profile

import (
	"strings"
	"testing"
)

func TestBuiltinsAreValidAndStable(t *testing.T) {
	definitions := Builtins()
	if len(definitions) < 7 {
		t.Fatalf("built-in profiles = %d, want at least 7", len(definitions))
	}
	keys := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		if strings.TrimSpace(definition.Key) == "" || strings.TrimSpace(definition.Name) == "" {
			t.Fatalf("profile %q missing key or name", definition.Key)
		}
		if _, exists := keys[definition.Key]; exists {
			t.Fatalf("duplicate profile key %q", definition.Key)
		}
		keys[definition.Key] = struct{}{}
		if definition.Key != strings.ToLower(definition.Key) {
			t.Fatalf("profile key %q must be lowercase", definition.Key)
		}
		if strings.TrimSpace(definition.Description) == "" || strings.TrimSpace(definition.Instructions) == "" {
			t.Fatalf("profile %q missing description or instructions", definition.Key)
		}
		if _, err := ValidCapabilities(definition.SuggestedCapabilities); err != nil {
			t.Fatalf("profile %q has invalid suggested capabilities: %v", definition.Key, err)
		}
		if _, err := definition.ValidateTools(definition.SuggestedTools); err != nil {
			t.Fatalf("profile %q has invalid suggested tools: %v", definition.Key, err)
		}
	}
}

func TestLookup(t *testing.T) {
	if _, ok := Lookup("qa"); !ok {
		t.Fatal("expected qa profile to exist")
	}
	if _, ok := Lookup(" QA "); !ok {
		t.Fatal("expected lookup to be case/space insensitive")
	}
	if _, ok := Lookup("unknown"); ok {
		t.Fatal("expected unknown profile to be absent")
	}
}

func TestValidCapabilitiesRejectsInvalidAndDeduplicates(t *testing.T) {
	values, err := ValidCapabilities([]string{"task.read", "task.read", "task.review"})
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || values[0] != "task.read" || values[1] != "task.review" {
		t.Fatalf("capabilities = %v, want deduplicated [task.read task.review]", values)
	}
	if _, err := ValidCapabilities([]string{"Not-A-Cap"}); err == nil {
		t.Fatal("expected invalid capability to be rejected")
	}
}

func TestValidateToolsRejectsUnavailableTools(t *testing.T) {
	definition, ok := Lookup("qa")
	if !ok {
		t.Fatal("qa profile missing")
	}
	if _, err := definition.ValidateTools([]string{"filesystem"}); err != nil {
		t.Fatal(err)
	}
	if _, err := definition.ValidateTools([]string{"shell"}); err == nil {
		t.Fatal("expected shell tool to be rejected for the qa profile")
	}
}
