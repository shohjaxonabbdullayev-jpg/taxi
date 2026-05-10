package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"taxi-mvp/internal/domain"
	"taxi-mvp/internal/repositories"
	"taxi-mvp/internal/services"

	_ "modernc.org/sqlite"
)

func setupRiderNotificationTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:rider_notif_mem?mode=memory&cache=shared")
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
		role TEXT NOT NULL,
		telegram_id INTEGER NOT NULL DEFAULT 0,
		phone TEXT
	)`)
	exec(`CREATE TABLE rider_auth_sessions (
		id                 INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id            INTEGER NOT NULL,
		refresh_hash       TEXT NOT NULL UNIQUE,
		refresh_expires_at INTEGER NOT NULL,
		revoked            INTEGER NOT NULL DEFAULT 0,
		created_at         INTEGER NOT NULL DEFAULT (strftime('%s','now'))
	)`)
	exec(`CREATE TABLE rider_login_codes (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		phone      TEXT NOT NULL,
		code_hash  TEXT NOT NULL,
		salt       TEXT NOT NULL,
		expires_at INTEGER NOT NULL,
		attempts   INTEGER NOT NULL DEFAULT 0,
		consumed   INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
	)`)
	exec(`CREATE TABLE rider_app_notifications (
		id TEXT PRIMARY KEY,
		rider_user_id INTEGER NOT NULL,
		title TEXT,
		body TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`)
	return db
}

func newRiderNotificationEngine(t *testing.T, db *sql.DB, riderUserID int64) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	codes := repositories.NewRiderLoginCodesRepo(db)
	sessions := repositories.NewRiderAuthSessionsRepo(db)
	tokens := services.NewRiderAuthTokenService(sessions, "test-rider-bot-token")
	riderAuthSvc := services.NewRiderAuthService(db, codes, tokens, fakeRiderBotHandler{}, services.RiderAuthConfig{})
	r := gin.New()
	RegisterRiderNotificationRoutes(r, RiderNotificationDeps{DB: db, RiderAuthSvc: riderAuthSvc})
	toks, err := tokens.Issue(context.Background(), riderUserID)
	if err != nil {
		t.Fatal(err)
	}
	return r, toks.AccessToken
}

func TestRiderNotifications_NoBearer_401(t *testing.T) {
	db := setupRiderNotificationTestDB(t)
	defer db.Close()
	_, err := db.Exec(`INSERT INTO users (id, role, telegram_id) VALUES (1, ?, 0)`, domain.RoleRider)
	if err != nil {
		t.Fatal(err)
	}
	r, _ := newRiderNotificationEngine(t, db, 1)

	req := httptest.NewRequest(http.MethodGet, "/v1/rider/notifications", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	code, _ := decodeRiderRequestErr(t, rr)
	if code != "invalid_token" {
		t.Fatalf("code=%q", code)
	}
}

func TestRiderNotifications_EmptyAndList(t *testing.T) {
	db := setupRiderNotificationTestDB(t)
	defer db.Close()
	_, err := db.Exec(`INSERT INTO users (id, role, telegram_id) VALUES (1, ?, 0), (2, ?, 0)`,
		domain.RoleRider, domain.RoleRider)
	if err != nil {
		t.Fatal(err)
	}
	r, token := newRiderNotificationEngine(t, db, 1)
	h := http.Header{"Authorization": []string{"Bearer " + token}}

	req := httptest.NewRequest(http.MethodGet, "/v1/rider/notifications", nil)
	req.Header = h.Clone()
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("empty list status=%d body=%s", rr.Code, rr.Body.String())
	}
	var out1 struct {
		Notifications []map[string]any `json:"notifications"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out1); err != nil {
		t.Fatal(err)
	}
	if out1.Notifications == nil || len(out1.Notifications) != 0 {
		t.Fatalf("expected empty notifications, got %#v", out1.Notifications)
	}

	_, err = db.Exec(`INSERT INTO rider_app_notifications (id, rider_user_id, title, body, created_at) VALUES
		('n-old', 1, 'T1', 'First', '2026-05-09 10:00:00'),
		('n-new', 1, '', 'Second', '2026-05-10 12:00:00'),
		('n-other', 2, '', 'Other user', '2026-05-11 12:00:00')`)
	if err != nil {
		t.Fatal(err)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/v1/rider/notifications?limit=1", nil)
	req2.Header = h.Clone()
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rr2.Code, rr2.Body.String())
	}
	var out2 struct {
		Notifications []struct {
			ID        string `json:"id"`
			Title     string `json:"title"`
			Body      string `json:"body"`
			CreatedAt string `json:"created_at"`
		} `json:"notifications"`
	}
	if err := json.Unmarshal(rr2.Body.Bytes(), &out2); err != nil {
		t.Fatal(err)
	}
	if len(out2.Notifications) != 1 {
		t.Fatalf("limit=1: got %d items", len(out2.Notifications))
	}
	if out2.Notifications[0].ID != "n-new" || out2.Notifications[0].Body != "Second" {
		t.Fatalf("unexpected first item: %+v", out2.Notifications[0])
	}
	if out2.Notifications[0].CreatedAt != "2026-05-10T12:00:00Z" {
		t.Fatalf("created_at=%q", out2.Notifications[0].CreatedAt)
	}

	req3 := httptest.NewRequest(http.MethodGet, "/v1/rider/notifications", nil)
	req3.Header = h.Clone()
	rr3 := httptest.NewRecorder()
	r.ServeHTTP(rr3, req3)
	if err := json.Unmarshal(rr3.Body.Bytes(), &out2); err != nil {
		t.Fatal(err)
	}
	if len(out2.Notifications) != 2 {
		t.Fatalf("expected 2 for user 1, got %d", len(out2.Notifications))
	}
	if out2.Notifications[0].ID != "n-new" || out2.Notifications[1].ID != "n-old" {
		t.Fatalf("order: %+v", out2.Notifications)
	}
}
