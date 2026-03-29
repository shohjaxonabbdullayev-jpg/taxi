-- +goose Up
-- Canonical driver→driver referral edge: one inviter per referred user (referred is unique).

CREATE TABLE IF NOT EXISTS driver_referrals (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  inviter_user_id INTEGER NOT NULL REFERENCES users(id),
  referred_user_id INTEGER NOT NULL UNIQUE REFERENCES users(id),
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_driver_referrals_inviter ON driver_referrals(inviter_user_id);

INSERT OR IGNORE INTO driver_referrals (inviter_user_id, referred_user_id, created_at)
SELECT i.id, u.id, datetime('now')
FROM users u
JOIN users i ON i.referral_code = u.referred_by
WHERE u.referred_by IS NOT NULL AND TRIM(u.referred_by) != ''
  AND u.id != i.id
  AND EXISTS (SELECT 1 FROM drivers d WHERE d.user_id = u.id);

-- +goose Down
DROP INDEX IF EXISTS idx_driver_referrals_inviter;
DROP TABLE IF EXISTS driver_referrals;
