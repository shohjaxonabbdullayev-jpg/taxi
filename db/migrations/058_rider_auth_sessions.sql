-- +goose Up
-- Refresh-token storage for the rider native auth flow.
-- Access tokens are stateless HS256 (validated without DB lookup); only
-- refresh tokens are persisted so that /v1/rider/auth/logout can revoke
-- them and /v1/rider/auth/refresh can validate them.
CREATE TABLE IF NOT EXISTS rider_auth_sessions (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id           INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    refresh_hash      TEXT NOT NULL UNIQUE,
    refresh_expires_at INTEGER NOT NULL,
    revoked           INTEGER NOT NULL DEFAULT 0 CHECK (revoked IN (0, 1)),
    created_at        INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);

CREATE INDEX IF NOT EXISTS idx_rider_auth_sessions_user
    ON rider_auth_sessions(user_id, revoked, refresh_expires_at);

-- +goose Down
DROP INDEX IF EXISTS idx_rider_auth_sessions_user;
DROP TABLE IF EXISTS rider_auth_sessions;
