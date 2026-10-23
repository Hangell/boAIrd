CREATE TABLE users (
    id TEXT PRIMARY KEY CHECK(length(trim(id)) BETWEEN 1 AND 200),
    name TEXT NOT NULL CHECK(length(trim(name)) BETWEEN 1 AND 200),
    role TEXT NOT NULL DEFAULT 'member' CHECK(role IN ('administrator', 'member')),
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO users(id, name, role)
VALUES ('local-administrator', 'Administrator', 'administrator');
