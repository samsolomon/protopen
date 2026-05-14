package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestParseSitePath(t *testing.T) {
	t.Parallel()

	username, slug, deployID, assetPath, ok := parseSitePath("/~sam/product-teardown/docs/index.html")
	if !ok {
		t.Fatal("expected parseSitePath to succeed")
	}
	if username != "sam" || slug != "product-teardown" || deployID != "" || assetPath != "docs/index.html" {
		t.Fatalf("unexpected parse result: %q %q %q %q", username, slug, deployID, assetPath)
	}

	// Versioned path
	username, slug, deployID, assetPath, ok = parseSitePath("/~sam/product-teardown/_v/dep_abc123/styles.css")
	if !ok {
		t.Fatal("expected versioned parseSitePath to succeed")
	}
	if username != "sam" || slug != "product-teardown" || deployID != "dep_abc123" || assetPath != "styles.css" {
		t.Fatalf("unexpected versioned parse result: %q %q %q %q", username, slug, deployID, assetPath)
	}

	// Versioned root (no asset)
	_, _, deployID, assetPath, ok = parseSitePath("/~sam/product-teardown/_v/dep_abc123/")
	if !ok || deployID != "dep_abc123" || assetPath != "" {
		t.Fatalf("unexpected versioned root result: deployID=%q assetPath=%q ok=%v", deployID, assetPath, ok)
	}

	// Invalid: _v without deploy ID
	_, _, _, _, ok = parseSitePath("/~sam/product-teardown/_v/")
	if ok {
		t.Fatal("expected _v without deploy ID to fail")
	}
}

func TestHashTokenIsDeterministic(t *testing.T) {
	t.Parallel()

	first := hashToken("protopen-session-token")
	second := hashToken("protopen-session-token")
	if first != second {
		t.Fatal("expected token hashes to be deterministic")
	}
	if len(first) != 64 {
		t.Fatalf("expected sha256 hex hash length 64, got %d", len(first))
	}
}

func TestCountForMatchReturnsOneForNewSite(t *testing.T) {
	t.Parallel()

	got := countForMatch(nil)
	if got != 1 {
		t.Fatalf("expected new site deploy count 1, got %d", got)
	}
}

func TestCountForMatchIncrementsExistingSiteDeploys(t *testing.T) {
	t.Parallel()

	got := countForMatch([]siteRecord{{Deploys: 4}})
	if got != 5 {
		t.Fatalf("expected redeploy count 5, got %d", got)
	}
}

func TestValidateSiteMatchCountAllowsZeroAndOneMatch(t *testing.T) {
	t.Parallel()

	if err := validateSiteMatchCount(nil, "Prototype"); err != nil {
		t.Fatalf("expected zero matches to be allowed, got %v", err)
	}

	if err := validateSiteMatchCount([]siteRecord{{ID: "site_1"}}, "Prototype"); err != nil {
		t.Fatalf("expected one exact match to be allowed, got %v", err)
	}
}

func TestValidateSiteMatchCountRejectsMultipleMatches(t *testing.T) {
	t.Parallel()

	err := validateSiteMatchCount([]siteRecord{{ID: "site_1"}, {ID: "site_2"}}, "Prototype")
	if err == nil {
		t.Fatal("expected multiple exact matches to be rejected")
	}
	if !strings.Contains(err.Error(), "multiple existing sites match") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeUploadPathRejectsTraversal(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		desc  string
	}{
		{"../etc/passwd", "parent directory traversal"},
		{"foo/../../bar", "embedded traversal"},
		{"..", "bare dotdot"},
		{"a/../../../etc/shadow", "deep traversal"},
	}
	for _, tc := range cases {
		if _, err := normalizeUploadPath(tc.input); err == nil {
			t.Errorf("expected error for %s (%q)", tc.desc, tc.input)
		}
	}
}

func TestNormalizeUploadPathRejectsReservedPrefix(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"_v/something", "_v"} {
		if _, err := normalizeUploadPath(input); err == nil {
			t.Errorf("expected error for reserved prefix %q", input)
		}
	}
}

func TestNormalizeUploadPathRejectsEmptyAndDot(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"", "  ", "."} {
		if _, err := normalizeUploadPath(input); err == nil {
			t.Errorf("expected error for empty/dot input %q", input)
		}
	}
}

func TestNormalizeUploadPathAcceptsValidPaths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  string
	}{
		{"index.html", "index.html"},
		{"docs/page.html", "docs/page.html"},
		{"assets/css/styles.css", "assets/css/styles.css"},
		{"/index.html", "index.html"},
		{"  index.html  ", "index.html"},
	}
	for _, tc := range cases {
		got, err := normalizeUploadPath(tc.input)
		if err != nil {
			t.Errorf("normalizeUploadPath(%q) unexpected error: %v", tc.input, err)
		} else if got != tc.want {
			t.Errorf("normalizeUploadPath(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestSlugify(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  string
	}{
		{"My Cool Project", "my-cool-project"},
		{"site.zip", "site"},
		{"test@#$%project", "test-project"},
		{"", "prototype"},
		{"   ", "prototype"},
		{"--hello--", "hello"},
		{"hello_world", "hello-world"},
		{"Lots   Of   Spaces", "lots-of-spaces"},
		{"UPPERCASE", "uppercase"},
		{"123numbers", "123numbers"},
		{"@@@", "prototype"},
	}
	for _, tc := range cases {
		got := slugify(tc.input)
		if got != tc.want {
			t.Errorf("slugify(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestValidateUploadRejectsEmptyName(t *testing.T) {
	t.Parallel()

	payload := uploadRequest{
		Name:  "   ",
		Mode:  "files",
		Files: []fileMeta{{Name: "index.html", Path: "index.html", Size: 128}},
	}
	if err := validateUpload(payload); err == nil {
		t.Fatal("expected empty name to be rejected")
	}
}

func TestValidateUploadRejectsEmptyFiles(t *testing.T) {
	t.Parallel()

	payload := uploadRequest{Name: "Test", Mode: "files", Files: nil}
	if err := validateUpload(payload); err == nil {
		t.Fatal("expected empty files to be rejected")
	}
}

func TestValidateUploadRejectsOversizedFile(t *testing.T) {
	t.Parallel()

	payload := uploadRequest{
		Name: "Test",
		Mode: "files",
		Files: []fileMeta{
			{Name: "index.html", Path: "index.html", Size: 128},
			{Name: "huge.bin", Path: "huge.bin", Size: 251 * 1024 * 1024},
		},
	}
	err := validateUpload(payload)
	if err == nil {
		t.Fatal("expected oversized file to be rejected")
	}
	if !strings.Contains(err.Error(), "huge.bin") {
		t.Fatalf("expected error to mention filename, got: %v", err)
	}
}

func TestValidateUploadRejectsTotalOverLimit(t *testing.T) {
	t.Parallel()

	files := make([]fileMeta, 0, 26)
	for i := 0; i < 26; i++ {
		files = append(files, fileMeta{Name: "chunk.bin", Path: "chunk.bin", Size: 10 * 1024 * 1024})
	}
	files = append(files, fileMeta{Name: "index.html", Path: "index.html", Size: 128})

	payload := uploadRequest{Name: "Test", Mode: "files", Files: files}
	err := validateUpload(payload)
	if err == nil {
		t.Fatal("expected total size over 250MB to be rejected")
	}
	if !strings.Contains(err.Error(), "250MB") {
		t.Fatalf("expected error to mention 250MB limit, got: %v", err)
	}
}

func TestValidateUploadZipModeSingleZipPasses(t *testing.T) {
	t.Parallel()

	payload := uploadRequest{
		Name:  "My Site",
		Mode:  "zip",
		Files: []fileMeta{{Name: "site.zip", Path: "site.zip", Size: 5000}},
	}
	if err := validateUpload(payload); err != nil {
		t.Fatalf("expected single zip upload to pass, got: %v", err)
	}
}

func TestValidateUploadZipModeRejectsMultipleZips(t *testing.T) {
	t.Parallel()

	payload := uploadRequest{
		Name: "My Site",
		Mode: "zip",
		Files: []fileMeta{
			{Name: "a.zip", Path: "a.zip", Size: 1000},
			{Name: "b.zip", Path: "b.zip", Size: 1000},
		},
	}
	if err := validateUpload(payload); err == nil {
		t.Fatal("expected multiple zip files to be rejected")
	}
}

func TestWithCORSAllowsMatchingOrigin(t *testing.T) {
	t.Parallel()

	handler := withCORS("http://localhost:5173", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatal("expected matching origin to be reflected")
	}
	if rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal("expected credentials header for matching origin")
	}
}

func TestWithCORSRejectsNonMatchingOrigin(t *testing.T) {
	t.Parallel()

	handler := withCORS("http://localhost:5173", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	req.Header.Set("Origin", "http://evil.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("expected no Allow-Origin header for non-matching origin")
	}
	if rec.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Fatal("expected no credentials header for non-matching origin")
	}
	if rec.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Fatal("expected Allow-Methods to always be set")
	}
}

func TestWithCORSOptionsReturnNoContent(t *testing.T) {
	t.Parallel()

	called := false
	handler := withCORS("http://localhost:5173", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/projects", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for OPTIONS, got %d", rec.Code)
	}
	if called {
		t.Fatal("expected OPTIONS to short-circuit without calling next handler")
	}
}

func TestResolveAssetPathRejectsTraversal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := resolveAssetPath(root, "../../../etc/passwd")
	if err == nil {
		t.Fatal("expected path traversal to be rejected")
	}
}

func TestResolveAssetPathReturnErrorForMissingFileWithExtension(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := resolveAssetPath(root, "missing.css")
	if err == nil {
		t.Fatal("expected error for missing file with extension")
	}
}

func TestResolveAssetPathEmptyReturnsRootIndex(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	resolved, fallback, err := resolveAssetPath(root, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fallback {
		t.Fatal("expected no SPA fallback for empty path")
	}
	want := filepath.Join(root, "index.html")
	if resolved != want {
		t.Fatalf("expected %q, got %q", want, resolved)
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

func TestRelativeTime(t *testing.T) {
	t.Parallel()

	cases := []struct {
		ago  time.Duration
		want string
	}{
		{0, "Just now"},
		{30 * time.Second, "Just now"},
		{59 * time.Second, "Just now"},
		{1 * time.Minute, "1 minute ago"},
		{5 * time.Minute, "5 minutes ago"},
		{59 * time.Minute, "59 minutes ago"},
		{1 * time.Hour, "1 hour ago"},
		{3 * time.Hour, "3 hours ago"},
		{23 * time.Hour, "23 hours ago"},
		{25 * time.Hour, "Yesterday"},
		{47 * time.Hour, "Yesterday"},
		{49 * time.Hour, "2 days ago"},
		{10 * 24 * time.Hour, "10 days ago"},
	}
	for _, tc := range cases {
		got := relativeTime(time.Now().Add(-tc.ago))
		if got != tc.want {
			t.Errorf("relativeTime(-%v) = %q, want %q", tc.ago, got, tc.want)
		}
	}
}

func TestOrgRole(t *testing.T) {
	t.Parallel()

	user := sessionUser{
		Orgs: []orgInfo{
			{ID: "org_1", Slug: "team-a", Role: "admin"},
			{ID: "org_2", Slug: "team-b", Role: "member"},
		},
	}

	role, ok := orgRole(user, "org_1")
	if !ok || role != "admin" {
		t.Fatalf("expected admin role for org_1, got %q %v", role, ok)
	}

	role, ok = orgRole(user, "org_99")
	if ok {
		t.Fatalf("expected not found for org_99, got %q", role)
	}
}

func TestPersonalOrg(t *testing.T) {
	t.Parallel()

	user := sessionUser{
		Orgs: []orgInfo{
			{ID: "org_1", Slug: "sam", IsPersonal: true, Role: "admin"},
			{ID: "org_2", Slug: "team", IsPersonal: false, Role: "member"},
		},
	}

	org := personalOrg(user)
	if org == nil || org.ID != "org_1" {
		t.Fatal("expected personal org to be returned")
	}

	noPersonal := sessionUser{
		Orgs: []orgInfo{{ID: "org_2", Slug: "team", IsPersonal: false}},
	}
	if personalOrg(noPersonal) != nil {
		t.Fatal("expected nil when no personal org exists")
	}
}

func TestResolveOrgFromParam(t *testing.T) {
	t.Parallel()

	user := sessionUser{
		Orgs: []orgInfo{
			{ID: "org_1", Slug: "sam", IsPersonal: true, Role: "admin"},
			{ID: "org_2", Slug: "my-team", IsPersonal: false, Role: "member"},
		},
	}

	// Empty param returns personal org
	id, slug, err := resolveOrgFromParam(user, "")
	if err != nil || id != "org_1" || slug != "sam" {
		t.Fatalf("expected personal org, got (%q, %q, %v)", id, slug, err)
	}

	// Matching slug
	id, slug, err = resolveOrgFromParam(user, "my-team")
	if err != nil || id != "org_2" || slug != "my-team" {
		t.Fatalf("expected team org, got (%q, %q, %v)", id, slug, err)
	}

	// Non-matching slug
	_, _, err = resolveOrgFromParam(user, "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-matching slug")
	}

	// Empty param with no personal org
	noPersonal := sessionUser{Orgs: []orgInfo{{ID: "org_2", Slug: "team", IsPersonal: false}}}
	_, _, err = resolveOrgFromParam(noPersonal, "")
	if err == nil {
		t.Fatal("expected error when no personal org exists")
	}
}

func TestUserInOrg(t *testing.T) {
	t.Parallel()

	user := sessionUser{
		Orgs: []orgInfo{
			{ID: "org_1", Slug: "sam"},
			{ID: "org_2", Slug: "team"},
		},
	}

	if !userInOrg(user, "sam") {
		t.Fatal("expected user to be in org 'sam'")
	}
	if userInOrg(user, "other") {
		t.Fatal("expected user to not be in org 'other'")
	}
}

