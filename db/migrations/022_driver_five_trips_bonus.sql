-- +goose Up
-- One-time 80k so'm bonus when driver completes 5 successful trips.
ALTER TABLE drivers ADD COLUMN five_trips_bonus_paid INTEGER NOT NULL DEFAULT 0;
-- +goose Down
SELECT 1;
