package ingest

import (
	"sync"
	"testing"
	"time"
)

func TestBatcher(t *testing.T) {
	t.Run("size_threshold_triggers_flush", func(t *testing.T) {
		var mu sync.Mutex
		var batches [][]int

		b := NewBatcher[int](3, time.Hour, func(items []int) {
			mu.Lock()
			defer mu.Unlock()
			batches = append(batches, items)
		})
		defer b.Stop()

		b.Add(1)
		b.Add(2)
		b.Add(3) // should trigger flush

		// Wait for async flush goroutine
		time.Sleep(50 * time.Millisecond)

		mu.Lock()
		defer mu.Unlock()
		if len(batches) != 1 {
			t.Fatalf("expected 1 flush, got %d", len(batches))
		}
		if len(batches[0]) != 3 || batches[0][0] != 1 || batches[0][1] != 2 || batches[0][2] != 3 {
			t.Errorf("flushed items = %v, want [1 2 3]", batches[0])
		}
	})

	t.Run("under_threshold_no_immediate_flush", func(t *testing.T) {
		var mu sync.Mutex
		var flushed bool

		b := NewBatcher[int](10, time.Hour, func(items []int) {
			mu.Lock()
			defer mu.Unlock()
			flushed = true
		})
		defer b.Stop()

		b.Add(1)
		b.Add(2)

		time.Sleep(50 * time.Millisecond)

		mu.Lock()
		defer mu.Unlock()
		if flushed {
			t.Error("expected no flush under threshold")
		}
	})

	t.Run("stop_flushes_remaining_and_blocks_adds", func(t *testing.T) {
		var mu sync.Mutex
		var batches [][]int

		b := NewBatcher[int](100, time.Hour, func(items []int) {
			mu.Lock()
			defer mu.Unlock()
			batches = append(batches, items)
		})

		b.Add(10)
		b.Add(20)
		b.Stop()

		// After Stop, adds should be silently dropped
		b.Add(30)

		mu.Lock()
		defer mu.Unlock()
		if len(batches) != 1 {
			t.Fatalf("expected 1 flush on stop, got %d", len(batches))
		}
		if len(batches[0]) != 2 || batches[0][0] != 10 || batches[0][1] != 20 {
			t.Errorf("flushed items = %v, want [10 20]", batches[0])
		}
	})

	t.Run("time_based_flush", func(t *testing.T) {
		var mu sync.Mutex
		var batches [][]int

		b := NewBatcher[int](100, 50*time.Millisecond, func(items []int) {
			mu.Lock()
			defer mu.Unlock()
			batches = append(batches, items)
		})
		defer b.Stop()

		b.Add(1)
		b.Add(2)

		// Wait for timer-based flush
		time.Sleep(150 * time.Millisecond)

		mu.Lock()
		defer mu.Unlock()
		if len(batches) != 1 {
			t.Fatalf("expected 1 time-based flush, got %d", len(batches))
		}
		if len(batches[0]) != 2 || batches[0][0] != 1 || batches[0][1] != 2 {
			t.Errorf("flushed items = %v, want [1 2]", batches[0])
		}
	})
}
