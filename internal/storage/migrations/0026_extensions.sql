CREATE TABLE extensions (
    name TEXT PRIMARY KEY,
    version TEXT NOT NULL,
    manifest TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('pending', 'active')),
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE extension_audit (
    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_id TEXT NOT NULL,
    extension_name TEXT NOT NULL,
    event_type TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
