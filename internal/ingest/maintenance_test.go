package ingest

import (
	"context"
	"testing"
	"time"
)

// TestMaintenanceStepIndependentBudgets verifies that each maintenance step gets
// its own timeout budget. A step that consumes its entire budget (simulating a
// slow state-table decimation) must not leave the following step with an
// already-expired context — the regression that caused later retention cleanup
// steps to be silently skipped.
func TestMaintenanceStepIndependentBudgets(t *testing.T) {
	p := &Pipeline{ctx: context.Background()}

	// Step 1: a slow step that blocks until its own (short) deadline elapses.
	var step1Err error
	p.maintenanceStep(50*time.Millisecond, func(ctx context.Context) {
		<-ctx.Done()
		step1Err = ctx.Err()
	})
	if step1Err != context.DeadlineExceeded {
		t.Fatalf("step 1: expected DeadlineExceeded after exhausting its budget, got %v", step1Err)
	}

	// Step 2: must receive a fresh, unexpired context with its full budget,
	// proving budgets are independent rather than shared across the run.
	var step2Err error
	var step2Remaining time.Duration
	p.maintenanceStep(5*time.Second, func(ctx context.Context) {
		step2Err = ctx.Err()
		if dl, ok := ctx.Deadline(); ok {
			step2Remaining = time.Until(dl)
		} else {
			t.Error("step 2: context unexpectedly has no deadline")
		}
	})
	if step2Err != nil {
		t.Fatalf("step 2: context was already expired (starved by step 1): %v", step2Err)
	}
	if step2Remaining < 4*time.Second {
		t.Fatalf("step 2: budget was starved, only %v remained (expected ~5s)", step2Remaining)
	}
}

// TestMaintenanceStepHonorsShutdown verifies that step contexts derive from the
// pipeline's lifetime context, so a shutdown (cancelled p.ctx) immediately
// cancels in-flight steps instead of letting them run for the full timeout.
func TestMaintenanceStepHonorsShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // simulate pipeline shutdown

	p := &Pipeline{ctx: ctx}

	var got error
	p.maintenanceStep(5*time.Second, func(stepCtx context.Context) {
		got = stepCtx.Err()
	})
	if got == nil {
		t.Fatal("expected step context to be cancelled when the pipeline context is cancelled")
	}
}
