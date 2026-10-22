package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestRedactingHandlerSanitizesSensitiveAttributesAndText(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(NewRedactingHandler(slog.NewJSONHandler(&output, nil)))
	secret := "fake-secret-123456789"
	logger.Info("request Authorization: Bearer abcdefghijklmnop sk-1234567890abcdefgh api_key=message-key",
		"api_key", secret,
		"nested", slog.GroupValue(slog.String("client_secret", secret), slog.String("note", "Bearer abcdefghijklmnop")),
		"details", map[string]string{"authorization": "Bearer map-token-123456"},
		"credential_id", "cred-1")
	result := output.String()
	if strings.Contains(result, secret) || strings.Contains(result, "abcdefghijklmnop") || strings.Contains(result, "sk-1234567890abcdefgh") || strings.Contains(result, "message-key") || strings.Contains(result, "map-token-123456") {
		t.Fatalf("log leaked a secret: %s", result)
	}
	if !strings.Contains(result, "cred-1") || !strings.Contains(result, "[REDACTED]") {
		t.Fatalf("expected redaction and safe metadata, got %s", result)
	}
}

func TestRedactSensitiveTextHandlesJSONAndCommonTokens(t *testing.T) {
	input := `{"api_key":"raw-secret-value","authorization":"Bearer raw-token-value","key":"ghp_012345678901234567890123456789012345"}`
	output := RedactSensitiveText(input)
	for _, secret := range []string{"raw-secret-value", "raw-token-value", "ghp_012345678901234567890123456789012345"} {
		if strings.Contains(output, secret) {
			t.Errorf("redaction leaked %q in %q", secret, output)
		}
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Fatalf("redacted text = %q", output)
	}
}
