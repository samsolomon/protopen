// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// withChromiumStubs swaps the package-level lookup hooks for the duration of a
// test and restores them on cleanup. Tests use this rather than touching the
// real filesystem or PATH.
func withChromiumStubs(t *testing.T, goos string, lookPath func(string) (string, error), stat func(string) (os.FileInfo, error)) {
	t.Helper()
	origGOOS, origLookPath, origStat := chromiumGOOS, chromiumLookPath, chromiumStat
	chromiumGOOS = goos
	chromiumLookPath = lookPath
	chromiumStat = stat
	t.Cleanup(func() {
		chromiumGOOS = origGOOS
		chromiumLookPath = origLookPath
		chromiumStat = origStat
	})
}

func notFoundLookPath(string) (string, error) {
	return "", &exec.Error{Name: "x", Err: exec.ErrNotFound}
}

func notFoundStat(string) (os.FileInfo, error) {
	return nil, &fs.PathError{Op: "stat", Path: "x", Err: fs.ErrNotExist}
}

func TestResolveChromiumPathPrefersEnvOverride(t *testing.T) {
	t.Setenv("CHROMIUM_PATH", "/custom/chromium")
	withChromiumStubs(t, "linux", notFoundLookPath, notFoundStat)

	if got := resolveChromiumPath(); got != "/custom/chromium" {
		t.Errorf("got %q, want /custom/chromium", got)
	}
}

func TestResolveChromiumPathUsesPATHWhenAvailable(t *testing.T) {
	t.Setenv("CHROMIUM_PATH", "")
	withChromiumStubs(t, "linux", func(name string) (string, error) {
		if name == "google-chrome" {
			return "/usr/bin/google-chrome", nil
		}
		return "", &exec.Error{Name: name, Err: exec.ErrNotFound}
	}, notFoundStat)

	if got := resolveChromiumPath(); got != "/usr/bin/google-chrome" {
		t.Errorf("got %q, want /usr/bin/google-chrome", got)
	}
}

func TestResolveChromiumPathFallsBackToDarwinAppBundle(t *testing.T) {
	t.Setenv("CHROMIUM_PATH", "")
	const chromePath = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	withChromiumStubs(t, "darwin", notFoundLookPath, func(name string) (os.FileInfo, error) {
		if name == chromePath {
			// Real FileInfo isn't needed by callers; the only check is err == nil.
			return fakeFileInfo{}, nil
		}
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
	})

	if got := resolveChromiumPath(); got != chromePath {
		t.Errorf("got %q, want %q", got, chromePath)
	}
}

func TestResolveChromiumPathSkipsDarwinProbeOnLinux(t *testing.T) {
	t.Setenv("CHROMIUM_PATH", "")
	withChromiumStubs(t, "linux", notFoundLookPath, func(string) (os.FileInfo, error) {
		t.Fatal("stat should not be called on non-darwin hosts")
		return nil, nil
	})

	if got := resolveChromiumPath(); got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestResolveChromiumPathReturnsEmptyWhenNothingFound(t *testing.T) {
	t.Setenv("CHROMIUM_PATH", "")
	withChromiumStubs(t, "darwin", notFoundLookPath, notFoundStat)

	if got := resolveChromiumPath(); got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestDarwinChromiumCandidatesIncludesUserApplications(t *testing.T) {
	t.Setenv("HOME", "/Users/test")
	candidates := darwinChromiumCandidates()
	wantUser := "/Users/test/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	var found bool
	for _, c := range candidates {
		if c == wantUser {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("candidates missing %q: %v", wantUser, candidates)
	}
}

func TestDarwinChromiumCandidatesOmitsUserPathsWhenHomeUnset(t *testing.T) {
	t.Setenv("HOME", "")
	for _, c := range darwinChromiumCandidates() {
		if strings.Contains(c, "Applications/Google Chrome.app") && !strings.HasPrefix(c, "/Applications/") {
			t.Errorf("unexpected user-home candidate when HOME is unset: %q", c)
		}
	}
}

// fakeFileInfo satisfies os.FileInfo for stub returns. The chromium probe only
// checks err == nil, so the fields can be zero-valued.
type fakeFileInfo struct{}

func (fakeFileInfo) Name() string       { return "" }
func (fakeFileInfo) Size() int64        { return 0 }
func (fakeFileInfo) Mode() os.FileMode  { return 0 }
func (fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (fakeFileInfo) IsDir() bool        { return false }
func (fakeFileInfo) Sys() any           { return nil }
