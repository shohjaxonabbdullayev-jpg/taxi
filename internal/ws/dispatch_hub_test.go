package ws

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"taxi-mvp/internal/auth"
	"taxi-mvp/internal/domain"
)

func TestDispatchHub_FanoutToTwoClients(t *testing.T) {
	h := NewDispatchHub()
	go h.Run()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mimic tryDriverID+driverAuth by injecting a driver into context.
		r = r.WithContext(auth.WithUser(r.Context(), &auth.User{
			UserID: 1,
			Role:   domain.RoleDriver,
		}))
		ServeDriverDispatchWs(h, w, r)
	}))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"

	dial := websocket.Dialer{}
	c1, _, err := dial.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial c1: %v", err)
	}
	t.Cleanup(func() { _ = c1.Close() })
	c2, _, err := dial.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial c2: %v", err)
	}
	t.Cleanup(func() { _ = c2.Close() })

	readText := func(t *testing.T, c *websocket.Conn) string {
		t.Helper()
		_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, msg, err := c.ReadMessage()
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		return string(msg)
	}

	// hello on connect
	_ = readText(t, c1)
	_ = readText(t, c2)

	h.NotifyDispatchChanged()

	m1 := readText(t, c1)
	m2 := readText(t, c2)
	if !strings.Contains(m1, `"type":"dispatch_changed"`) {
		t.Fatalf("c1 want dispatch_changed, got %s", m1)
	}
	if !strings.Contains(m2, `"type":"dispatch_changed"`) {
		t.Fatalf("c2 want dispatch_changed, got %s", m2)
	}
}
