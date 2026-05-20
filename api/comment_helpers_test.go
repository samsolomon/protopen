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

func TestNormalizeAnchor(t *testing.T) {
	t.Parallel()
	parent := "cm_parent"
	custom := 0.25

	cases := []struct {
		name     string
		selector string
		offX     *float64
		offY     *float64
		parent   *string
		wantSel  *string
		wantX    *float64
		wantY    *float64
	}{
		{
			name:    "root with explicit anchor preserved",
			selector: ".cta", offX: &custom, offY: &custom, parent: nil,
			wantSel: ptr(".cta"), wantX: &custom, wantY: &custom,
		},
		{
			name:    "root missing selector defaults to body at center",
			selector: "", offX: nil, offY: nil, parent: nil,
			wantSel: ptr("body"), wantX: fptr(0.5), wantY: fptr(0.5),
		},
		{
			name:    "root missing one offset defaults that axis only",
			selector: ".cta", offX: &custom, offY: nil, parent: nil,
			wantSel: ptr(".cta"), wantX: &custom, wantY: fptr(0.5),
		},
		{
			name:    "reply gets no anchor regardless of payload",
			selector: ".cta", offX: &custom, offY: &custom, parent: &parent,
			wantSel: nil, wantX: nil, wantY: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sel, x, y := normalizeAnchor(tc.selector, tc.offX, tc.offY, tc.parent)
			if !strPtrEqual(sel, tc.wantSel) {
				t.Fatalf("selector: got %v want %v", strPtrDeref(sel), strPtrDeref(tc.wantSel))
			}
			if !floatPtrEqual(x, tc.wantX) {
				t.Fatalf("offsetX: got %v want %v", floatPtrDeref(x), floatPtrDeref(tc.wantX))
			}
			if !floatPtrEqual(y, tc.wantY) {
				t.Fatalf("offsetY: got %v want %v", floatPtrDeref(y), floatPtrDeref(tc.wantY))
			}
		})
	}
}

func ptr(s string) *string   { return &s }
func fptr(f float64) *float64 { return &f }
func strPtrEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
func strPtrDeref(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}
func floatPtrEqual(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
func floatPtrDeref(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
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
