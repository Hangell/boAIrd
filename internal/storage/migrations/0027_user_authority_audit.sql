CREATE TABLE user_authority_audit_events (
    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    id TEXT NOT NULL UNIQUE DEFAULT (lower(hex(randomblob(16)))),
    actor_type TEXT NOT NULL DEFAULT 'human' CHECK(actor_type = 'human'),
    actor_id TEXT NOT NULL CHECK(length(trim(actor_id)) BETWEEN 1 AND 200),
    event_type TEXT NOT NULL CHECK(length(trim(event_type)) BETWEEN 1 AND 100),
    target_user_id TEXT NOT NULL CHECK(length(trim(target_user_id)) BETWEEN 1 AND 200),
    project_id TEXT NOT NULL DEFAULT '',
    previous_role TEXT NOT NULL DEFAULT '',
    next_role TEXT NOT NULL DEFAULT '',
    result TEXT NOT NULL CHECK(length(trim(result)) BETWEEN 1 AND 100),
    reason TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX user_authority_audit_target_idx
    ON user_authority_audit_events(target_user_id, sequence DESC);
CREATE INDEX user_authority_audit_project_idx
    ON user_authority_audit_events(project_id, sequence DESC);

CREATE TRIGGER user_authority_audit_prevent_update
BEFORE UPDATE ON user_authority_audit_events
BEGIN
    SELECT RAISE(ABORT, 'user authority audit events are immutable');
END;

CREATE TRIGGER user_authority_audit_prevent_delete
BEFORE DELETE ON user_authority_audit_events
BEGIN
    SELECT RAISE(ABORT, 'user authority audit events are immutable');
END;
