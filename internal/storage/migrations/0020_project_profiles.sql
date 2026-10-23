CREATE TABLE project_profiles (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
    profile_key TEXT NOT NULL,
    instructions TEXT NOT NULL DEFAULT '',
    capabilities TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(capabilities) AND json_type(capabilities) = 'array'),
    tools TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(tools) AND json_type(tools) = 'array'),
    created_by TEXT NOT NULL,
    updated_by TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(project_id, profile_key)
);

CREATE INDEX project_profiles_project_idx ON project_profiles(project_id, profile_key);
