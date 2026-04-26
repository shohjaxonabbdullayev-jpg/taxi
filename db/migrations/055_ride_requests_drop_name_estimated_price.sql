-- +goose Up
-- Backfill: some DBs may miss destination estimate columns on ride_requests.

ALTER TABLE ride_requests ADD COLUMN drop_name TEXT;
ALTER TABLE ride_requests ADD COLUMN estimated_price INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- libSQL/SQLite 3.35+ supports DROP COLUMN
ALTER TABLE ride_requests DROP COLUMN drop_name;
ALTER TABLE ride_requests DROP COLUMN estimated_price;

