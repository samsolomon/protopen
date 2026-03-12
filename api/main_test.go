package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateUploadAcceptsExtractedZipContents(t *testing.T) {
	t.Parallel()

	payload := uploadRequest{
		Name: "Demo Zip",
		Mode: "zip",
		Files: []fileMeta{
			{Name: "index.html", Path: "index.html", Size: 128},
			{Name: "styles.css", Path: "styles.css", Size: 64},
		},
	}

	if err := validateUpload(payload); err != nil {
		t.Fatalf("expected extracted zip contents to validate, got %v", err)
	}
}

func TestValidateUploadRejectsFolderWithoutIndex(t *testing.T) {
	t.Parallel()

	payload := uploadRequest{
		Name:  "Broken Folder",
		Mode:  "files",
		Files: []fileMeta{{Name: "app.js", Path: "prototype/app.js", Size: 128}},
	}

	err := validateUpload(payload)
	if err == nil {
		t.Fatal("expected missing index.html error")
	}
}

func TestDetectSiteRootPrefersDirectoryWithIndex(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "wrapper", "site", "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "wrapper", "site", "index.html"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "wrapper", "site", "docs", "index.html"), []byte("docs"), 0o644); err != nil {
		t.Fatal(err)
	}

	siteRoot, err := detectSiteRoot(root)
	if err != nil {
		t.Fatalf("detectSiteRoot returned error: %v", err)
	}

	want := filepath.Join(root, "wrapper", "site")
	if siteRoot != want {
		t.Fatalf("expected %q, got %q", want, siteRoot)
	}
}

func TestResolveAssetPathPrefersDirectoryIndex(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("root"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "index.html"), []byte("docs"), 0o644); err != nil {
		t.Fatal(err)
	}

	resolved, fallback, err := resolveAssetPath(root, "docs")
	if err != nil {
		t.Fatalf("resolveAssetPath returned error: %v", err)
	}
	if fallback {
		t.Fatal("expected directory index to resolve without SPA fallback")
	}

	want := filepath.Join(root, "docs", "index.html")
	if resolved != want {
		t.Fatalf("expected %q, got %q", want, resolved)
	}
}

func TestResolveAssetPathFallsBackToRootIndexForSpaRoute(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("root"), 0o644); err != nil {
		t.Fatal(err)
	}

	resolved, fallback, err := resolveAssetPath(root, "dashboard/settings")
	if err != nil {
		t.Fatalf("resolveAssetPath returned error: %v", err)
	}
	if !fallback {
		t.Fatal("expected SPA fallback for missing extensionless route")
	}

	want := filepath.Join(root, "index.html")
	if resolved != want {
		t.Fatalf("expected %q, got %q", want, resolved)
	}
}

func TestParseProjectPath(t *testing.T) {
	t.Parallel()

	username, slug, assetPath, ok := parseProjectPath("/~sam/product-teardown/docs/index.html")
	if !ok {
		t.Fatal("expected parseProjectPath to succeed")
	}
	if username != "sam" || slug != "product-teardown" || assetPath != "docs/index.html" {
		t.Fatalf("unexpected parse result: %q %q %q", username, slug, assetPath)
	}
}

func TestHashTokenIsDeterministic(t *testing.T) {
	t.Parallel()

	first := hashToken("velori-session-token")
	second := hashToken("velori-session-token")
	if first != second {
		t.Fatal("expected token hashes to be deterministic")
	}
	if len(first) != 64 {
		t.Fatalf("expected sha256 hex hash length 64, got %d", len(first))
	}
}
