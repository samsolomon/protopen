// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"sync"
	"time"
)

// deviceTokenStore holds the raw `ptk_` token between device-code approval
// and the CLI's poll-claim step. Single-instance only: multi-replica
// deployments break the flow (see docs/self-hosting.md).
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

func (s *deviceTokenStore) put(code, token string, expiresAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[code] = deviceToken{token: token, expiresAt: expiresAt}
}

// claim atomically reads-and-removes the token. Returns "" if unknown or
// expired. Single-use: subsequent calls return "".
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

func (s *deviceTokenStore) expire(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for code, entry := range s.items {
		if entry.expiresAt.Before(now) {
			delete(s.items, code)
		}
	}
}
