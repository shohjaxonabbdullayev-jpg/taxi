package ws

import (
	"encoding/json"
	"sync/atomic"
	"time"
)

// DispatchHub is a best-effort pub/sub hub for driver dispatch pokes.
//
// It intentionally does NOT carry any rider/trip payload; it is only a "refetch"
// signal for polling GET /driver/available-requests.
//
// All broadcasting is non-blocking: if clients are slow, messages may be dropped.
type DispatchHub struct {
	register   chan *dispatchClient
	unregister chan *dispatchClient
	broadcast  chan []byte

	clients map[*dispatchClient]struct{}

	version      atomic.Int64
	updatedAtUTC atomic.Int64 // unix nano
}

// DispatchHubDefault is an optional global hub used by publisher hooks.
// It may be nil; NotifyDispatchChanged is a no-op in that case.
var DispatchHubDefault *DispatchHub

func NewDispatchHub() *DispatchHub {
	return &DispatchHub{
		register:   make(chan *dispatchClient, 128),
		unregister: make(chan *dispatchClient, 128),
		broadcast:  make(chan []byte, 256),
		clients:    make(map[*dispatchClient]struct{}),
	}
}

func (h *DispatchHub) Run() {
	for {
		select {
		case c := <-h.register:
			h.clients[c] = struct{}{}
		case c := <-h.unregister:
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
			}
		case msg := <-h.broadcast:
			for c := range h.clients {
				select {
				case c.send <- msg:
				default:
					// Best-effort: drop for slow clients.
				}
			}
		}
	}
}

// NotifyDispatchChanged broadcasts a "dispatch_changed" poke.
// It is safe to call from any goroutine and must never block request handlers.
func (h *DispatchHub) NotifyDispatchChanged() {
	if h == nil {
		return
	}
	now := time.Now().UTC()
	h.version.Add(1)
	h.updatedAtUTC.Store(now.UnixNano())

	payload := map[string]string{
		"type":       "dispatch_changed",
		"emitted_at": now.Format(time.RFC3339),
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	select {
	case h.broadcast <- b:
	default:
		// Best-effort: drop if hub is under backpressure.
	}
}

// NotifyDispatchChanged is the global, best-effort publisher hook.
func NotifyDispatchChanged() {
	if DispatchHubDefault == nil {
		return
	}
	DispatchHubDefault.NotifyDispatchChanged()
}
