DROP TRIGGER brain_configuration_requires_api_agent;
DROP TRIGGER brain_configuration_update_requires_api_agent;

CREATE TRIGGER brain_configuration_requires_supported_agent
BEFORE INSERT ON brain_configuration
WHEN NOT EXISTS (SELECT 1 FROM agents a JOIN agent_capabilities c ON c.agent_id = a.id
    WHERE a.id = NEW.agent_id AND a.runtime IN ('API', 'MCP') AND c.capability = 'brain.execute')
BEGIN
    SELECT RAISE(ABORT, 'Brain must use an API or MCP agent with brain.execute capability');
END;

CREATE TRIGGER brain_configuration_update_requires_supported_agent
BEFORE UPDATE OF agent_id ON brain_configuration
WHEN NOT EXISTS (SELECT 1 FROM agents a JOIN agent_capabilities c ON c.agent_id = a.id
    WHERE a.id = NEW.agent_id AND a.runtime IN ('API', 'MCP') AND c.capability = 'brain.execute')
BEGIN
    SELECT RAISE(ABORT, 'Brain must use an API or MCP agent with brain.execute capability');
END;

DROP TRIGGER active_brain_agent_must_remain_api_runtime;
CREATE TRIGGER active_brain_agent_must_remain_brain_runtime
BEFORE UPDATE OF runtime ON agents
WHEN NEW.runtime NOT IN ('API', 'MCP') AND EXISTS (
    SELECT 1 FROM brain_configuration bc
    JOIN brains b ON b.id = bc.brain_id
    JOIN projects p ON p.id = b.project_id
    WHERE bc.agent_id = OLD.id AND b.active = 1 AND p.brain_id = b.id
)
BEGIN
    SELECT RAISE(ABORT, 'active Brain agent must use API or MCP runtime');
END;

DROP TRIGGER projects_require_active_brain_on_insert;
DROP TRIGGER projects_require_active_brain_on_update;

CREATE TRIGGER projects_require_active_brain_on_insert
BEFORE INSERT ON projects
WHEN NEW.status IN ('READY', 'RUNNING') AND NOT EXISTS (
    SELECT 1 FROM brains b
    JOIN brain_configuration bc ON bc.brain_id = b.id
    JOIN agents a ON a.id = bc.agent_id
    WHERE b.id = NEW.brain_id AND b.project_id = NEW.id AND b.active = 1 AND a.runtime IN ('API', 'MCP')
)
BEGIN
    SELECT RAISE(ABORT, 'project requires exactly one configured active API or MCP Brain');
END;

CREATE TRIGGER projects_require_active_brain_on_update
BEFORE UPDATE OF status, brain_id ON projects
WHEN NEW.status IN ('READY', 'RUNNING') AND NOT EXISTS (
    SELECT 1 FROM brains b
    JOIN brain_configuration bc ON bc.brain_id = b.id
    JOIN agents a ON a.id = bc.agent_id
    WHERE b.id = NEW.brain_id AND b.project_id = NEW.id AND b.active = 1 AND a.runtime IN ('API', 'MCP')
)
BEGIN
    SELECT RAISE(ABORT, 'project requires exactly one configured active API or MCP Brain');
END;
