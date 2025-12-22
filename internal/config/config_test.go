package config

import (
	"os"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Server.Port != 50051 {
		t.Errorf("Expected default port 50051, got %d", cfg.Server.Port)
	}
	if cfg.Database.MaxOpenConns != 25 {
		t.Errorf("Expected default max_open_conns 25, got %d", cfg.Database.MaxOpenConns)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Default config should be valid: %v", err)
	}
}

func TestLoadFromEnv(t *testing.T) {
	// Set some environment variables
	os.Setenv("ZENITH_SERVER_PORT", "8080")
	os.Setenv("ZENITH_CACHE_ENABLED", "false")
	os.Setenv("ZENITH_ENGINE_MAX_DEPTH", "20")
	defer func() {
		os.Unsetenv("ZENITH_SERVER_PORT")
		os.Unsetenv("ZENITH_CACHE_ENABLED")
		os.Unsetenv("ZENITH_ENGINE_MAX_DEPTH")
	}()

	cfg := LoadFromEnv()
	if cfg.Server.Port != 8080 {
		t.Errorf("Expected port 8080 from env, got %d", cfg.Server.Port)
	}
	if cfg.Cache.Enabled != false {
		t.Errorf("Expected cache disabled from env, got %v", cfg.Cache.Enabled)
	}
	if cfg.Engine.MaxDepth != 20 {
		t.Errorf("Expected max_depth 20 from env, got %d", cfg.Engine.MaxDepth)
	}
}

func TestLoadFromFile(t *testing.T) {
	// Create a temporary YAML file
	yamlContent := `
server:
  port: 9000
  metrics_port: 9091
cache:
  enabled: false
  size: 5000
`
	tmpfile, err := os.CreateTemp("", "test-config-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(yamlContent)); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	tmpfile.Close()

	cfg, err := LoadFromFile(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to load config from file: %v", err)
	}

	if cfg.Server.Port != 9000 {
		t.Errorf("Expected port 9000 from file, got %d", cfg.Server.Port)
	}
	if cfg.Server.MetricsPort != 9091 {
		t.Errorf("Expected metrics_port 9091 from file, got %d", cfg.Server.MetricsPort)
	}
	if cfg.Cache.Enabled != false {
		t.Errorf("Expected cache disabled from file, got %v", cfg.Cache.Enabled)
	}
	if cfg.Cache.Size != 5000 {
		t.Errorf("Expected cache size 5000 from file, got %d", cfg.Cache.Size)
	}
}

func TestValidate(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("Default config should be valid: %v", err)
	}

	// Test invalid port
	cfg.Server.Port = 0
	if err := cfg.Validate(); err == nil {
		t.Error("Expected validation error for port 0")
	}

	cfg = DefaultConfig()
	cfg.Server.Port = 70000
	if err := cfg.Validate(); err == nil {
		t.Error("Expected validation error for port > 65535")
	}

	cfg = DefaultConfig()
	cfg.Database.MaxOpenConns = 0
	if err := cfg.Validate(); err == nil {
		t.Error("Expected validation error for max_open_conns = 0")
	}

	cfg = DefaultConfig()
	cfg.Database.MaxIdleConns = 100
	cfg.Database.MaxOpenConns = 50
	if err := cfg.Validate(); err == nil {
		t.Error("Expected validation error when max_idle_conns > max_open_conns")
	}
}

func TestMergeConfig(t *testing.T) {
	base := DefaultConfig()
	base.Server.Port = 50051

	env := DefaultConfig()
	env.Server.Port = 8080

	// Set env var to trigger merge
	os.Setenv("ZENITH_SERVER_PORT", "8080")
	defer os.Unsetenv("ZENITH_SERVER_PORT")

	merged := mergeConfig(base, env)
	if merged.Server.Port != 8080 {
		t.Errorf("Expected merged port 8080, got %d", merged.Server.Port)
	}
}

