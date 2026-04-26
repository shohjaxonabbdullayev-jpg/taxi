-- +goose Up
-- Backfill: older installs may have places without updated_at.
ALTER TABLE places ADD COLUMN updated_at TEXT;
UPDATE places SET updated_at = COALESCE(updated_at, created_at, datetime('now'));

-- +goose Down
-- libSQL/SQLite 3.35+ supports DROP COLUMN
ALTER TABLE places DROP COLUMN updated_at;
