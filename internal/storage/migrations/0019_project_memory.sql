CREATE TABLE project_memory (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
    category TEXT NOT NULL CHECK(category IN ('architecture_decision', 'project_decision', 'important_context', 'known_problem', 'current_state', 'task_history')),
    version INTEGER NOT NULL CHECK(version >= 1),
    content TEXT NOT NULL CHECK(length(trim(content)) BETWEEN 1 AND 65536),
    source_ids TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(source_ids) AND json_type(source_ids) = 'array'),
    created_by_type TEXT NOT NULL,
    created_by TEXT NOT NULL CHECK(length(trim(created_by)) > 0),
    created_at TEXT NOT NULL,
    UNIQUE(project_id, category, version)
);

CREATE INDEX project_memory_project_idx ON project_memory(project_id, category, version DESC);
