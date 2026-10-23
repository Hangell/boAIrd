CREATE TABLE notifications (
    id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
    source_key TEXT NOT NULL UNIQUE,
    event_type TEXT NOT NULL,
    resource_id TEXT NOT NULL DEFAULT '',
    severity TEXT NOT NULL CHECK(severity IN ('INFO', 'WARNING', 'CRITICAL')),
    summary TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    delivery_state TEXT NOT NULL DEFAULT 'PENDING' CHECK(delivery_state IN ('PENDING', 'DELIVERED', 'FAILED')),
    delivery_attempts INTEGER NOT NULL DEFAULT 0 CHECK(delivery_attempts BETWEEN 0 AND 5),
    delivery_error TEXT NOT NULL DEFAULT '',
    delivered_at TEXT,
    read_at TEXT,
    acknowledged_at TEXT
);

CREATE INDEX notifications_project_created_idx ON notifications(project_id, created_at DESC, id);
CREATE INDEX notifications_delivery_idx ON notifications(delivery_state, created_at, id);

CREATE TRIGGER notifications_task_status
AFTER INSERT ON audit_events
WHEN NEW.event_type = 'task.status.changed' AND NEW.to_status IN ('BLOCKED', 'FAILED', 'HUMAN_REVIEW')
BEGIN
    INSERT OR IGNORE INTO notifications(project_id, source_key, event_type, resource_id, severity, summary)
    VALUES (NEW.project_id, 'audit:' || NEW.id, NEW.to_status, NEW.resource_id,
        CASE WHEN NEW.to_status = 'FAILED' THEN 'CRITICAL' ELSE 'WARNING' END,
        CASE NEW.to_status
            WHEN 'BLOCKED' THEN 'A task is blocked and needs administrator attention.'
            WHEN 'FAILED' THEN 'A task failed and needs administrator attention.'
            ELSE 'A task is waiting for human review.'
        END);
END;

CREATE TRIGGER notifications_run_failed
AFTER INSERT ON audit_events
WHEN NEW.event_type = 'run.status.changed' AND NEW.to_status = 'FAILED'
BEGIN
    INSERT OR IGNORE INTO notifications(project_id, source_key, event_type, resource_id, severity, summary)
    VALUES (NEW.project_id, 'audit:' || NEW.id, 'run.failed', NEW.resource_id, 'CRITICAL', 'An agent or provider run failed.');
END;

CREATE TRIGGER notifications_security_denial
AFTER INSERT ON authorization_denials
WHEN NEW.project_id <> '' AND EXISTS (SELECT 1 FROM projects WHERE id = NEW.project_id)
BEGIN
    INSERT OR IGNORE INTO notifications(project_id, source_key, event_type, resource_id, severity, summary)
    VALUES (NEW.project_id, 'denial:' || NEW.sequence, 'security.authorization_denied', '', 'CRITICAL', 'A project operation was denied by security policy.');
END;

CREATE TRIGGER notifications_agent_error
AFTER INSERT ON agent_status_events
WHEN NEW.to_status IN ('ERROR', 'STALE', 'OFFLINE')
BEGIN
    INSERT OR IGNORE INTO notifications(project_id, source_key, event_type, resource_id, severity, summary)
    SELECT project_id, 'agent-status:' || NEW.sequence || ':' || project_id,
        CASE WHEN NEW.to_status = 'ERROR' THEN 'agent.failed' ELSE 'agent.disconnected' END,
        NEW.agent_id,
        CASE WHEN NEW.to_status = 'ERROR' THEN 'CRITICAL' ELSE 'WARNING' END,
        CASE WHEN NEW.to_status = 'ERROR' THEN 'An agent entered an error state.' ELSE 'An external agent disconnected or became stale.' END
    FROM (
        SELECT ap.project_id FROM agent_projects ap WHERE ap.agent_id = NEW.agent_id
        UNION
        SELECT b.project_id FROM brains b JOIN projects p ON p.id = b.project_id AND p.brain_id = b.id
        WHERE b.id = NEW.agent_id AND b.active = 1
    );
END;
