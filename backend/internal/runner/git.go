package runner

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"kanban_board/internal/proc"
)

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s string) string {
	s = strings.Trim(slugRe.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(s) > 40 {
		s = strings.Trim(s[:40], "-")
	}
	if s == "" {
		s = "task"
	}
	return s
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	proc.Hide(cmd)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		return s, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, s)
	}
	return s, nil
}

// BranchExists reports whether the branch is still in the repo (a merged/discarded run's branch is deleted).
func BranchExists(projectPath, branch string) bool {
	_, err := git(projectPath, "rev-parse", "--verify", "refs/heads/"+branch)
	return err == nil
}

func isGitRepo(dir string) bool {
	_, err := git(dir, "rev-parse", "--git-dir")
	return err == nil
}

// ensureWorktree creates (or reuses) an isolated worktree + branch for the card.
func ensureWorktree(projectPath, projectName string, cardID int64, title string) (branch, path string, err error) {
	if !isGitRepo(projectPath) {
		return "", "", fmt.Errorf("%s não é um repositório git", projectPath)
	}
	branch = fmt.Sprintf("kb/card-%d-%s", cardID, slug(title))
	path = filepath.Join(filepath.Dir(projectPath), ".kanban-worktrees", slug(projectName), fmt.Sprintf("card-%d", cardID))
	if _, statErr := os.Stat(path); statErr == nil {
		return branch, path, nil
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	// branch may already exist from an earlier run whose worktree was removed
	if _, e := git(projectPath, "rev-parse", "--verify", "refs/heads/"+branch); e == nil {
		_, err = git(projectPath, "worktree", "add", path, branch)
	} else {
		_, err = git(projectPath, "worktree", "add", "-b", branch, path)
	}
	return
}

// commitAll commits leftovers so the branch always carries the full result.
func commitAll(worktree, msg string) error {
	st, err := git(worktree, "status", "--porcelain")
	if err != nil || st == "" {
		return err
	}
	if _, err := git(worktree, "add", "-A"); err != nil {
		return err
	}
	_, err = git(worktree, "-c", "user.name=Kanban AI", "-c", "user.email=kanban@localhost", "commit", "-m", msg)
	return err
}

// Diff returns the branch's changes relative to the project's current HEAD.
func Diff(projectPath, branch string) (string, error) {
	return git(projectPath, "diff", "HEAD..."+branch)
}

func Merge(projectPath, branch string) error {
	_, err := git(projectPath, "merge", "--no-ff", "-m", "Merge "+branch, branch)
	return err
}

func Discard(projectPath, worktree, branch string) {
	if worktree != "" {
		_, _ = git(projectPath, "worktree", "remove", "--force", worktree)
	}
	if branch != "" {
		_, _ = git(projectPath, "branch", "-D", branch)
	}
}
