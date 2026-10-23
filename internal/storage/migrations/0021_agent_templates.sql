CREATE TABLE agent_templates (
    id TEXT PRIMARY KEY,
    template_key TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    role TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    instructions TEXT NOT NULL DEFAULT '',
    default_runtime TEXT NOT NULL,
    suggested_tools TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(suggested_tools) AND json_type(suggested_tools) = 'array'),
    suggested_capabilities TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(suggested_capabilities) AND json_type(suggested_capabilities) = 'array'),
    version INTEGER NOT NULL CHECK(version >= 1),
    created_by TEXT NOT NULL,
    updated_by TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE template_instances (
    agent_id TEXT PRIMARY KEY REFERENCES agents(id) ON DELETE CASCADE,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
    template_key TEXT NOT NULL,
    template_version INTEGER NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX template_instances_project_idx ON template_instances(project_id, template_key);

CREATE TABLE template_audit (
    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    template_key TEXT NOT NULL,
    action TEXT NOT NULL,
    version INTEGER NOT NULL,
    actor_id TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TRIGGER template_audit_prevent_update
BEFORE UPDATE ON template_audit
BEGIN
    SELECT RAISE(ABORT, 'template audit events are immutable');
END;

CREATE TRIGGER template_audit_prevent_delete
BEFORE DELETE ON template_audit
BEGIN
    SELECT RAISE(ABORT, 'template audit events are immutable');
END;
