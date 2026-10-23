CREATE TABLE extensions (
    name TEXT PRIMARY KEY,
    version TEXT NOT NULL,
    manifest TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('pending', 'active')),
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE extension_audit (
    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_id TEXT NOT NULL,
    extension_name TEXT NOT NULL,
    event_type TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE extension_tool_names (
    tool_name TEXT PRIMARY KEY,
    extension_name TEXT NOT NULL REFERENCES extensions(name) ON DELETE CASCADE
);

CREATE INDEX extension_tool_names_extension_idx ON extension_tool_names(extension_name);

CREATE TRIGGER extension_audit_prevent_update
BEFORE UPDATE ON extension_audit
BEGIN
    SELECT RAISE(ABORT, 'extension audit events are immutable');
END;

CREATE TRIGGER extension_audit_prevent_delete
BEFORE DELETE ON extension_audit
BEGIN
    SELECT RAISE(ABORT, 'extension audit events are immutable');
END;
