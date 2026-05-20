// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// writeTestConfig redirects configHome to a fresh tmp dir for the test's
// lifetime and writes cfg as config.json. Restoring the previous value via
// t.Cleanup keeps the suite safe under -shuffle and future parallel tests.
func writeTestConfig(t *testing.T, cfg config) string {
	t.Helper()
	prev := configHome
	tmp := t.TempDir()
	configHome = tmp
	t.Cleanup(func() { configHome = prev })
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "config.json"), data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return tmp
}

// newTestMCPSession spins up an in-process MCP server with the production
// tool set and returns a connected client session. The caller can then
// exercise tools/list and tools/call without going through stdio.
func newTestMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	server := mcp.NewServer(&mcp.Implementation{Name: "protopen", Version: version}, nil)
	registerMCPTools(server)

	serverT, clientT := mcp.NewInMemoryTransports()

	ctx := context.Background()
	if _, err := server.Connect(ctx, serverT, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func TestMCPExposesExpectedTools(t *testing.T) {
	session := newTestMCPSession(t)

	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	want := []string{"deploy", "list_comments", "list_deploys", "list_sites", "rollback", "set_visibility"}
	got := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		got = append(got, tool.Name)
	}
	sort.Strings(got)

	if len(got) != len(want) {
		t.Fatalf("expected %d tools %v, got %d %v", len(want), want, len(got), got)
	}
	for i, name := range want {
		if got[i] != name {
			t.Errorf("tool[%d]: want %q, got %q", i, name, got[i])
		}
	}
}

func TestMCPListSitesAgainstStubbedAPI(t *testing.T) {
	// Stub the backend so the in-process MCP handler hits a known endpoint
	// rather than a real Protopen instance.
	var stubURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/sites" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"sites": []map[string]any{
				{"id": "site_1", "name": "Demo", "slug": "demo", "liveUrl": stubURL + "/~u/demo", "deployCount": 2, "isPublic": true},
			},
		})
	}))
	defer server.Close()
	stubURL = server.URL

	// Point the MCP server's config resolution at the stub.
	writeTestConfig(t, config{Token: "ptk_test", URL: server.URL})

	session := newTestMCPSession(t)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "list_sites",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool returned IsError: %+v", res.Content)
	}

	// StructuredContent holds the typed output; unmarshal and check.
	raw, _ := json.Marshal(res.StructuredContent)
	var out listSitesOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode structured content: %v", err)
	}
	if len(out.Sites) != 1 || out.Sites[0].Name != "Demo" {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestMCPDeployRejectsRelativePath(t *testing.T) {
	writeTestConfig(t, config{Token: "ptk_test", URL: "http://unused"})

	session := newTestMCPSession(t)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "deploy",
		Arguments: map[string]any{"path": "./relative-path"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected IsError=true for relative path")
	}
}
