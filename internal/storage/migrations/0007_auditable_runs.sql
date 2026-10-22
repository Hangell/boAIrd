ALTER TABLE runs ADD COLUMN tools_used TEXT NOT NULL DEFAULT '[]';

CREATE TABLE audit_events (
    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    id TEXT NOT NULL UNIQUE DEFAULT (lower(hex(randomblob(16)))),
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
    task_id TEXT,
    actor_type TEXT NOT NULL,
    actor_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    from_status TEXT NOT NULL DEFAULT '',
    to_status TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX runs_project_task_created_idx ON runs(project_id, task_id, created_at DESC, id);
CREATE INDEX runs_project_agent_created_idx ON runs(project_id, agent_id, created_at DESC, id);
CREATE INDEX audit_events_run_idx ON audit_events(resource_type, resource_id, sequence);
CREATE INDEX audit_events_project_idx ON audit_events(project_id, sequence DESC);

CREATE TRIGGER runs_audit_insert
AFTER INSERT ON runs
BEGIN
    INSERT INTO audit_events(project_id, task_id, actor_type, actor_id, event_type, resource_type, resource_id, to_status)
    VALUES (NEW.project_id, NEW.task_id, 'agent', NEW.agent_id, 'run.created', 'run', NEW.id, NEW.status);
END;

CREATE TRIGGER runs_audit_status_change
AFTER UPDATE OF status ON runs
WHEN OLD.status <> NEW.status
BEGIN
    INSERT INTO audit_events(project_id, task_id, actor_type, actor_id, event_type, resource_type, resource_id, from_status, to_status)
    VALUES (NEW.project_id, NEW.task_id, 'agent', NEW.agent_id, 'run.status.changed', 'run', NEW.id, OLD.status, NEW.status);
END;

CREATE TRIGGER audit_events_prevent_update
BEFORE UPDATE ON audit_events
BEGIN
    SELECT RAISE(ABORT, 'audit events are immutable');
END;

CREATE TRIGGER audit_events_prevent_delete
BEFORE DELETE ON audit_events
BEGIN
    SELECT RAISE(ABORT, 'audit events are immutable');
END;
