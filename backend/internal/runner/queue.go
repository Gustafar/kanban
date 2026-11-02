package runner

// Parallelism limit: at most `max` runs execute at once; the rest wait in a FIFO queue
// and start as slots free up. Slots are held by runSlot for the whole life of a run.

const (
	DefaultMaxParallel = 2
	MaxParallelCeiling = 8
)

type Status struct {
	Max     int `json:"max"`
	Running int `json:"running"`
	Queued  int `json:"queued"`
}

func (r *Runner) Status() Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	return Status{Max: r.max, Running: r.running, Queued: len(r.queue)}
}

func (r *Runner) publishStatus() { r.publish("status", r.Status()) }

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
	for {
		r.mu.Lock()
		if r.running >= r.max || len(r.queue) == 0 {
			r.mu.Unlock()
			break
		}
		id := r.queue[0]
		r.queue = r.queue[1:]
		r.running++
		r.mu.Unlock()
		r.markStatus(id, "running")
		go r.runSlot(id)
	}
	r.publishStatus()
}

// schedule starts the run now if a slot is free, otherwise queues it.
func (r *Runner) schedule(runID int64) {
	r.mu.Lock()
	if r.running < r.max {
		r.running++
		r.mu.Unlock()
		r.markStatus(runID, "running")
		go r.runSlot(runID)
	} else {
		r.queue = append(r.queue, runID)
		r.mu.Unlock()
		r.markStatus(runID, "queued")
	}
	r.publishStatus()
}

// runSlot owns one slot: it executes its run, then keeps the slot for the next queued run (if any).
func (r *Runner) runSlot(runID int64) {
	for runID != 0 {
		r.execute(runID)
		runID = r.takeNext()
		if runID != 0 {
			r.markStatus(runID, "running")
		}
		r.publishStatus()
	}
}

// takeNext pops the next queued run, or releases the slot when there is none (or the limit was lowered).
func (r *Runner) takeNext() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running > r.max || len(r.queue) == 0 {
		r.running--
		return 0
	}
	id := r.queue[0]
	r.queue = r.queue[1:]
	return id
}

// dequeue removes a waiting run; it reports whether the run was in the queue.
func (r *Runner) dequeue(runID int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, id := range r.queue {
		if id == runID {
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
	}
}
