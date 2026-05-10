package riderappnotifications

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestEnsure_Idempotent(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if err := Ensure(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := Ensure(ctx, db); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='rider_app_notifications'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("table missing, n=%d", n)
	}
}
