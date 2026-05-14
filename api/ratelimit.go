// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type rateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limit    int
	window   time.Duration
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
}

func (rl *rateLimiter) allow(ip string) (bool, time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-rl.window)

	timestamps := rl.requests[ip]
	valid := timestamps[:0]
	for _, ts := range timestamps {
		if ts.After(windowStart) {
			valid = append(valid, ts)
		}
	}

	if len(valid) >= rl.limit {
		retryAfter := valid[0].Add(rl.window).Sub(now)
		rl.requests[ip] = valid
		return false, retryAfter
	}

	valid = append(valid, now)
	rl.requests[ip] = valid
	return true, 0
}

func (rl *rateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	windowStart := time.Now().Add(-rl.window)
	for ip, timestamps := range rl.requests {
		active := false
		for _, ts := range timestamps {
			if ts.After(windowStart) {
				active = true
				break
			}
		}
		if !active {
			delete(rl.requests, ip)
		}
	}
}

func (rl *rateLimiter) startCleanup(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	go func() {
		for {
			select {
			case <-ticker.C:
				rl.cleanup()
			case <-ctx.Done():
				ticker.Stop()
				return
			}
		}
	}()
}

// clientIP resolves the rate-limit key for a request. If trustedHeader is set
// (canonical form, e.g. "X-Forwarded-For" or "Cf-Connecting-Ip"), the value of
// that header is used — operators must only set this behind a proxy that
// strips client-supplied values for the same header. Otherwise the connection
// remote-addr is used. Falls back to remote-addr when the trusted header is
// missing or empty.
func clientIP(r *http.Request, trustedHeader string) string {
	if trustedHeader != "" {
		if v := strings.TrimSpace(r.Header.Get(trustedHeader)); v != "" {
			// XFF can be a comma-separated chain; take the leftmost (original client).
			if comma := strings.IndexByte(v, ','); comma != -1 {
				v = strings.TrimSpace(v[:comma])
			}
			if v != "" {
				return v
			}
		}
	}
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return ip
}

func (app *application) rateLimit(rl *rateLimiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r, app.trustedProxyHeader)

		allowed, retryAfter := rl.allow(ip)
		if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
			writeJSON(w, http.StatusTooManyRequests, map[string]string{
				"error": "rate limit exceeded, try again later",
			})
			return
		}
		next(w, r)
	}
}
