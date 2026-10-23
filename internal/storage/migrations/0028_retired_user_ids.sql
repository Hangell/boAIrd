CREATE TABLE user_identity_tombstones (
    user_id TEXT PRIMARY KEY CHECK(length(trim(user_id)) BETWEEN 1 AND 200),
    deleted_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO user_identity_tombstones(user_id, deleted_at)
SELECT target_user_id, MAX(created_at)
FROM user_authority_audit_events
WHERE event_type = 'user.deleted'
GROUP BY target_user_id;

CREATE TRIGGER users_reject_retired_identity
BEFORE INSERT ON users
WHEN EXISTS (SELECT 1 FROM user_identity_tombstones WHERE user_id = NEW.id)
BEGIN
    SELECT RAISE(ABORT, 'user identity was retired');
END;

CREATE TRIGGER user_identity_tombstones_prevent_update
BEFORE UPDATE ON user_identity_tombstones
BEGIN
    SELECT RAISE(ABORT, 'user identity tombstones are immutable');
END;

CREATE TRIGGER user_identity_tombstones_prevent_delete
BEFORE DELETE ON user_identity_tombstones
BEGIN
    SELECT RAISE(ABORT, 'user identity tombstones are immutable');
END;
