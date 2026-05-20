// SPDX-License-Identifier: AGPL-3.0-only

package main

import "fmt"

// Typed error categories so non-CLI surfaces (MCP) can react programmatically
// without parsing human-readable strings. Each wrapper preserves the original
// message verbatim so existing CLI output and tests are unchanged, and carries
// an optional cause so callers can use errors.Unwrap / errors.Is to reach the
// underlying transport or decode error.

// AuthError wraps authentication failures (HTTP 401, missing token).
type AuthError struct {
	Msg string
	Err error
}

func (e *AuthError) Error() string { return e.Msg }
func (e *AuthError) Unwrap() error { return e.Err }

// NotFoundError wraps "site not found", "deploy not found", etc.
type NotFoundError struct {
	Msg string
	Err error
}

func (e *NotFoundError) Error() string { return e.Msg }
func (e *NotFoundError) Unwrap() error { return e.Err }

// ValidationError wraps HTTP 400 responses and client-side input rejections.
type ValidationError struct {
	Msg string
	Err error
}

func (e *ValidationError) Error() string { return e.Msg }
func (e *ValidationError) Unwrap() error { return e.Err }

// ForbiddenError wraps HTTP 403 responses.
type ForbiddenError struct {
	Msg string
	Err error
}

func (e *ForbiddenError) Error() string { return e.Msg }
func (e *ForbiddenError) Unwrap() error { return e.Err }

// NetworkError wraps transport-level failures (DNS, connection refused, TLS).
type NetworkError struct {
	Msg string
	Err error
}

func (e *NetworkError) Error() string { return e.Msg }
func (e *NetworkError) Unwrap() error { return e.Err }

// ServerError wraps HTTP 5xx and other unexpected upstream failures.
type ServerError struct {
	Msg string
	Err error
}

func (e *ServerError) Error() string { return e.Msg }
func (e *ServerError) Unwrap() error { return e.Err }

// categorize returns a typed error appropriate for the given HTTP status code,
// using msg as the human-readable message. The Error() string of the returned
// error equals msg verbatim, so existing call sites and tests that compare
// strings are unaffected. Non-error statuses (< 400) are treated as a server
// bug rather than silently labeled — callers should not pass 2xx/3xx here.
func categorize(status int, msg string) error {
	switch {
	case status == 401:
		return &AuthError{Msg: msg}
	case status == 403:
		return &ForbiddenError{Msg: msg}
	case status == 404:
		return &NotFoundError{Msg: msg}
	case status >= 400 && status < 500:
		return &ValidationError{Msg: msg}
	case status >= 500:
		return &ServerError{Msg: msg}
	}
	return &ServerError{Msg: fmt.Sprintf("unexpected status %d: %s", status, msg)}
}
