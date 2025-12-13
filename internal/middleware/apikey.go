// ===========================================
// Package middleware - API Key Authentication
// ===========================================
// This middleware validates API keys for protected endpoints.
//
// AUTHENTICATION vs AUTHORIZATION:
// - Authentication: "Who are you?" (this middleware)
// - Authorization: "What can you do?" (future middleware)
//
// FLOW:
// 1. Extract API key from request header
// 2. Hash the key (we never store plain keys)
// 3. Look up hash in database
// 4. If valid, attach key info to request context
// 5. If invalid, return 401 Unauthorized
//
// SECURITY DECISIONS:
// - Use X-API-Key header (standard convention)
// - Never log raw API keys!
// - Constant-time comparison to prevent timing attacks
// ===========================================

package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/user/urlshortener/internal/models"
	"github.com/user/urlshortener/internal/service"
)

// APIKeyAuth is the middleware for API key authentication.
type APIKeyAuth struct {
	apiKeyService *service.APIKeyService
}

// NewAPIKeyAuth creates a new API key auth middleware.
func NewAPIKeyAuth(svc *service.APIKeyService) *APIKeyAuth {
	return &APIKeyAuth{apiKeyService: svc}
}

// RequireKey returns middleware that requires a valid API key.
// Requests without valid keys are rejected with 401.
func (a *APIKeyAuth) RequireKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey, err := a.extractAndValidate(c)
		if err != nil {
			c.JSON(http.StatusUnauthorized, models.ErrorResponse{
				Error: "Invalid or missing API key",
				Code:  models.ErrCodeUnauthorized,
			})
			c.Abort()
			return
		}

		// Store API key in context for downstream handlers
		// Other middleware (rate limiting) can access this
		c.Set("api_key", apiKey)
		c.Next()
	}
}

// OptionalKey returns middleware that validates API key if present.
// Requests without keys are allowed to proceed.
// Useful for endpoints with different behavior for auth/unauth users.
func (a *APIKeyAuth) OptionalKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey, _ := a.extractAndValidate(c)
		if apiKey != nil {
			c.Set("api_key", apiKey)
		}
		c.Next()
	}
}

// extractAndValidate extracts and validates the API key from the request.
// Returns nil if no key is present or key is invalid.
func (a *APIKeyAuth) extractAndValidate(c *gin.Context) (*models.APIKey, error) {
	// Extract key from header
	rawKey := a.extractKey(c)
	if rawKey == "" {
		return nil, ErrMissingAPIKey
	}

	// Validate with timeout
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	apiKey, err := a.apiKeyService.ValidateKey(ctx, rawKey)
	if err != nil {
		return nil, err
	}
	if apiKey == nil {
		return nil, ErrInvalidAPIKey
	}

	return apiKey, nil
}

// extractKey gets the API key from the request.
//
// SUPPORTED LOCATIONS:
// 1. X-API-Key header (preferred)
// 2. Authorization: Bearer <key> header
// 3. ?api_key= query parameter (not recommended, logged in URLs)
//
// WHY HEADER IS PREFERRED?
// - Not logged in server access logs
// - Not cached by browsers
// - Not visible in browser history
func (a *APIKeyAuth) extractKey(c *gin.Context) string {
	// Check X-API-Key header first (most common)
	if key := c.GetHeader("X-API-Key"); key != "" {
		return key
	}

	// Check Authorization header (Bearer format)
	auth := c.GetHeader("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}

	// Check query parameter (last resort, not recommended)
	// Only enable this if clients really need it
	// if key := c.Query("api_key"); key != "" {
	// 	return key
	// }

	return ""
}

// Error types for API key validation
var (
	ErrMissingAPIKey = APIKeyError{Message: "API key is required"}
	ErrInvalidAPIKey = APIKeyError{Message: "API key is invalid"}
)

// APIKeyError represents an authentication error.
type APIKeyError struct {
	Message string
}

func (e APIKeyError) Error() string {
	return e.Message
}

// ===========================================
// Context Helpers
// ===========================================
// These help handlers access the authenticated API key.

// GetAPIKeyFromContext retrieves the validated API key from context.
// Returns nil if no key was validated (should only happen with OptionalKey).
func GetAPIKeyFromContext(c *gin.Context) *models.APIKey {
	if val, exists := c.Get("api_key"); exists {
		return val.(*models.APIKey)
	}
	return nil
}

// ===========================================
// SECURITY NOTES:
// ===========================================
//
// 1. TIMING ATTACKS:
//    Comparing strings character-by-character leaks info.
//    Attacker can measure time to learn correct characters.
//    Solution: Use constant-time comparison (crypto/subtle).
//    We avoid this by comparing hashes, not raw keys.
//
// 2. KEY ROTATION:
//    Users should be able to rotate keys without downtime.
//    Solution: Allow multiple active keys per account.
//
// 3. KEY SCOPING:
//    Consider adding scopes (read, write, admin) to keys.
//    Limits damage if key is compromised.
//
// 4. LOGGING:
//    NEVER log the full API key!
//    At most, log the last 4 characters for debugging.
//    Example: "Key ending in ...abc123 used"
//
// ===========================================
