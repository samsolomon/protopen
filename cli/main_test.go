package main

import "testing"

func TestResolveTokenPrefersFlag(t *testing.T) {
	configHome = t.TempDir()
	defer func() { configHome = "" }()
	t.Setenv("PROTOPEN_TOKEN", "env-token")

	got := resolveToken("flag-token")
	if got != "flag-token" {
		t.Fatalf("expected flag-token, got %q", got)
	}
}

func TestResolveTokenFallsToEnv(t *testing.T) {
	configHome = t.TempDir()
	defer func() { configHome = "" }()
	t.Setenv("PROTOPEN_TOKEN", "env-token")

	got := resolveToken("")
	if got != "env-token" {
		t.Fatalf("expected env-token, got %q", got)
	}
}

func TestResolveTokenReturnsEmpty(t *testing.T) {
	configHome = t.TempDir()
	defer func() { configHome = "" }()
	t.Setenv("PROTOPEN_TOKEN", "")

	got := resolveToken("")
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestResolveURLDefaults(t *testing.T) {
	configHome = t.TempDir()
	defer func() { configHome = "" }()
	t.Setenv("PROTOPEN_URL", "")

	got := resolveURL("")
	if got != "http://localhost:8080" {
		t.Fatalf("expected http://localhost:8080, got %q", got)
	}
}

func TestResolveURLPrefersFlag(t *testing.T) {
	configHome = t.TempDir()
	defer func() { configHome = "" }()
	t.Setenv("PROTOPEN_URL", "https://env.example.com")

	got := resolveURL("https://flag.example.com")
	if got != "https://flag.example.com" {
		t.Fatalf("expected https://flag.example.com, got %q", got)
	}
}

func TestResolveURLFallsToEnv(t *testing.T) {
	configHome = t.TempDir()
	defer func() { configHome = "" }()
	t.Setenv("PROTOPEN_URL", "https://env.example.com")

	got := resolveURL("")
	if got != "https://env.example.com" {
		t.Fatalf("expected https://env.example.com, got %q", got)
	}
}

func TestStripZipExt(t *testing.T) {
	tests := []struct{ input, want string }{
		{"site.zip", "site"},
		{"site", "site"},
		{"my-project.zip", "my-project"},
		{".zip", ".zip"},
		{"", ""},
	}

	for _, tc := range tests {
		got := stripZipExt(tc.input)
		if got != tc.want {
			t.Errorf("stripZipExt(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
