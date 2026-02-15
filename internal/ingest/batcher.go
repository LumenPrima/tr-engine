package ingest

import (
	"sync"
	"time"
)

// Batcher collects items and flushes them in batches by size or time threshold.
type Batcher[T any] struct {
	mu       sync.Mutex
	items    []T
	maxSize  int
	interval time.Duration
	flushFn  func([]T)
	timer    *time.Timer
	stopped  bool
	wg       sync.WaitGroup
}

// NewBatcher creates a batcher that calls flushFn when maxSize items accumulate
// or interval elapses since the first item, whichever comes first.
func NewBatcher[T any](maxSize int, interval time.Duration, flushFn func([]T)) *Batcher[T] {
	return &Batcher[T]{
		maxSize:  maxSize,
		interval: interval,
		flushFn:  flushFn,
	}
}

// Add adds an item to the batch. May trigger a flush.
func (b *Batcher[T]) Add(item T) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.stopped {
		return
	}

	b.items = append(b.items, item)

	if len(b.items) >= b.maxSize {
		b.flushLocked()
		return
	}

	// Start timer on first item
	if len(b.items) == 1 {
		b.timer = time.AfterFunc(b.interval, func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			if !b.stopped && len(b.items) > 0 {
				b.flushLocked()
			}
		})
	}
}

// Flush forces a flush of any pending items.
func (b *Batcher[T]) Flush() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.items) > 0 {
		b.flushLocked()
	}
}

// Stop flushes remaining items, waits for in-flight flushes, and prevents future adds.
func (b *Batcher[T]) Stop() {
	b.mu.Lock()
	b.stopped = true
	if b.timer != nil {
		b.timer.Stop()
	}
	if len(b.items) > 0 {
		b.flushLocked()
	}
	b.mu.Unlock()
	b.wg.Wait()
}

func (b *Batcher[T]) flushLocked() {
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	items := b.items
	b.items = nil
	// Run flush outside lock to avoid deadlock
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		b.flushFn(items)
	}()
}
