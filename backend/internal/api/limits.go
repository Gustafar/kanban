package api

import (
	"net/http"
	"time"

	"kanban_board/internal/limits"
)

func (s *Server) getLimits(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.limits.Get())
}

// refreshLimits reads the current plan limits with one tiny Claude call. Calls are serialized and spaced out,
// since each one spends a few tokens of the very plan being measured.
func (s *Server) refreshLimits(w http.ResponseWriter, r *http.Request) {
	s.probeMu.Lock()
	defer s.probeMu.Unlock()
	if time.Since(s.lastProbe) < 15*time.Second {
		writeJSON(w, 200, s.limits.Get())
		return
	}
	s.lastProbe = time.Now()
	snap, err := limits.Probe(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	merged := s.limits.Set(snap)
	s.hub.Publish("limits", merged)
	writeJSON(w, 200, merged)
}
