// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestDeviceTokenStorePutAndClaim(t *testing.T) {
	t.Parallel()
	s := newDeviceTokenStore()
	future := time.Now().Add(10 * time.Minute)
	s.put("code-a", "ptk_aaa", future)
	if got := s.claim("code-a", time.Now()); got != "ptk_aaa" {
		t.Fatalf("expected ptk_aaa, got %q", got)
	}
	// Second claim returns empty (single-use).
	if got := s.claim("code-a", time.Now()); got != "" {
		t.Fatalf("expected empty on second claim, got %q", got)
	}
}

func TestDeviceTokenStoreUnknownCode(t *testing.T) {
	t.Parallel()
	s := newDeviceTokenStore()
	if got := s.claim("never-put", time.Now()); got != "" {
		t.Fatalf("expected empty for unknown code, got %q", got)
	}
}

func TestDeviceTokenStoreExpired(t *testing.T) {
	t.Parallel()
	s := newDeviceTokenStore()
	past := time.Now().Add(-1 * time.Minute)
	s.put("code-b", "ptk_bbb", past)
	if got := s.claim("code-b", time.Now()); got != "" {
		t.Fatalf("expected empty for expired code, got %q", got)
	}
}

func TestDeviceTokenStoreExpireSweep(t *testing.T) {
	t.Parallel()
	s := newDeviceTokenStore()
	now := time.Now()
	s.put("alive", "ptk_alive", now.Add(10*time.Minute))
	s.put("dead", "ptk_dead", now.Add(-1*time.Minute))
	s.expire(now)
	if got := s.claim("alive", now); got != "ptk_alive" {
		t.Fatalf("alive should still be claimable, got %q", got)
	}
	if got := s.claim("dead", now); got != "" {
		t.Fatalf("dead should have been swept, got %q", got)
	}
}

func TestDeviceTokenStoreConcurrent(t *testing.T) {
	t.Parallel()
	s := newDeviceTokenStore()
	future := time.Now().Add(time.Minute)

	const n = 200
	for i := 0; i < n; i++ {
		s.put(strconv.Itoa(i), "ptk_"+strconv.Itoa(i), future)
	}

	// Race: many goroutines try to claim each code; exactly one wins per code.
	var wg sync.WaitGroup
	results := make(chan string, n*4)
	for g := 0; g < 4; g++ {
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(code string) {
				defer wg.Done()
				if t := s.claim(code, time.Now()); t != "" {
					results <- code
				}
			}(strconv.Itoa(i))
		}
	}
	wg.Wait()
	close(results)

	seen := make(map[string]int)
	for code := range results {
		seen[code]++
	}
	for code, count := range seen {
		if count != 1 {
			t.Fatalf("code %s was claimed %d times", code, count)
		}
	}
	if len(seen) != n {
		t.Fatalf("expected %d codes claimed exactly once, got %d", n, len(seen))
	}
}
