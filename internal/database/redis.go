// ===========================================
// Package database - Redis Connection
// ===========================================
// Redis is an in-memory data store used for:
// 1. Caching hot URLs (avoid PostgreSQL queries)
// 2. Rate limiting counters
// 3. Distributed locks (future feature)
//
// WHY REDIS?
// - Sub-millisecond response times (100x faster than PostgreSQL)
// - Built-in expiration (perfect for caching)
// - Atomic operations (perfect for counters)
// - Pub/Sub (future: real-time analytics)
//
// CACHE STRATEGY: Cache-Aside (Lazy Loading)
// 1. Check cache first
// 2. If miss, query database
// 3. Store result in cache
// 4. Return result
// ===========================================

package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/user/urlshortener/internal/config"
)

// RedisDB wraps the Redis client with application-specific methods.
type RedisDB struct {
	Client   *redis.Client
	CacheTTL time.Duration
}

// NewRedisDB creates a new Redis connection.
// It validates the connection before returning.
func NewRedisDB(ctx context.Context, cfg config.RedisConfig) (*RedisDB, error) {
	// Parse Redis URL (redis://localhost:6379)
	opt, err := redis.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}

	// Apply additional configuration (only if set, don't overwrite URL values)
	if cfg.Password != "" {
		opt.Password = cfg.Password
	}
	if cfg.DB != 0 {
		opt.DB = cfg.DB
	}
	if cfg.PoolSize > 0 {
		opt.PoolSize = cfg.PoolSize
	}
	if cfg.MinIdleConns > 0 {
		opt.MinIdleConns = cfg.MinIdleConns
	}

	// Connection timeouts prevent hanging
	opt.DialTimeout = 5 * time.Second
	opt.ReadTimeout = 3 * time.Second
	opt.WriteTimeout = 3 * time.Second

	// Create the client
	client := redis.NewClient(opt)

	// Verify connection
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to ping Redis: %w", err)
	}

	return &RedisDB{
		Client:   client,
		CacheTTL: cfg.CacheTTL,
	}, nil
}

// Close gracefully shuts down the Redis connection.
func (r *RedisDB) Close() error {
	if r.Client != nil {
		return r.Client.Close()
	}
	return nil
}

// Health checks if Redis is responsive.
func (r *RedisDB) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return r.Client.Ping(ctx).Err()
}

// ===========================================
// CACHE OPERATIONS
// ===========================================
// These methods provide type-safe caching for our URL model.
// They handle JSON serialization/deserialization internally.

// CacheKey generates a consistent key format for URLs.
// Pattern: "url:{shortCode}"
//
// WHY PREFIXES?
// - Prevents key collisions between different data types
// - Easier to debug with redis-cli: KEYS url:*
// - Can set different TTLs per prefix
func CacheKey(shortCode string) string {
	return fmt.Sprintf("url:%s", shortCode)
}

// RateLimitKey generates a key for rate limiting.
// Pattern: "ratelimit:{identifier}:{window}"
//
// Identifier can be IP address or API key ID.
// Window is the minute (for requests-per-minute limiting).
func RateLimitKey(identifier string, window time.Time) string {
	return fmt.Sprintf("ratelimit:%s:%d", identifier, window.Unix()/60)
}

// Get retrieves a cached value by key.
// Returns nil, nil if key doesn't exist (cache miss).
// Returns error only for actual errors (not cache misses).
//
// LEARNING NOTE - Cache Miss vs Error:
// Cache miss is NORMAL, not an error. We handle it by:
// 1. Checking if err == redis.Nil
// 2. Returning (nil, nil) for cache miss
// 3. Returning (nil, err) for actual errors
func (r *RedisDB) Get(ctx context.Context, key string) ([]byte, error) {
	result, err := r.Client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		// Cache miss - not an error!
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis get failed: %w", err)
	}
	return result, nil
}

// Set stores a value with the default TTL.
func (r *RedisDB) Set(ctx context.Context, key string, value []byte) error {
	return r.SetWithTTL(ctx, key, value, r.CacheTTL)
}

// SetWithTTL stores a value with a custom TTL.
//
// LEARNING NOTE - TTL (Time To Live):
// After TTL expires, Redis automatically deletes the key.
// This prevents stale data and manages memory usage.
func (r *RedisDB) SetWithTTL(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	err := r.Client.Set(ctx, key, value, ttl).Err()
	if err != nil {
		return fmt.Errorf("redis set failed: %w", err)
	}
	return nil
}

// Delete removes a key from the cache.
// Used when a URL is updated or deleted.
//
// WHY DELETE ON UPDATE?
// Ensures cache consistency. Alternative is "write-through"
// where we update cache and DB simultaneously.
func (r *RedisDB) Delete(ctx context.Context, key string) error {
	err := r.Client.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("redis delete failed: %w", err)
	}
	return nil
}

// ===========================================
// RATE LIMITING OPERATIONS
// ===========================================
// Token bucket algorithm implemented with Redis INCR.
//
// HOW IT WORKS:
// 1. Key = "ratelimit:{client}:{minute}"
// 2. INCR key (atomic increment)
// 3. If first request, SET expiry to 60 seconds
// 4. If count > limit, reject request
//
// WHY THIS APPROACH?
// - Simple and fast (2 Redis commands)
// - Naturally cleans up (keys expire)
// - Distributed (works across multiple servers)

// IncrementRateLimit increments the rate limit counter.
// Returns the new count and whether this is a new window.
//
// ATOMIC OPERATIONS:
// INCR is atomic - even with 1000 concurrent requests,
// each gets a unique count. No race conditions!
func (r *RedisDB) IncrementRateLimit(ctx context.Context, key string, windowSize time.Duration) (int64, error) {
	// INCR returns the new value after incrementing
	count, err := r.Client.Incr(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("rate limit incr failed: %w", err)
	}

	// If this is the first request in the window, set expiry
	// We check count == 1 to avoid resetting expiry on every request
	if count == 1 {
		err = r.Client.Expire(ctx, key, windowSize).Err()
		if err != nil {
			// Non-fatal: key will eventually be cleaned up
			// Log this in production
		}
	}

	return count, nil
}

// GetRateLimit returns the current count for a rate limit key.
// Returns 0 if key doesn't exist.
func (r *RedisDB) GetRateLimit(ctx context.Context, key string) (int64, error) {
	count, err := r.Client.Get(ctx, key).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("rate limit get failed: %w", err)
	}
	return count, nil
}

// ===========================================
// JSON HELPERS
// ===========================================
// Type-safe methods for caching Go structs.

// GetJSON retrieves and unmarshals a JSON value.
func (r *RedisDB) GetJSON(ctx context.Context, key string, dest interface{}) (bool, error) {
	data, err := r.Get(ctx, key)
	if err != nil {
		return false, err
	}
	if data == nil {
		return false, nil // Cache miss
	}

	if err := json.Unmarshal(data, dest); err != nil {
		return false, fmt.Errorf("failed to unmarshal cached value: %w", err)
	}
	return true, nil
}

// SetJSON marshals and stores a value as JSON.
func (r *RedisDB) SetJSON(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}
	return r.SetWithTTL(ctx, key, data, ttl)
}

// ===========================================
// LEARNING NOTES - Redis Data Types:
// ===========================================
//
// STRING: Basic key-value (what we use for caching)
//   SET key value
//   GET key
//
// HASH: Object with fields (alternative for URLs)
//   HSET url:abc code abc original https://...
//   HGET url:abc original
//
// LIST: Ordered list (for queues)
//   LPUSH queue job1
//   RPOP queue
//
// SET: Unique values (for tracking unique visitors)
//   SADD visitors:abc user123
//   SCARD visitors:abc  // count unique
//
// SORTED SET: Ranked items (for leaderboards)
//   ZADD popular 100 abc
//   ZREVRANGE popular 0 10  // top 10
//
// ===========================================
