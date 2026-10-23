CREATE TABLE agent_integration_credentials (
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE RESTRICT,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
    purpose TEXT NOT NULL CHECK(purpose = 'github'),
    credential_id TEXT NOT NULL REFERENCES credentials(id) ON DELETE RESTRICT,
    PRIMARY KEY(agent_id, project_id, purpose)
);

CREATE INDEX agent_integration_credentials_credential_idx
    ON agent_integration_credentials(credential_id);
