// ===========================================
// Package config - Application Configuration
// ===========================================
// This package handles loading configuration from environment variables.
//
// WHY ENVIRONMENT VARIABLES?
// - 12-Factor App methodology (https://12factor.net/config)
// - Same binary can run in dev/staging/prod with different configs
// - Secrets never committed to source control
// - Docker/Kubernetes inject env vars easily
//
// PATTERN: Load once at startup, pass config struct around
// This is better than reading env vars everywhere because:
// 1. Validation happens once
// 2. Easier to test (just pass mock config)
// 3. IDE autocomplete on config fields
// ===========================================

package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all application configuration.
// Fields are grouped by concern for readability.
type Config struct {
	// Server settings
	Server ServerConfig

	// Database connection
	Database DatabaseConfig

	// Redis connection
	Redis RedisConfig

	// Rate limiting settings
	RateLimit RateLimitConfig

	// URL shortener specific settings
	Shortener ShortenerConfig
}

// ServerConfig contains HTTP server settings.
type ServerConfig struct {
	Port         string        // Port to listen on (e.g., "8080")
	ReadTimeout  time.Duration // Max time to read request
	WriteTimeout time.Duration // Max time to write response
	IdleTimeout  time.Duration // Max time for keep-alive connections
}

// DatabaseConfig contains PostgreSQL connection settings.
type DatabaseConfig struct {
	URL             string        // Connection string
	MaxOpenConns    int           // Max simultaneous connections
	MaxIdleConns    int           // Max idle connections in pool
	ConnMaxLifetime time.Duration // Max time a connection can be reused
}

// RedisConfig contains Redis connection settings.
type RedisConfig struct {
	URL          string        // Connection string (redis://host:port)
	Password     string        // Optional password
	DB           int           // Database number (0-15)
	PoolSize     int           // Connection pool size
	MinIdleConns int           // Minimum idle connections
	CacheTTL     time.Duration // Default cache TTL
}

// RateLimitConfig contains rate limiting settings.
type RateLimitConfig struct {
	RequestsPerMinute int           // Max requests per minute per client
	BurstSize         int           // Allow burst above limit temporarily
	CleanupInterval   time.Duration // How often to clean expired entries
}

// ShortenerConfig contains URL shortening settings.
type ShortenerConfig struct {
	DefaultCodeLength int           // Length of generated short codes
	MaxCustomLength   int           // Max length for custom aliases
	DefaultExpiry     time.Duration // Default URL expiration (0 = never)
	BaseURL           string        // Base URL for short links
}

// Load reads configuration from environment variables.
// It uses sensible defaults for development.
//
// LEARNING NOTE:
// This function uses a helper pattern: getEnv(key, default)
// This keeps the code DRY and makes defaults obvious.
func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Port:         getEnv("PORT", "8080"),
			ReadTimeout:  getDurationEnv("SERVER_READ_TIMEOUT", 5*time.Second),
			WriteTimeout: getDurationEnv("SERVER_WRITE_TIMEOUT", 10*time.Second),
			IdleTimeout:  getDurationEnv("SERVER_IDLE_TIMEOUT", 120*time.Second),
		},
		Database: DatabaseConfig{
			// SECURITY: In production, use secrets management!
			URL:             getEnv("DATABASE_URL", "postgres://shortener:shortener_secret_password@localhost:5432/shortener?sslmode=disable"),
			MaxOpenConns:    getIntEnv("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getIntEnv("DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getDurationEnv("DB_CONN_MAX_LIFETIME", 5*time.Minute),
		},
		Redis: RedisConfig{
			URL:          getEnv("REDIS_URL", "redis://localhost:6379"),
			Password:     getEnv("REDIS_PASSWORD", ""),
			DB:           getIntEnv("REDIS_DB", 0),
			PoolSize:     getIntEnv("REDIS_POOL_SIZE", 10),
			MinIdleConns: getIntEnv("REDIS_MIN_IDLE_CONNS", 3),
			CacheTTL:     getDurationEnv("REDIS_CACHE_TTL", 1*time.Hour),
		},
		RateLimit: RateLimitConfig{
			RequestsPerMinute: getIntEnv("RATE_LIMIT_RPM", 60),
			BurstSize:         getIntEnv("RATE_LIMIT_BURST", 10),
			CleanupInterval:   getDurationEnv("RATE_LIMIT_CLEANUP", 10*time.Minute),
		},
		Shortener: ShortenerConfig{
			DefaultCodeLength: getIntEnv("SHORT_CODE_LENGTH", 8),
			MaxCustomLength:   getIntEnv("SHORT_CODE_MAX_LENGTH", 16),
			DefaultExpiry:     getDurationEnv("DEFAULT_URL_EXPIRY", 0), // 0 = never
			BaseURL:           getEnv("BASE_URL", "http://localhost:8080"),
		},
	}
}

// ===========================================
// Helper Functions
// ===========================================
// These reduce boilerplate when reading env vars.
// Each handles type conversion and defaults.

// getEnv reads a string env var with a default.
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getIntEnv reads an integer env var with a default.
// Returns default if parsing fails (fail-safe behavior).
func getIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
		// Log warning in production: invalid int, using default
	}
	return defaultValue
}

// getDurationEnv reads a duration env var with a default.
// Accepts formats like "5s", "10m", "1h".
func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
		// Log warning in production: invalid duration, using default
	}
	return defaultValue
}
