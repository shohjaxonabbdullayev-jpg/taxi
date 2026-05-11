package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"taxi-mvp/internal/auth"
	"taxi-mvp/internal/domain"

	_ "modernc.org/sqlite"
)

func setupDriverTripsTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:driver_trips?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	exec := func(q string) {
		t.Helper()
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY,
		telegram_id INTEGER NOT NULL DEFAULT 0,
		role TEXT NOT NULL DEFAULT 'driver'
	);`)
	exec(`CREATE TABLE trips (
		id TEXT PRIMARY KEY,
		request_id TEXT NOT NULL,
		driver_user_id INTEGER NOT NULL,
		rider_user_id INTEGER NOT NULL,
		status TEXT NOT NULL,
		started_at TEXT,
		finished_at TEXT,
		cancelled_at TEXT,
		fare_amount INTEGER NOT NULL DEFAULT 0
	);`)
	exec(`CREATE TABLE payments (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		driver_id INTEGER NOT NULL,
		amount INTEGER NOT NULL,
		type TEXT NOT NULL,
		note TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		trip_id TEXT
	);`)
	return db
}

// TestDriverTrips_JSONContract locks GET /driver/trips response shape for the Flutter driver app.
func TestDriverTrips_JSONContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupDriverTripsTestDB(t)
	defer db.Close()

	const driverID int64 = 7
	_, _ = db.Exec(`INSERT INTO users (id, telegram_id, role) VALUES (7, 700, 'driver'), (8, 800, 'rider')`)
	_, _ = db.Exec(`INSERT INTO trips (id, request_id, driver_user_id, rider_user_id, status, started_at, finished_at, cancelled_at, fare_amount) VALUES
		('trip-finish-1', 'req-1', 7, 8, 'FINISHED', '2026-03-10 10:00:00', '2026-03-10 10:45:00', NULL, 25000),
		('trip-cancel-1', 'req-2', 7, 8, 'CANCELLED_BY_RIDER', '2026-03-09 09:00:00', NULL, '2026-03-09 09:05:00', 0)`)
	_, _ = db.Exec(`INSERT INTO payments (driver_id, amount, type, note, trip_id) VALUES (7, -2500, 'commission', 'trip', 'trip-finish-1')`)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/driver/trips?limit=10&offset=0", nil)
	req = req.WithContext(context.Background())
	c.Request = req
	injectDriverContext(c, driverID)

	DriverTrips(db)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var body struct {
		Trips []map[string]any `json:"trips"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("json: %v body=%s", err, w.Body.String())
	}
	if len(body.Trips) != 2 {
		t.Fatalf("len(trips) = %d, want 2", len(body.Trips))
	}
	// Newest first: FINISHED has later finished_at than cancel's cancelled_at
	first := body.Trips[0]
	tid, _ := first["trip_id"].(string)
	if tid != "trip-finish-1" {
		t.Fatalf("first trip_id = %q want trip-finish-1", tid)
	}
	if first["status"] != "FINISHED" {
		t.Fatalf("status = %v", first["status"])
	}
	if fv, ok := first["fare_som"].(float64); !ok || fv != 25000 {
		t.Fatalf("fare_som = %v (%T)", first["fare_som"], first["fare_som"])
	}
	if cv, ok := first["commission_som"].(float64); !ok || cv != 2500 {
		t.Fatalf("commission_som = %v (%T)", first["commission_som"], first["commission_som"])
	}
	finishedAt, _ := first["finished_at"].(string)
	if finishedAt == "" || finishedAt[0] != '2' {
		t.Fatalf("finished_at RFC3339 missing or bad: %q", finishedAt)
	}
}

func TestDriverTrips_FallbackWithoutPaymentTripIDColumn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := sql.Open("sqlite", "file:driver_trips_legacy?mode=memory&cache=shared")
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
	exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, telegram_id INTEGER NOT NULL DEFAULT 0, role TEXT NOT NULL DEFAULT 'driver');`)
	exec(`CREATE TABLE trips (
		id TEXT PRIMARY KEY,
		request_id TEXT NOT NULL,
		driver_user_id INTEGER NOT NULL,
		rider_user_id INTEGER NOT NULL,
		status TEXT NOT NULL,
		started_at TEXT,
		finished_at TEXT,
		cancelled_at TEXT,
		fare_amount INTEGER NOT NULL DEFAULT 0
	);`)
	exec(`CREATE TABLE payments (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		driver_id INTEGER NOT NULL,
		amount INTEGER NOT NULL,
		type TEXT NOT NULL,
		note TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`)
	_, _ = db.Exec(`INSERT INTO users (id) VALUES (1)`)
	_, _ = db.Exec(`INSERT INTO trips (id, request_id, driver_user_id, rider_user_id, status, finished_at, fare_amount) VALUES
		('t1', 'r1', 1, 2, 'FINISHED', '2026-01-02 12:00:00', 1000)`)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/driver/trips", nil)
	c.Request = req
	c.Request = c.Request.WithContext(auth.WithUser(c.Request.Context(), &auth.User{UserID: 1, Role: domain.RoleDriver}))

	DriverTrips(db)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"trips"`) {
		t.Fatalf("expected trips key: %s", w.Body.String())
	}
}
