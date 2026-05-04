package riderlogincodes

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T, name string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+name+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestEnsure_FreshDB_CreatesBothTables(t *testing.T) {
	db := openTestDB(t, "rlc_fresh")
	defer db.Close()

	if err := Ensure(context.Background(), db); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	for _, table := range []string{"rider_login_codes", "rider_auth_sessions"} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatalf("table %s missing", table)
		}
	}

	if _, err := db.Exec(`INSERT INTO rider_login_codes (phone, code_hash, salt, expires_at) VALUES ('+998901111111','h','s',9999999999)`); err != nil {
		t.Fatalf("insert into fresh table: %v", err)
	}
}

// TestEnsure_LegacySchema_GetsRebuilt simulates the production scenario:
// a previous deploy left a rider_login_codes table with a different shape
// (e.g. user_id-based, like driver_login_codes). Ensure() must detect the
// drift and rebuild the table; the index creation must then succeed.
func TestEnsure_LegacySchema_GetsRebuilt(t *testing.T) {
	db := openTestDB(t, "rlc_legacy_codes")
	defer db.Close()

	// Pre-create the legacy shape that broke the deploy.
	if _, err := db.Exec(`
		CREATE TABLE rider_login_codes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			code TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			used INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO rider_login_codes (user_id, code, expires_at) VALUES (1, '0000', '2099-01-01 00:00:00')`); err != nil {
		t.Fatal(err)
	}

	if err := Ensure(context.Background(), db); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	// Brand-new schema must be present.
	rows, err := db.Query(`SELECT name FROM pragma_table_info('rider_login_codes')`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]bool{}
	for rows.Next() {
		var c string
		_ = rows.Scan(&c)
		got[c] = true
	}
	for _, want := range []string{"phone", "code_hash", "salt", "expires_at", "attempts", "consumed", "created_at"} {
		if !got[want] {
			t.Fatalf("rebuilt table missing column %q (have %v)", want, got)
		}
	}
	// The index that was failing in production must now exist and be usable.
	if _, err := db.Exec(`INSERT INTO rider_login_codes (phone, code_hash, salt, expires_at) VALUES ('+998902222222','h','s',9999999999)`); err != nil {
		t.Fatalf("insert into rebuilt table: %v", err)
	}
}

// TestEnsure_LegacyAuthSessions_GetRebuilt covers the same drift case for the
// rider_auth_sessions companion table.
func TestEnsure_LegacyAuthSessions_GetRebuilt(t *testing.T) {
	db := openTestDB(t, "rlc_legacy_sessions")
	defer db.Close()

	if _, err := db.Exec(`
		CREATE TABLE rider_auth_sessions (
			id INTEGER PRIMARY KEY,
			user_id INTEGER NOT NULL,
			access_token TEXT,
			expires_at TEXT
		)`); err != nil {
		t.Fatal(err)
	}

	if err := Ensure(context.Background(), db); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	rows, err := db.Query(`SELECT name FROM pragma_table_info('rider_auth_sessions')`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]bool{}
	for rows.Next() {
		var c string
		_ = rows.Scan(&c)
		got[c] = true
	}
	for _, want := range []string{"user_id", "refresh_hash", "refresh_expires_at", "revoked", "created_at"} {
		if !got[want] {
			t.Fatalf("rebuilt sessions table missing column %q (have %v)", want, got)
		}
	}
}

// TestEnsure_AlreadyCorrect_IsIdempotent guards against accidentally dropping
// data on a healthy DB.
func TestEnsure_AlreadyCorrect_IsIdempotent(t *testing.T) {
	db := openTestDB(t, "rlc_idempotent")
	defer db.Close()

	if err := Ensure(context.Background(), db); err != nil {
		t.Fatalf("first Ensure: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO rider_login_codes (phone, code_hash, salt, expires_at) VALUES ('+998903333333','h','s',9999999999)`); err != nil {
		t.Fatal(err)
	}

	if err := Ensure(context.Background(), db); err != nil {
		t.Fatalf("second Ensure: %v", err)
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM rider_login_codes WHERE phone='+998903333333'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("row count after second Ensure = %d, want 1 (must not drop a healthy table)", n)
	}
}
