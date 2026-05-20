// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseMentions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body string
		want []string
	}{
		{"none", "no mentions here", nil},
		{"empty", "", nil},
		{"single", "hey @admin look", []string{"admin"}},
		{"at start", "@admin first thing", []string{"admin"}},
		{"multiple", "@admin and @owner please", []string{"admin", "owner"}},
		{"dedup case-insensitive", "@Bob then @bob again", []string{"bob"}},
		{"ignores email", "mail foo@bar.com here", nil},
		{"too short", "@a is too short", nil},
		// The regex caps the captured username at 32 chars; a longer run
		// yields a 32-char match rather than being rejected outright.
		{"caps at 32 chars", "@" + strings.Repeat("a", 40), []string{strings.Repeat("a", 32)}},
		{"hyphen and underscore", "@jane-doe and @jane_doe", []string{"jane-doe", "jane_doe"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := parseMentions(tc.body)
			if !equalUnorderedStrings(got, tc.want) {
				t.Fatalf("parseMentions(%q) = %v, want %v", tc.body, got, tc.want)
			}
		})
	}
}

func TestClampSelector(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", 250)

	if got := clampSelector(""); got != nil {
		t.Fatalf("empty: expected nil, got %q", *got)
	}
	if got := clampSelector("   "); got != nil {
		t.Fatalf("whitespace: expected nil, got %q", *got)
	}
	if got := clampSelector("  .foo > #bar  "); got == nil || *got != ".foo > #bar" {
		t.Fatalf("normal: expected trimmed %q, got %v", ".foo > #bar", got)
	}
	if got := clampSelector(long); got == nil || len(*got) != 200 {
		t.Fatalf("long: expected truncation to 200 chars, got len=%v", got)
	}
}

func TestRequireRuntimeOrigin(t *testing.T) {
	t.Parallel()
	app := &application{appOrigin: "https://app.test", frontendOrigin: "https://front.test"}

	cases := []struct {
		name          string
		origin        string
		clientHeader  string
		wantProceed   bool
		wantStatus    int
	}{
		{"empty origin passes", "", "", true, http.StatusOK},
		{"app origin passes", "https://app.test", "", true, http.StatusOK},
		{"frontend origin passes", "https://front.test", "", true, http.StatusOK},
		{"cross-origin without header is rejected", "https://evil.test", "", false, http.StatusForbidden},
		{"cross-origin with runtime header passes", "https://evil.test", "runtime", true, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodPost, "/api/sites/s/comments", nil)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.clientHeader != "" {
				req.Header.Set("X-Protopen-Client", tc.clientHeader)
			}
			rec := httptest.NewRecorder()
			got := app.requireRuntimeOrigin(rec, req)
			if got != tc.wantProceed {
				t.Fatalf("requireRuntimeOrigin proceed = %v, want %v", got, tc.wantProceed)
			}
			if !tc.wantProceed && rec.Code != tc.wantStatus {
				t.Fatalf("expected status %d on rejection, got %d", tc.wantStatus, rec.Code)
			}
		})
	}
}
