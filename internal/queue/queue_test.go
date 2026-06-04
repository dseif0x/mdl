package queue

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dseif0x/mdl/internal/provider"
)

func waitFor(t *testing.T, q *Queue, id string, want Status) Job {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			j, _ := q.Get(id)
			t.Fatalf("job %s did not reach %q (last=%q, err=%q)", id, want, j.Status, j.Error)
		default:
		}
		if j, ok := q.Get(id); ok && j.Status == want {
			return j
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestQueueCompletesJob(t *testing.T) {
	run := func(_ context.Context, name string, track provider.Track) (*provider.DownloadResult, error) {
		return &provider.DownloadResult{Files: []string{"/x/" + track.Title + ".mp3"}}, nil
	}
	q := New(run, Options{Workers: 2})
	q.Start(context.Background())
	defer q.Shutdown()

	job := q.Submit("fake", provider.Track{Title: "Song", URL: "u"})
	if job.Status != StatusQueued {
		t.Fatalf("submit status = %q, want queued", job.Status)
	}
	done := waitFor(t, q, job.ID, StatusCompleted)
	if done.Result == nil || len(done.Result.Files) != 1 {
		t.Fatalf("unexpected result: %+v", done.Result)
	}
	if done.StartedAt == nil || done.FinishedAt == nil {
		t.Fatal("expected StartedAt and FinishedAt to be set")
	}
}

func TestQueueRecordsFailure(t *testing.T) {
	run := func(context.Context, string, provider.Track) (*provider.DownloadResult, error) {
		return nil, errors.New("boom")
	}
	q := New(run, Options{Workers: 1})
	q.Start(context.Background())
	defer q.Shutdown()

	job := q.Submit("fake", provider.Track{URL: "u"})
	done := waitFor(t, q, job.ID, StatusFailed)
	if done.Error != "boom" {
		t.Fatalf("error = %q, want boom", done.Error)
	}
}

func TestQueueTimeout(t *testing.T) {
	run := func(ctx context.Context, _ string, _ provider.Track) (*provider.DownloadResult, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	q := New(run, Options{Workers: 1, Timeout: 20 * time.Millisecond})
	q.Start(context.Background())
	defer q.Shutdown()

	job := q.Submit("fake", provider.Track{URL: "u"})
	done := waitFor(t, q, job.ID, StatusFailed)
	if done.Error != "download timed out" {
		t.Fatalf("error = %q, want timeout", done.Error)
	}
}

func TestQueueCancelRunning(t *testing.T) {
	started := make(chan struct{})
	run := func(ctx context.Context, _ string, _ provider.Track) (*provider.DownloadResult, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	q := New(run, Options{Workers: 1})
	q.Start(context.Background())
	defer q.Shutdown()

	job := q.Submit("fake", provider.Track{URL: "u"})
	<-started
	if !q.Cancel(job.ID) {
		t.Fatal("Cancel returned false for a running job")
	}
	done := waitFor(t, q, job.ID, StatusCancelled)
	if done.FinishedAt == nil {
		t.Fatal("cancelled job should have FinishedAt")
	}
}

func TestQueueCancelTerminalReturnsFalse(t *testing.T) {
	run := func(context.Context, string, provider.Track) (*provider.DownloadResult, error) {
		return &provider.DownloadResult{}, nil
	}
	q := New(run, Options{Workers: 1})
	q.Start(context.Background())
	defer q.Shutdown()

	job := q.Submit("fake", provider.Track{URL: "u"})
	waitFor(t, q, job.ID, StatusCompleted)
	if q.Cancel(job.ID) {
		t.Fatal("Cancel of a completed job should return false")
	}
	if q.Cancel("does-not-exist") {
		t.Fatal("Cancel of unknown job should return false")
	}
}

func TestQueuePrunesOldTerminalJobs(t *testing.T) {
	var wg sync.WaitGroup
	run := func(context.Context, string, provider.Track) (*provider.DownloadResult, error) {
		defer wg.Done()
		return &provider.DownloadResult{}, nil
	}
	q := New(run, Options{Workers: 4, MaxJobs: 5})
	q.Start(context.Background())
	defer q.Shutdown()

	const total = 20
	wg.Add(total)
	for i := 0; i < total; i++ {
		q.Submit("fake", provider.Track{URL: "u"})
	}
	wg.Wait()

	// Submit one more to trigger a prune pass after the others are terminal.
	last := q.Submit("fake", provider.Track{URL: "u"})
	wg.Add(1)
	waitFor(t, q, last.ID, StatusCompleted)

	if got := len(q.List()); got > 5 {
		t.Fatalf("retained %d jobs, want <= 5", got)
	}
}

func TestQueueListNewestFirst(t *testing.T) {
	run := func(context.Context, string, provider.Track) (*provider.DownloadResult, error) {
		return &provider.DownloadResult{}, nil
	}
	q := New(run, Options{Workers: 1})
	q.Start(context.Background())
	defer q.Shutdown()

	a := q.Submit("fake", provider.Track{Title: "a", URL: "u"})
	b := q.Submit("fake", provider.Track{Title: "b", URL: "u"})
	waitFor(t, q, a.ID, StatusCompleted)
	waitFor(t, q, b.ID, StatusCompleted)

	list := q.List()
	if len(list) != 2 || list[0].ID != b.ID {
		t.Fatalf("expected newest-first ordering, got %+v", list)
	}
}
