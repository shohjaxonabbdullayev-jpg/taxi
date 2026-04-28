package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"taxi-mvp/internal/domain"

	_ "modernc.org/sqlite"
)

// Regression: GET .../map/ride-requests includes top-level user_id, telegram_id, rider_phone (string, may be ""), rider_name.
func TestListActiveRideRequestsForMap_IncludesRiderPhone(t *testing.T) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:admin_map_rr?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	exec := func(q string) {
		t.Helper()
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY,
		telegram_id INTEGER NOT NULL DEFAULT 0,
		phone TEXT,
		name TEXT
	);`)
	exec(`CREATE TABLE ride_requests (
		id TEXT PRIMARY KEY,
		rider_user_id INTEGER,
		pickup_lat REAL NOT NULL,
		pickup_lng REAL NOT NULL,
		status TEXT NOT NULL,
		expires_at TEXT
	);`)

	expires := time.Now().UTC().Add(time.Hour).Format("2006-01-02 15:04:05")
	exec(`INSERT INTO users (id, telegram_id, phone, name) VALUES (1, 6891986798, '  +998901112233  ', 'Ali');`)
	exec(`INSERT INTO ride_requests (id, rider_user_id, pickup_lat, pickup_lng, status, expires_at)
		VALUES ('req-with-phone', 1, 41.3, 69.2, '` + domain.RequestStatusPending + `', '` + expires + `');`)
	exec(`INSERT INTO users (id, telegram_id, phone, name) VALUES (2, 7000000001, NULL, NULL);`)
	exec(`INSERT INTO ride_requests (id, rider_user_id, pickup_lat, pickup_lng, status, expires_at)
		VALUES ('req-no-phone', 2, 41.3, 69.2, '` + domain.RequestStatusPending + `', '` + expires + `');`)

	svc := &AdminService{db: db}
	ctx := context.Background()
	out, err := svc.ListActiveRideRequestsForMap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("len(out) = %d, want 2", len(out))
	}

	var withPhone, withoutPhone *AdminMapRideRequest
	for i := range out {
		switch out[i].ID {
		case "req-with-phone":
			withPhone = &out[i]
		case "req-no-phone":
			withoutPhone = &out[i]
		}
	}
	if withPhone == nil || withoutPhone == nil {
		t.Fatalf("missing row: %#v", out)
	}
	if withPhone.UserID != 1 || withPhone.TelegramID != 6891986798 || withPhone.RiderPhone != "+998901112233" || withPhone.RiderName != "Ali" {
		t.Fatalf("withPhone = %#v", withPhone)
	}
	if withoutPhone.UserID != 2 || withoutPhone.TelegramID != 7000000001 || withoutPhone.RiderPhone != "" || withoutPhone.RiderName != "" {
		t.Fatalf("withoutPhone = %#v", withoutPhone)
	}

	raw, err := json.Marshal(withPhone)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, key := range []string{`"user_id"`, `"telegram_id"`, `"rider_phone"`, `"rider_name"`} {
		if !strings.Contains(s, key) {
			t.Fatalf("JSON missing %s: %s", key, raw)
		}
	}
	rawNo, err := json.Marshal(withoutPhone)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rawNo), `"rider_phone":""`) {
		t.Fatalf("JSON should include empty rider_phone: %s", rawNo)
	}
}

func TestListActiveRideRequestsForMap_FiltersUnconfirmedDestination(t *testing.T) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:admin_map_rr_filter?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	exec := func(q string) {
		t.Helper()
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY,
		telegram_id INTEGER NOT NULL DEFAULT 0,
		phone TEXT,
		name TEXT
	);`)
	// Include destination fields + confirmation flag to exercise strict filter.
	exec(`CREATE TABLE ride_requests (
		id TEXT PRIMARY KEY,
		rider_user_id INTEGER,
		pickup_lat REAL NOT NULL,
		pickup_lng REAL NOT NULL,
		drop_lat REAL,
		drop_lng REAL,
		estimated_price INTEGER,
		destination_confirmed INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL,
		expires_at TEXT
	);`)

	expires := time.Now().UTC().Add(time.Hour).Format("2006-01-02 15:04:05")
	exec(`INSERT INTO users (id, telegram_id, phone, name) VALUES (1, 6891986798, '+998901112233', 'Ali');`)

	// Not confirmed yet: should NOT appear on map.
	exec(`INSERT INTO ride_requests (id, rider_user_id, pickup_lat, pickup_lng, drop_lat, drop_lng, estimated_price, destination_confirmed, status, expires_at)
		VALUES ('req-unconfirmed', 1, 41.3, 69.2, 41.31, 69.21, 12000, 0, '` + domain.RequestStatusPending + `', '` + expires + `');`)

	// Confirmed: should appear.
	exec(`INSERT INTO ride_requests (id, rider_user_id, pickup_lat, pickup_lng, drop_lat, drop_lng, estimated_price, destination_confirmed, status, expires_at)
		VALUES ('req-confirmed', 1, 41.3, 69.2, 41.31, 69.21, 12000, 1, '` + domain.RequestStatusPending + `', '` + expires + `');`)

	svc := &AdminService{db: db}
	ctx := context.Background()
	out, err := svc.ListActiveRideRequestsForMap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("len(out) = %d, want 1 (%#v)", len(out), out)
	}
	if out[0].ID != "req-confirmed" {
		t.Fatalf("out[0].ID = %q, want req-confirmed", out[0].ID)
	}
}
