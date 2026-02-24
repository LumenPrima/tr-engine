package transcribe

import (
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func newTestPool(workers, queueSize int) *WorkerPool {
	return NewWorkerPool(WorkerPoolOptions{
		Workers:     workers,
		QueueSize:   queueSize,
		MinDuration: 1.0,
		MaxDuration: 300.0,
		Log:         zerolog.Nop(),
	})
}

func TestNewWorkerPool(t *testing.T) {
	wp := newTestPool(4, 100)
	if wp == nil {
		t.Fatal("NewWorkerPool returned nil")
	}
	if cap(wp.jobs) != 100 {
		t.Errorf("queue capacity = %d, want 100", cap(wp.jobs))
	}
}

func TestWorkerPool_EnqueueBeforeStart(t *testing.T) {
	wp := newTestPool(2, 5)
	// Enqueue should work even before Start() — it just buffers
	ok := wp.Enqueue(Job{CallID: 1})
	if !ok {
		t.Error("Enqueue should return true when queue has space")
	}
}

func TestWorkerPool_EnqueueFull(t *testing.T) {
	wp := newTestPool(0, 2) // 0 workers = nobody draining

	wp.Enqueue(Job{CallID: 1})
	wp.Enqueue(Job{CallID: 2})

	// Queue is full (cap=2), third enqueue should return false
	ok := wp.Enqueue(Job{CallID: 3})
	if ok {
		t.Error("Enqueue should return false when queue is full")
	}
}

func TestWorkerPool_EnqueueAfterStop(t *testing.T) {
	wp := newTestPool(1, 10)
	wp.Start()
	wp.Stop()

	ok := wp.Enqueue(Job{CallID: 1})
	if ok {
		t.Error("Enqueue should return false after Stop()")
	}
}

func TestWorkerPool_Stats(t *testing.T) {
	wp := newTestPool(0, 10) // 0 workers so nothing drains

	wp.Enqueue(Job{CallID: 1})
	wp.Enqueue(Job{CallID: 2})

	stats := wp.Stats()
	if stats.Pending != 2 {
		t.Errorf("Pending = %d, want 2", stats.Pending)
	}
	if stats.Completed != 0 {
		t.Errorf("Completed = %d, want 0", stats.Completed)
	}
	if stats.Failed != 0 {
		t.Errorf("Failed = %d, want 0", stats.Failed)
	}
}

func TestWorkerPool_StopDrains(t *testing.T) {
	wp := newTestPool(2, 10)
	wp.Start()

	// Stop should return (not hang) even with no jobs
	done := make(chan struct{})
	go func() {
		wp.Stop()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() did not return within 5 seconds")
	}
}

func TestWorkerPool_MinMaxDuration(t *testing.T) {
	wp := newTestPool(1, 10)
	if wp.MinDuration() != 1.0 {
		t.Errorf("MinDuration = %f, want 1.0", wp.MinDuration())
	}
	if wp.MaxDuration() != 300.0 {
		t.Errorf("MaxDuration = %f, want 300.0", wp.MaxDuration())
	}
}

func TestWorkerPool_Workers(t *testing.T) {
	wp := newTestPool(4, 10)
	if wp.Workers() != 4 {
		t.Errorf("Workers = %d, want 4", wp.Workers())
	}
}
