CREATE TABLE projects (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL CHECK(length(trim(name)) BETWEEN 1 AND 200),
    description TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK(status IN ('CREATED', 'CONFIGURING', 'READY', 'RUNNING', 'PAUSED', 'ARCHIVED')),
    brain_id TEXT,
    workspace_path TEXT NOT NULL DEFAULT '',
    repository_url TEXT NOT NULL DEFAULT '',
    human_review_required INTEGER NOT NULL CHECK(human_review_required IN (0, 1)),
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK(status NOT IN ('READY', 'RUNNING') OR brain_id IS NOT NULL)
);

CREATE INDEX projects_status_name_idx ON projects(status, name COLLATE NOCASE);
