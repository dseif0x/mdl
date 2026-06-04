// Package queue provides an in-memory, worker-pool backed download queue.
//
// Downloads can be long-running, so the HTTP layer enqueues a Job and returns
// immediately; workers process jobs in the background and the client polls for
// status. The queue is decoupled from providers via a RunFunc, so it only
// depends on the provider types, not the registry.
package queue

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/dseif0x/mdl/internal/provider"
)

// Status is the lifecycle state of a Job.
type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

// IsTerminal reports whether the status is final (no further transitions).
func (s Status) IsTerminal() bool {
	return s == StatusCompleted || s == StatusFailed || s == StatusCancelled
}

// Job is a single download request and its current state. Values returned by
// the queue are snapshots; the live copy is owned by the queue.
type Job struct {
	ID         string                   `json:"id"`
	Provider   string                   `json:"provider"`
	Track      provider.Track           `json:"track"`
	Status     Status                   `json:"status"`
	Error      string                   `json:"error,omitempty"`
	Result     *provider.DownloadResult `json:"result,omitempty"`
	CreatedAt  time.Time                `json:"created_at"`
	StartedAt  *time.Time               `json:"started_at,omitempty"`
	FinishedAt *time.Time               `json:"finished_at,omitempty"`
}

// RunFunc performs the actual download for a job.
type RunFunc func(ctx context.Context, providerName string, track provider.Track) (*provider.DownloadResult, error)

// Queue is a concurrency-safe download queue with a fixed worker pool.
type Queue struct {
	run     RunFunc
	timeout time.Duration
	maxJobs int
	workers int
	log     *slog.Logger

	ch    chan string
	chMu  sync.RWMutex // guards send-vs-close on ch
	close bool

	mu      sync.Mutex // guards jobs, order, cancels
	jobs    map[string]*Job
	order   []string // job IDs in submission order, for listing and pruning
	cancels map[string]context.CancelFunc

	wg sync.WaitGroup
}

// Options configures a Queue.
type Options struct {
	// Workers is the number of concurrent downloads (default 2).
	Workers int
	// Timeout caps how long a single download may run (0 means no timeout).
	Timeout time.Duration
	// MaxJobs caps retained jobs; oldest terminal jobs are pruned (default 500).
	MaxJobs int
	Logger  *slog.Logger
}

// New creates a Queue. Call Start to launch its workers.
func New(run RunFunc, opts Options) *Queue {
	if opts.Workers <= 0 {
		opts.Workers = 2
	}
	if opts.MaxJobs <= 0 {
		opts.MaxJobs = 500
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	q := &Queue{
		run:     run,
		timeout: opts.Timeout,
		maxJobs: opts.MaxJobs,
		log:     opts.Logger,
		// Generous buffer so Submit rarely blocks; workers drain it.
		ch:      make(chan string, 1024),
		jobs:    make(map[string]*Job),
		cancels: make(map[string]context.CancelFunc),
	}
	q.workers = opts.Workers
	return q
}

// Start launches the worker pool. ctx is the parent context for all jobs;
// cancelling it (e.g. on shutdown) aborts in-flight downloads.
func (q *Queue) Start(ctx context.Context) {
	for i := 0; i < q.workers; i++ {
		q.wg.Add(1)
		go q.worker(ctx)
	}
}

// Submit enqueues a download and returns a snapshot of the new job.
func (q *Queue) Submit(providerName string, track provider.Track) Job {
	job := &Job{
		ID:        newID(),
		Provider:  providerName,
		Track:     track,
		Status:    StatusQueued,
		CreatedAt: time.Now(),
	}

	q.mu.Lock()
	q.jobs[job.ID] = job
	q.order = append(q.order, job.ID)
	q.prune()
	snapshot := *job
	q.mu.Unlock()

	if !q.enqueue(job.ID) {
		q.mu.Lock()
		q.fail(job, "queue is shutting down")
		snapshot = *job
		q.mu.Unlock()
	}
	return snapshot
}

// Get returns a snapshot of a job by ID.
func (q *Queue) Get(id string) (Job, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	j, ok := q.jobs[id]
	if !ok {
		return Job{}, false
	}
	return *j, true
}

// List returns snapshots of all retained jobs, newest first.
func (q *Queue) List() []Job {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]Job, 0, len(q.order))
	for i := len(q.order) - 1; i >= 0; i-- {
		if j, ok := q.jobs[q.order[i]]; ok {
			out = append(out, *j)
		}
	}
	return out
}

// Cancel requests cancellation of a queued or running job. It returns false if
// the job is unknown or already in a terminal state.
func (q *Queue) Cancel(id string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	j, ok := q.jobs[id]
	if !ok || j.Status.IsTerminal() {
		return false
	}
	switch j.Status {
	case StatusQueued:
		// Mark cancelled now; the worker will skip it when picked up.
		now := time.Now()
		j.Status = StatusCancelled
		j.FinishedAt = &now
	case StatusRunning:
		if cancel, ok := q.cancels[id]; ok {
			cancel()
		}
	}
	return true
}

// Shutdown stops accepting new jobs and waits for workers to finish.
func (q *Queue) Shutdown() {
	q.chMu.Lock()
	if !q.close {
		q.close = true
		close(q.ch)
	}
	q.chMu.Unlock()
	q.wg.Wait()
}

// --- internals -------------------------------------------------------------

func (q *Queue) worker(ctx context.Context) {
	defer q.wg.Done()
	for id := range q.ch {
		q.process(ctx, id)
	}
}

func (q *Queue) process(parent context.Context, id string) {
	q.mu.Lock()
	j, ok := q.jobs[id]
	if !ok || j.Status != StatusQueued {
		// Unknown, or cancelled while queued.
		q.mu.Unlock()
		return
	}
	now := time.Now()
	j.Status = StatusRunning
	j.StartedAt = &now
	providerName, track := j.Provider, j.Track

	jobCtx, cancel := context.WithCancel(parent)
	if q.timeout > 0 {
		jobCtx, cancel = context.WithTimeout(parent, q.timeout)
	}
	q.cancels[id] = cancel
	q.mu.Unlock()

	result, err := q.run(jobCtx, providerName, track)
	cancel()

	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.cancels, id)
	fin := time.Now()
	j.FinishedAt = &fin
	switch {
	case err == nil:
		j.Status = StatusCompleted
		j.Result = result
	case errors.Is(err, context.Canceled):
		j.Status = StatusCancelled
	case errors.Is(err, context.DeadlineExceeded):
		j.Status = StatusFailed
		j.Error = "download timed out"
	default:
		j.Status = StatusFailed
		j.Error = err.Error()
	}
}

// fail marks a job failed. Caller must hold q.mu.
func (q *Queue) fail(j *Job, msg string) {
	now := time.Now()
	j.Status = StatusFailed
	j.Error = msg
	j.FinishedAt = &now
}

// enqueue sends a job ID to the worker channel unless the queue is closed.
func (q *Queue) enqueue(id string) bool {
	q.chMu.RLock()
	defer q.chMu.RUnlock()
	if q.close {
		return false
	}
	q.ch <- id
	return true
}

// prune drops the oldest terminal jobs once the retention cap is exceeded.
// Caller must hold q.mu.
func (q *Queue) prune() {
	if len(q.order) <= q.maxJobs {
		return
	}
	kept := q.order[:0:0]
	removed := 0
	target := len(q.order) - q.maxJobs
	for _, id := range q.order {
		j, ok := q.jobs[id]
		if removed < target && ok && j.Status.IsTerminal() {
			delete(q.jobs, id)
			removed++
			continue
		}
		kept = append(kept, id)
	}
	q.order = kept
}

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
