CREATE TABLE authorization_denials (
    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    principal_id TEXT NOT NULL DEFAULT '',
    project_id TEXT NOT NULL DEFAULT '',
    required_capability TEXT NOT NULL DEFAULT '',
    resource TEXT NOT NULL,
    reason TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TRIGGER authorization_denials_prevent_update
BEFORE UPDATE ON authorization_denials
BEGIN
    SELECT RAISE(ABORT, 'authorization denial events are immutable');
END;

CREATE TRIGGER authorization_denials_prevent_delete
BEFORE DELETE ON authorization_denials
BEGIN
    SELECT RAISE(ABORT, 'authorization denial events are immutable');
END;
