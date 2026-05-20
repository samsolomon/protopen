// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// TestMCPBinarySmoke builds the CLI binary and speaks newline-delimited
// JSON-RPC to `protopen mcp` over real stdio. Existing MCP tests wire the
// server up in-process via mcp.NewInMemoryTransports, so they can't catch
// regressions in the binary entry point — stdout-guard breaks, future
// build-tag drops, or subcommand routing going away. This test is the
// black-box check that the released binary actually talks MCP.
func TestMCPBinarySmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping smoke test in -short mode")
	}

	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "protopen")

	build := exec.Command("go", "build", "-o", binPath, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, "mcp")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		stdin.Close()
		_ = cmd.Wait()
	})

	reader := bufio.NewReader(stdout)

	// initialize — pin to a stable supported protocol version so this test
	// keeps passing across SDK upgrades that change the latest default.
	send(t, stdin, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "smoke", "version": "1"},
		},
	})

	initResp := readResp(t, reader, &stderr)
	result, ok := initResp["result"].(map[string]any)
	if !ok {
		t.Fatalf("initialize: no result field: %v\nstderr: %s", initResp, stderr.String())
	}
	serverInfo, ok := result["serverInfo"].(map[string]any)
	if !ok {
		t.Fatalf("initialize: no serverInfo: %v", result)
	}
	if name, _ := serverInfo["name"].(string); name != "protopen" {
		t.Errorf("serverInfo.name = %q, want %q", name, "protopen")
	}

	// initialized notification — the SDK currently no-ops if it's missing,
	// but the spec requires it and a future SDK may enforce it.
	send(t, stdin, map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	})

	// tools/list — assert the same five tools TestMCPExposesExpectedTools
	// checks in-process, so the two stay in lock-step.
	send(t, stdin, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
	})

	listResp := readResp(t, reader, &stderr)
	listResult, ok := listResp["result"].(map[string]any)
	if !ok {
		t.Fatalf("tools/list: no result: %v\nstderr: %s", listResp, stderr.String())
	}
	tools, ok := listResult["tools"].([]any)
	if !ok {
		t.Fatalf("tools/list: tools not an array: %v", listResult)
	}
	got := make([]string, 0, len(tools))
	for _, tool := range tools {
		m, _ := tool.(map[string]any)
		name, _ := m["name"].(string)
		got = append(got, name)
	}
	sort.Strings(got)
	want := []string{"deploy", "list_comments", "list_deploys", "list_sites", "rollback", "set_visibility"}
	if len(got) != len(want) {
		t.Fatalf("tools = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("tool[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func send(t *testing.T, w io.Writer, msg map[string]any) {
	t.Helper()
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := w.Write(append(data, '\n')); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func readResp(t *testing.T, r *bufio.Reader, stderr *strings.Builder) map[string]any {
	t.Helper()
	line, err := r.ReadBytes('\n')
	if err != nil {
		t.Fatalf("read: %v\nstderr: %s", err, stderr.String())
	}
	var out map[string]any
	if err := json.Unmarshal(line, &out); err != nil {
		t.Fatalf("decode %q: %v", line, err)
	}
	return out
}
