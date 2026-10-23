CREATE TABLE project_memory_versions (
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    category TEXT NOT NULL CHECK(category IN ('architecture_decision', 'project_decision', 'important_context', 'known_problem', 'current_state', 'task_history')),
    version INTEGER NOT NULL CHECK (version > 0),
    PRIMARY KEY (project_id, category)
);

INSERT INTO project_memory_versions(project_id, category, version)
SELECT project_id, category, MAX(version)
FROM project_memory
GROUP BY project_id, category;
