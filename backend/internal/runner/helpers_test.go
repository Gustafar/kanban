package runner

import (
	"path/filepath"
	"testing"

	"kanban_board/internal/store"
)

// newTestStore opens a throwaway sqlite file for the test.
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func newTestRunner(t *testing.T, s *store.Store) *Runner {
	t.Helper()
	return New(s, t.TempDir(), func(string, any) {})
}
