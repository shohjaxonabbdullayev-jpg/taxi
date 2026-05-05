package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"taxi-mvp/internal/config"
	"taxi-mvp/internal/domain"
	"taxi-mvp/internal/repositories"
	"taxi-mvp/internal/services"

	_ "modernc.org/sqlite"
)

func setupRiderRequestHandlerDB(t *testing.T, name string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+name+"?mode=memory&cache=shared")
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
	exec(`CREATE TABLE legal_documents (
		document_type TEXT NOT NULL,
		version INTEGER NOT NULL,
		is_active INTEGER NOT NULL DEFAULT 1,
		content TEXT
	)`)
	exec(`CREATE TABLE legal_acceptances (
		user_id INTEGER NOT NULL,
		document_type TEXT NOT NULL,
		version INTEGER NOT NULL,
		PRIMARY KEY (user_id, document_type)
	)`)
	exec(`CREATE TABLE ride_requests (
		id TEXT PRIMARY KEY,
		rider_user_id INTEGER NOT NULL,
		pickup_lat REAL NOT NULL,
		pickup_lng REAL NOT NULL,
		radius_km REAL NOT NULL,
		status TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		expires_at TEXT NOT NULL,
		assigned_driver_user_id INTEGER,
		assigned_at TEXT,
		drop_lat REAL,
		drop_lng REAL,
		drop_name TEXT,
		estimated_price INTEGER NOT NULL DEFAULT 0,
		destination_confirmed INTEGER NOT NULL DEFAULT 0,
		pickup_grid TEXT,
		radius_expanded_at TEXT
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
	exec(`CREATE TABLE rider_auth_sessions (
		id                 INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id            INTEGER NOT NULL,
		refresh_hash       TEXT NOT NULL UNIQUE,
		refresh_expires_at INTEGER NOT NULL,
		revoked            INTEGER NOT NULL DEFAULT 0,
		created_at         INTEGER NOT NULL DEFAULT (strftime('%s','now'))
	)`)
	return db
}

func seedRiderLegalAndUser(t *testing.T, db *sql.DB, userID int64) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO legal_documents (document_type, version, is_active, content) VALUES
		('user_terms', 1, 1, 'x'),
		('privacy_policy_user', 1, 1, 'y')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO legal_acceptances (user_id, document_type, version) VALUES
		(?1, 'user_terms', 1), (?1, 'privacy_policy_user', 1)`, userID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO users (id, role, telegram_id, phone) VALUES (?1, ?2, 999001, '+998901112233')`,
		userID, domain.RoleRider)
	if err != nil {
		t.Fatal(err)
	}
}

func newRiderRequestTestEngine(t *testing.T, db *sql.DB) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		RequestExpiresSeconds: 3600,
		MatchRadiusKm:         3,
		StartingFee:           4000,
		PricePerKm:            1500,
		InfiniteDriverBalance: true,
	}
	codes := repositories.NewRiderLoginCodesRepo(db)
	sessions := repositories.NewRiderAuthSessionsRepo(db)
	tokens := services.NewRiderAuthTokenService(sessions, "test-rider-bot-token")
	riderAuthSvc := services.NewRiderAuthService(db, codes, tokens, fakeRiderBotHandler{}, services.RiderAuthConfig{})
	matchSvc := services.NewMatchService(db, (*tgbotapi.BotAPI)(nil), cfg)
	riderReqSvc := services.NewRiderRequestAppService(db, cfg, matchSvc)
	r := gin.New()
	RegisterRiderRequestRoutes(r, RiderRequestDeps{
		DB: db, Cfg: cfg, RiderAuthSvc: riderAuthSvc, RiderReqSvc: riderReqSvc,
	})
	toks, err := tokens.Issue(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	return r, toks.AccessToken
}

func TestRiderRequest_NoBearer_401(t *testing.T) {
	db := setupRiderRequestHandlerDB(t, "rider_req_no_bearer")
	defer db.Close()
	seedRiderLegalAndUser(t, db, 1)
	r, _ := newRiderRequestTestEngine(t, db)

	rr := postJSON(r, "/v1/rider/requests", map[string]any{
		"pickup_lat": 41.3, "pickup_lng": 69.28,
	}, nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	code, _ := decodeRiderRequestErr(t, rr)
	if code != "invalid_token" {
		t.Fatalf("code=%q body=%s", code, rr.Body.String())
	}
}

func decodeRiderRequestErr(t *testing.T, rr *httptest.ResponseRecorder) (code, msg string) {
	t.Helper()
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v body=%s", err, rr.Body.String())
	}
	return env.Error.Code, env.Error.Message
}

func TestRiderRequest_DuplicatePending_409(t *testing.T) {
	db := setupRiderRequestHandlerDB(t, "rider_req_dup")
	defer db.Close()
	seedRiderLegalAndUser(t, db, 1)
	r, token := newRiderRequestTestEngine(t, db)
	h := map[string]string{"Authorization": "Bearer " + token}

	rr1 := postJSON(r, "/v1/rider/requests", map[string]any{"pickup_lat": 41.3, "pickup_lng": 69.28}, h)
	if rr1.Code != http.StatusOK {
		t.Fatalf("first create status=%d body=%s", rr1.Code, rr1.Body.String())
	}
	rr2 := postJSON(r, "/v1/rider/requests", map[string]any{"pickup_lat": 41.31, "pickup_lng": 69.29}, h)
	if rr2.Code != http.StatusConflict {
		t.Fatalf("second create status=%d body=%s", rr2.Code, rr2.Body.String())
	}
	code, _ := decodeRiderRequestErr(t, rr2)
	if code != "duplicate_pending" {
		t.Fatalf("code=%q body=%s", code, rr2.Body.String())
	}
}

func TestRiderRequest_HappyPath_CreateDestinationConfirm(t *testing.T) {
	db := setupRiderRequestHandlerDB(t, "rider_req_happy")
	defer db.Close()
	seedRiderLegalAndUser(t, db, 1)
	r, token := newRiderRequestTestEngine(t, db)
	h := map[string]string{"Authorization": "Bearer " + token}

	rr := postJSON(r, "/v1/rider/requests", map[string]any{"pickup_lat": 41.3, "pickup_lng": 69.28}, h)
	if rr.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", rr.Code, rr.Body.String())
	}
	var createOut struct {
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &createOut); err != nil || createOut.RequestID == "" {
		t.Fatalf("decode create: %v body=%s", err, rr.Body.String())
	}

	path := "/v1/rider/requests/" + createOut.RequestID + "/destination"
	rr2 := postJSON(r, path, map[string]any{
		"drop_lat": 41.31, "drop_lng": 69.29, "drop_name": "Test",
	}, h)
	if rr2.Code != http.StatusOK {
		t.Fatalf("destination status=%d body=%s", rr2.Code, rr2.Body.String())
	}
	var destOut struct {
		OK             bool  `json:"ok"`
		EstimatedPrice int64 `json:"estimated_price"`
	}
	if err := json.Unmarshal(rr2.Body.Bytes(), &destOut); err != nil || !destOut.OK || destOut.EstimatedPrice <= 0 {
		t.Fatalf("destination body: %v %+v raw=%s", err, destOut, rr2.Body.String())
	}

	var estDB int64
	var destConf int
	err := db.QueryRow(`SELECT estimated_price, COALESCE(destination_confirmed,0) FROM ride_requests WHERE id = ?1`, createOut.RequestID).Scan(&estDB, &destConf)
	if err != nil {
		t.Fatal(err)
	}
	if destConf != 0 {
		t.Fatalf("destination_confirmed before confirm: %d", destConf)
	}
	if estDB != destOut.EstimatedPrice {
		t.Fatalf("db estimated_price=%d json=%d", estDB, destOut.EstimatedPrice)
	}

	pathConfirm := "/v1/rider/requests/" + createOut.RequestID + "/confirm"
	rr3 := postJSON(r, pathConfirm, map[string]any{}, h)
	if rr3.Code != http.StatusOK {
		t.Fatalf("confirm status=%d body=%s", rr3.Code, rr3.Body.String())
	}
	var confirmed int
	err = db.QueryRow(`SELECT COALESCE(destination_confirmed,0) FROM ride_requests WHERE id = ?1`, createOut.RequestID).Scan(&confirmed)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed != 1 {
		t.Fatalf("destination_confirmed=%d want 1", confirmed)
	}
}

func TestRiderRequest_CamelCaseBody_EstimatedPriceAlias(t *testing.T) {
	db := setupRiderRequestHandlerDB(t, "rider_req_camel")
	defer db.Close()
	seedRiderLegalAndUser(t, db, 1)
	r, token := newRiderRequestTestEngine(t, db)
	h := map[string]string{"Authorization": "Bearer " + token}

	rr := postJSON(r, "/v1/rider/requests", map[string]any{"pickupLat": 41.3, "pickupLng": 69.28}, h)
	if rr.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", rr.Code, rr.Body.String())
	}
	var createOut struct {
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &createOut); err != nil || createOut.RequestID == "" {
		t.Fatalf("decode create requestId: %v body=%s", err, rr.Body.String())
	}

	path := "/v1/rider/requests/" + createOut.RequestID + "/destination"
	rr2 := postJSON(r, path, map[string]any{
		"dropLat": 41.31, "dropLng": 69.29, "dropName": "Test",
	}, h)
	if rr2.Code != http.StatusOK {
		t.Fatalf("destination status=%d body=%s", rr2.Code, rr2.Body.String())
	}
	var destOut struct {
		OK             bool  `json:"ok"`
		EstimatedPrice int64 `json:"estimatedPrice"`
	}
	if err := json.Unmarshal(rr2.Body.Bytes(), &destOut); err != nil || !destOut.OK || destOut.EstimatedPrice <= 0 {
		t.Fatalf("destination body (camel estimatedPrice): %v %+v raw=%s", err, destOut, rr2.Body.String())
	}
}

func TestRiderRequest_ConfirmBeforeDestination_409(t *testing.T) {
	db := setupRiderRequestHandlerDB(t, "rider_req_confirm_early")
	defer db.Close()
	seedRiderLegalAndUser(t, db, 1)
	r, token := newRiderRequestTestEngine(t, db)
	h := map[string]string{"Authorization": "Bearer " + token}

	rr := postJSON(r, "/v1/rider/requests", map[string]any{"pickup_lat": 41.3, "pickup_lng": 69.28}, h)
	if rr.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", rr.Code, rr.Body.String())
	}
	var createOut struct {
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &createOut); err != nil {
		t.Fatal(err)
	}
	pathConfirm := "/v1/rider/requests/" + createOut.RequestID + "/confirm"
	rr3 := postJSON(r, pathConfirm, map[string]any{}, h)
	if rr3.Code != http.StatusConflict {
		t.Fatalf("confirm status=%d body=%s", rr3.Code, rr3.Body.String())
	}
}

func TestRiderRequest_LegalRequired_403(t *testing.T) {
	db := setupRiderRequestHandlerDB(t, "rider_req_legal")
	defer db.Close()
	_, err := db.Exec(`INSERT INTO users (id, role, telegram_id, phone) VALUES (1, ?, 999001, '+998901112233')`, domain.RoleRider)
	if err != nil {
		t.Fatal(err)
	}
	// No legal_documents / acceptances
	r, token := newRiderRequestTestEngine(t, db)
	h := map[string]string{"Authorization": "Bearer " + token}

	rr := postJSON(r, "/v1/rider/requests", map[string]any{"pickup_lat": 41.3, "pickup_lng": 69.28}, h)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	code, _ := decodeRiderRequestErr(t, rr)
	if code != "legal_required" {
		t.Fatalf("code=%q", code)
	}
}