-- +goose Up
-- When we last successfully sent the rider "arrived at pickup" Telegram (cooldown for repeat taps).
ALTER TABLE trips ADD COLUMN arrived_rider_notified_at TEXT NULL;
-- +goose Down
SELECT 1;
