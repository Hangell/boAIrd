CREATE TABLE brains (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
    active INTEGER NOT NULL CHECK(active IN (0, 1)),
    UNIQUE(id, project_id)
);

CREATE UNIQUE INDEX brains_one_active_per_project_idx ON brains(project_id) WHERE active = 1;

CREATE TRIGGER projects_require_active_brain_on_insert
BEFORE INSERT ON projects
WHEN NEW.status IN ('READY', 'RUNNING') AND NOT EXISTS (
    SELECT 1 FROM brains WHERE id = NEW.brain_id AND project_id = NEW.id AND active = 1
)
BEGIN
    SELECT RAISE(ABORT, 'project requires exactly one active Brain');
END;

CREATE TRIGGER projects_require_active_brain_on_update
BEFORE UPDATE OF status, brain_id ON projects
WHEN NEW.status IN ('READY', 'RUNNING') AND NOT EXISTS (
    SELECT 1 FROM brains WHERE id = NEW.brain_id AND project_id = NEW.id AND active = 1
)
BEGIN
    SELECT RAISE(ABORT, 'project requires exactly one active Brain');
END;

CREATE TRIGGER brains_prevent_deactivating_project_brain
BEFORE UPDATE OF active ON brains
WHEN OLD.active = 1 AND NEW.active = 0 AND EXISTS (
    SELECT 1 FROM projects WHERE id = OLD.project_id AND brain_id = OLD.id AND status IN ('READY', 'RUNNING')
)
BEGIN
    SELECT RAISE(ABORT, 'cannot deactivate the Brain of an executable project');
END;

CREATE TRIGGER brains_prevent_deleting_project_brain
BEFORE DELETE ON brains
WHEN EXISTS (
    SELECT 1 FROM projects WHERE id = OLD.project_id AND brain_id = OLD.id
)
BEGIN
    SELECT RAISE(ABORT, 'cannot delete the Brain associated with a project');
END;
