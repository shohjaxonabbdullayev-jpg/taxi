// Package riderappnotifications ensures the rider_app_notifications table exists
// (native rider app GET /v1/rider/notifications). Production Turso DBs may not
// run goose for every new migration; mirroring driverapprepair / riderlogincodes.
package riderappnotifications

import (
	"context"
	"database/sql"
	"fmt"
)

// Ensure creates rider_app_notifications and its index when missing.
func Ensure(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("riderappnotifications: nil db")
	}
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS rider_app_notifications (
			id              TEXT PRIMARY KEY,
			rider_user_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			title           TEXT,
			body            TEXT NOT NULL,
			created_at      TEXT NOT NULL DEFAULT (datetime('now'))
		)`); err != nil {
		return fmt.Errorf("riderappnotifications: create table: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
		CREATE INDEX IF NOT EXISTS idx_rider_app_notifications_user_created
			ON rider_app_notifications(rider_user_id, created_at DESC)`); err != nil {
		return fmt.Errorf("riderappnotifications: create index: %w", err)
	}
	return nil
}
