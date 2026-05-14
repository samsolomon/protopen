package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigMissingFile(t *testing.T) {
	configHome = t.TempDir()
	defer func() { configHome = "" }()

	cfg := loadConfig()
	if cfg.Token != "" || cfg.URL != "" {
		t.Fatalf("expected zero config, got %+v", cfg)
	}
}

func TestLoadConfigCorruptJSON(t *testing.T) {
	dir := t.TempDir()
	configHome = dir
	defer func() { configHome = "" }()

	os.WriteFile(filepath.Join(dir, "config.json"), []byte("{invalid"), 0600)

	cfg := loadConfig()
	if cfg.Token != "" || cfg.URL != "" {
		t.Fatalf("expected zero config for corrupt JSON, got %+v", cfg)
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	configHome = t.TempDir()
	defer func() { configHome = "" }()

	want := config{Token: "ptk_test123", URL: "https://example.com"}
	if err := saveConfig(want); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	got := loadConfig()
	if got.Token != want.Token || got.URL != want.URL {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestSaveConfigCreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested")
	configHome = dir
	defer func() { configHome = "" }()

	if err := saveConfig(config{Token: "ptk_abc"}); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected directory")
	}
}

func TestSaveConfigPreservesOtherFields(t *testing.T) {
	configHome = t.TempDir()
	defer func() { configHome = "" }()

	saveConfig(config{Token: "ptk_original", URL: "https://example.com"})

	cfg := loadConfig()
	cfg.Token = "ptk_updated"
	saveConfig(cfg)

	got := loadConfig()
	if got.Token != "ptk_updated" {
		t.Fatalf("token not updated: got %q", got.Token)
	}
	if got.URL != "https://example.com" {
		t.Fatalf("url lost: got %q", got.URL)
	}
}

func TestResolveTokenWithConfigFallback(t *testing.T) {
	configHome = t.TempDir()
	defer func() { configHome = "" }()
	t.Setenv("PROTOPEN_TOKEN", "")

	saveConfig(config{Token: "ptk_from_config"})

	got := resolveToken("")
	if got != "ptk_from_config" {
		t.Fatalf("expected ptk_from_config, got %q", got)
	}
}

func TestResolveTokenEnvBeatsConfig(t *testing.T) {
	configHome = t.TempDir()
	defer func() { configHome = "" }()
	t.Setenv("PROTOPEN_TOKEN", "ptk_from_env")

	saveConfig(config{Token: "ptk_from_config"})

	got := resolveToken("")
	if got != "ptk_from_env" {
		t.Fatalf("expected ptk_from_env, got %q", got)
	}
}

func TestResolveURLWithConfigFallback(t *testing.T) {
	configHome = t.TempDir()
	defer func() { configHome = "" }()
	t.Setenv("PROTOPEN_URL", "")

	saveConfig(config{URL: "https://config.example.com"})

	got := resolveURL("")
	if got != "https://config.example.com" {
		t.Fatalf("expected https://config.example.com, got %q", got)
	}
}

func TestResolveURLEnvBeatsConfig(t *testing.T) {
	configHome = t.TempDir()
	defer func() { configHome = "" }()
	t.Setenv("PROTOPEN_URL", "https://env.example.com")

	saveConfig(config{URL: "https://config.example.com"})

	got := resolveURL("")
	if got != "https://env.example.com" {
		t.Fatalf("expected https://env.example.com, got %q", got)
	}
}
