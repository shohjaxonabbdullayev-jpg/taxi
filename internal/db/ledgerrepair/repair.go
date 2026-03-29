// Package ledgerrepair fixes common driver_ledger schema drift (e.g. user_id vs driver_id).
package ledgerrepair

import (
	"context"
	"database/sql"
	"fmt"
	"log"
)

// Ensure aligns driver_ledger with application code when the table exists but uses legacy column names.
func Ensure(ctx context.Context, db *sql.DB) error {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='driver_ledger'`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return nil
	}
	rows, err := db.QueryContext(ctx, `SELECT name FROM pragma_table_info('driver_ledger')`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var hasDriverID, hasUserID bool
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		switch name {
		case "driver_id":
			hasDriverID = true
		case "user_id":
			hasUserID = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if hasDriverID {
		return nil
	}
	if hasUserID {
		log.Printf("ledgerrepair: driver_ledger column user_id → driver_id (legacy schema)")
		if _, err := db.ExecContext(ctx, `ALTER TABLE driver_ledger RENAME COLUMN user_id TO driver_id`); err != nil {
			return fmt.Errorf("ledgerrepair: rename user_id to driver_id: %w", err)
		}
		return nil
	}
	return fmt.Errorf("ledgerrepair: driver_ledger has no driver_id column; apply migration 035 or fix schema manually")
}
