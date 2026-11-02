package api

import "testing"

func TestValidModelAndEffort(t *testing.T) {
	for _, ok := range []string{"", "opus", "sonnet", "haiku", "fable", "claude-sonnet-5-5", "sonnet[1m]"} {
		if !validModel(ok) {
			t.Errorf("model %q should be accepted", ok)
		}
	}
	// anything that could pass for a CLI flag or carries odd characters must be refused
	for _, bad := range []string{"--dangerously-skip-permissions", "-x", "Opus", "a b", "opus;rm", "../x"} {
		if validModel(bad) {
			t.Errorf("model %q should be rejected", bad)
		}
	}
	for _, ok := range []string{"", "low", "medium", "high", "xhigh", "max"} {
		if !validEffort(ok) {
			t.Errorf("effort %q should be accepted", ok)
		}
	}
	for _, bad := range []string{"ultra", "HIGH", "--max"} {
		if validEffort(bad) {
			t.Errorf("effort %q should be rejected", bad)
		}
	}
}
