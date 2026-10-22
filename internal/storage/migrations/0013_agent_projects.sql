CREATE TABLE agent_projects (
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (agent_id, project_id)
);

CREATE INDEX agent_projects_project_idx ON agent_projects(project_id, agent_id);
