package store

import (
	"os"
	"path/filepath"
	"sort"
	"time"
)

// keepBackups is how many periodic backups survive pruning.
const keepBackups = 14

// Backup writes a consistent snapshot of the database into dir (created if needed)
// and removes older snapshots beyond keepBackups.
func (s *Store) Backup(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	dest := filepath.Join(dir, "kanban-"+time.Now().Format("20060102-150405")+".db")
	if _, err := s.db.Exec(`VACUUM INTO ?`, dest); err != nil {
		return err
	}
	return prune(dir)
}

// prune keeps only the keepBackups most recent backup files in dir.
func prune(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names) // timestamped names sort chronologically
	if len(names) <= keepBackups {
		return nil
	}
	for _, n := range names[:len(names)-keepBackups] {
		_ = os.Remove(filepath.Join(dir, n))
	}
	return nil
}
