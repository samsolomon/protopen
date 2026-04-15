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

func (app *application) rateLimit(rl *rateLimiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if idx := strings.LastIndex(ip, ":"); idx != -1 {
			ip = ip[:idx]
		}
		if cfIP := r.Header.Get("CF-Connecting-IP"); cfIP != "" {
			ip = strings.TrimSpace(cfIP)
		} else if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			ip = strings.TrimSpace(strings.Split(forwarded, ",")[0])
		}

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
