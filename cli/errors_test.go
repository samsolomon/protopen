// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCategorizeMapsStatusToType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		status int
		target any
	}{
		{401, new(*AuthError)},
		{403, new(*ForbiddenError)},
		{404, new(*NotFoundError)},
		{400, new(*ValidationError)},
		{409, new(*ValidationError)},
		{500, new(*ServerError)},
		{502, new(*ServerError)},
	}
	for _, tc := range cases {
		err := categorize(tc.status, "msg")
		if !errors.As(err, tc.target) {
			t.Errorf("status %d: errors.As did not match expected type", tc.status)
		}
	}
}

func TestCategorizePreservesMessage(t *testing.T) {
	t.Parallel()

	err := categorize(401, "unauthorized — check your PROTOPEN_TOKEN")
	if err.Error() != "unauthorized — check your PROTOPEN_TOKEN" {
		t.Fatalf("message not preserved: %q", err.Error())
	}
}

func TestClientReturnsTypedAuthErrorOn401(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	c := newClient("ptk_bad", server.URL)
	_, err := c.listSites("")
	if err == nil {
		t.Fatal("expected error")
	}
	var authErr *AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *AuthError, got %T: %v", err, err)
	}
}

func TestClientReturnsTypedNotFoundFromFindSite(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"sites":[]}`))
	}))
	defer server.Close()

	c := newClient("ptk_test", server.URL)
	_, err := c.findSite("absent", "")
	if err == nil {
		t.Fatal("expected error")
	}
	var nf *NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("expected *NotFoundError, got %T: %v", err, err)
	}
}

func TestClientReturnsTypedValidationOn400(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"deploy not found"}`))
	}))
	defer server.Close()

	c := newClient("ptk_test", server.URL)
	err := c.rollback("proj_abc", "dep_missing")
	if err == nil {
		t.Fatal("expected error")
	}
	var v *ValidationError
	if !errors.As(err, &v) {
		t.Fatalf("expected *ValidationError, got %T: %v", err, err)
	}
}
