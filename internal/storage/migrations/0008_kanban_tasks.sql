ALTER TABLE audit_events ADD COLUMN reason TEXT NOT NULL DEFAULT '';

CREATE TABLE tasks (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
    title TEXT NOT NULL CHECK(length(trim(title)) BETWEEN 1 AND 200),
    description TEXT NOT NULL DEFAULT '' CHECK(length(description) <= 16000),
    status TEXT NOT NULL CHECK(status IN ('BACKLOG', 'READY', 'WORKING', 'REVIEW', 'HUMAN_REVIEW', 'BLOCKED', 'FAILED', 'DONE', 'CANCELLED')),
    priority TEXT NOT NULL DEFAULT 'NORMAL' CHECK(priority IN ('LOW', 'NORMAL', 'HIGH', 'URGENT')),
    created_by_type TEXT NOT NULL CHECK(created_by_type IN ('LOCAL_API_CLIENT', 'ADMINISTRATOR', 'BRAIN', 'AGENT')),
    created_by TEXT NOT NULL CHECK(length(trim(created_by)) > 0),
    assigned_to TEXT REFERENCES agents(id) ON DELETE RESTRICT,
    human_review INTEGER NOT NULL DEFAULT 0 CHECK(human_review IN (0, 1)),
    parent_id TEXT REFERENCES tasks(id) ON DELETE RESTRICT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    last_actor_type TEXT NOT NULL CHECK(last_actor_type IN ('LOCAL_API_CLIENT', 'ADMINISTRATOR', 'BRAIN', 'AGENT')),
    last_actor_id TEXT NOT NULL CHECK(length(trim(last_actor_id)) > 0),
    last_reason TEXT NOT NULL CHECK(length(trim(last_reason)) BETWEEN 1 AND 2000)
);

CREATE INDEX tasks_project_status_created_idx ON tasks(project_id, status, created_at DESC, id);
CREATE INDEX tasks_project_assigned_created_idx ON tasks(project_id, assigned_to, created_at DESC, id);
CREATE INDEX tasks_parent_idx ON tasks(parent_id) WHERE parent_id IS NOT NULL;

CREATE TRIGGER tasks_audit_insert
AFTER INSERT ON tasks
BEGIN
    INSERT INTO audit_events(project_id, task_id, actor_type, actor_id, event_type, resource_type, resource_id, to_status, reason)
    VALUES (NEW.project_id, NEW.id, NEW.last_actor_type, NEW.last_actor_id, 'task.created', 'task', NEW.id, NEW.status, NEW.last_reason);
END;

CREATE TRIGGER tasks_audit_status_change
AFTER UPDATE OF status ON tasks
WHEN OLD.status <> NEW.status
BEGIN
    INSERT INTO audit_events(project_id, task_id, actor_type, actor_id, event_type, resource_type, resource_id, from_status, to_status, reason)
    VALUES (NEW.project_id, NEW.id, NEW.last_actor_type, NEW.last_actor_id, 'task.status.changed', 'task', NEW.id, OLD.status, NEW.status, NEW.last_reason);
END;

CREATE TRIGGER tasks_audit_assignment_change
AFTER UPDATE OF assigned_to ON tasks
WHEN OLD.assigned_to IS NOT NEW.assigned_to
BEGIN
    INSERT INTO audit_events(project_id, task_id, actor_type, actor_id, event_type, resource_type, resource_id, reason)
    VALUES (NEW.project_id, NEW.id, NEW.last_actor_type, NEW.last_actor_id, 'task.assigned', 'task', NEW.id, NEW.last_reason);
END;

CREATE TRIGGER tasks_prevent_self_parent
BEFORE INSERT ON tasks
WHEN NEW.parent_id = NEW.id
BEGIN
    SELECT RAISE(ABORT, 'task cannot be its own parent');
END;
