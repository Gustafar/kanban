package api

import "sync"

// Event is pushed to every connected browser via SSE.
type Event struct {
	Type string `json:"type"` // card | project | run | run_event | notify
	Data any    `json:"data"`
}

type Hub struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

func NewHub() *Hub { return &Hub{subs: map[chan Event]struct{}{}} }

func (h *Hub) Subscribe() (chan Event, func()) {
	ch := make(chan Event, 64)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}
}

// Publish never blocks: a slow client simply drops events (it refetches on reconnect).
func (h *Hub) Publish(typ string, data any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- Event{Type: typ, Data: data}:
		default:
		}
	}
}
