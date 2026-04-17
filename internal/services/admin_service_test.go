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

// Regression: admin Live Map loads GET .../map/ride-requests; details card should show rider phone when users.phone is set.
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
		phone TEXT
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
	exec(`INSERT INTO users (id, phone) VALUES (1, '  +998901112233  ');`)
	exec(`INSERT INTO ride_requests (id, rider_user_id, pickup_lat, pickup_lng, status, expires_at)
		VALUES ('req-with-phone', 1, 41.3, 69.2, '` + domain.RequestStatusPending + `', '` + expires + `');`)
	exec(`INSERT INTO users (id, phone) VALUES (2, NULL);`)
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
	if withPhone.RiderPhone == nil || *withPhone.RiderPhone != "+998901112233" {
		t.Fatalf("rider_phone = %v, want +998901112233", withPhone.RiderPhone)
	}
	if withoutPhone.RiderPhone != nil {
		t.Fatalf("expected nil rider_phone for null DB phone, got %q", *withoutPhone.RiderPhone)
	}

	raw, err := json.Marshal(withPhone)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"rider_phone"`) {
		t.Fatalf("JSON missing rider_phone: %s", raw)
	}
	rawNo, err := json.Marshal(withoutPhone)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rawNo), `"rider_phone"`) {
		t.Fatalf("JSON should omit rider_phone when absent: %s", rawNo)
	}
}
