// ===========================================
// Package middleware - Rate Limiting
// ===========================================
// Rate limiting protects your API from abuse.
// Without it, attackers can:
// - Exhaust server resources (DoS)
// - Spam URLs to fill your database
// - Brute force API keys
//
// ALGORITHM: Sliding Window Counter
// Simpler and more efficient than token bucket for our use case.
// Uses Redis INCR with expiration to count requests per minute.
//
// HOW IT WORKS:
// 1. Key = "ratelimit:{identifier}:{minute}"
// 2. INCR key → get current count
// 3. If count == 1, set expiry to 60 seconds
// 4. If count > limit, reject with 429
// ===========================================

package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/user/urlshortener/internal/database"
	"github.com/user/urlshortener/internal/models"
)

// RateLimiter is the middleware for rate limiting.
type RateLimiter struct {
	redis        *database.RedisDB
	defaultLimit int
	windowSize   time.Duration
}

// NewRateLimiter creates a new rate limiter middleware.
func NewRateLimiter(redis *database.RedisDB, defaultLimit int) *RateLimiter {
	return &RateLimiter{
		redis:        redis,
		defaultLimit: defaultLimit,
		windowSize:   time.Minute, // 1-minute sliding window
	}
}

// Middleware returns the Gin middleware handler.
//
// MIDDLEWARE PATTERN:
// Middleware wraps handlers with cross-cutting concerns.
// Request flows through: RateLimit → Auth → Handler
// Response flows back:    Handler → Auth → RateLimit
//
// c.Next() calls the next handler in the chain.
// c.Abort() stops the chain and returns immediately.
func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Step 1: Determine rate limit
		// API key users may have custom limits
		limit := rl.defaultLimit
		identifier := rl.getClientIdentifier(c)

		// Check if we have an authenticated user with custom limit
		if apiKey, exists := c.Get("api_key"); exists {
			key := apiKey.(*models.APIKey)
			limit = key.RateLimit
			identifier = key.ID.String() // Use API key ID instead of IP
		}

		// Step 2: Get current window
		window := time.Now().Truncate(rl.windowSize)
		key := database.RateLimitKey(identifier, window)

		// Step 3: Increment counter
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		count, err := rl.redis.IncrementRateLimit(ctx, key, rl.windowSize)
		if err != nil {
			// Redis error - fail open (allow request)
			// In production, log this and consider fallback
			c.Next()
			return
		}

		// Step 4: Set rate limit headers
		// These help clients understand their limits
		c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", max(0, limit-int(count))))
		c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", window.Add(rl.windowSize).Unix()))

		// Step 5: Check if over limit
		if int(count) > limit {
			retryAfter := rl.windowSize - time.Since(window)
			c.Header("Retry-After", fmt.Sprintf("%d", int(retryAfter.Seconds())))

			c.JSON(http.StatusTooManyRequests, models.ErrorResponse{
				Error:   "Rate limit exceeded",
				Code:    models.ErrCodeRateLimited,
				Details: fmt.Sprintf("Try again in %d seconds", int(retryAfter.Seconds())),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// getClientIdentifier returns a unique identifier for rate limiting.
// Uses IP address by default, falls back to X-Forwarded-For for proxies.
//
// SECURITY NOTE:
// X-Forwarded-For can be spoofed! Only trust it if you're behind
// a trusted proxy (like Nginx or a load balancer).
// In production, configure trusted proxies in Gin.
func (rl *RateLimiter) getClientIdentifier(c *gin.Context) string {
	// Check for forwarded IP (from reverse proxy)
	forwarded := c.GetHeader("X-Forwarded-For")
	if forwarded != "" {
		// X-Forwarded-For can contain multiple IPs: "client, proxy1, proxy2"
		// The first one is the original client
		ips := strings.Split(forwarded, ",")
		return strings.TrimSpace(ips[0])
	}

	// Check for real IP header (some proxies use this)
	realIP := c.GetHeader("X-Real-IP")
	if realIP != "" {
		return realIP
	}

	// Fall back to direct client IP
	return c.ClientIP()
}

// ===========================================
// Helper
// ===========================================

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ===========================================
// LEARNING NOTES - Rate Limiting Strategies:
// ===========================================
//
// 1. FIXED WINDOW:
//    Count requests per minute (0:00 - 0:59, 1:00 - 1:59)
//    Problem: Burst at window boundaries
//    (60 requests at 0:59, 60 more at 1:00 = 120 in 2 seconds!)
//
// 2. SLIDING WINDOW (what we use):
//    Same as fixed, but window slides with time
//    Less bursty, easy to implement with Redis
//
// 3. TOKEN BUCKET:
//    Tokens accumulate over time up to burst limit
//    More complex, allows controlled bursts
//    Good for APIs that need burst capacity
//
// 4. SLIDING LOG:
//    Store timestamp of every request
//    Most accurate, but memory-intensive
//    Good for strict rate limiting
//
// ===========================================
