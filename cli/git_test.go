// SPDX-License-Identifier: AGPL-3.0-only

package main

import "testing"

func TestNormalizeGitRemote(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  string
	}{
		{"git@github.com:user/repo.git", "https://github.com/user/repo"},
		{"git@github.com:user/repo", "https://github.com/user/repo"},
		{"https://github.com/user/repo.git", "https://github.com/user/repo"},
		{"https://user:pass@github.com/user/repo.git", "https://github.com/user/repo"},
		{"http://github.com/user/repo", "https://github.com/user/repo"},
		{"ssh://git@github.com/user/repo.git", "https://github.com/user/repo"},
		{"ssh://git@github.com:22/user/repo.git", "https://github.com/user/repo"},
		{"git@gitlab.com:org/sub/repo.git", "https://gitlab.com/org/sub/repo"},
		{"git@git.internal.co:team/project", "https://git.internal.co/team/project"},
		{"git@github.com:user/repo/", "https://github.com/user/repo"},
		{"  git@github.com:u/r.git  ", "https://github.com/u/r"},
		{"", ""},
		{"github.com", ""},
		{"https://github.com", ""},
		{"ssh://git@github.com", ""},
	}
	for _, tc := range cases {
		got := normalizeGitRemote(tc.input)
		if got != tc.want {
			t.Errorf("normalizeGitRemote(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestStripUserInfo(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  string
	}{
		{"user@host", "host"},
		{"host", "host"},
		{"u:p@host", "host"},
		{"", ""},
	}
	for _, tc := range cases {
		got := stripUserInfo(tc.input)
		if got != tc.want {
			t.Errorf("stripUserInfo(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
