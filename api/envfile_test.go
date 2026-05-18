// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDotenvStripsCommentsAndQuotes(t *testing.T) {
	t.Parallel()

	body := strings.NewReader(`# a comment
DATABASE_URL=postgres://x@y/z
APP_ORIGIN="http://localhost:8080"
INGEST_ROOT='/tmp/ingest'
THUMBNAILS_ENABLED=1 # inline note
export SEED_DEMO=1

# trailing blank
`)
	got, err := parseDotenv(body)
	if err != nil {
		t.Fatalf("parseDotenv: %v", err)
	}
	want := map[string]string{
		"DATABASE_URL":       "postgres://x@y/z",
		"APP_ORIGIN":         "http://localhost:8080",
		"INGEST_ROOT":        "/tmp/ingest",
		"THUMBNAILS_ENABLED": "1",
		"SEED_DEMO":          "1",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d pairs, want %d: %v", len(got), len(want), got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestParseDotenvRejectsMissingEquals(t *testing.T) {
	t.Parallel()

	_, err := parseDotenv(strings.NewReader("FOO\nBAR=baz\n"))
	if err == nil {
		t.Fatal("expected error for line with no `=`")
	}
}

func TestParseDotenvRejectsInvalidKey(t *testing.T) {
	t.Parallel()

	_, err := parseDotenv(strings.NewReader("1FOO=bar\n"))
	if err == nil {
		t.Fatal("expected error for key starting with digit")
	}
}

func TestLoadDotenvDoesNotOverwriteExistingEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	body := "DOTENV_TEST_OVERRIDE=from-file\nDOTENV_TEST_NEW=from-file\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	t.Setenv("DOTENV_TEST_OVERRIDE", "from-shell")
	if err := os.Unsetenv("DOTENV_TEST_NEW"); err != nil {
		t.Fatalf("unset: %v", err)
	}
	t.Cleanup(func() { os.Unsetenv("DOTENV_TEST_NEW") })

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := loadDotenv(); err != nil {
		t.Fatalf("loadDotenv: %v", err)
	}
	if got := os.Getenv("DOTENV_TEST_OVERRIDE"); got != "from-shell" {
		t.Errorf("existing env should win: got %q, want from-shell", got)
	}
	if got := os.Getenv("DOTENV_TEST_NEW"); got != "from-file" {
		t.Errorf("unset key should be filled from file: got %q, want from-file", got)
	}
}

func TestLoadDotenvWalksUpToParent(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "api")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("DOTENV_TEST_PARENT=ok\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	t.Cleanup(func() { os.Unsetenv("DOTENV_TEST_PARENT") })

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(subdir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := loadDotenv(); err != nil {
		t.Fatalf("loadDotenv: %v", err)
	}
	if got := os.Getenv("DOTENV_TEST_PARENT"); got != "ok" {
		t.Errorf("expected parent-dir .env to load, got %q", got)
	}
}

func TestLoadDotenvMissingFileIsNoError(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if err := loadDotenv(); err != nil {
		t.Errorf("loadDotenv on missing file should be nil, got %v", err)
	}
}
