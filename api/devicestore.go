// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"sync"
	"time"
)

// deviceTokenStore holds the raw `ptk_` API token between a successful
// device-code approval and the CLI's poll-claim step. The token lives only
// in memory — never persisted — so a compromise of the database can't expose
// in-flight tokens.
//
// Single-instance only. With multiple API replicas the approve and poll
// could land on different processes; see docs/self-hosting.md.
type deviceTokenStore struct {
	mu    sync.Mutex
	items map[string]deviceToken
}

type deviceToken struct {
	token     string
	expiresAt time.Time
}

func newDeviceTokenStore() *deviceTokenStore {
	return &deviceTokenStore{items: make(map[string]deviceToken)}
}

// put records a token for the given device code with an expiry. Overwrites
// any existing entry; approval is a single-writer step.
func (s *deviceTokenStore) put(code, token string, expiresAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[code] = deviceToken{token: token, expiresAt: expiresAt}
}

// claim atomically reads-and-removes the token for the given code. Returns
// the empty string if the code is unknown or expired. Subsequent calls with
// the same code return empty (single-use).
func (s *deviceTokenStore) claim(code string, now time.Time) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.items[code]
	if !ok {
		return ""
	}
	delete(s.items, code)
	if entry.expiresAt.Before(now) {
		return ""
	}
	return entry.token
}

// expire drops any entries older than now. Called from the periodic cleanup
// loop so the map can't grow unbounded if codes are approved but never polled.
func (s *deviceTokenStore) expire(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for code, entry := range s.items {
		if entry.expiresAt.Before(now) {
			delete(s.items, code)
		}
	}
}
