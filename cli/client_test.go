package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeployDirectorySuccess(t *testing.T) {
	t.Parallel()

	var receivedName, receivedMode string
	var receivedAuth string
	var hasFile bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		r.ParseMultipartForm(32 << 20)
		receivedName = r.FormValue("name")
		receivedMode = r.FormValue("mode")
		_, header, err := r.FormFile("files")
		hasFile = err == nil && header != nil

		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]any{
			"project": map[string]string{"liveUrl": "https://sites.example.com/~user/test"},
		})
	}))
	defer server.Close()

	dir := filepath.Join(t.TempDir(), "test-site")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>test</h1>"), 0o644)

	c := newClient("vtk_testtoken", server.URL)
	result, err := c.deployDirectory(dir, "test-site", "", "", nil)
	if err != nil {
		t.Fatalf("deployDirectory: %v", err)
	}

	if result.liveURL != "https://sites.example.com/~user/test" {
		t.Fatalf("expected live URL, got %q", result.liveURL)
	}
	if receivedName != "test-site" {
		t.Fatalf("expected name=test-site, got %q", receivedName)
	}
	if receivedMode != "zip" {
		t.Fatalf("expected mode=zip, got %q", receivedMode)
	}
	if !hasFile {
		t.Fatal("expected files field in multipart form")
	}
	if receivedAuth != "Bearer vtk_testtoken" {
		t.Fatalf("expected Bearer token, got %q", receivedAuth)
	}
}

func TestDeployReturnsErrorOnUnauthorized(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
	}))
	defer server.Close()

	c := newClient("vtk_bad", server.URL)
	_, err := c.upload("test", "zip", "test.zip", []byte("fake"), "", "", nil)
	if err == nil {
		t.Fatal("expected error for 401")
	}
}

func TestDeployReturnsErrorMessage(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "no index.html found"})
	}))
	defer server.Close()

	c := newClient("vtk_test", server.URL)
	_, err := c.upload("test", "zip", "test.zip", []byte("fake"), "", "", nil)
	if err == nil {
		t.Fatal("expected error for 400")
	}
	if !strings.Contains(err.Error(), "no index.html found") {
		t.Fatalf("expected error message, got %q", err.Error())
	}
}

func TestListProjectsSuccess(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"projects": []map[string]any{
				{"name": "My Site", "slug": "my-site", "deployCount": 3, "liveUrl": "https://example.com/~user/my-site"},
			},
		})
	}))
	defer server.Close()

	c := newClient("vtk_test", server.URL)
	projects, err := c.listProjects("")
	if err != nil {
		t.Fatalf("listProjects: %v", err)
	}

	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0].Name != "My Site" {
		t.Fatalf("expected name 'My Site', got %q", projects[0].Name)
	}
	if projects[0].DeployCount != 3 {
		t.Fatalf("expected 3 deploys, got %d", projects[0].DeployCount)
	}
}

func TestListProjectsUnauthorized(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	c := newClient("vtk_bad", server.URL)
	_, err := c.listProjects("")
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("expected unauthorized error, got %q", err.Error())
	}
}

func TestSetAuthHeaderWithToken(t *testing.T) {
	t.Parallel()

	c := newClient("vtk_mytoken", "http://localhost")
	req, _ := http.NewRequest("GET", "http://localhost/api/test", nil)
	c.setAuth(req)

	got := req.Header.Get("Authorization")
	if got != "Bearer vtk_mytoken" {
		t.Fatalf("expected 'Bearer vtk_mytoken', got %q", got)
	}
}

func TestSetAuthHeaderEmpty(t *testing.T) {
	t.Parallel()

	c := newClient("", "http://localhost")
	req, _ := http.NewRequest("GET", "http://localhost/api/test", nil)
	c.setAuth(req)

	got := req.Header.Get("Authorization")
	if got != "" {
		t.Fatalf("expected no Authorization header, got %q", got)
	}
}

func TestGetSessionSuccess(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"user": map[string]string{"name": "Sam", "email": "sam@example.com"},
		})
	}))
	defer server.Close()

	c := newClient("vtk_test", server.URL)
	user, err := c.getSession()
	if err != nil {
		t.Fatalf("getSession: %v", err)
	}
	if user.Name != "Sam" || user.Email != "sam@example.com" {
		t.Fatalf("unexpected user: %+v", user)
	}
}

func TestGetSessionUnauthorized(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	c := newClient("vtk_bad", server.URL)
	_, err := c.getSession()
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("expected unauthorized error, got %q", err.Error())
	}
}

func TestFindProjectByName(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"projects": []map[string]any{
				{"id": "proj_1", "name": "My Site", "slug": "my-site"},
				{"id": "proj_2", "name": "Other", "slug": "other"},
			},
		})
	}))
	defer server.Close()

	c := newClient("vtk_test", server.URL)
	p, err := c.findProject("My Site", "")
	if err != nil {
		t.Fatalf("findProject: %v", err)
	}
	if p.ID != "proj_1" {
		t.Fatalf("expected proj_1, got %q", p.ID)
	}
}

func TestFindProjectBySlug(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"projects": []map[string]any{
				{"id": "proj_1", "name": "My Site", "slug": "my-site"},
			},
		})
	}))
	defer server.Close()

	c := newClient("vtk_test", server.URL)
	p, err := c.findProject("my-site", "")
	if err != nil {
		t.Fatalf("findProject: %v", err)
	}
	if p.ID != "proj_1" {
		t.Fatalf("expected proj_1, got %q", p.ID)
	}
}

func TestFindProjectNotFound(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"projects": []map[string]any{}})
	}))
	defer server.Close()

	c := newClient("vtk_test", server.URL)
	_, err := c.findProject("nonexistent", "")
	if err == nil {
		t.Fatal("expected not found error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found message, got %q", err.Error())
	}
}

func TestListDeploysSuccess(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/deploys") {
			t.Errorf("expected path ending in /deploys, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"deploys": []map[string]any{
				{"id": "dep_1", "status": "live", "isCurrent": true},
				{"id": "dep_2", "status": "previous", "isCurrent": false},
			},
		})
	}))
	defer server.Close()

	c := newClient("vtk_test", server.URL)
	deploys, err := c.listDeploys("proj_abc")
	if err != nil {
		t.Fatalf("listDeploys: %v", err)
	}
	if len(deploys) != 2 {
		t.Fatalf("expected 2 deploys, got %d", len(deploys))
	}
	if deploys[0].ID != "dep_1" || !deploys[0].IsCurrent {
		t.Fatalf("unexpected first deploy: %+v", deploys[0])
	}
}

func TestListDeploysUnauthorized(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	c := newClient("vtk_bad", server.URL)
	_, err := c.listDeploys("proj_abc")
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("expected unauthorized error, got %q", err.Error())
	}
}

func TestRollbackSuccess(t *testing.T) {
	t.Parallel()

	var receivedBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	c := newClient("vtk_test", server.URL)
	err := c.rollback("proj_abc", "dep_old")
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if receivedBody["deployId"] != "dep_old" {
		t.Fatalf("expected deployId=dep_old, got %q", receivedBody["deployId"])
	}
}

func TestRollbackUnauthorized(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	c := newClient("vtk_bad", server.URL)
	err := c.rollback("proj_abc", "dep_old")
	if err == nil {
		t.Fatal("expected error for 401")
	}
}

func TestRollbackServerError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "deploy not found"})
	}))
	defer server.Close()

	c := newClient("vtk_test", server.URL)
	err := c.rollback("proj_abc", "dep_missing")
	if err == nil {
		t.Fatal("expected error for 400")
	}
	if !strings.Contains(err.Error(), "deploy not found") {
		t.Fatalf("expected error message, got %q", err.Error())
	}
}

func TestUpdateVisibilitySuccess(t *testing.T) {
	t.Parallel()

	var receivedBody map[string]bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	c := newClient("vtk_test", server.URL)
	err := c.updateVisibility("proj_abc", true)
	if err != nil {
		t.Fatalf("updateVisibility: %v", err)
	}
	if receivedBody["isPublic"] != true {
		t.Fatal("expected isPublic=true")
	}
}

func TestUpdateVisibilityUnauthorized(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	c := newClient("vtk_bad", server.URL)
	err := c.updateVisibility("proj_abc", true)
	if err == nil {
		t.Fatal("expected error for 401")
	}
}
