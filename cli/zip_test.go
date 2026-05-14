// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestZipDirectoryIncludesAllFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "index.html"), "<h1>hello</h1>")
	writeFile(t, filepath.Join(dir, "styles.css"), "body {}")
	os.MkdirAll(filepath.Join(dir, "docs"), 0o755)
	writeFile(t, filepath.Join(dir, "docs", "index.html"), "<h1>docs</h1>")

	data, err := zipDirectory(dir)
	if err != nil {
		t.Fatalf("zipDirectory: %v", err)
	}

	entries := zipEntryNames(t, data)
	sort.Strings(entries)

	base := filepath.Base(dir)
	want := []string{
		base + "/docs/index.html",
		base + "/index.html",
		base + "/styles.css",
	}

	if len(entries) != len(want) {
		t.Fatalf("expected %d entries, got %d: %v", len(want), len(entries), entries)
	}
	for i := range want {
		if entries[i] != want[i] {
			t.Fatalf("entry %d: expected %q, got %q", i, want[i], entries[i])
		}
	}
}

func TestZipDirectoryPreservesContent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	content := "<!doctype html><html><body>preserved</body></html>"
	writeFile(t, filepath.Join(dir, "index.html"), content)

	data, err := zipDirectory(dir)
	if err != nil {
		t.Fatalf("zipDirectory: %v", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}

	if len(reader.File) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(reader.File))
	}

	rc, err := reader.File[0].Open()
	if err != nil {
		t.Fatalf("open entry: %v", err)
	}
	defer rc.Close()

	buf := new(bytes.Buffer)
	buf.ReadFrom(rc)
	if buf.String() != content {
		t.Fatalf("content mismatch: got %q", buf.String())
	}
}

func TestZipDirectoryUsesBaseNameAsRoot(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "mysite")
	os.MkdirAll(dir, 0o755)
	writeFile(t, filepath.Join(dir, "index.html"), "<h1>test</h1>")

	data, err := zipDirectory(dir)
	if err != nil {
		t.Fatalf("zipDirectory: %v", err)
	}

	entries := zipEntryNames(t, data)
	if len(entries) != 1 || entries[0] != "mysite/index.html" {
		t.Fatalf("expected [mysite/index.html], got %v", entries)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func zipEntryNames(t *testing.T, data []byte) []string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}
	var names []string
	for _, f := range reader.File {
		names = append(names, f.Name)
	}
	return names
}
