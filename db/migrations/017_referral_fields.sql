-- +goose Up
-- Referral system: every user has a unique code; optional inviter; bonus balance (no logic yet).
-- SQLite: cannot add UNIQUE in ALTER ADD COLUMN, so add column then create unique index.
ALTER TABLE users ADD COLUMN referral_code TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_referral_code ON users(referral_code);
ALTER TABLE users ADD COLUMN referred_by TEXT;
ALTER TABLE users ADD COLUMN referral_bonus_balance INTEGER NOT NULL DEFAULT 0;
-- +goose Down
DROP INDEX IF EXISTS idx_users_referral_code;
SELECT 1;
