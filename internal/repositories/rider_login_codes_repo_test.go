package repositories

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func setupRiderLoginCodesDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:rider_login_codes_repo?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE IF EXISTS rider_login_codes`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TABLE rider_login_codes (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			phone      TEXT NOT NULL,
			code_hash  TEXT NOT NULL,
			salt       TEXT NOT NULL,
			expires_at INTEGER NOT NULL,
			attempts   INTEGER NOT NULL DEFAULT 0,
			consumed   INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
		)`); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestRiderLoginCodesRepo_InsertAndGetLatestActive(t *testing.T) {
	db := setupRiderLoginCodesDB(t)
	defer db.Close()
	repo := NewRiderLoginCodesRepo(db)
	ctx := context.Background()
	now := time.Now().Unix()
	exp := now + 300

	if _, err := repo.Insert(ctx, "+998901234567", "hash1", "salt1", exp); err != nil {
		t.Fatalf("Insert older: %v", err)
	}
	id2, err := repo.Insert(ctx, "+998901234567", "hash2", "salt2", exp)
	if err != nil {
		t.Fatalf("Insert newer: %v", err)
	}

	row, err := repo.GetLatestActiveByPhone(ctx, "+998901234567", now)
	if err != nil {
		t.Fatalf("GetLatestActiveByPhone: %v", err)
	}
	if row.ID != id2 {
		t.Fatalf("got id=%d want %d (latest)", row.ID, id2)
	}
	if row.CodeHash != "hash2" {
		t.Fatalf("code_hash=%q want %q", row.CodeHash, "hash2")
	}
}

func TestRiderLoginCodesRepo_GetLatestActiveSkipsExpiredAndConsumed(t *testing.T) {
	db := setupRiderLoginCodesDB(t)
	defer db.Close()
	repo := NewRiderLoginCodesRepo(db)
	ctx := context.Background()
	now := time.Now().Unix()

	if _, err := db.Exec(`INSERT INTO rider_login_codes (phone, code_hash, salt, expires_at, consumed) VALUES (?1, 'h', 's', ?2, 1)`,
		"+998900000000", now+300); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO rider_login_codes (phone, code_hash, salt, expires_at, consumed) VALUES (?1, 'h', 's', ?2, 0)`,
		"+998900000000", now-10); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.GetLatestActiveByPhone(ctx, "+998900000000", now); !errors.Is(err, ErrRiderLoginCodeNotFound) {
		t.Fatalf("want ErrRiderLoginCodeNotFound, got %v", err)
	}
}

func TestRiderLoginCodesRepo_IncAttemptsAndMarkConsumed(t *testing.T) {
	db := setupRiderLoginCodesDB(t)
	defer db.Close()
	repo := NewRiderLoginCodesRepo(db)
	ctx := context.Background()
	now := time.Now().Unix()

	id, err := repo.Insert(ctx, "+998900112233", "h", "s", now+300)
	if err != nil {
		t.Fatal(err)
	}

	for want := 1; want <= 3; want++ {
		got, err := repo.IncAttempts(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("attempt#%d returned %d want %d", want, got, want)
		}
	}

	if err := repo.MarkConsumed(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetLatestActiveByPhone(ctx, "+998900112233", now); !errors.Is(err, ErrRiderLoginCodeNotFound) {
		t.Fatalf("after consume: want ErrRiderLoginCodeNotFound got %v", err)
	}
}

func TestRiderLoginCodesRepo_PurgeExpired(t *testing.T) {
	db := setupRiderLoginCodesDB(t)
	defer db.Close()
	repo := NewRiderLoginCodesRepo(db)
	ctx := context.Background()
	now := time.Now().Unix()

	if _, err := db.Exec(`INSERT INTO rider_login_codes (phone, code_hash, salt, expires_at) VALUES (?1, 'h', 's', ?2)`,
		"+998901111111", now-100); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO rider_login_codes (phone, code_hash, salt, expires_at) VALUES (?1, 'h', 's', ?2)`,
		"+998901111111", now+200); err != nil {
		t.Fatal(err)
	}

	deleted, err := repo.PurgeExpired(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("deleted=%d want 1", deleted)
	}
}

func TestRiderLoginCodesRepo_CountSinceAndLastCreatedAt(t *testing.T) {
	db := setupRiderLoginCodesDB(t)
	defer db.Close()
	repo := NewRiderLoginCodesRepo(db)
	ctx := context.Background()

	now := time.Now().Unix()
	for i := 0; i < 3; i++ {
		if _, err := db.Exec(`INSERT INTO rider_login_codes (phone, code_hash, salt, expires_at, created_at) VALUES (?1, 'h', 's', ?2, ?3)`,
			"+998905555555", now+300, now-int64(i*5)); err != nil {
			t.Fatal(err)
		}
	}

	last, err := repo.LastCreatedAtForPhone(ctx, "+998905555555")
	if err != nil {
		t.Fatal(err)
	}
	if last != now {
		t.Fatalf("last=%d want %d", last, now)
	}

	count, err := repo.CountSinceForPhone(ctx, "+998905555555", now-7)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("count=%d want 2", count)
	}
}

func TestRiderLoginCodesRepo_InvalidateActiveForPhone(t *testing.T) {
	db := setupRiderLoginCodesDB(t)
	defer db.Close()
	repo := NewRiderLoginCodesRepo(db)
	ctx := context.Background()
	now := time.Now().Unix()

	for i := 0; i < 3; i++ {
		if _, err := repo.Insert(ctx, "+998906666666", "h", "s", now+300); err != nil {
			t.Fatal(err)
		}
	}

	if err := repo.InvalidateActiveForPhone(ctx, "+998906666666"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetLatestActiveByPhone(ctx, "+998906666666", now); !errors.Is(err, ErrRiderLoginCodeNotFound) {
		t.Fatalf("after invalidate: want ErrRiderLoginCodeNotFound got %v", err)
	}
}
