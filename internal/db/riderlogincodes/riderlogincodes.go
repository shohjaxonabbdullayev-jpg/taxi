package riderlogincodes

import (
	"context"
	"database/sql"
)

// Ensure creates rider_login_codes and rider_auth_sessions if missing.
// Mirrors the legalrepair / ledgerrepair / driverlogincodes startup pattern so
// that a deploy which skipped goose still has the rider native-auth tables.
//
// The schema here is kept in sync with migrations 057 and 058.
func Ensure(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS rider_login_codes (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			phone      TEXT NOT NULL,
			code_hash  TEXT NOT NULL,
			salt       TEXT NOT NULL,
			expires_at INTEGER NOT NULL,
			attempts   INTEGER NOT NULL DEFAULT 0,
			consumed   INTEGER NOT NULL DEFAULT 0 CHECK (consumed IN (0, 1)),
			created_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_rider_login_codes_phone
			ON rider_login_codes(phone, consumed, expires_at)`,
		`CREATE INDEX IF NOT EXISTS idx_rider_login_codes_phone_created
			ON rider_login_codes(phone, created_at)`,
		`CREATE TABLE IF NOT EXISTS rider_auth_sessions (
			id                 INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id            INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			refresh_hash       TEXT NOT NULL UNIQUE,
			refresh_expires_at INTEGER NOT NULL,
			revoked            INTEGER NOT NULL DEFAULT 0 CHECK (revoked IN (0, 1)),
			created_at         INTEGER NOT NULL DEFAULT (strftime('%s','now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_rider_auth_sessions_user
			ON rider_auth_sessions(user_id, revoked, refresh_expires_at)`,
	}
	for _, q := range stmts {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}
