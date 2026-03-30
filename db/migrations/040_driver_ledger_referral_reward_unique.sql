-- +goose Up
-- One PROMO_GRANTED referral_reward row per inviter per referred driver (reference_id = referred user id).

CREATE UNIQUE INDEX IF NOT EXISTS idx_driver_ledger_referral_reward
ON driver_ledger(driver_id, reference_id)
WHERE reference_type = 'referral_reward' AND entry_type = 'PROMO_GRANTED';

-- +goose Down
DROP INDEX IF EXISTS idx_driver_ledger_referral_reward;
