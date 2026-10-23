CREATE TABLE user_identity_tombstones (
    user_id TEXT PRIMARY KEY CHECK(length(trim(user_id)) BETWEEN 1 AND 200),
    deleted_at TEXT NOT NULL DEFAULT (to_char(clock_timestamp() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'))
);

INSERT INTO user_identity_tombstones(user_id, deleted_at)
SELECT target_user_id, MAX(created_at)
FROM user_authority_audit_events
WHERE event_type = 'user.deleted'
GROUP BY target_user_id;

CREATE FUNCTION users_reject_retired_identity_fn() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS (SELECT 1 FROM user_identity_tombstones WHERE user_id = NEW.id) THEN
        RAISE EXCEPTION 'user identity was retired' USING ERRCODE = '23505';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER users_reject_retired_identity
BEFORE INSERT ON users
FOR EACH ROW EXECUTE FUNCTION users_reject_retired_identity_fn();

CREATE FUNCTION user_identity_tombstones_prevent_mutation_fn() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'user identity tombstones are immutable';
    RETURN OLD;
END;
$$;

CREATE TRIGGER user_identity_tombstones_prevent_update
BEFORE UPDATE ON user_identity_tombstones
FOR EACH ROW EXECUTE FUNCTION user_identity_tombstones_prevent_mutation_fn();

CREATE TRIGGER user_identity_tombstones_prevent_delete
BEFORE DELETE ON user_identity_tombstones
FOR EACH ROW EXECUTE FUNCTION user_identity_tombstones_prevent_mutation_fn();
