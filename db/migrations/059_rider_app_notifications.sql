-- +goose Up
-- In-app / push-style messages for the native rider app (GET /v1/rider/notifications).
CREATE TABLE IF NOT EXISTS rider_app_notifications (
    id              TEXT PRIMARY KEY,
    rider_user_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title           TEXT,
    body            TEXT NOT NULL,
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_rider_app_notifications_user_created
    ON rider_app_notifications(rider_user_id, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_rider_app_notifications_user_created;
DROP TABLE IF EXISTS rider_app_notifications;
