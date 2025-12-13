// ===========================================
// Package middleware - Security Headers
// ===========================================
// This middleware sets HTTP headers that improve security.
// These are "defense in depth" - multiple layers of protection.
//
// OWASP RECOMMENDATION:
// Always set security headers. They're free protection!
// ===========================================

package middleware

import (
	"github.com/gin-gonic/gin"
)

// SecurityHeaders returns middleware that sets security headers.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		// ===========================================
		// Content-Type Options
		// ===========================================
		// Prevents browsers from MIME-sniffing.
		// Without this, a malicious file could be executed as JavaScript.
		// Example attack: Upload image.jpg containing <script>alert('xss')</script>
		c.Header("X-Content-Type-Options", "nosniff")

		// ===========================================
		// Frame Options
		// ===========================================
		// Prevents clickjacking by disabling iframes.
		// Attacker could overlay invisible iframe to capture clicks.
		// DENY = never allow framing
		// SAMEORIGIN = only allow framing by same domain
		c.Header("X-Frame-Options", "DENY")

		// ===========================================
		// XSS Protection
		// ===========================================
		// Legacy browser XSS filter. Modern browsers ignore this,
		// but old browsers benefit. No harm in including it.
		c.Header("X-XSS-Protection", "1; mode=block")

		// ===========================================
		// Referrer Policy
		// ===========================================
		// Controls what info is sent in Referer header.
		// "strict-origin-when-cross-origin" sends:
		// - Full URL for same-origin requests
		// - Only origin (domain) for cross-origin
		// - Nothing for HTTPS → HTTP downgrade
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

		// ===========================================
		// Content Security Policy (CSP)
		// ===========================================
		// Restricts what resources can be loaded.
		// For an API, we want minimal permissions.
		// default-src 'none' = block everything by default
		// frame-ancestors 'none' = no embedding (like X-Frame-Options)
		c.Header("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")

		// ===========================================
		// Strict Transport Security (HSTS)
		// ===========================================
		// Forces HTTPS for future requests.
		// max-age = how long to remember (1 year = 31536000 seconds)
		// includeSubDomains = also apply to subdomains
		//
		// CAUTION: Only enable in production with proper HTTPS!
		// Once set, browsers will REFUSE HTTP connections.
		// c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

		// ===========================================
		// Permissions Policy (formerly Feature-Policy)
		// ===========================================
		// Disables browser features we don't need.
		// Reduces attack surface.
		c.Header("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		// ===========================================
		// Cache Control
		// ===========================================
		// Prevent caching of API responses (contains user data).
		// no-store = don't cache at all
		// no-cache = always revalidate with server
		c.Header("Cache-Control", "no-store, no-cache, must-revalidate")
		c.Header("Pragma", "no-cache") // HTTP/1.0 compatibility

		c.Next()
	}
}

// ===========================================
// CORS Configuration
// ===========================================
// Cross-Origin Resource Sharing controls which domains
// can call your API from JavaScript.
//
// WITHOUT CORS:
// - Only same-domain requests allowed
// - Your React app on localhost:3000 can't call API on localhost:8080
//
// WITH CORS:
// - Specify allowed origins
// - Control allowed methods and headers

// CORSConfig holds CORS configuration.
type CORSConfig struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	AllowCredentials bool
	MaxAge           int // Preflight cache duration in seconds
}

// DefaultCORSConfig returns secure defaults.
// Override in production with specific origins!
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowedOrigins:   []string{"*"}, // TODO: Restrict in production!
		AllowedMethods:   []string{"GET", "POST", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Origin", "Content-Type", "Accept", "Authorization", "X-API-Key"},
		AllowCredentials: false, // Don't allow with "*" origin
		MaxAge:           86400, // Cache preflight for 24 hours
	}
}

// CORS returns CORS middleware with the given config.
func CORS(cfg CORSConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")

		// Check if origin is allowed
		allowedOrigin := ""
		for _, allowed := range cfg.AllowedOrigins {
			if allowed == "*" || allowed == origin {
				allowedOrigin = allowed
				break
			}
		}

		if allowedOrigin != "" {
			c.Header("Access-Control-Allow-Origin", allowedOrigin)
		}

		// Vary header tells caches that response depends on Origin
		c.Header("Vary", "Origin")

		// Handle preflight OPTIONS request
		if c.Request.Method == "OPTIONS" {
			c.Header("Access-Control-Allow-Methods", joinStrings(cfg.AllowedMethods))
			c.Header("Access-Control-Allow-Headers", joinStrings(cfg.AllowedHeaders))
			c.Header("Access-Control-Max-Age", string(rune(cfg.MaxAge)))

			if cfg.AllowCredentials {
				c.Header("Access-Control-Allow-Credentials", "true")
			}

			c.AbortWithStatus(204) // No content
			return
		}

		c.Next()
	}
}

func joinStrings(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += ", " + strs[i]
	}
	return result
}

// ===========================================
// LEARNING NOTES - Security Headers:
// ===========================================
//
// Think of security headers as a checklist.
// Each one blocks a specific attack vector.
//
// MINIMUM SET (always include):
// - X-Content-Type-Options: nosniff
// - X-Frame-Options: DENY
// - Content-Security-Policy: default-src 'none'
//
// PRODUCTION ADDITIONS:
// - Strict-Transport-Security (requires HTTPS)
// - Report-To/Report-URI (monitor violations)
//
// TESTING:
// Use https://securityheaders.com to check your headers
// Use browser DevTools → Network tab → Response Headers
//
// ===========================================
