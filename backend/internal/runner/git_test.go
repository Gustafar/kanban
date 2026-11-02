package runner

import (
	"os"
	"path/filepath"
	"testing"
)

// initRepo creates a git repo with one commit, so `git worktree add` has a HEAD to branch from.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		if _, err := git(dir, args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	run("init")
	run("-c", "user.name=test", "-c", "user.email=test@test", "commit", "--allow-empty", "-m", "init")
	return dir
}

func TestEnsureWorktreeCreatesBranchAndPath(t *testing.T) {
	repo := initRepo(t)
	branch, wt, err := ensureWorktree(repo, "myproj", 7, "Fix the thing!")
	if err != nil {
		t.Fatalf("ensureWorktree: %v", err)
	}
	if branch != "kb/card-7-fix-the-thing" {
		t.Fatalf("unexpected branch name: %q", branch)
	}
	if _, err := os.Stat(wt); err != nil {
		t.Fatalf("worktree path should exist: %v", err)
	}
	if !BranchExists(repo, branch) {
		t.Fatalf("branch should exist in the repo")
	}
}

func TestEnsureWorktreeReusesExistingPath(t *testing.T) {
	repo := initRepo(t)
	branch1, wt1, err := ensureWorktree(repo, "myproj", 7, "Fix the thing")
	if err != nil {
		t.Fatalf("ensureWorktree (first): %v", err)
	}
	branch2, wt2, err := ensureWorktree(repo, "myproj", 7, "Fix the thing")
	if err != nil {
		t.Fatalf("ensureWorktree (second): %v", err)
	}
	if branch1 != branch2 || wt1 != wt2 {
		t.Fatalf("a second call for the same card should reuse branch/path: (%q,%q) vs (%q,%q)", branch1, wt1, branch2, wt2)
	}
}

func TestEnsureWorktreeRecreatesBranchAfterWorktreeRemoved(t *testing.T) {
	repo := initRepo(t)
	branch, wt, err := ensureWorktree(repo, "myproj", 7, "Fix the thing")
	if err != nil {
		t.Fatalf("ensureWorktree: %v", err)
	}
	if _, err := git(repo, "worktree", "remove", "--force", wt); err != nil {
		t.Fatalf("worktree remove: %v", err)
	}
	if !BranchExists(repo, branch) {
		t.Fatalf("branch should survive worktree removal")
	}
	// the branch still exists but the worktree folder is gone: ensureWorktree must reuse the branch
	branch2, wt2, err := ensureWorktree(repo, "myproj", 7, "Fix the thing")
	if err != nil {
		t.Fatalf("ensureWorktree (recreate): %v", err)
	}
	if branch2 != branch {
		t.Fatalf("branch should be reused, got %q want %q", branch2, branch)
	}
	if _, err := os.Stat(wt2); err != nil {
		t.Fatalf("worktree should be recreated: %v", err)
	}
}

func TestEnsureWorktreeRejectsNonGitFolder(t *testing.T) {
	if _, _, err := ensureWorktree(t.TempDir(), "myproj", 1, "task"); err == nil {
		t.Fatalf("want an error for a non-git folder")
	}
}

func TestCommitAllCommitsChanges(t *testing.T) {
	repo := initRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := commitAll(repo, "test commit"); err != nil {
		t.Fatalf("commitAll: %v", err)
	}
	st, err := git(repo, "status", "--porcelain")
	if err != nil {
		t.Fatalf("git status: %v", err)
	}
	if st != "" {
		t.Fatalf("working tree should be clean after commitAll, got status: %q", st)
	}
	log, err := git(repo, "log", "-1", "--pretty=%s")
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	if log != "test commit" {
		t.Fatalf("unexpected commit message: %q", log)
	}
}

func TestCommitAllNoopWhenClean(t *testing.T) {
	repo := initRepo(t)
	before, _ := git(repo, "rev-parse", "HEAD")
	if err := commitAll(repo, "should not happen"); err != nil {
		t.Fatalf("commitAll on a clean tree should not error: %v", err)
	}
	after, _ := git(repo, "rev-parse", "HEAD")
	if before != after {
		t.Fatalf("commitAll should not create a commit when there is nothing to commit")
	}
}

func TestDiscardRemovesWorktreeAndBranch(t *testing.T) {
	repo := initRepo(t)
	branch, wt, err := ensureWorktree(repo, "myproj", 9, "task")
	if err != nil {
		t.Fatalf("ensureWorktree: %v", err)
	}
	Discard(repo, wt, branch)
	if _, err := os.Stat(wt); err == nil {
		t.Fatalf("worktree folder should be gone after Discard")
	}
	if BranchExists(repo, branch) {
		t.Fatalf("branch should be deleted after Discard")
	}
}

func TestMergeBringsBranchIntoCurrentHEAD(t *testing.T) {
	repo := initRepo(t)
	branch, wt, err := ensureWorktree(repo, "myproj", 3, "task")
	if err != nil {
		t.Fatalf("ensureWorktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wt, "new.txt"), []byte("content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := commitAll(wt, "add new.txt"); err != nil {
		t.Fatalf("commitAll: %v", err)
	}
	if err := Merge(repo, branch); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "new.txt")); err != nil {
		t.Fatalf("merged file should exist on the project's checkout: %v", err)
	}
}
