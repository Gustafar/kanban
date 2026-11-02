// Package limits tracks how much of the Claude plan's usage windows (5 hours and 7 days) is consumed.
// The Claude CLI reports this itself: every call emits a rate_limit_event on its stream-json output.
package limits

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"kanban_board/internal/proc"
)

type Window struct {
	Utilization float64 `json:"utilization"` // 0..1
	ResetsAt    int64   `json:"resetsAt"`    // unix seconds
}

type Snapshot struct {
	FiveHour  *Window `json:"fiveHour"`
	SevenDay  *Window `json:"sevenDay"`
	Status    string  `json:"status"`    // allowed | allowed_warning | rejected
	UpdatedAt int64   `json:"updatedAt"` // unix seconds of the last event received
}

// Parse reads one stream-json line; ok is false when it is not a rate_limit_event carrying window data.
func Parse(line []byte) (Snapshot, bool) {
	if !bytes.Contains(line, []byte("rate_limit_event")) {
		return Snapshot{}, false
	}
	var m struct {
		Type string `json:"type"`
		Info struct {
			Status         string  `json:"status"`
			RateLimitType  string  `json:"rateLimitType"`
			ResetsAt       int64   `json:"resetsAt"`
			Utilization    float64 `json:"utilization"`
			UnifiedWindows map[string]struct {
				Utilization float64 `json:"utilization"`
				ResetsAt    int64   `json:"resetsAt"`
			} `json:"unifiedWindows"`
		} `json:"rate_limit_info"`
	}
	if json.Unmarshal(line, &m) != nil || m.Type != "rate_limit_event" {
		return Snapshot{}, false
	}
	s := Snapshot{Status: m.Info.Status, UpdatedAt: time.Now().Unix()}
	if w, ok := m.Info.UnifiedWindows["five_hour"]; ok {
		s.FiveHour = &Window{Utilization: w.Utilization, ResetsAt: w.ResetsAt}
	}
	if w, ok := m.Info.UnifiedWindows["seven_day"]; ok {
		s.SevenDay = &Window{Utilization: w.Utilization, ResetsAt: w.ResetsAt}
	}
	if s.FiveHour == nil && s.SevenDay == nil && m.Info.Utilization > 0 { // older, single-window shape
		w := &Window{Utilization: m.Info.Utilization, ResetsAt: m.Info.ResetsAt}
		switch m.Info.RateLimitType {
		case "five_hour":
			s.FiveHour = w
		case "seven_day":
			s.SevenDay = w
		}
	}
	return s, s.FiveHour != nil || s.SevenDay != nil
}

// Tracker keeps the latest snapshot, survives restarts through save/load callbacks, and merges partial updates.
type Tracker struct {
	mu   sync.Mutex
	snap Snapshot
	save func(Snapshot)
}

func NewTracker(load func() (Snapshot, bool), save func(Snapshot)) *Tracker {
	t := &Tracker{save: save}
	if load != nil {
		if s, ok := load(); ok {
			t.snap = s
		}
	}
	return t
}

func (t *Tracker) Get() Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.snap
}

// Set merges an update: a window missing from it keeps its previous value.
func (t *Tracker) Set(s Snapshot) Snapshot {
	t.mu.Lock()
	if s.FiveHour == nil {
		s.FiveHour = t.snap.FiveHour
	}
	if s.SevenDay == nil {
		s.SevenDay = t.snap.SevenDay
	}
	t.snap = s
	save := t.save
	t.mu.Unlock()
	if save != nil {
		save(s)
	}
	return s
}

// Probe makes the smallest possible Claude call (Haiku, no tools, a few tokens) just to read the current limits.
func Probe(parent context.Context) (Snapshot, error) {
	ctx, cancel := context.WithTimeout(parent, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "claude", "-p", "--model", "haiku", "--tools", "", "--no-session-persistence",
		"--disable-slash-commands", "--strict-mcp-config", "--output-format", "stream-json", "--verbose",
		"--system-prompt", "Responda somente: ok")
	proc.Hide(cmd)
	cmd.Env = append(os.Environ(), "MAX_THINKING_TOKENS=0")
	cmd.Dir = os.TempDir()
	cmd.Stdin = strings.NewReader("ok")
	out, err := cmd.StdoutPipe()
	if err != nil {
		return Snapshot{}, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return Snapshot{}, fmt.Errorf("não consegui iniciar o claude: %w", err)
	}
	var found Snapshot
	ok := false
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 1<<20), 8<<20)
	for sc.Scan() {
		if s, good := Parse(sc.Bytes()); good {
			found, ok = s, true // keep reading: a later event may carry the other window
			if found.FiveHour != nil && found.SevenDay != nil {
				break
			}
		}
	}
	_ = cmd.Wait()
	if !ok {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return Snapshot{}, fmt.Errorf("claude: %s", msg)
		}
		return Snapshot{}, fmt.Errorf("o claude não informou os limites do plano (isso só existe para login com assinatura)")
	}
	return found, nil
}
