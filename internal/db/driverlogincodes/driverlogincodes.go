package driverlogincodes

import (
	"context"
	"database/sql"
)

// Ensure creates driver_login_codes if missing (same shape as migration 051).
// Apps do not run goose on startup; this avoids 500s when a deploy skips migrate.
func Ensure(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS driver_login_codes (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  code TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  used INTEGER NOT NULL DEFAULT 0 CHECK (used IN (0, 1)),
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
)`); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `
CREATE INDEX IF NOT EXISTS idx_driver_login_codes_user_created ON driver_login_codes(user_id, created_at)`)
	return err
}
