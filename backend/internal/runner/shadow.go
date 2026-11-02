package runner

import (
	"fmt"
	"os"
	"path/filepath"
)

// ---- folder inspection / git init (used by the project dialog) ----

type Inspection struct {
	Exists       bool `json:"exists"`
	IsGit        bool `json:"isGit"`
	HasCommit    bool `json:"hasCommit"`
	HasGitignore bool `json:"hasGitignore"`
}

func Inspect(path string) Inspection {
	var in Inspection
	fi, err := os.Stat(path)
	if err != nil || !fi.IsDir() {
		return in
	}
	in.Exists = true
	in.IsGit = isGitRepo(path)
	if in.IsGit {
		_, err := git(path, "rev-parse", "--verify", "HEAD")
		in.HasCommit = err == nil
	}
	_, err = os.Stat(filepath.Join(path, ".gitignore"))
	in.HasGitignore = err == nil
	return in
}

const defaultIgnore = "node_modules/\ndist/\nbuild/\nbin/\nobj/\n.venv/\n__pycache__/\n.next/\n*.log\n.env\n"

// InitRepo turns a plain folder into a git repo with an initial commit.
// A basic .gitignore is added only if missing, so the first commit does not swallow node_modules and friends.
func InitRepo(path string) error {
	in := Inspect(path)
	if !in.Exists {
		return fmt.Errorf("pasta não encontrada")
	}
	if !in.IsGit {
		if _, err := git(path, "init", "-b", "main"); err != nil {
			return err
		}
	}
	if !in.HasGitignore {
		if err := os.WriteFile(filepath.Join(path, ".gitignore"), []byte(defaultIgnore), 0o644); err != nil {
			return err
		}
	}
	if _, err := git(path, "add", "-A"); err != nil {
		return err
	}
	_, err := git(path, "-c", "user.name=Kanban AI", "-c", "user.email=kanban@localhost", "commit", "--allow-empty", "-m", "Commit inicial")
	return err
}

// ---- shadow repo: snapshots of a folder that has no git of its own ----

// Shadow keeps its git dir under the app's data folder, so the project never gets a .git.
type Shadow struct{ gitDir, workTree string }

const shadowExclude = ".git\nnode_modules\ndist\nbuild\nbin\nobj\n.venv\n__pycache__\n.next\n*.log\n"

func NewShadow(dataDir string, projectID int64, workTree string) (*Shadow, error) {
	sh := &Shadow{gitDir: filepath.Join(dataDir, "shadow", fmt.Sprintf("project-%d", projectID)), workTree: workTree}
	if _, err := os.Stat(filepath.Join(sh.gitDir, "HEAD")); err != nil {
		if err := os.MkdirAll(filepath.Join(sh.gitDir, "info"), 0o755); err != nil {
			return nil, err
		}
		if _, err := sh.run("init"); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(sh.gitDir, "info", "exclude"), []byte(shadowExclude), 0o644); err != nil {
			return nil, err
		}
	}
	return sh, nil
}

func (s *Shadow) run(args ...string) (string, error) {
	base := []string{"-c", "core.autocrlf=false", "-c", "core.safecrlf=false", "-c", "user.name=Kanban AI", "-c", "user.email=kanban@localhost",
		"--git-dir=" + s.gitDir, "--work-tree=" + s.workTree}
	return git(s.workTree, append(base, args...)...)
}

// Snapshot commits the folder's current state and returns the commit id.
func (s *Shadow) Snapshot(msg string) (string, error) {
	if _, err := s.run("add", "-A"); err != nil {
		return "", err
	}
	if _, err := s.run("commit", "--allow-empty", "-m", msg); err != nil {
		return "", err
	}
	return s.run("rev-parse", "HEAD")
}

func (s *Shadow) Diff(base string) (string, error) { return s.run("diff", base, "HEAD") }

// Restore puts the folder back to the snapshot (files in the exclude list are left alone).
func (s *Shadow) Restore(base string) error {
	if _, err := s.run("reset", "--hard", base); err != nil {
		return err
	}
	_, err := s.run("clean", "-fd")
	return err
}
