package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	_ "modernc.org/sqlite"

	"taxi-mvp/internal/repositories"
	"taxi-mvp/internal/services"
)

type fakeRiderBotHandler struct{}

func (fakeRiderBotHandler) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	return tgbotapi.Message{}, nil
}

func setupRiderAuthHandlerDB(t *testing.T, name string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+name+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	stmts := []string{
		`CREATE TABLE users (
			id INTEGER PRIMARY KEY,
			role TEXT NOT NULL,
			telegram_id INTEGER NOT NULL DEFAULT 0,
			phone TEXT
		)`,
		`CREATE TABLE rider_login_codes (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			phone      TEXT NOT NULL,
			code_hash  TEXT NOT NULL,
			salt       TEXT NOT NULL,
			expires_at INTEGER NOT NULL,
			attempts   INTEGER NOT NULL DEFAULT 0,
			consumed   INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
		)`,
		`CREATE TABLE rider_auth_sessions (
			id                 INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id            INTEGER NOT NULL,
			refresh_hash       TEXT NOT NULL UNIQUE,
			refresh_expires_at INTEGER NOT NULL,
			revoked            INTEGER NOT NULL DEFAULT 0,
			created_at         INTEGER NOT NULL DEFAULT (strftime('%s','now'))
		)`,
	}
	for _, q := range stmts {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func newRiderAuthHandlerEngine(t *testing.T, db *sql.DB) (*gin.Engine, *services.RiderAuthService) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	codes := repositories.NewRiderLoginCodesRepo(db)
	sessions := repositories.NewRiderAuthSessionsRepo(db)
	tokens := services.NewRiderAuthTokenService(sessions, "test-rider-bot-token")
	svc := services.NewRiderAuthService(db, codes, tokens, fakeRiderBotHandler{}, services.RiderAuthConfig{})
	r := gin.New()
	RegisterRiderAuthRoutes(r, RiderAuthDeps{Service: svc})
	return r, svc
}

func postJSON(r http.Handler, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}

func decodeErrEnvelope(t *testing.T, rr *httptest.ResponseRecorder) (string, string) {
	t.Helper()
	var env riderAuthErrorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode error envelope: %v body=%s", err, rr.Body.String())
	}
	return env.Error.Code, env.Error.Message
}

// TestRiderAuth_RequestCode_UnknownPhone_TelegramNotLinked checks that an
// unknown phone is reported as telegram_not_linked (per the spec we deliberately
// do NOT distinguish "no users row" from "users row but telegram_id is null"
// at the HTTP layer — the rider app shows one clear instruction).
func TestRiderAuth_RequestCode_UnknownPhone_TelegramNotLinked(t *testing.T) {
	db := setupRiderAuthHandlerDB(t, "rider_auth_h_unknown_phone")
	defer db.Close()
	r, _ := newRiderAuthHandlerEngine(t, db)

	rr := postJSON(r, "/v1/rider/auth/request-code", map[string]string{"phone": "+998901111111"}, nil)
	if rr.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	code, _ := decodeErrEnvelope(t, rr)
	if code != "telegram_not_linked" {
		t.Fatalf("code=%q body=%s", code, rr.Body.String())
	}
}

func TestRiderAuth_RequestCode_InvalidPhone_400(t *testing.T) {
	db := setupRiderAuthHandlerDB(t, "rider_auth_h_invalid")
	defer db.Close()
	r, _ := newRiderAuthHandlerEngine(t, db)

	rr := postJSON(r, "/v1/rider/auth/request-code", map[string]string{"phone": "abc"}, nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	code, _ := decodeErrEnvelope(t, rr)
	if code != "invalid_phone" {
		t.Fatalf("code=%q body=%s", code, rr.Body.String())
	}
}

func TestRiderAuth_RequestCode_TelegramNotLinked_409(t *testing.T) {
	db := setupRiderAuthHandlerDB(t, "rider_auth_h_no_tg")
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO users (id, role, telegram_id, phone) VALUES (1, 'rider', 0, '+998901234567')`); err != nil {
		t.Fatal(err)
	}
	r, _ := newRiderAuthHandlerEngine(t, db)

	rr := postJSON(r, "/v1/rider/auth/request-code", map[string]string{"phone": "+998901234567"}, nil)
	if rr.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	code, _ := decodeErrEnvelope(t, rr)
	if code != "telegram_not_linked" {
		t.Fatalf("code=%q body=%s", code, rr.Body.String())
	}
}

func TestRiderAuth_RequestCode_NoRateLimit_AllowsImmediateResend(t *testing.T) {
	db := setupRiderAuthHandlerDB(t, "rider_auth_h_rl")
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO users (id, role, telegram_id, phone) VALUES (1, 'rider', 555, '+998901234567')`); err != nil {
		t.Fatal(err)
	}
	r, _ := newRiderAuthHandlerEngine(t, db)

	if rr := postJSON(r, "/v1/rider/auth/request-code", map[string]string{"phone": "+998901234567"}, nil); rr.Code != http.StatusOK {
		t.Fatalf("first status=%d body=%s", rr.Code, rr.Body.String())
	}
	rr := postJSON(r, "/v1/rider/auth/request-code", map[string]string{"phone": "+998901234567"}, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("second status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestRiderAuth_VerifyCode_HappyPathReturnsTokens(t *testing.T) {
	db := setupRiderAuthHandlerDB(t, "rider_auth_h_verify")
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO users (id, role, telegram_id, phone) VALUES (1, 'rider', 555, '+998901234567')`); err != nil {
		t.Fatal(err)
	}
	r, svc := newRiderAuthHandlerEngine(t, db)
	svc.SetCodeGeneratorForTest(func() (string, error) { return "9988", nil })

	if rr := postJSON(r, "/v1/rider/auth/request-code", map[string]string{"phone": "+998901234567"}, nil); rr.Code != http.StatusOK {
		t.Fatalf("request-code status=%d", rr.Code)
	}

	rr := postJSON(r, "/v1/rider/auth/verify-code",
		map[string]string{"phone": "+998901234567", "code": "9988"}, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("verify-code status=%d body=%s", rr.Code, rr.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	for _, k := range []string{"access_token", "refresh_token", "expires_in"} {
		if _, ok := out[k]; !ok {
			t.Fatalf("missing %q in response: %s", k, rr.Body.String())
		}
	}
}

func TestRiderAuth_Logout_BearerRequired(t *testing.T) {
	db := setupRiderAuthHandlerDB(t, "rider_auth_h_logout_no_bearer")
	defer db.Close()
	r, _ := newRiderAuthHandlerEngine(t, db)

	rr := postJSON(r, "/v1/rider/auth/logout", map[string]any{}, nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	code, _ := decodeErrEnvelope(t, rr)
	if code != "unauthorized" {
		t.Fatalf("code=%q body=%s", code, rr.Body.String())
	}
}

func TestRiderAuth_RefreshAndLogoutFlow(t *testing.T) {
	db := setupRiderAuthHandlerDB(t, "rider_auth_h_refresh")
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO users (id, role, telegram_id, phone) VALUES (1, 'rider', 555, '+998901234567')`); err != nil {
		t.Fatal(err)
	}
	r, svc := newRiderAuthHandlerEngine(t, db)
	svc.SetCodeGeneratorForTest(func() (string, error) { return "5544", nil })

	if rr := postJSON(r, "/v1/rider/auth/request-code", map[string]string{"phone": "+998901234567"}, nil); rr.Code != http.StatusOK {
		t.Fatalf("request-code status=%d", rr.Code)
	}
	rr := postJSON(r, "/v1/rider/auth/verify-code",
		map[string]string{"phone": "+998901234567", "code": "5544"}, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("verify-code status=%d body=%s", rr.Code, rr.Body.String())
	}
	var first map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &first)

	// Refresh.
	rr = postJSON(r, "/v1/rider/auth/refresh",
		map[string]string{"refresh_token": first["refresh_token"]}, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("refresh status=%d body=%s", rr.Code, rr.Body.String())
	}
	var rotated map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &rotated)
	if rotated["refresh_token"] == first["refresh_token"] {
		t.Fatalf("refresh token must rotate")
	}

	// Old refresh token now invalid.
	rr = postJSON(r, "/v1/rider/auth/refresh",
		map[string]string{"refresh_token": first["refresh_token"]}, nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("old refresh status=%d body=%s", rr.Code, rr.Body.String())
	}

	// Logout with the new bearer.
	rr = postJSON(r, "/v1/rider/auth/logout", map[string]any{}, map[string]string{
		"Authorization": "Bearer " + rotated["access_token"],
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("logout status=%d body=%s", rr.Code, rr.Body.String())
	}

	// New refresh token also revoked after logout.
	rr = postJSON(r, "/v1/rider/auth/refresh",
		map[string]string{"refresh_token": rotated["refresh_token"]}, nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("post-logout refresh status=%d body=%s", rr.Code, rr.Body.String())
	}
}
