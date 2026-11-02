package runner

import "testing"

func newQueueRunner(max int) *Runner {
	return &Runner{busy: map[int64]bool{}, max: max}
}

func TestPopStartableLockedFIFO(t *testing.T) {
	r := newQueueRunner(2)
	r.queue = []queued{{runID: 1}, {runID: 2}, {runID: 3}}
	q, ok := r.popStartableLocked()
	if !ok || q.runID != 1 {
		t.Fatalf("want first queued run (1), got %+v ok=%v", q, ok)
	}
	if len(r.queue) != 2 || r.queue[0].runID != 2 {
		t.Fatalf("want remaining queue [2,3], got %+v", r.queue)
	}
}

func TestPopStartableLockedSkipsBusyDirectFolder(t *testing.T) {
	r := newQueueRunner(2)
	r.busy[1] = true // project 1's folder is being edited by another run
	r.queue = []queued{
		{runID: 1, projectID: 1, direct: true},
		{runID: 2, projectID: 2, direct: true},
	}
	q, ok := r.popStartableLocked()
	if !ok || q.runID != 2 {
		t.Fatalf("want run 2 (project 1 is busy), got %+v ok=%v", q, ok)
	}
	if len(r.queue) != 1 || r.queue[0].runID != 1 {
		t.Fatalf("run 1 should remain queued, got %+v", r.queue)
	}
	if !r.busy[2] {
		t.Fatalf("starting a direct run must reserve its project's folder")
	}
}

func TestPopStartableLockedEmptyQueue(t *testing.T) {
	r := newQueueRunner(2)
	if _, ok := r.popStartableLocked(); ok {
		t.Fatalf("want no startable run on an empty queue")
	}
}

func TestPopStartableLockedWaitsForDependency(t *testing.T) {
	s := newTestStore(t)
	r := newQueueRunner(2)
	r.store = s

	proj, err := s.CreateProject("p", t.TempDir(), "", "worktree")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	blocker, err := s.CreateCard(proj.ID, "blocker", "", "doing", "approval")
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	done, err := s.CreateCard(proj.ID, "done", "", "backlog", "approval")
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	if _, err := s.MoveCard(done.ID, "done", 0); err != nil {
		t.Fatalf("MoveCard: %v", err)
	}

	r.queue = []queued{
		{runID: 100, dependsOn: blocker.ID}, // blocker is not "done" yet
		{runID: 200, dependsOn: done.ID},
	}

	q, ok := r.popStartableLocked()
	if !ok || q.runID != 200 {
		t.Fatalf("want run 200 (no unmet dependency), got %+v ok=%v", q, ok)
	}
	if len(r.queue) != 1 || r.queue[0].runID != 100 {
		t.Fatalf("run 100 should still be waiting on its dependency, got %+v", r.queue)
	}

	if _, err := s.MoveCard(blocker.ID, "done", 0); err != nil {
		t.Fatalf("MoveCard: %v", err)
	}
	q, ok = r.popStartableLocked()
	if !ok || q.runID != 100 {
		t.Fatalf("want run 100 once its dependency reached done, got %+v ok=%v", q, ok)
	}
}

func TestDequeue(t *testing.T) {
	r := newQueueRunner(1)
	r.queue = []queued{{runID: 1}, {runID: 2}}
	if !r.dequeue(2) {
		t.Fatalf("want run 2 to be found and removed")
	}
	if len(r.queue) != 1 || r.queue[0].runID != 1 {
		t.Fatalf("want only run 1 left, got %+v", r.queue)
	}
	if r.dequeue(2) {
		t.Fatalf("run 2 was already removed, dequeue should report false")
	}
}

func TestSetMaxClampsToRange(t *testing.T) {
	s := newTestStore(t)
	r := newTestRunner(t, s)

	r.SetMax(0)
	if got := r.Status().Max; got != 1 {
		t.Fatalf("SetMax(0) should clamp to 1, got %d", got)
	}
	r.SetMax(1000)
	if got := r.Status().Max; got != MaxParallelCeiling {
		t.Fatalf("SetMax(1000) should clamp to %d, got %d", MaxParallelCeiling, got)
	}
	r.SetMax(3)
	if got := r.Status().Max; got != 3 {
		t.Fatalf("SetMax(3) should set 3, got %d", got)
	}
}
