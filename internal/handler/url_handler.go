// ===========================================
// Package handler - HTTP Request Handlers
// ===========================================
// Handlers are the entry point for HTTP requests.
// They are "thin" - minimal logic, mostly:
// 1. Parse request
// 2. Validate input
// 3. Call service
// 4. Format response
//
// WHY THIN HANDLERS?
// - Easy to test (mock the service)
// - Reusable services (CLI, gRPC, etc.)
// - Clear separation of concerns
// ===========================================

package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/user/urlshortener/internal/models"
	"github.com/user/urlshortener/internal/service"
)

// URLHandler handles URL-related HTTP requests.
type URLHandler struct {
	urlService *service.URLService
}

// NewURLHandler creates a new URL handler.
func NewURLHandler(svc *service.URLService) *URLHandler {
	return &URLHandler{urlService: svc}
}

// ===========================================
// POST /api/shorten
// ===========================================
// Creates a new shortened URL.
//
// Request:
//
//	{
//	  "url": "https://example.com/very/long/url",
//	  "custom_alias": "mylink",  // optional
//	  "expires_in": 3600         // optional, seconds
//	}
//
// Response (201):
//
//	{
//	  "short_code": "mylink",
//	  "short_url": "http://localhost:8080/mylink",
//	  "expires_at": "2024-01-01T00:00:00Z"
//	}
func (h *URLHandler) Shorten(c *gin.Context) {
	// Step 1: Parse request body
	var req models.CreateURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// ShouldBindJSON validates based on struct tags
		// binding:"required,url" etc.
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "Invalid request body",
			Code:    models.ErrCodeInvalidInput,
			Details: err.Error(),
		})
		return
	}

	// Step 2: Call service
	resp, err := h.urlService.Create(c.Request.Context(), req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	// Step 3: Return success
	// 201 Created for new resources (not 200 OK)
	c.JSON(http.StatusCreated, resp)
}

// ===========================================
// GET /:shortCode
// ===========================================
// Redirects to the original URL.
// This is the MAIN functionality - must be FAST!
//
// Response: 302 Found with Location header
//
// WHY 302 INSTEAD OF 301?
// - 301 (Permanent): Browser caches forever, no analytics
// - 302 (Temporary): Browser asks every time, we can track clicks
// - 307/308: Preserve method (not needed for redirects)
func (h *URLHandler) Redirect(c *gin.Context) {
	shortCode := c.Param("shortCode")

	// Resolve short code to original URL
	originalURL, err := h.urlService.Resolve(c.Request.Context(), shortCode)
	if err != nil {
		h.handleError(c, err)
		return
	}

	// 302 redirect - client will follow the Location header
	c.Redirect(http.StatusFound, originalURL)
}

// ===========================================
// GET /api/stats/:shortCode
// ===========================================
// Returns analytics for a short URL.
//
// Response (200):
//
//	{
//	  "short_code": "abc123",
//	  "original_url": "https://example.com",
//	  "clicks": 42,
//	  "created_at": "2024-01-01T00:00:00Z"
//	}
func (h *URLHandler) GetStats(c *gin.Context) {
	shortCode := c.Param("shortCode")

	stats, err := h.urlService.GetStats(c.Request.Context(), shortCode)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, stats)
}

// ===========================================
// DELETE /api/:shortCode
// ===========================================
// Deletes a short URL.
//
// Response: 204 No Content (success, no body)
//
// WHY 204?
// - 200 OK typically returns a body
// - 204 means "success, nothing to return"
// - Client knows it worked without parsing response
func (h *URLHandler) Delete(c *gin.Context) {
	shortCode := c.Param("shortCode")

	err := h.urlService.Delete(c.Request.Context(), shortCode)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// ===========================================
// Error Handling
// ===========================================
// Centralized error-to-HTTP mapping.
// Converts service errors to appropriate HTTP responses.
//
// WHY CENTRALIZED?
// - Consistent error format across all endpoints
// - Easy to add logging, metrics, etc.
// - Service layer doesn't need to know about HTTP

func (h *URLHandler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrURLNotFound):
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: "URL not found",
			Code:  models.ErrCodeNotFound,
		})

	case errors.Is(err, service.ErrURLExpired):
		c.JSON(http.StatusGone, models.ErrorResponse{
			Error: "URL has expired",
			Code:  models.ErrCodeExpired,
		})

	case errors.Is(err, service.ErrCodeTaken):
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error: "Short code already taken",
			Code:  models.ErrCodeConflict,
		})

	case errors.Is(err, service.ErrInvalidCode):
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "Invalid short code format",
			Code:    models.ErrCodeInvalidInput,
			Details: "Code must be 3-16 alphanumeric characters",
		})

	case errors.Is(err, service.ErrInvalidURL):
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "Invalid URL format",
			Code:    models.ErrCodeInvalidInput,
			Details: "URL must start with http:// or https://",
		})

	default:
		// Unknown error - log it and return generic 500
		// SECURITY: Don't expose internal error details!
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error: "Internal server error",
			Code:  models.ErrCodeInternalError,
		})
	}
}

// ===========================================
// LEARNING NOTES - HTTP Status Codes:
// ===========================================
//
// 2xx SUCCESS:
// - 200 OK: Request succeeded
// - 201 Created: New resource created (POST)
// - 204 No Content: Success, no response body (DELETE)
//
// 3xx REDIRECT:
// - 301 Moved Permanently: Cached forever (avoid for analytics)
// - 302 Found: Temporary redirect (use this!)
// - 304 Not Modified: Use cached version
//
// 4xx CLIENT ERROR:
// - 400 Bad Request: Invalid input
// - 401 Unauthorized: No/invalid credentials
// - 403 Forbidden: Valid credentials, no permission
// - 404 Not Found: Resource doesn't exist
// - 409 Conflict: Resource already exists (duplicate)
// - 410 Gone: Resource existed but was deleted/expired
// - 429 Too Many Requests: Rate limited
//
// 5xx SERVER ERROR:
// - 500 Internal Error: Bug or unexpected failure
// - 502 Bad Gateway: Upstream service failed
// - 503 Service Unavailable: Overloaded or maintenance
//
// ===========================================
