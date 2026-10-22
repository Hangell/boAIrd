CREATE TABLE conversations (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
    task_id TEXT NOT NULL UNIQUE REFERENCES tasks(id) ON DELETE RESTRICT,
    created_at TEXT NOT NULL
);

CREATE INDEX conversations_project_idx ON conversations(project_id, created_at DESC, id);

CREATE TRIGGER conversations_validate_task_project
BEFORE INSERT ON conversations
WHEN NOT EXISTS (SELECT 1 FROM tasks WHERE id = NEW.task_id AND project_id = NEW.project_id)
BEGIN
    SELECT RAISE(ABORT, 'conversation project must match its task project');
END;

CREATE TRIGGER tasks_create_conversation
AFTER INSERT ON tasks
BEGIN
    INSERT INTO conversations(id, project_id, task_id, created_at)
    VALUES(lower(hex(randomblob(16))), NEW.project_id, NEW.id, NEW.created_at);
END;

INSERT INTO conversations(id, project_id, task_id, created_at)
SELECT lower(hex(randomblob(16))), project_id, id, created_at FROM tasks WHERE 1
ON CONFLICT(task_id) DO NOTHING;

CREATE TABLE conversation_messages (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE RESTRICT,
    sender_type TEXT NOT NULL CHECK(sender_type IN ('USER', 'AGENT', 'BRAIN', 'SYSTEM')),
    sender_id TEXT NOT NULL CHECK(length(trim(sender_id)) > 0),
    content TEXT NOT NULL CHECK(length(trim(content)) > 0 AND length(content) <= 1048576),
    content_type TEXT NOT NULL CHECK(length(trim(content_type)) BETWEEN 1 AND 128),
    metadata TEXT NOT NULL CHECK(json_valid(metadata) AND json_type(metadata) = 'object' AND length(metadata) <= 65536),
    created_at TEXT NOT NULL
);

CREATE INDEX conversation_messages_history_idx ON conversation_messages(conversation_id, created_at, id);
