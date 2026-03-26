package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// configHome overrides the config directory for testing.
var configHome string

type config struct {
	Token string `json:"token,omitempty"`
	URL   string `json:"url,omitempty"`
}

func configDir() string {
	if configHome != "" {
		return configHome
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".velori")
}

func configPath() string {
	dir := configDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "config.json")
}

func loadConfig() config {
	path := configPath()
	if path == "" {
		return config{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return config{}
	}
	var cfg config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return config{}
	}
	return cfg
}

func saveConfig(cfg config) error {
	dir := configDir()
	if dir == "" {
		return os.ErrNotExist
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), data, 0600)
}
