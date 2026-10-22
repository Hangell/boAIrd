CREATE TABLE project_members (
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL CHECK(length(trim(user_id)) BETWEEN 1 AND 200),
    role TEXT NOT NULL CHECK(role IN ('ADMIN', 'MEMBER')),
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY(project_id, user_id)
);

CREATE INDEX project_members_user_idx ON project_members(user_id, project_id);

INSERT INTO project_members(project_id, user_id, role)
SELECT id, 'local-administrator', 'ADMIN' FROM projects;

CREATE TRIGGER projects_add_local_administrator_membership
AFTER INSERT ON projects
BEGIN
    INSERT INTO project_members(project_id, user_id, role)
    VALUES (NEW.id, 'local-administrator', 'ADMIN');
END;
