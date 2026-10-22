CREATE TABLE agent_sessions (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE RESTRICT,
    token_id TEXT NOT NULL REFERENCES mcp_agent_tokens(id) ON DELETE RESTRICT,
    started_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    ended_at TEXT
);

CREATE UNIQUE INDEX agent_sessions_open_idx ON agent_sessions(agent_id, token_id) WHERE ended_at IS NULL;

CREATE INDEX agent_sessions_agent_seen_idx ON agent_sessions(agent_id, last_seen_at DESC);

CREATE TABLE agent_status_events (
    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE RESTRICT,
    session_id TEXT NOT NULL DEFAULT '',
    from_status TEXT NOT NULL CHECK(from_status IN ('ONLINE', 'IDLE', 'BUSY', 'WAITING', 'OFFLINE', 'STALE', 'ERROR')),
    to_status TEXT NOT NULL CHECK(to_status IN ('ONLINE', 'IDLE', 'BUSY', 'WAITING', 'OFFLINE', 'STALE', 'ERROR')),
    reason TEXT NOT NULL CHECK(reason IN ('session.opened', 'session.resumed', 'session.expired', 'work.changed', 'status.reconciled')),
    created_at TEXT NOT NULL
);

CREATE INDEX agent_status_events_agent_idx ON agent_status_events(agent_id, sequence DESC);

CREATE TRIGGER agent_status_events_prevent_update
BEFORE UPDATE ON agent_status_events
BEGIN
    SELECT RAISE(ABORT, 'agent status events are immutable');
END;

CREATE TRIGGER agent_status_events_prevent_delete
BEFORE DELETE ON agent_status_events
BEGIN
    SELECT RAISE(ABORT, 'agent status events are immutable');
END;
