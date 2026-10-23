package template

import (
	"testing"

	"boAIrd/internal/agent"
)

func TestValidateKey(t *testing.T) {
	for _, valid := range []string{"go-senior", "qa", "security-reviewer"} {
		if err := validateKey(valid); err != nil {
			t.Fatalf("validateKey(%q) = %v", valid, err)
		}
	}
	for _, invalid := range []string{"", "UPPER", "a", "-lead", "has space", "under_score"} {
		if err := validateKey(invalid); err == nil {
			t.Fatalf("validateKey(%q) accepted invalid key", invalid)
		}
	}
}

func TestValidateRuntime(t *testing.T) {
	for _, runtime := range []agent.Runtime{agent.RuntimeAPI, agent.RuntimeMCP, agent.RuntimeLocal, agent.RuntimeRemote} {
		if err := validateRuntime(runtime); err != nil {
			t.Fatalf("validateRuntime(%q) = %v", runtime, err)
		}
	}
	if err := validateRuntime("BOGUS"); err == nil {
		t.Fatal("validateRuntime accepted an unknown runtime")
	}
}

func TestValidateCapabilitiesAndTools(t *testing.T) {
	capabilities, err := ValidateCapabilities([]string{"task.read", "task.read", "task.review"})
	if err != nil {
		t.Fatal(err)
	}
	if len(capabilities) != 2 || capabilities[0] != "task.read" || capabilities[1] != "task.review" {
		t.Fatalf("capabilities = %v", capabilities)
	}
	if _, err := ValidateCapabilities([]string{"Not-A-Cap"}); err == nil {
		t.Fatal("expected invalid capability to be rejected")
	}
	tools, err := ValidateTools([]string{"mermaid", "drawio"}, []string{"mermaid", "plantuml", "drawio"})
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 2 {
		t.Fatalf("tools = %v", tools)
	}
	if _, err := ValidateTools([]string{"shell"}, []string{"mermaid", "drawio"}); err == nil {
		t.Fatal("expected unavailable tool to be rejected")
	}
}
