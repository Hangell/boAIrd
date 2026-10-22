ALTER TABLE agents ADD COLUMN provider_base_url TEXT NOT NULL DEFAULT '';

CREATE TABLE brain_configuration (
    brain_id TEXT PRIMARY KEY REFERENCES brains(id) ON DELETE CASCADE,
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE RESTRICT,
    autonomy TEXT NOT NULL CHECK(autonomy IN ('MANUAL', 'SEMI_AUTO', 'AUTONOMOUS'))
);

CREATE TRIGGER brain_configuration_requires_api_agent
BEFORE INSERT ON brain_configuration
WHEN NOT EXISTS (SELECT 1 FROM agents a JOIN agent_capabilities c ON c.agent_id = a.id
    WHERE a.id = NEW.agent_id AND a.runtime = 'API' AND c.capability = 'brain.execute')
BEGIN
    SELECT RAISE(ABORT, 'Brain must use an API agent with brain.execute capability');
END;

CREATE TRIGGER brain_configuration_update_requires_api_agent
BEFORE UPDATE OF agent_id ON brain_configuration
WHEN NOT EXISTS (SELECT 1 FROM agents a JOIN agent_capabilities c ON c.agent_id = a.id
    WHERE a.id = NEW.agent_id AND a.runtime = 'API' AND c.capability = 'brain.execute')
BEGIN
    SELECT RAISE(ABORT, 'Brain must use an API agent with brain.execute capability');
END;

CREATE TRIGGER active_brain_agent_must_keep_execute_capability
BEFORE DELETE ON agent_capabilities
WHEN OLD.capability = 'brain.execute' AND EXISTS (
    SELECT 1 FROM brain_configuration bc
    JOIN brains b ON b.id = bc.brain_id
    JOIN projects p ON p.id = b.project_id
    WHERE bc.agent_id = OLD.agent_id AND b.active = 1 AND p.brain_id = b.id
)
BEGIN
    SELECT RAISE(ABORT, 'active Brain agent must retain brain.execute capability');
END;

CREATE TRIGGER active_brain_agent_must_remain_api_runtime
BEFORE UPDATE OF runtime ON agents
WHEN NEW.runtime <> 'API' AND EXISTS (
    SELECT 1 FROM brain_configuration bc
    JOIN brains b ON b.id = bc.brain_id
    JOIN projects p ON p.id = b.project_id
    WHERE bc.agent_id = OLD.id AND b.active = 1 AND p.brain_id = b.id
)
BEGIN
    SELECT RAISE(ABORT, 'active Brain agent must use API runtime');
END;

CREATE TABLE runs (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
    task_id TEXT,
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE RESTRICT,
    status TEXT NOT NULL CHECK(status IN ('PENDING', 'RUNNING', 'SUCCESS', 'FAILED', 'CANCELLED')),
    started_at TEXT,
    finished_at TEXT,
    runtime TEXT NOT NULL,
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    input TEXT NOT NULL,
    output TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX runs_project_created_idx ON runs(project_id, created_at DESC, id);

DROP TRIGGER projects_require_active_brain_on_insert;
DROP TRIGGER projects_require_active_brain_on_update;

CREATE TRIGGER projects_require_active_brain_on_insert
BEFORE INSERT ON projects
WHEN NEW.status IN ('READY', 'RUNNING') AND NOT EXISTS (
    SELECT 1 FROM brains b
    JOIN brain_configuration bc ON bc.brain_id = b.id
    JOIN agents a ON a.id = bc.agent_id
    WHERE b.id = NEW.brain_id AND b.project_id = NEW.id AND b.active = 1 AND a.runtime = 'API'
)
BEGIN
    SELECT RAISE(ABORT, 'project requires exactly one configured active API Brain');
END;

CREATE TRIGGER projects_require_active_brain_on_update
BEFORE UPDATE OF status, brain_id ON projects
WHEN NEW.status IN ('READY', 'RUNNING') AND NOT EXISTS (
    SELECT 1 FROM brains b
    JOIN brain_configuration bc ON bc.brain_id = b.id
    JOIN agents a ON a.id = bc.agent_id
    WHERE b.id = NEW.brain_id AND b.project_id = NEW.id AND b.active = 1 AND a.runtime = 'API'
)
BEGIN
    SELECT RAISE(ABORT, 'project requires exactly one configured active API Brain');
END;
