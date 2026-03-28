package main

import (
	"os"
	"path/filepath"
	"strings"
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

	username, slug, deployID, assetPath, ok := parseProjectPath("/~sam/product-teardown/docs/index.html")
	if !ok {
		t.Fatal("expected parseProjectPath to succeed")
	}
	if username != "sam" || slug != "product-teardown" || deployID != "" || assetPath != "docs/index.html" {
		t.Fatalf("unexpected parse result: %q %q %q %q", username, slug, deployID, assetPath)
	}

	// Versioned path
	username, slug, deployID, assetPath, ok = parseProjectPath("/~sam/product-teardown/_v/dep_abc123/styles.css")
	if !ok {
		t.Fatal("expected versioned parseProjectPath to succeed")
	}
	if username != "sam" || slug != "product-teardown" || deployID != "dep_abc123" || assetPath != "styles.css" {
		t.Fatalf("unexpected versioned parse result: %q %q %q %q", username, slug, deployID, assetPath)
	}

	// Versioned root (no asset)
	_, _, deployID, assetPath, ok = parseProjectPath("/~sam/product-teardown/_v/dep_abc123/")
	if !ok || deployID != "dep_abc123" || assetPath != "" {
		t.Fatalf("unexpected versioned root result: deployID=%q assetPath=%q ok=%v", deployID, assetPath, ok)
	}

	// Invalid: _v without deploy ID
	_, _, _, _, ok = parseProjectPath("/~sam/product-teardown/_v/")
	if ok {
		t.Fatal("expected _v without deploy ID to fail")
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

func TestCountForMatchReturnsOneForNewProject(t *testing.T) {
	t.Parallel()

	got := countForMatch(nil)
	if got != 1 {
		t.Fatalf("expected new project deploy count 1, got %d", got)
	}
}

func TestCountForMatchIncrementsExistingProjectDeploys(t *testing.T) {
	t.Parallel()

	got := countForMatch([]projectRecord{{Deploys: 4}})
	if got != 5 {
		t.Fatalf("expected redeploy count 5, got %d", got)
	}
}

func TestValidateProjectMatchCountAllowsZeroAndOneMatch(t *testing.T) {
	t.Parallel()

	if err := validateProjectMatchCount(nil, "Prototype"); err != nil {
		t.Fatalf("expected zero matches to be allowed, got %v", err)
	}

	if err := validateProjectMatchCount([]projectRecord{{ID: "proj_1"}}, "Prototype"); err != nil {
		t.Fatalf("expected one exact match to be allowed, got %v", err)
	}
}

func TestValidateProjectMatchCountRejectsMultipleMatches(t *testing.T) {
	t.Parallel()

	err := validateProjectMatchCount([]projectRecord{{ID: "proj_1"}, {ID: "proj_2"}}, "Prototype")
	if err == nil {
		t.Fatal("expected multiple exact matches to be rejected")
	}
	if !strings.Contains(err.Error(), "multiple existing projects match") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateSeedDeployFilesUsesProjectRelativeLinks(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	siteRoot, err := createSeedDeployFiles(root, "sam", "product-teardown", 0)
	if err != nil {
		t.Fatalf("createSeedDeployFiles returned error: %v", err)
	}

	indexHTML, err := os.ReadFile(filepath.Join(siteRoot, "index.html"))
	if err != nil {
		t.Fatalf("read seeded index.html: %v", err)
	}

	markup := string(indexHTML)
	if strings.Contains(markup, "href=\"/docs\"") {
		t.Fatal("expected seeded docs link to be project-relative")
	}
	if !strings.Contains(markup, "href=\"docs\"") {
		t.Fatal("expected seeded page to include project-relative docs link")
	}
}
