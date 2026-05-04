-- +goose Up
-- Rider login OTPs delivered via the rider Telegram bot.
-- The 4-digit code is hashed (sha256 + per-row salt) before storage.
CREATE TABLE IF NOT EXISTS rider_login_codes (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    phone      TEXT NOT NULL,
    code_hash  TEXT NOT NULL,
    salt       TEXT NOT NULL,
    expires_at INTEGER NOT NULL,
    attempts   INTEGER NOT NULL DEFAULT 0,
    consumed   INTEGER NOT NULL DEFAULT 0 CHECK (consumed IN (0, 1)),
    created_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);

CREATE INDEX IF NOT EXISTS idx_rider_login_codes_phone
    ON rider_login_codes(phone, consumed, expires_at);

CREATE INDEX IF NOT EXISTS idx_rider_login_codes_phone_created
    ON rider_login_codes(phone, created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_rider_login_codes_phone_created;
DROP INDEX IF EXISTS idx_rider_login_codes_phone;
DROP TABLE IF EXISTS rider_login_codes;
