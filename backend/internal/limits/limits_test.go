package limits

import "testing"

func TestParse(t *testing.T) {
	line := `{"type":"rate_limit_event","rate_limit_info":{"status":"allowed","resetsAt":1793644800,"rateLimitType":"five_hour","unifiedWindows":{"five_hour":{"utilization":0.07,"resetsAt":1793644800},"seven_day":{"utilization":0.23,"resetsAt":1794168000}}}}`
	s, ok := Parse([]byte(line))
	if !ok || s.FiveHour == nil || s.SevenDay == nil {
		t.Fatalf("expected both windows, got %+v ok=%v", s, ok)
	}
	if s.FiveHour.Utilization != 0.07 || s.SevenDay.ResetsAt != 1794168000 || s.Status != "allowed" {
		t.Fatalf("wrong values: %+v %+v", s.FiveHour, s.SevenDay)
	}
	for _, bad := range []string{`{"type":"result"}`, `not json rate_limit_event`, `{"type":"rate_limit_event","rate_limit_info":{"status":"allowed"}}`} {
		if _, ok := Parse([]byte(bad)); ok {
			t.Fatalf("should not parse %q", bad)
		}
	}
}

func TestTrackerMergesPartialUpdates(t *testing.T) {
	var saved Snapshot
	tr := NewTracker(nil, func(s Snapshot) { saved = s })
	tr.Set(Snapshot{FiveHour: &Window{0.1, 100}, SevenDay: &Window{0.2, 200}})
	got := tr.Set(Snapshot{FiveHour: &Window{0.5, 300}}) // seven_day missing: keep the old one
	if got.FiveHour.Utilization != 0.5 || got.SevenDay == nil || got.SevenDay.Utilization != 0.2 {
		t.Fatalf("merge failed: %+v", got)
	}
	if saved.SevenDay == nil {
		t.Fatal("merged snapshot was not persisted")
	}
}
