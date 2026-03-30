package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type gitMeta struct {
	CommitHash    string
	Branch        string
	CommitMessage string
	Dirty         bool
	Author        string
	RemoteURL     string
}

func detectGitMeta(path string) *gitMeta {
	dir := path
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		dir = filepath.Dir(path)
	}

	combined := gitOutput(dir, "log", "-1", "--format=%H%n%s%n%an")
	if combined == "" {
		return nil
	}

	parts := strings.SplitN(combined, "\n", 3)
	if len(parts) < 3 {
		return nil
	}

	branch := gitOutput(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if branch == "HEAD" {
		branch = ""
	}

	return &gitMeta{
		CommitHash:    parts[0],
		Branch:        branch,
		CommitMessage: parts[1],
		Dirty:         gitOutput(dir, "status", "--porcelain") != "",
		Author:        parts[2],
		RemoteURL:     normalizeGitRemote(gitOutput(dir, "remote", "get-url", "origin")),
	}
}

// normalizeGitRemote converts a git remote URL to a browsable HTTPS base URL.
// Handles SSH (git@host:path), ssh:// protocol, and HTTPS with credentials.
func normalizeGitRemote(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	var host, path string

	switch {
	case strings.HasPrefix(raw, "ssh://"):
		raw = strings.TrimPrefix(raw, "ssh://")
		raw = stripUserInfo(raw)
		if i := strings.Index(raw, "/"); i != -1 {
			hostPort := raw[:i]
			path = raw[i+1:]
			if j := strings.LastIndex(hostPort, ":"); j != -1 {
				host = hostPort[:j]
			} else {
				host = hostPort
			}
		} else {
			return ""
		}

	case strings.HasPrefix(raw, "https://") || strings.HasPrefix(raw, "http://"):
		trimmed := raw
		if after, ok := strings.CutPrefix(raw, "https://"); ok {
			trimmed = after
		} else if after, ok := strings.CutPrefix(raw, "http://"); ok {
			trimmed = after
		}
		trimmed = stripUserInfo(trimmed)
		if i := strings.Index(trimmed, "/"); i != -1 {
			host = trimmed[:i]
			path = trimmed[i+1:]
		} else {
			return ""
		}

	default:
		raw = stripUserInfo(raw)
		if i := strings.Index(raw, ":"); i != -1 {
			host = raw[:i]
			path = raw[i+1:]
		} else {
			return ""
		}
	}

	if i := strings.LastIndex(host, ":"); i != -1 {
		host = host[:i]
	}

	path = strings.TrimSuffix(path, ".git")
	path = strings.TrimRight(path, "/")

	if host == "" || path == "" {
		return ""
	}

	return "https://" + host + "/" + path
}

func stripUserInfo(s string) string {
	if i := strings.Index(s, "@"); i != -1 {
		return s[i+1:]
	}
	return s
}

func gitOutput(dir string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

