package mcptoken_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"boAIrd/internal/agent"
	"boAIrd/internal/config"
	"boAIrd/internal/mcptoken"
	"boAIrd/internal/storage"
)

func TestCreateRotateRevokeAndAuditLifecycle(t *testing.T) {
	ctx := context.Background()
	database, service, agentID := newTestService(t)
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})

	issued, raw, err := service.Create(ctx, agentID, time.Hour, "local-administrator")
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 64 || issued.Status != "active" || issued.AgentID != agentID {
		t.Fatalf("issued metadata = %+v, token length = %d", issued, len(raw))
	}
	if got, err := service.Verify(ctx, raw); err != nil || got.AgentID != agentID || got.TokenID != issued.ID {
		t.Fatalf("Verify() = %q, %v", got, err)
	}
	decoded := mustDecode(t, raw)
	digest := sha256.Sum256(decoded)
	clear(decoded)
	var stored []byte
	if err := database.QueryRowContext(ctx, `SELECT token_hash FROM mcp_agent_tokens WHERE id = ?`, issued.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, digest[:]) {
		t.Fatal("database did not store the one-way token verifier")
	}

	replacement, rotatedRaw, err := service.Rotate(ctx, issued.ID, 2*time.Hour, "local-administrator")
	if err != nil {
		t.Fatal(err)
	}
	if replacement.AgentID != agentID || replacement.Status != "active" || rotatedRaw == raw {
		t.Fatalf("rotated metadata = %+v, tokens equal = %v", replacement, rotatedRaw == raw)
	}
	if _, err := service.Verify(ctx, raw); err == nil {
		t.Fatal("old token remained valid after rotation")
	}
	if got, err := service.Verify(ctx, rotatedRaw); err != nil || got.AgentID != agentID || got.TokenID != replacement.ID {
		t.Fatalf("replacement token Verify() = %q, %v", got, err)
	}
	if err := service.Revoke(ctx, replacement.ID, "local-administrator"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Verify(ctx, rotatedRaw); err == nil {
		t.Fatal("revoked token remained valid")
	}
	values, err := service.List(ctx)
	if err != nil || len(values) != 2 || values[0].Status != "revoked" || values[1].Status != "revoked" {
		t.Fatalf("token metadata after rotation/revocation = %+v, error = %v", values, err)
	}
	var events int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM mcp_agent_token_events`).Scan(&events); err != nil || events != 3 {
		t.Fatalf("immutable lifecycle audit count = %d, error = %v", events, err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE mcp_agent_token_events SET actor_id='attacker'`); err == nil {
		t.Fatal("token audit events allowed modification")
	}
}

func TestExpiryAndBootstrapAreFiniteAndOneTime(t *testing.T) {
	ctx := context.Background()
	database, service, agentID := newTestService(t)
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	issued, raw, err := service.Create(ctx, agentID, time.Nanosecond, "local-administrator")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	if _, err := service.Verify(ctx, raw); err == nil {
		t.Fatal("expired token remained valid")
	}
	metadata, err := service.List(ctx)
	if err != nil || len(metadata) != 1 || metadata[0].ID != issued.ID || metadata[0].Status != "expired" {
		t.Fatalf("expired token listing = %+v, error = %v", metadata, err)
	}

	var legacy config.MCPAgentToken
	legacy.AgentID = agentID
	copy(legacy.Token[:], mustDecode(t, strings.Repeat("04", 32)))
	seeded, err := service.Bootstrap(ctx, []config.MCPAgentToken{legacy}, "local-administrator")
	if err != nil || !seeded {
		t.Fatalf("first Bootstrap() = %v, %v", seeded, err)
	}
	seeded, err = service.Bootstrap(ctx, []config.MCPAgentToken{legacy}, "local-administrator")
	if err != nil || seeded {
		t.Fatalf("second Bootstrap() = %v, %v", seeded, err)
	}
	if _, err := service.Verify(ctx, strings.Repeat("04", 32)); err != nil {
		t.Fatalf("bootstrapped token not active: %v", err)
	}
	bootstrappedTokens, err := service.List(ctx)
	if err != nil || len(bootstrappedTokens) != 2 {
		t.Fatalf("list bootstrapped tokens = %+v, error = %v", bootstrappedTokens, err)
	}
	var expiry string
	digest := sha256.Sum256(legacy.Token[:])
	if err := database.QueryRowContext(ctx, `SELECT expires_at FROM mcp_agent_tokens WHERE token_hash = ?`, digest[:]).Scan(&expiry); err != nil {
		t.Fatal(err)
	}
	parsed, err := time.Parse(time.RFC3339Nano, expiry)
	if err != nil || !parsed.Equal(bootstrappedTokens[0].IssuedAt.Add(90*24*time.Hour)) {
		t.Fatalf("bootstrapped expiry = %q, error = %v", expiry, err)
	}
}

func TestCreateRequiresExistingMCPAgentAndBoundedExpiry(t *testing.T) {
	ctx := context.Background()
	database, service, _ := newTestService(t)
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	if _, _, err := service.Create(ctx, "missing-agent", time.Hour, "local-administrator"); err == nil {
		t.Fatal("Create() accepted a missing Agent")
	}
	if _, _, err := service.Create(ctx, "agent-local", time.Hour, "local-administrator"); err == nil {
		t.Fatal("Create() accepted an Agent without MCP runtime")
	}
	if _, _, err := service.Create(ctx, "agent-1", 365*24*time.Hour+time.Second, "local-administrator"); err == nil {
		t.Fatal("Create() accepted a lifetime beyond the maximum")
	}
}

func newTestService(t *testing.T) (*storage.Database, *mcptoken.Service, string) {
	t.Helper()
	ctx := context.Background()
	database, err := storage.OpenSQLite(ctx, filepath.Join(t.TempDir(), "boaird.db"))
	if err != nil {
		t.Fatal(err)
	}
	closeDatabase := func() {
		if err := database.Close(); err != nil {
			t.Errorf("close test database after setup failure: %v", err)
		}
	}
	agentRepository, err := storage.NewAgentRepository(database)
	if err != nil {
		closeDatabase()
		t.Fatal(err)
	}
	value, err := agent.NewConfigured("agent-1", agent.Configuration{Name: "External Agent", Role: "worker", Runtime: agent.RuntimeMCP})
	if err != nil {
		closeDatabase()
		t.Fatal(err)
	}
	if err := agentRepository.Create(ctx, value); err != nil {
		closeDatabase()
		t.Fatal(err)
	}
	localValue, err := agent.NewConfigured("agent-local", agent.Configuration{Name: "Local Agent", Role: "worker", Runtime: agent.RuntimeLocal})
	if err == nil {
		if err := agentRepository.Create(ctx, localValue); err != nil {
			closeDatabase()
			t.Fatal(err)
		}
	}
	tokenRepository, err := storage.NewMCPTokenRepository(database)
	if err != nil {
		closeDatabase()
		t.Fatal(err)
	}
	manager, err := agent.NewManager(agentRepository)
	if err != nil {
		closeDatabase()
		t.Fatal(err)
	}
	service, err := mcptoken.NewService(tokenRepository, manager)
	if err != nil {
		closeDatabase()
		t.Fatal(err)
	}
	return database, service, value.ID()
}

func mustDecode(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := hex.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}
