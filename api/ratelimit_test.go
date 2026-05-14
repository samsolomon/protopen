// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"net/http/httptest"
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

func TestClientIPSource(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		trustedHeader string
		headers       map[string]string
		remoteAddr    string
		want          string
	}{
		{
			name:       "no trusted header falls back to remote addr",
			headers:    map[string]string{"X-Forwarded-For": "9.9.9.9"},
			remoteAddr: "1.2.3.4:5555",
			want:       "1.2.3.4",
		},
		{
			name:          "XFF trusted, leftmost wins",
			trustedHeader: "X-Forwarded-For",
			headers:       map[string]string{"X-Forwarded-For": "9.9.9.9, 10.0.0.1"},
			remoteAddr:    "1.2.3.4:5555",
			want:          "9.9.9.9",
		},
		{
			name:          "Cf-Connecting-Ip trusted",
			trustedHeader: "Cf-Connecting-Ip",
			headers:       map[string]string{"Cf-Connecting-Ip": "9.9.9.9"},
			remoteAddr:    "1.2.3.4:5555",
			want:          "9.9.9.9",
		},
		{
			name:          "trusted header empty falls back to remote addr",
			trustedHeader: "X-Forwarded-For",
			headers:       map[string]string{},
			remoteAddr:    "1.2.3.4:5555",
			want:          "1.2.3.4",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = tc.remoteAddr
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			got := clientIP(req, tc.trustedHeader)
			if got != tc.want {
				t.Fatalf("clientIP = %q, want %q", got, tc.want)
			}
		})
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
