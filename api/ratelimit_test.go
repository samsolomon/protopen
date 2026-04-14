package main

import (
	"testing"
	"time"
)

func TestRateLimiterAllowsWithinLimit(t *testing.T) {
	t.Parallel()

	rl := newRateLimiter(3, time.Second)

	for i := 0; i < 3; i++ {
		allowed, _ := rl.allow("1.2.3.4")
		if !allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
}

func TestRateLimiterBlocksOverLimit(t *testing.T) {
	t.Parallel()

	rl := newRateLimiter(2, time.Second)
	rl.allow("1.2.3.4")
	rl.allow("1.2.3.4")

	allowed, retryAfter := rl.allow("1.2.3.4")
	if allowed {
		t.Fatal("third request should be blocked")
	}
	if retryAfter <= 0 {
		t.Fatal("expected positive retryAfter duration")
	}
}

func TestRateLimiterIsolatesIPs(t *testing.T) {
	t.Parallel()

	rl := newRateLimiter(1, time.Second)
	rl.allow("1.1.1.1")

	allowed, _ := rl.allow("2.2.2.2")
	if !allowed {
		t.Fatal("different IP should have its own limit")
	}
}

func TestRateLimiterResetsAfterWindow(t *testing.T) {
	t.Parallel()

	rl := newRateLimiter(1, 50*time.Millisecond)
	rl.allow("1.2.3.4")

	allowed, _ := rl.allow("1.2.3.4")
	if allowed {
		t.Fatal("second request should be blocked within window")
	}

	time.Sleep(60 * time.Millisecond)

	allowed, _ = rl.allow("1.2.3.4")
	if !allowed {
		t.Fatal("request should be allowed after window expires")
	}
}

func TestRateLimiterCleanupRemovesStaleEntries(t *testing.T) {
	t.Parallel()

	rl := newRateLimiter(10, 50*time.Millisecond)
	rl.allow("stale-ip")

	time.Sleep(60 * time.Millisecond)
	rl.allow("active-ip")

	rl.cleanup()

	rl.mu.Lock()
	_, hasStale := rl.requests["stale-ip"]
	_, hasActive := rl.requests["active-ip"]
	rl.mu.Unlock()

	if hasStale {
		t.Fatal("expected stale entry to be cleaned up")
	}
	if !hasActive {
		t.Fatal("expected active entry to be preserved")
	}
}
