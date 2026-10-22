CREATE TABLE mcp_agent_tokens (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE RESTRICT,
    token_hash BLOB NOT NULL UNIQUE CHECK(length(token_hash) = 32),
    issued_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    revoked_at TEXT
);

CREATE INDEX mcp_agent_tokens_agent_idx ON mcp_agent_tokens(agent_id, issued_at DESC);

CREATE TABLE mcp_agent_token_events (
    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    token_id TEXT NOT NULL REFERENCES mcp_agent_tokens(id) ON DELETE RESTRICT,
    related_token_id TEXT NOT NULL DEFAULT '',
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE RESTRICT,
    actor_id TEXT NOT NULL,
    event_type TEXT NOT NULL CHECK(event_type IN ('token.issued', 'token.rotated', 'token.revoked', 'token.bootstrapped')),
    created_at TEXT NOT NULL
);

CREATE INDEX mcp_agent_token_events_token_idx ON mcp_agent_token_events(token_id, sequence);

CREATE TRIGGER mcp_agent_token_events_prevent_update
BEFORE UPDATE ON mcp_agent_token_events
BEGIN
    SELECT RAISE(ABORT, 'MCP token events are immutable');
END;

CREATE TRIGGER mcp_agent_token_events_prevent_delete
BEFORE DELETE ON mcp_agent_token_events
BEGIN
    SELECT RAISE(ABORT, 'MCP token events are immutable');
END;
