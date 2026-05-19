// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVerifyOriginAllowsSameOrigin(t *testing.T) {
	t.Parallel()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := verifyOrigin([]string{"https://app.example.com"}, next)

	req := httptest.NewRequest(http.MethodPatch, "/api/account/profile", strings.NewReader(`{}`))
	req.Header.Set("Origin", "https://app.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("same-origin PATCH should pass, got %d", rec.Code)
	}
}

func TestVerifyOriginRejectsCrossOrigin(t *testing.T) {
	t.Parallel()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("next handler should not have been called")
	})
	h := verifyOrigin([]string{"https://app.example.com"}, next)

	req := httptest.NewRequest(http.MethodPost, "/api/sign-out", strings.NewReader(`{}`))
	req.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-origin POST should 403, got %d", rec.Code)
	}
}

func TestVerifyOriginAllowsEmptyOrigin(t *testing.T) {
	t.Parallel()
	// CLI / scripts authenticating via Bearer token don't send Origin.
	// Cookie-auth is protected separately by SameSite=Lax.
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := verifyOrigin([]string{"https://app.example.com"}, next)

	req := httptest.NewRequest(http.MethodDelete, "/api/sites/abc", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("empty-origin DELETE should pass, got %d", rec.Code)
	}
}

func TestVerifyOriginSkipsForLocalDev(t *testing.T) {
	t.Parallel()
	// When appOrigin is empty (local dev), the middleware short-circuits
	// regardless of Origin header.
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := verifyOrigin(nil, next)

	req := httptest.NewRequest(http.MethodPost, "/api/sign-in", strings.NewReader(`{}`))
	req.Header.Set("Origin", "https://anything.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("empty appOrigin should allow any origin, got %d", rec.Code)
	}
}

func TestVerifyOriginIgnoresSafeMethods(t *testing.T) {
	t.Parallel()
	// GET requests are not state-changing; cross-origin GETs are
	// expected (e.g. preflight-less fetches for public resources).
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := verifyOrigin([]string{"https://app.example.com"}, next)

	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET should bypass origin check, got %d", rec.Code)
	}
}
