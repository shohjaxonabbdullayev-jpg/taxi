-- +goose Up
-- Idempotent first-3-trip promo: one PROMO_GRANTED row per (driver, trip) for this program.

CREATE UNIQUE INDEX IF NOT EXISTS idx_driver_ledger_first3_trip
ON driver_ledger(driver_id, reference_id)
WHERE reference_type = 'first_3_trip_bonus' AND entry_type = 'PROMO_GRANTED';

-- +goose Down
DROP INDEX IF EXISTS idx_driver_ledger_first3_trip;
