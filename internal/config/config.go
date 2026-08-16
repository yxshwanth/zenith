package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration for Zenith
type Config struct {
	Server        ServerConfig        `yaml:"server" json:"server"`
	Database      DatabaseConfig      `yaml:"database" json:"database"`
	Cache         CacheConfig         `yaml:"cache" json:"cache"`
	Engine        EngineConfig        `yaml:"engine" json:"engine"`
	Tracing       TracingConfig       `yaml:"tracing" json:"tracing"`
	RateLimit     RateLimitConfig     `yaml:"rate_limit" json:"rate_limit"`
	CircuitBreaker CircuitBreakerConfig `yaml:"circuit_breaker" json:"circuit_breaker"`
	Redis         RedisConfig         `yaml:"redis" json:"redis"`
}

// ServerConfig holds server-related configuration
type ServerConfig struct {
	Port         int  `yaml:"port" json:"port"`
	MetricsPort  int  `yaml:"metrics_port" json:"metrics_port"`
	HTTPPort     int  `yaml:"http_port" json:"http_port"` // HTTP gateway port
	Reflection   bool `yaml:"reflection" json:"reflection"`
}

// DatabaseConfig holds database connection configuration
type DatabaseConfig struct {
	ConnectionString string        `yaml:"connection_string" json:"connection_string"`
	MaxOpenConns     int           `yaml:"max_open_conns" json:"max_open_conns"`
	MaxIdleConns     int           `yaml:"max_idle_conns" json:"max_idle_conns"`
	ConnMaxLifetime  time.Duration `yaml:"conn_max_lifetime" json:"conn_max_lifetime"`
	ConnMaxIdleTime  time.Duration `yaml:"conn_max_idle_time" json:"conn_max_idle_time"`
	HealthCheckInterval time.Duration `yaml:"health_check_interval" json:"health_check_interval"`
}

// CacheConfig holds cache configuration
type CacheConfig struct {
	Enabled      bool          `yaml:"enabled" json:"enabled"`
	Size         int           `yaml:"size" json:"size"`
	TTLPositive  time.Duration `yaml:"ttl_positive" json:"ttl_positive"`
	TTLNegative  time.Duration `yaml:"ttl_negative" json:"ttl_negative"`
}

// EngineConfig holds expansion engine configuration
type EngineConfig struct {
	MaxDepth     int   `yaml:"max_depth" json:"max_depth"`
	CheckTimeout int64 `yaml:"check_timeout_ms" json:"check_timeout_ms"`
}

// TracingConfig holds OpenTelemetry tracing configuration
type TracingConfig struct {
	Enabled  bool   `yaml:"enabled" json:"enabled"`
	Endpoint string `yaml:"endpoint" json:"endpoint"`
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	Enabled      bool  `yaml:"enabled" json:"enabled"`
	GlobalRPS    int   `yaml:"global_rps" json:"global_rps"`       // Requests per second globally
	PerClientRPS int   `yaml:"per_client_rps" json:"per_client_rps"` // Requests per second per client
	BurstSize    int   `yaml:"burst_size" json:"burst_size"`      // Burst allowance
}

// CircuitBreakerConfig holds circuit breaker configuration
type CircuitBreakerConfig struct {
	Enabled            bool          `yaml:"enabled" json:"enabled"`
	FailureThreshold   float64       `yaml:"failure_threshold" json:"failure_threshold"`     // Percentage (0-100)
	FailureWindow      time.Duration `yaml:"failure_window" json:"failure_window"`           // Time window for failure calculation
	OpenDuration       time.Duration `yaml:"open_duration" json:"open_duration"`             // How long circuit stays open
	HalfOpenMaxRequests int          `yaml:"half_open_max_requests" json:"half_open_max_requests"` // Max requests in half-open
	MinRequests        int           `yaml:"min_requests" json:"min_requests"`               // Min requests before opening
}

// RedisConfig holds Redis configuration for distributed deduplication
type RedisConfig struct {
	Enabled        bool          `yaml:"enabled" json:"enabled"`
	ConnectionString string      `yaml:"connection_string" json:"connection_string"` // Redis connection string
	PoolSize      int           `yaml:"pool_size" json:"pool_size"`                   // Connection pool size
	MinIdleConns  int           `yaml:"min_idle_conns" json:"min_idle_conns"`         // Minimum idle connections
	KeyPrefix     string        `yaml:"key_prefix" json:"key_prefix"`                 // Key prefix for Redis keys
	LockTTL       time.Duration `yaml:"lock_ttl" json:"lock_ttl"`                     // TTL for lock keys
	ResultTTL     time.Duration `yaml:"result_ttl" json:"result_ttl"`                 // TTL for result cache
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:        50051,
			MetricsPort: 9090,
			HTTPPort:    8080,
			Reflection:  true,
		},
		Database: DatabaseConfig{
			ConnectionString: "postgres://root@localhost:26257/zenith?sslmode=disable",
			MaxOpenConns:     25,
			MaxIdleConns:     5,
			ConnMaxLifetime:  5 * time.Minute,
			ConnMaxIdleTime:  1 * time.Minute,
			HealthCheckInterval: 30 * time.Second,
		},
		Cache: CacheConfig{
			Enabled:     true,
			Size:        10000,
			TTLPositive: 30 * time.Second,
			TTLNegative: 5 * time.Second,
		},
		Engine: EngineConfig{
			MaxDepth:     10,
			CheckTimeout: 10, // milliseconds
		},
		Tracing: TracingConfig{
			Enabled:  true,
			Endpoint: "localhost:4317",
		},
		RateLimit: RateLimitConfig{
			Enabled:      false,
			GlobalRPS:    1000,
			PerClientRPS: 100,
			BurstSize:    10,
		},
		CircuitBreaker: CircuitBreakerConfig{
			Enabled:            false,
			FailureThreshold:   50.0,
			FailureWindow:      1 * time.Minute,
			OpenDuration:        30 * time.Second,
			HalfOpenMaxRequests: 5,
			MinRequests:         10,
		},
		Redis: RedisConfig{
			Enabled:         false,
			ConnectionString: "redis://localhost:6379/0",
			PoolSize:       10,
			MinIdleConns:   5,
			KeyPrefix:      "zenith:dedup:",
			LockTTL:        5 * time.Second,
			ResultTTL:      10 * time.Second,
		},
	}
}

// LoadFromFile loads configuration from a YAML or JSON file
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	config := DefaultConfig()

	// Try YAML first
	if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
		if err := yaml.Unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("failed to parse YAML config: %w", err)
		}
	} else if strings.HasSuffix(path, ".json") {
		if err := json.Unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("failed to parse JSON config: %w", err)
		}
	} else {
		// Try YAML first, then JSON
		if err := yaml.Unmarshal(data, config); err != nil {
			if err2 := json.Unmarshal(data, config); err2 != nil {
				return nil, fmt.Errorf("failed to parse config (tried YAML and JSON): %w, %w", err, err2)
			}
		}
	}

	return config, nil
}

// LoadFromEnv loads configuration from environment variables
// Environment variables use ZENITH_ prefix and underscore notation
// e.g., ZENITH_SERVER_PORT, ZENITH_DATABASE_CONNECTION_STRING
func LoadFromEnv() *Config {
	config := DefaultConfig()

	// Server config
	if v := os.Getenv("ZENITH_SERVER_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			config.Server.Port = port
		}
	}
	if v := os.Getenv("ZENITH_SERVER_METRICS_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			config.Server.MetricsPort = port
		}
	}
	if v := os.Getenv("ZENITH_SERVER_REFLECTION"); v != "" {
		config.Server.Reflection = v == "true" || v == "1"
	}

	// Database config
	if v := os.Getenv("ZENITH_DATABASE_CONNECTION_STRING"); v != "" {
		config.Database.ConnectionString = v
	}
	if v := os.Getenv("ZENITH_DATABASE_MAX_OPEN_CONNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			config.Database.MaxOpenConns = n
		}
	}
	if v := os.Getenv("ZENITH_DATABASE_MAX_IDLE_CONNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			config.Database.MaxIdleConns = n
		}
	}
	if v := os.Getenv("ZENITH_DATABASE_CONN_MAX_LIFETIME"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			config.Database.ConnMaxLifetime = d
		}
	}
	if v := os.Getenv("ZENITH_DATABASE_CONN_MAX_IDLE_TIME"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			config.Database.ConnMaxIdleTime = d
		}
	}

	// Cache config
	if v := os.Getenv("ZENITH_CACHE_ENABLED"); v != "" {
		config.Cache.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("ZENITH_CACHE_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			config.Cache.Size = n
		}
	}
	if v := os.Getenv("ZENITH_CACHE_TTL_POSITIVE"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			config.Cache.TTLPositive = d
		}
	}
	if v := os.Getenv("ZENITH_CACHE_TTL_NEGATIVE"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			config.Cache.TTLNegative = d
		}
	}

	// Engine config
	if v := os.Getenv("ZENITH_ENGINE_MAX_DEPTH"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			config.Engine.MaxDepth = n
		}
	}
	if v := os.Getenv("ZENITH_ENGINE_CHECK_TIMEOUT_MS"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			config.Engine.CheckTimeout = n
		}
	}

	// Tracing config
	if v := os.Getenv("ZENITH_TRACING_ENABLED"); v != "" {
		config.Tracing.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("ZENITH_TRACING_ENDPOINT"); v != "" {
		config.Tracing.Endpoint = v
	}

	// Rate limit config
	if v := os.Getenv("ZENITH_RATE_LIMIT_ENABLED"); v != "" {
		config.RateLimit.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("ZENITH_RATE_LIMIT_GLOBAL_RPS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			config.RateLimit.GlobalRPS = n
		}
	}
	if v := os.Getenv("ZENITH_RATE_LIMIT_PER_CLIENT_RPS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			config.RateLimit.PerClientRPS = n
		}
	}
	if v := os.Getenv("ZENITH_RATE_LIMIT_BURST_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			config.RateLimit.BurstSize = n
		}
	}

	return config
}

// Load loads configuration with priority: flags > env vars > config file > defaults
func Load(configFile string) (*Config, error) {
	config := DefaultConfig()

	// Load from file if provided
	if configFile != "" {
		fileConfig, err := LoadFromFile(configFile)
		if err != nil {
			return nil, err
		}
		config = fileConfig
	}

	// Override with environment variables
	envConfig := LoadFromEnv()
	// Only merge if env var is explicitly set (check via os.Getenv)
	if os.Getenv("ZENITH_SERVER_HTTP_PORT") == "" {
		// Don't merge HTTPPort from env if env var not set
		envConfig.Server.HTTPPort = 0
	}
	config = mergeConfig(config, envConfig)

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
}

// mergeConfig merges envConfig into baseConfig (env vars override file config)
func mergeConfig(base, env *Config) *Config {
	merged := *base

	// Only override if env var is set (non-zero for numbers, non-empty for strings)
	if env.Server.Port != 0 {
		merged.Server.Port = env.Server.Port
	}
	if env.Server.MetricsPort != 0 {
		merged.Server.MetricsPort = env.Server.MetricsPort
	}
	if env.Server.HTTPPort != 0 {
		merged.Server.HTTPPort = env.Server.HTTPPort
	}
	if os.Getenv("ZENITH_SERVER_REFLECTION") != "" {
		merged.Server.Reflection = env.Server.Reflection
	}

	if env.Database.ConnectionString != "" {
		merged.Database.ConnectionString = env.Database.ConnectionString
	}
	if env.Database.MaxOpenConns != 0 {
		merged.Database.MaxOpenConns = env.Database.MaxOpenConns
	}
	if env.Database.MaxIdleConns != 0 {
		merged.Database.MaxIdleConns = env.Database.MaxIdleConns
	}
	if env.Database.ConnMaxLifetime != 0 {
		merged.Database.ConnMaxLifetime = env.Database.ConnMaxLifetime
	}
	if env.Database.ConnMaxIdleTime != 0 {
		merged.Database.ConnMaxIdleTime = env.Database.ConnMaxIdleTime
	}
	if env.Database.HealthCheckInterval != 0 {
		merged.Database.HealthCheckInterval = env.Database.HealthCheckInterval
	}

	if os.Getenv("ZENITH_CACHE_ENABLED") != "" {
		merged.Cache.Enabled = env.Cache.Enabled
	}
	if env.Cache.Size != 0 {
		merged.Cache.Size = env.Cache.Size
	}
	if env.Cache.TTLPositive != 0 {
		merged.Cache.TTLPositive = env.Cache.TTLPositive
	}
	if env.Cache.TTLNegative != 0 {
		merged.Cache.TTLNegative = env.Cache.TTLNegative
	}

	if env.Engine.MaxDepth != 0 {
		merged.Engine.MaxDepth = env.Engine.MaxDepth
	}
	if env.Engine.CheckTimeout != 0 {
		merged.Engine.CheckTimeout = env.Engine.CheckTimeout
	}

	if os.Getenv("ZENITH_TRACING_ENABLED") != "" {
		merged.Tracing.Enabled = env.Tracing.Enabled
	}
	if env.Tracing.Endpoint != "" {
		merged.Tracing.Endpoint = env.Tracing.Endpoint
	}

	if os.Getenv("ZENITH_RATE_LIMIT_ENABLED") != "" {
		merged.RateLimit.Enabled = env.RateLimit.Enabled
	}
	if env.RateLimit.GlobalRPS != 0 {
		merged.RateLimit.GlobalRPS = env.RateLimit.GlobalRPS
	}
	if env.RateLimit.PerClientRPS != 0 {
		merged.RateLimit.PerClientRPS = env.RateLimit.PerClientRPS
	}
	if env.RateLimit.BurstSize != 0 {
		merged.RateLimit.BurstSize = env.RateLimit.BurstSize
	}

	return &merged
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("server port must be between 1 and 65535, got %d", c.Server.Port)
	}
	if c.Server.MetricsPort <= 0 || c.Server.MetricsPort > 65535 {
		return fmt.Errorf("metrics port must be between 1 and 65535, got %d", c.Server.MetricsPort)
	}
	if c.Server.HTTPPort <= 0 || c.Server.HTTPPort > 65535 {
		return fmt.Errorf("http port must be between 1 and 65535, got %d", c.Server.HTTPPort)
	}
	if c.Server.Port == c.Server.MetricsPort || c.Server.Port == c.Server.HTTPPort || c.Server.MetricsPort == c.Server.HTTPPort {
		return fmt.Errorf("server, metrics, and http ports must be different")
	}
	if c.Database.ConnectionString == "" {
		return fmt.Errorf("database connection string is required")
	}
	if c.Database.MaxOpenConns <= 0 {
		return fmt.Errorf("max_open_conns must be positive, got %d", c.Database.MaxOpenConns)
	}
	if c.Database.MaxIdleConns < 0 {
		return fmt.Errorf("max_idle_conns must be non-negative, got %d", c.Database.MaxIdleConns)
	}
	if c.Database.MaxIdleConns > c.Database.MaxOpenConns {
		return fmt.Errorf("max_idle_conns (%d) cannot exceed max_open_conns (%d)", c.Database.MaxIdleConns, c.Database.MaxOpenConns)
	}
	if c.Cache.Enabled && c.Cache.Size <= 0 {
		return fmt.Errorf("cache size must be positive when cache is enabled, got %d", c.Cache.Size)
	}
	if c.Engine.MaxDepth <= 0 {
		return fmt.Errorf("max_depth must be positive, got %d", c.Engine.MaxDepth)
	}
	if c.Engine.CheckTimeout <= 0 {
		return fmt.Errorf("check_timeout_ms must be positive, got %d", c.Engine.CheckTimeout)
	}
	if c.RateLimit.Enabled {
		if c.RateLimit.GlobalRPS <= 0 {
			return fmt.Errorf("global_rps must be positive when rate limiting is enabled, got %d", c.RateLimit.GlobalRPS)
		}
		if c.RateLimit.PerClientRPS <= 0 {
			return fmt.Errorf("per_client_rps must be positive when rate limiting is enabled, got %d", c.RateLimit.PerClientRPS)
		}
		if c.RateLimit.BurstSize <= 0 {
			return fmt.Errorf("burst_size must be positive when rate limiting is enabled, got %d", c.RateLimit.BurstSize)
		}
	}
	return nil
}

