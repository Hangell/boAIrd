CREATE TABLE agents (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL CHECK(length(trim(name)) BETWEEN 1 AND 200),
    avatar TEXT NOT NULL DEFAULT '' CHECK(length(avatar) <= 1000),
    description TEXT NOT NULL DEFAULT '' CHECK(length(description) <= 4000),
    role TEXT NOT NULL CHECK(length(trim(role)) BETWEEN 1 AND 100),
    runtime TEXT NOT NULL CHECK(runtime IN ('API', 'MCP', 'LOCAL', 'REMOTE')),
    provider TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '',
    credential_id TEXT REFERENCES credentials(id) ON DELETE RESTRICT,
    instructions TEXT NOT NULL DEFAULT '' CHECK(length(instructions) <= 16000),
    status TEXT NOT NULL DEFAULT 'IDLE' CHECK(status IN ('ONLINE', 'IDLE', 'BUSY', 'WAITING', 'OFFLINE', 'STALE', 'ERROR')),
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK(
        (runtime = 'API' AND length(trim(provider)) > 0 AND length(trim(model)) > 0 AND credential_id IS NOT NULL)
        OR
        (runtime <> 'API' AND provider = '' AND model = '' AND credential_id IS NULL)
    )
);

CREATE INDEX agents_name_idx ON agents(name COLLATE NOCASE, id);

CREATE TRIGGER agents_validate_credential_insert
BEFORE INSERT ON agents
WHEN NEW.runtime = 'API' AND NOT EXISTS (
    SELECT 1 FROM credentials WHERE id = NEW.credential_id AND lower(provider) = NEW.provider
)
BEGIN
    SELECT RAISE(ABORT, 'agent credential does not match configured provider');
END;

CREATE TRIGGER agents_validate_credential_update
BEFORE UPDATE OF runtime, provider, credential_id ON agents
WHEN NEW.runtime = 'API' AND NOT EXISTS (
    SELECT 1 FROM credentials WHERE id = NEW.credential_id AND lower(provider) = NEW.provider
)
BEGIN
    SELECT RAISE(ABORT, 'agent credential does not match configured provider');
END;

CREATE TABLE agent_languages (
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    language TEXT NOT NULL CHECK(length(trim(language)) BETWEEN 1 AND 100),
    PRIMARY KEY(agent_id, language)
);

CREATE TABLE agent_capabilities (
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    capability TEXT NOT NULL CHECK(length(trim(capability)) BETWEEN 1 AND 200),
    PRIMARY KEY(agent_id, capability)
);
