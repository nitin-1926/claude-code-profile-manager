package lock

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestAcquireRelease(t *testing.T) {
	lp := filepath.Join(t.TempDir(), ".lock")

	h, err := Acquire(lp, DefaultTimeout)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if err := h.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	// Re-acquire after release must succeed.
	h2, err := Acquire(lp, DefaultTimeout)
	if err != nil {
		t.Fatalf("re-Acquire: %v", err)
	}
	_ = h2.Release()
}

func TestReleaseIsIdempotent(t *testing.T) {
	lp := filepath.Join(t.TempDir(), ".lock")
	h, err := Acquire(lp, DefaultTimeout)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if err := h.Release(); err != nil {
		t.Fatalf("first Release: %v", err)
	}
	if err := h.Release(); err != nil {
		t.Fatalf("second Release should be a no-op, got: %v", err)
	}
}

func TestAcquireTimesOutWhenHeld(t *testing.T) {
	lp := filepath.Join(t.TempDir(), ".lock")
	h, err := Acquire(lp, DefaultTimeout)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer h.Release()

	start := time.Now()
	if _, err := Acquire(lp, 200*time.Millisecond); err == nil {
		t.Fatal("expected timeout acquiring an already-held lock, got nil")
	}
	if elapsed := time.Since(start); elapsed < 150*time.Millisecond {
		t.Fatalf("returned too fast (%s); did it actually wait for the timeout?", elapsed)
	}
}

func TestGuardSerializesAccess(t *testing.T) {
	lp := filepath.Join(t.TempDir(), ".lock")

	var mu sync.Mutex
	inside := 0
	maxInside := 0
	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = Guard(lp, 5*time.Second, func() error {
				mu.Lock()
				inside++
				if inside > maxInside {
					maxInside = inside
				}
				mu.Unlock()

				time.Sleep(5 * time.Millisecond)

				mu.Lock()
				inside--
				mu.Unlock()
				return nil
			})
		}()
	}
	wg.Wait()

	if maxInside != 1 {
		t.Fatalf("Guard allowed %d concurrent holders; expected strict serialization (1)", maxInside)
	}
}
