-- +goose Up
-- Admin-published broadcasts for rider app (GET /v1/rider/notifications) and Telegram fan-out.
CREATE TABLE IF NOT EXISTS broadcast_posts (
    id                       TEXT PRIMARY KEY,
    title                    TEXT,
    body                     TEXT NOT NULL,
    created_at               TEXT NOT NULL DEFAULT (datetime('now')),
    status                   TEXT NOT NULL DEFAULT 'published' CHECK (status IN ('draft','published')),
    created_by_telegram_id   INTEGER NOT NULL DEFAULT 0,
    audience                 TEXT NOT NULL DEFAULT 'all_riders',
    cloudinary_public_id     TEXT,
    cloudinary_secure_url    TEXT,
    media_type               TEXT, -- image | video | raw (Cloudinary resource_type)
    width                    INTEGER,
    height                   INTEGER,
    format                   TEXT
);

CREATE INDEX IF NOT EXISTS idx_broadcast_posts_created
    ON broadcast_posts(created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS broadcast_telegram_deliveries (
    broadcast_id  TEXT NOT NULL REFERENCES broadcast_posts(id) ON DELETE CASCADE,
    chat_id       INTEGER NOT NULL,
    delivered_at  TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (broadcast_id, chat_id)
);

-- +goose Down
DROP TABLE IF EXISTS broadcast_telegram_deliveries;
DROP INDEX IF EXISTS idx_broadcast_posts_created;
DROP TABLE IF EXISTS broadcast_posts;

