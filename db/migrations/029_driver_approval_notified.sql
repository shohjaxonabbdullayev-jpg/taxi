-- +goose Up
-- Track whether driver has already received approval notification.
ALTER TABLE drivers ADD COLUMN approval_notified INTEGER NOT NULL DEFAULT 0;

-- +goose Down
SELECT 1;
