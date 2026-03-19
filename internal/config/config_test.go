package config

import "testing"

func TestParseConfigDefaults(t *testing.T) {
	getenv := func(string) string { return "" }

	cfg, err := ParseConfig(getenv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != "8080" {
		t.Errorf("port=%q, want 8080", cfg.Port)
	}
	if cfg.GitHubToken != "" {
		t.Errorf("token=%q, want empty", cfg.GitHubToken)
	}
	if cfg.Verbose {
		t.Error("verbose should default to false")
	}
	if cfg.APIEnabled {
		t.Error("api should default to false")
	}
}

func TestParseConfigCustom(t *testing.T) {
	env := map[string]string{
		"GITREAL_PORT":         "3000",
		"GITREAL_GITHUB_TOKEN": "ghp_test123",
		"GITREAL_VERBOSE":      "true",
		"GITREAL_API":          "true",
	}
	getenv := func(key string) string { return env[key] }

	cfg, err := ParseConfig(getenv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != "3000" {
		t.Errorf("port=%q, want 3000", cfg.Port)
	}
	if cfg.GitHubToken != "ghp_test123" {
		t.Errorf("token=%q, want ghp_test123", cfg.GitHubToken)
	}
	if !cfg.Verbose {
		t.Error("verbose should be true")
	}
	if !cfg.APIEnabled {
		t.Error("api should be true")
	}
}

func TestParseConfigInvalidBool(t *testing.T) {
	env := map[string]string{
		"GITREAL_VERBOSE": "notabool",
		"GITREAL_API":     "invalid",
	}
	getenv := func(key string) string { return env[key] }

	cfg, err := ParseConfig(getenv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Verbose {
		t.Error("invalid bool should fall back to false")
	}
	if cfg.APIEnabled {
		t.Error("invalid bool should fall back to false")
	}
}
