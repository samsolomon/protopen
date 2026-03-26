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
	result, err := c.deployDirectory(dir, "test-site", "")
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
	_, err := c.upload("test", "zip", "test.zip", []byte("fake"), "")
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
	_, err := c.upload("test", "zip", "test.zip", []byte("fake"), "")
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
	projects, err := c.listProjects()
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
	_, err := c.listProjects()
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
