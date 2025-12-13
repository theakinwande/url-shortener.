// ===========================================
// Package models - Domain Models
// ===========================================
// Models represent the core data structures of your application.
// They are "dumb" data containers with no business logic.
//
// WHY SEPARATE MODELS?
// - Single source of truth for data shapes
// - Shared between layers (handler, service, repository)
// - JSON tags control API response format
// - DB tags control database mapping (if using ORM)
//
// NAMING CONVENTION:
// - Singular nouns: URL, APIKey (not URLs, APIKeys)
// - Request/Response suffixes for DTOs
// ===========================================

package models

import (
	"time"

	"github.com/google/uuid"
)

// ===========================================
// Core Domain Models
// ===========================================

// URL represents a shortened URL in the system.
// This is our primary domain entity.
type URL struct {
	ID          uuid.UUID  `json:"id"`                   // Unique identifier
	ShortCode   string     `json:"short_code"`           // The short code (e.g., "abc123")
	OriginalURL string     `json:"original_url"`         // The destination URL
	Clicks      int        `json:"clicks"`               // Total click count
	APIKeyID    *uuid.UUID `json:"api_key_id,omitempty"` // Creator's API key (optional)
	ExpiresAt   *time.Time `json:"expires_at,omitempty"` // Expiration time (optional)
	CreatedAt   time.Time  `json:"created_at"`           // Creation timestamp
}

// IsExpired checks if the URL has passed its expiration time.
// Returns false if no expiration is set.
//
// LEARNING NOTE:
// This is the ONLY business logic on a model - simple state checks.
// Complex logic belongs in the service layer.
func (u *URL) IsExpired() bool {
	if u.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*u.ExpiresAt)
}

// APIKey represents an API key for authentication.
// The actual key value is NEVER stored - only its hash.
type APIKey struct {
	ID         uuid.UUID  `json:"id"`
	KeyHash    string     `json:"-"`          // "-" means never serialize to JSON!
	Name       string     `json:"name"`       // Human-readable identifier
	RateLimit  int        `json:"rate_limit"` // Requests per minute allowed
	IsActive   bool       `json:"is_active"`  // Soft delete flag
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// ===========================================
// Request DTOs (Data Transfer Objects)
// ===========================================
// DTOs carry data between layers.
// Request DTOs validate and sanitize user input.
// Response DTOs shape the API output.
//
// WHY SEPARATE FROM DOMAIN MODELS?
// - API contract != internal representation
// - Validation rules live here, not on domain models
// - Can evolve API without changing core logic

// CreateURLRequest is the DTO for creating a new short URL.
type CreateURLRequest struct {
	// URL to shorten (required)
	// binding:"required,url" uses Gin's validation
	URL string `json:"url" binding:"required,url"`

	// Custom alias (optional)
	// binding:"omitempty" means validate only if present
	CustomAlias string `json:"custom_alias,omitempty" binding:"omitempty,min=3,max=16,alphanum"`

	// Expiration in seconds (optional)
	// 0 or omitted means never expires
	ExpiresIn int `json:"expires_in,omitempty" binding:"omitempty,min=60"`
}

// CreateURLResponse is returned after successfully creating a short URL.
type CreateURLResponse struct {
	ShortCode string     `json:"short_code"`           // The generated/custom code
	ShortURL  string     `json:"short_url"`            // Full clickable URL
	ExpiresAt *time.Time `json:"expires_at,omitempty"` // When it expires (if set)
}

// URLStatsResponse contains analytics for a short URL.
type URLStatsResponse struct {
	ShortCode   string     `json:"short_code"`
	OriginalURL string     `json:"original_url"`
	Clicks      int        `json:"clicks"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

// ===========================================
// Error Response
// ===========================================

// ErrorResponse provides consistent error format across all endpoints.
// This is crucial for API consumers to handle errors programmatically.
type ErrorResponse struct {
	Error   string `json:"error"`             // Human-readable message
	Code    string `json:"code,omitempty"`    // Machine-readable error code
	Details string `json:"details,omitempty"` // Additional context
}

// Common error codes (use with ErrorResponse.Code).
// Clients can switch on these for programmatic handling.
const (
	ErrCodeInvalidInput  = "INVALID_INPUT"
	ErrCodeNotFound      = "NOT_FOUND"
	ErrCodeExpired       = "EXPIRED"
	ErrCodeRateLimited   = "RATE_LIMITED"
	ErrCodeUnauthorized  = "UNAUTHORIZED"
	ErrCodeConflict      = "CONFLICT"
	ErrCodeInternalError = "INTERNAL_ERROR"
)

// ===========================================
// Health Check Response
// ===========================================

// HealthResponse is returned by the /health endpoint.
// Useful for load balancers and Kubernetes probes.
type HealthResponse struct {
	Status   string            `json:"status"`   // "healthy" or "unhealthy"
	Version  string            `json:"version"`  // Application version
	Services map[string]string `json:"services"` // Dependency health (db, redis)
}

// ===========================================
// LEARNING NOTES - JSON Tags:
// ===========================================
//
// json:"name"                 → Field serialized as "name"
// json:"name,omitempty"       → Omit if zero value
// json:"-"                    → Never serialize (secrets!)
//
// binding:"required"          → Gin validation: must be present
// binding:"url"               → Gin validation: must be valid URL
// binding:"min=3,max=16"      → Length constraints
// binding:"alphanum"          → Only letters and numbers
// ===========================================
