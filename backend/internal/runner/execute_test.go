package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withStubClaude puts the built claudestub binary (named "claude") first on PATH.
func withStubClaude(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", stubBinDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func readArgs(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading recorded args: %v", err)
	}
	if len(b) == 0 {
		return nil
	}
	return strings.Split(string(b), "\x00")
}

func TestExecuteWorktreeRunSucceedsAndMovesToReview(t *testing.T) {
	withStubClaude(t)
	s := newTestStore(t)
	r := newTestRunner(t, s)

	repo := initRepo(t)
	proj, err := s.CreateProject("p", repo, "", "worktree")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	card, err := s.CreateCard(proj.ID, "Fix the thing", "do it", "doing", "auto")
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	run, err := s.CreateRun(card.ID, "queued", "")
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	argsFile := filepath.Join(t.TempDir(), "args.txt")
	t.Setenv("CLAUDE_STUB_ARGS_FILE", argsFile)
	t.Setenv("CLAUDE_STUB_SESSION", "sess-1")

	r.execute(run.ID)

	got, err := s.GetRun(run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.Status != "succeeded" {
		t.Fatalf("want succeeded, got %q (err=%q)", got.Status, got.Error)
	}
	if got.SessionID != "sess-1" {
		t.Fatalf("want recorded session id, got %q", got.SessionID)
	}
	if got.Branch == "" || got.WorktreePath == "" {
		t.Fatalf("want branch/worktree recorded, got %+v", got)
	}
	if _, err := os.Stat(got.WorktreePath); err != nil {
		t.Fatalf("worktree should exist on disk: %v", err)
	}

	args := readArgs(t, argsFile)
	if containsArg(args, "--resume") {
		t.Fatalf("a first run must not pass --resume, got args %v", args)
	}

	movedCard, err := s.GetCard(card.ID)
	if err != nil {
		t.Fatalf("GetCard: %v", err)
	}
	if movedCard.Column != "review" {
		t.Fatalf("a succeeded run should move its card to review, got column %q", movedCard.Column)
	}
}

func TestExecuteFollowUpRunResumesPriorSession(t *testing.T) {
	withStubClaude(t)
	s := newTestStore(t)
	r := newTestRunner(t, s)

	repo := initRepo(t)
	proj, err := s.CreateProject("p", repo, "", "worktree")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	card, err := s.CreateCard(proj.ID, "Fix the thing", "do it", "doing", "auto")
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}

	run1, err := s.CreateRun(card.ID, "queued", "")
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	argsFile1 := filepath.Join(t.TempDir(), "args1.txt")
	t.Setenv("CLAUDE_STUB_ARGS_FILE", argsFile1)
	t.Setenv("CLAUDE_STUB_SESSION", "sess-first")
	r.execute(run1.ID)
	if got, _ := s.GetRun(run1.ID); got.Status != "succeeded" {
		t.Fatalf("first run should succeed, got %q", got.Status)
	}
	// move the card back to doing, as the board does for a follow-up request
	if _, err := s.MoveCard(card.ID, "doing", 0); err != nil {
		t.Fatalf("MoveCard: %v", err)
	}

	run2, err := s.CreateRun(card.ID, "queued", "please also fix the other thing")
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	argsFile2 := filepath.Join(t.TempDir(), "args2.txt")
	t.Setenv("CLAUDE_STUB_ARGS_FILE", argsFile2)
	t.Setenv("CLAUDE_STUB_SESSION", "sess-second")
	r.execute(run2.ID)

	got, err := s.GetRun(run2.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.Status != "succeeded" {
		t.Fatalf("follow-up run should succeed, got %q (err=%q)", got.Status, got.Error)
	}
	if got.WorktreePath == "" {
		t.Fatalf("follow-up run should reuse the worktree")
	}

	run1Reloaded, _ := s.GetRun(run1.ID)
	if got.WorktreePath != run1Reloaded.WorktreePath {
		t.Fatalf("follow-up run should reuse the same worktree: %q vs %q", got.WorktreePath, run1Reloaded.WorktreePath)
	}

	args := readArgs(t, argsFile2)
	if !containsArgPair(args, "--resume", "sess-first") {
		t.Fatalf("follow-up run should pass --resume sess-first, got args %v", args)
	}
}

func TestExecuteDirectModeTakesShadowSnapshot(t *testing.T) {
	withStubClaude(t)
	s := newTestStore(t)
	r := newTestRunner(t, s)

	folder := t.TempDir()
	if err := os.WriteFile(filepath.Join(folder, "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	proj, err := s.CreateProject("direct-proj", folder, "", "direct")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	card, err := s.CreateCard(proj.ID, "Edit directly", "do it", "doing", "auto")
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	run, err := s.CreateRun(card.ID, "queued", "")
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	argsFile := filepath.Join(t.TempDir(), "args.txt")
	t.Setenv("CLAUDE_STUB_ARGS_FILE", argsFile)
	t.Setenv("CLAUDE_STUB_SESSION", "sess-direct")

	r.execute(run.ID)

	got, err := s.GetRun(run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.Status != "succeeded" {
		t.Fatalf("want succeeded, got %q (err=%q)", got.Status, got.Error)
	}
	if got.BaseRef == "" {
		t.Fatalf("direct mode should record a shadow base ref")
	}
	if got.WorktreePath != "" || got.Branch != "" {
		t.Fatalf("direct mode should not use a worktree/branch, got %+v", got)
	}
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func containsArgPair(args []string, flag, value string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag && args[i+1] == value {
			return true
		}
	}
	return false
}
