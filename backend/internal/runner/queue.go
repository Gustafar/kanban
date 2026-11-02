package runner

// Parallelism: at most `max` runs execute at once, and a project that runs "direct" (no git isolation)
// lets only one run edit its folder at a time. Runs that cannot start yet wait in a FIFO queue, without
// holding a slot, and start as soon as both a slot and their folder are free.

const (
	DefaultMaxParallel = 2
	MaxParallelCeiling = 8
)

type Status struct {
	Max     int `json:"max"`
	Running int `json:"running"`
	Queued  int `json:"queued"`
}

// queued is a run waiting to start; direct runs must also win their project's folder,
// and a run with a dependency must wait for that card to reach "done".
type queued struct {
	runID, projectID int64
	direct           bool
	dependsOn        int64
}

func (r *Runner) Status() Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	return Status{Max: r.max, Running: r.running, Queued: len(r.queue)}
}

func (r *Runner) publishStatus() { r.publish("status", r.Status()) }

// queueInfo looks up which project (and isolation mode) a run belongs to.
func (r *Runner) queueInfo(runID int64) queued {
	q := queued{runID: runID}
	run, err := r.store.GetRun(runID)
	if err != nil {
		return q
	}
	card, err := r.store.GetCard(run.CardID)
	if err != nil {
		return q
	}
	q.dependsOn = card.DependsOn
	if p, err := r.store.GetProject(card.ProjectID); err == nil {
		q.projectID, q.direct = p.ID, p.Isolation == "direct"
	}
	return q
}

// dependencyMet reports whether a run can start: it has no dependency, or that card already reached "done".
func (r *Runner) dependencyMet(dependsOn int64) bool {
	if dependsOn == 0 {
		return true
	}
	c, err := r.store.GetCard(dependsOn)
	if err != nil {
		return true // the dependency was deleted: nothing left to wait for
	}
	return c.Column == "done"
}

// popStartableLocked removes and returns the first queued run whose folder is free, reserving the folder.
// The caller holds r.mu and owns the slot accounting.
func (r *Runner) popStartableLocked() (queued, bool) {
	for i, q := range r.queue {
		if q.direct && r.busy[q.projectID] {
			continue
		}
		if !r.dependencyMet(q.dependsOn) {
			continue
		}
		r.queue = append(r.queue[:i], r.queue[i+1:]...)
		if q.direct {
			r.busy[q.projectID] = true
		}
		return q, true
	}
	return queued{}, false
}

// startWaiting launches queued runs into every free slot whose folder is available.
func (r *Runner) startWaiting() {
	for {
		r.mu.Lock()
		if r.running >= r.max {
			r.mu.Unlock()
			return
		}
		q, ok := r.popStartableLocked()
		if !ok {
			r.mu.Unlock()
			return
		}
		r.running++
		r.mu.Unlock()
		r.markStatus(q.runID, "running")
		go r.runSlot(q)
	}
}

// Recheck starts queued runs whose dependency card just reached "done" (or was deleted).
func (r *Runner) Recheck() { r.startWaiting() }

// Recover requeues runs left "running" or "queued" by a previous server process: the queue itself
// lives only in memory, so a restart would otherwise strand them forever. Call once at startup,
// after SetMax. Interrupted runs resume their own Claude session (see execute's --resume handling).
func (r *Runner) Recover() {
	runs, err := r.store.ResetInterruptedRuns()
	if err != nil {
		return
	}
	for _, run := range runs {
		r.emit(run.ID, "system", "servidor reiniciado: execução reenfileirada")
		r.schedule(run.ID)
	}
}

// SetMax changes the limit; a higher limit immediately starts waiting runs.
func (r *Runner) SetMax(n int) {
	if n < 1 {
		n = 1
	}
	if n > MaxParallelCeiling {
		n = MaxParallelCeiling
	}
	r.mu.Lock()
	r.max = n
	r.mu.Unlock()
	r.startWaiting()
	r.publishStatus()
}

// schedule starts the run now if a slot (and its folder) is free, otherwise queues it.
func (r *Runner) schedule(runID int64) {
	q := r.queueInfo(runID)
	r.mu.Lock()
	if r.running < r.max && !(q.direct && r.busy[q.projectID]) && r.dependencyMet(q.dependsOn) {
		r.running++
		if q.direct {
			r.busy[q.projectID] = true
		}
		r.mu.Unlock()
		r.markStatus(runID, "running")
		go r.runSlot(q)
	} else {
		r.queue = append(r.queue, q)
		r.mu.Unlock()
		r.markStatus(runID, "queued")
	}
	r.publishStatus()
}

// runSlot owns one slot: it executes its run, frees the folder, then keeps the slot for the next startable run.
func (r *Runner) runSlot(q queued) {
	for {
		r.execute(q.runID)
		r.mu.Lock()
		if q.direct {
			delete(r.busy, q.projectID)
		}
		var next queued
		ok := false
		if r.running <= r.max { // a lowered limit releases the slot instead of taking more work
			next, ok = r.popStartableLocked()
		}
		if !ok {
			r.running--
		}
		r.mu.Unlock()

		r.startWaiting() // the freed folder may unblock a run that fits in another free slot
		r.publishStatus()
		if !ok {
			return
		}
		r.markStatus(next.runID, "running")
		q = next
	}
}

// dequeue removes a waiting run; it reports whether the run was in the queue.
func (r *Runner) dequeue(runID int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, q := range r.queue {
		if q.runID == runID {
			r.queue = append(r.queue[:i], r.queue[i+1:]...)
			return true
		}
	}
	return false
}

func (r *Runner) markStatus(runID int64, status string) {
	if err := r.store.UpdateRun(runID, status, "", "", "", "", nil, nil); err != nil {
		return
	}
	if run, err := r.store.GetRun(runID); err == nil {
		r.publish("run", run)
		r.publishCard(run.CardID)
	}
}

// publishCard pushes the card as it is now: its status badge is derived from its latest run,
// so every run state change must be followed by a card update or the board shows a stale badge.
func (r *Runner) publishCard(cardID int64) {
	if c, err := r.store.GetCard(cardID); err == nil {
		r.publish("card", c)
	}
}
