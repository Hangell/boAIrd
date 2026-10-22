CREATE TABLE credentials (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL CHECK(length(trim(name)) BETWEEN 1 AND 200),
    provider TEXT NOT NULL CHECK(length(trim(provider)) BETWEEN 1 AND 100),
    secret_ciphertext BLOB NOT NULL CHECK(length(secret_ciphertext) >= 28),
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX credentials_provider_name_idx ON credentials(provider, name COLLATE NOCASE);
